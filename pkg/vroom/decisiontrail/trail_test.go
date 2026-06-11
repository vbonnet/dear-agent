package decisiontrail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// nopWriteCloser wraps a bytes.Buffer to satisfy io.WriteCloser for tests.
type nopWriteCloser struct {
	*bytes.Buffer
	closed bool
}

func (n *nopWriteCloser) Close() error { n.closed = true; return nil }

func newBufferTrail() (*JSONLTrail, *nopWriteCloser) {
	b := &nopWriteCloser{Buffer: &bytes.Buffer{}}
	return NewJSONLTrail(b), b
}

func TestAppend_WritesOneLineWithRequiredFields(t *testing.T) {
	trail, buf := newBufferTrail()
	t.Cleanup(func() { _ = trail.Close() })

	err := trail.Append(context.Background(), Record{
		Role: "orchestrator",
		Kind: "supervisor.tick.start",
		Payload: map[string]any{
			"interval_ms": 5000,
		},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	out := buf.String()
	if !bytes.HasSuffix([]byte(out), []byte("\n")) {
		t.Errorf("output missing trailing newline: %q", out)
	}
	var got Record
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v (raw=%q)", err, out)
	}
	if got.EventID == "" {
		t.Error("Append did not populate EventID")
	}
	if got.Timestamp.IsZero() {
		t.Error("Append did not populate Timestamp")
	}
	if got.Role != "orchestrator" || got.Kind != "supervisor.tick.start" {
		t.Errorf("Role/Kind preserved incorrectly: %+v", got)
	}
	if got.Payload["interval_ms"].(float64) != 5000 {
		t.Errorf("payload round-trip lost data: %+v", got.Payload)
	}
}

func TestAppend_PreservesProvidedEventIDAndTimestamp(t *testing.T) {
	trail, buf := newBufferTrail()
	t.Cleanup(func() { _ = trail.Close() })

	given := Record{
		EventID:   "fixed-id-7",
		Timestamp: mustParseTime(t, "2026-05-26T12:00:00Z"),
		Role:      "meta-orchestrator",
		Kind:      "supervisor.decision.gated",
	}
	if err := trail.Append(context.Background(), given); err != nil {
		t.Fatalf("Append: %v", err)
	}
	var got Record
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.EventID != "fixed-id-7" {
		t.Errorf("EventID = %q, want preserved %q", got.EventID, "fixed-id-7")
	}
	if !got.Timestamp.Equal(given.Timestamp) {
		t.Errorf("Timestamp = %v, want preserved %v", got.Timestamp, given.Timestamp)
	}
}

func TestAppend_RejectsCanceledContext(t *testing.T) {
	trail, _ := newBufferTrail()
	t.Cleanup(func() { _ = trail.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := trail.Append(ctx, Record{Role: "overseer", Kind: "x"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Append err = %v, want context.Canceled", err)
	}
}

func TestAppend_ConcurrentWritesAreLineAtomic(t *testing.T) {
	trail, buf := newBufferTrail()
	t.Cleanup(func() { _ = trail.Close() })

	const goroutines = 32
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_ = trail.Append(context.Background(), Record{
					Role:    "orchestrator",
					Kind:    "supervisor.tick",
					Payload: map[string]any{"i": i, "j": j},
				})
			}
		}(i)
	}
	wg.Wait()

	// Every line must parse independently as a Record. If any line is
	// interleaved with another goroutine's write, JSON parse will fail.
	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		var rec Record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Fatalf("line %d failed to parse: %v\nline=%q", count, err, scanner.Text())
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	if want := goroutines * perGoroutine; count != want {
		t.Errorf("wrote %d records, want %d", count, want)
	}
}

func TestAppend_AfterCloseFails(t *testing.T) {
	trail, _ := newBufferTrail()
	if err := trail.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	err := trail.Append(context.Background(), Record{Role: "orchestrator", Kind: "x"})
	if err == nil {
		t.Error("Append after Close returned nil error")
	}
}

func TestClose_Idempotent(t *testing.T) {
	trail, buf := newBufferTrail()
	if err := trail.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := trail.Close(); err != nil {
		t.Errorf("second Close returned err: %v", err)
	}
	if !buf.closed {
		t.Error("underlying writer not closed")
	}
}

func TestOpenJSONL_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "subdir", "trail.jsonl")
	trail, err := OpenJSONL(path)
	if err != nil {
		t.Fatalf("OpenJSONL: %v", err)
	}
	t.Cleanup(func() { _ = trail.Close() })

	if err := trail.Append(context.Background(), Record{Role: "overseer", Kind: "x"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("trail file not created: %v", err)
	}
}

func TestOpenJSONL_AppendModeDoesNotTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trail.jsonl")

	t1, err := OpenJSONL(path)
	if err != nil {
		t.Fatalf("first OpenJSONL: %v", err)
	}
	if err := t1.Append(context.Background(), Record{Role: "orchestrator", Kind: "first"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := t1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	t2, err := OpenJSONL(path)
	if err != nil {
		t.Fatalf("second OpenJSONL: %v", err)
	}
	if err := t2.Append(context.Background(), Record{Role: "overseer", Kind: "second"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := t2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open for read: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Should have both lines — append mode preserves prior content.
	if got := bytes.Count(data, []byte("\n")); got != 2 {
		t.Errorf("file has %d lines, want 2 (append mode lost or duplicated content)\nraw=%q", got, data)
	}
}

func TestOpenJSONL_EmptyPath(t *testing.T) {
	_, err := OpenJSONL("")
	if err == nil {
		t.Error("OpenJSONL(\"\") returned nil error")
	}
}

func mustParseTime(t *testing.T, s string) (out time.Time) {
	t.Helper()
	out, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return out
}
