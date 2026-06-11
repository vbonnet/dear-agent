package checks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

// writeSpans writes a slice of jsonlSpan values as newline-delimited JSON to
// the given file path, creating parent directories as needed.
func writeSpans(t *testing.T, path string, spans []jsonlSpan) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, s := range spans {
		if err := enc.Encode(s); err != nil {
			t.Fatalf("encode span: %v", err)
		}
	}
}

func TestOTelSpansCheck_BasicFindings(t *testing.T) {
	now := time.Now().UTC()

	dir := t.TempDir()

	// Session A: one ERROR span (within 24h).
	writeSpans(t, filepath.Join(dir, "session-a", "spans.jsonl"), []jsonlSpan{
		{
			TraceID:    "trace-aaa",
			SpanID:     "span-aaa-1",
			Name:       "op.fail",
			StartTime:  now.Add(-1 * time.Hour),
			DurationMs: 100,
			StatusCode: "Error",
			StatusMsg:  "something went wrong",
		},
	})

	// Session B: one slow span, one healthy span (both within 24h).
	writeSpans(t, filepath.Join(dir, "session-b", "spans.jsonl"), []jsonlSpan{
		{
			TraceID:    "trace-bbb",
			SpanID:     "span-bbb-1",
			Name:       "op.slow",
			StartTime:  now.Add(-2 * time.Hour),
			DurationMs: 10000,
			StatusCode: "Ok",
		},
		{
			TraceID:    "trace-bbb",
			SpanID:     "span-bbb-2",
			Name:       "op.healthy",
			StartTime:  now.Add(-3 * time.Hour),
			DurationMs: 50,
			StatusCode: "Ok",
		},
	})

	// Session C: an ERROR span that is OUTSIDE the lookback window.
	writeSpans(t, filepath.Join(dir, "session-c", "spans.jsonl"), []jsonlSpan{
		{
			TraceID:    "trace-ccc",
			SpanID:     "span-ccc-1",
			Name:       "op.old-fail",
			StartTime:  now.Add(-48 * time.Hour),
			DurationMs: 200,
			StatusCode: "Error",
			StatusMsg:  "stale error",
		},
	})

	env := audit.Env{
		Config: map[string]any{
			"traces_dir": dir,
			"lookback":   "24h",
		},
		Now: func() time.Time { return now },
	}

	check := OTelSpansCheck{}
	result, err := check.Run(context.Background(), env)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != audit.StatusOK {
		t.Fatalf("expected StatusOK, got %q", result.Status)
	}

	// Bucket findings by severity.
	var p1, p2 []audit.Finding
	for _, f := range result.Findings {
		switch f.Severity {
		case audit.SeverityP1:
			p1 = append(p1, f)
		case audit.SeverityP2:
			p2 = append(p2, f)
		}
	}

	if len(p1) != 1 {
		t.Errorf("expected 1 P1 finding, got %d: %v", len(p1), p1)
	}
	if len(p2) != 1 {
		t.Errorf("expected 1 P2 finding, got %d: %v", len(p2), p2)
	}

	// Validate P1 fingerprint.
	if len(p1) > 0 {
		want := audit.Fingerprint("otel.span-errors", "trace-aaa", "span-aaa-1")
		if p1[0].Fingerprint != want {
			t.Errorf("P1 fingerprint: got %q, want %q", p1[0].Fingerprint, want)
		}
		if p1[0].CheckID != "otel.span-errors" {
			t.Errorf("P1 CheckID: got %q, want %q", p1[0].CheckID, "otel.span-errors")
		}
	}

	// Validate P2 fingerprint.
	if len(p2) > 0 {
		want := audit.Fingerprint("otel.slow-span", "trace-bbb", "span-bbb-1")
		if p2[0].Fingerprint != want {
			t.Errorf("P2 fingerprint: got %q, want %q", p2[0].Fingerprint, want)
		}
	}
}

func TestOTelSpansCheckRegisteredInDefault(t *testing.T) {
	c, ok := audit.Default.Lookup("otel.span-errors")
	if !ok || c == nil {
		t.Fatal("otel.span-errors not found in audit.Default registry")
	}
}
