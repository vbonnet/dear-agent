package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const sampleJSONL = `{"kind":"build_failure","subject":"go build"}
{"kind":"test_failure","subject":"TestX"}
{"kind":"doc_only","subject":"README"}
{"kind":"doc_only","subject":"CHANGELOG"}
{"kind":"cosmetic","subject":"trim"}
`

func TestSalienceCLITextOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runSalienceWith(
		[]string{"--capacity", "1", "--bypass", "high"},
		strings.NewReader(sampleJSONL), &stdout, &stderr,
	)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"build_failure", "test_failure", "doc_only",
		"notify", "suppress",
		"by tier:", "critical", "high", "low",
		"suppressed because:", "budget_exhausted", "noise_dropped",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q\ngot:\n%s", want, out)
		}
	}
}

func TestSalienceCLIJSONOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runSalienceWith(
		[]string{"--json", "--capacity", "1"},
		strings.NewReader(sampleJSONL), &stdout, &stderr,
	)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	var got struct {
		Outcomes []map[string]any `json:"outcomes"`
		Summary  struct {
			Total       int              `json:"total"`
			Notified    int              `json:"notified"`
			Suppressed  int              `json:"suppressed"`
			NotifyRatio float64          `json:"notifyRatio"`
			ByTier      map[string]int   `json:"byTier"`
			ByKind      map[string]int   `json:"byKind"`
			ByReason    map[string]int   `json:"byReason"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}
	if got.Summary.Total != 5 {
		t.Errorf("total=%d, want 5", got.Summary.Total)
	}
	if got.Summary.ByTier["critical"] == 0 {
		t.Errorf("expected at least one critical: %+v", got.Summary.ByTier)
	}
	if got.Summary.ByReason["noise_dropped"] != 1 {
		t.Errorf("noise_dropped count = %d, want 1", got.Summary.ByReason["noise_dropped"])
	}
}

func TestSalienceCLIInvalidBypass(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runSalienceWith(
		[]string{"--bypass", "urgent"},
		strings.NewReader(sampleJSONL), &stdout, &stderr,
	)
	if rc != 2 {
		t.Errorf("rc=%d, want 2", rc)
	}
	if !strings.Contains(stderr.String(), "unknown tier") {
		t.Errorf("expected error mentioning unknown tier; got %q", stderr.String())
	}
}

func TestSalienceCLIEmptyInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runSalienceWith(
		[]string{},
		strings.NewReader(""), &stdout, &stderr,
	)
	if rc != 1 {
		t.Errorf("rc=%d, want 1", rc)
	}
	if !strings.Contains(stderr.String(), "no signals") {
		t.Errorf("expected empty-input message; got %q", stderr.String())
	}
}

func TestSalienceCLIKeepNoise(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := runSalienceWith(
		[]string{"--json", "--keep-noise", "--capacity", "0"},
		strings.NewReader(`{"kind":"cosmetic","subject":"ws"}`+"\n"),
		&stdout, &stderr,
	)
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%s", rc, stderr.String())
	}
	// json.Encoder pretty-prints with a space after the colon.
	if !strings.Contains(stdout.String(), `"notify": true`) {
		t.Errorf("with --keep-noise + capacity 0, cosmetic should notify; got:\n%s",
			stdout.String())
	}
}
