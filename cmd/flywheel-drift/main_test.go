package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	healthchecker "github.com/vbonnet/dear-agent/pkg/health-checker"
)

type malformedStatusCheck struct{}

func (malformedStatusCheck) Name() string     { return "malformed" }
func (malformedStatusCheck) Category() string { return "compatibility" }
func (malformedStatusCheck) Run(context.Context) healthchecker.Result {
	return healthchecker.Result{
		Name:     "untrusted-name",
		Category: "untrusted-category",
		Status:   healthchecker.Status("future"),
		Message:  "producer diagnostic",
		Fixable:  true,
		Fix: &healthchecker.Fix{
			Apply: func(context.Context) error { return nil },
		},
	}
}

func TestToJSONResultsPreservesValidStatusWireValues(t *testing.T) {
	wantStatuses := []healthchecker.Status{
		healthchecker.StatusOK,
		healthchecker.StatusInfo,
		healthchecker.StatusWarning,
		healthchecker.StatusError,
	}
	results := make([]healthchecker.Result, len(wantStatuses))
	for i, status := range wantStatuses {
		results[i] = healthchecker.Result{
			Name:     "check-" + string(rune('a'+i)),
			Category: "wire",
			Status:   status,
			Message:  "message",
			Fixable:  i == 2,
		}
	}

	var output bytes.Buffer
	exitCode, err := writeJSONReport(&output, results)
	if err != nil {
		t.Fatalf("writeJSONReport() error = %v", err)
	}
	if exitCode != 2 {
		t.Errorf("writeJSONReport() exit code = %d, want 2", exitCode)
	}
	var wire []struct {
		Name     string `json:"name"`
		Category string `json:"category"`
		Status   string `json:"status"`
		Message  string `json:"message"`
		Fixable  bool   `json:"fixable"`
	}
	if err := json.Unmarshal(output.Bytes(), &wire); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(wire) != len(wantStatuses) {
		t.Fatalf("writeJSONReport() length = %d, want %d", len(wire), len(wantStatuses))
	}

	for i, wantStatus := range wantStatuses {
		if wire[i].Status != string(wantStatus) {
			t.Errorf("wire[%d].Status = %q, want %q", i, wire[i].Status, wantStatus)
		}
		if wire[i].Name != results[i].Name || wire[i].Category != results[i].Category || wire[i].Message != results[i].Message {
			t.Errorf("wire[%d] fields = %+v, want source fields preserved", i, wire[i])
		}
		if wire[i].Fixable != results[i].Fixable {
			t.Errorf("wire[%d].Fixable = %v, want %v", i, wire[i].Fixable, results[i].Fixable)
		}
	}
}

func TestRunnerMalformedResultSerializesAsError(t *testing.T) {
	results, err := healthchecker.NewRunner(malformedStatusCheck{}).RunAll(context.Background())
	if err != nil {
		t.Fatalf("RunAll() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("RunAll() returned %d results, want 1", len(results))
	}
	if results[0].Fixable || results[0].Fix != nil {
		t.Fatalf("RunAll() retained executable malformed metadata: %+v", results[0])
	}

	var output bytes.Buffer
	exitCode, err := writeJSONReport(&output, results)
	if err != nil {
		t.Fatalf("writeJSONReport() error = %v", err)
	}
	if !strings.Contains(output.String(), `"status": "error"`) {
		t.Errorf("serialized result = %s, want error status", output.String())
	}
	if exitCode != 2 {
		t.Errorf("writeJSONReport() exit code = %d, want 2", exitCode)
	}
}

func TestParseBeads(t *testing.T) {
	raw := `[
		{"id":"foo-123","title":"Do stuff","status":"open","updated_at":"2026-01-01T00:00:00Z"},
		{"id":"foo-456","title":"Another","status":"open","updated_at":"2026-06-10T12:00:00Z"}
	]`
	beads, err := parseBeads([]byte(raw))
	if err != nil {
		t.Fatalf("parseBeads: %v", err)
	}
	if len(beads) != 2 {
		t.Fatalf("len = %d, want 2", len(beads))
	}
	if beads[0].ID != "foo-123" {
		t.Errorf("ID = %q, want foo-123", beads[0].ID)
	}
	if beads[0].UpdatedAt.Year() != 2026 {
		t.Errorf("UpdatedAt year = %d, want 2026", beads[0].UpdatedAt.Year())
	}
}

func TestParseBeadsEmpty(t *testing.T) {
	beads, err := parseBeads([]byte("[]"))
	if err != nil {
		t.Fatalf("parseBeads([]): %v", err)
	}
	if len(beads) != 0 {
		t.Errorf("len = %d, want 0", len(beads))
	}
}

func TestParseBeadsInvalid(t *testing.T) {
	if _, err := parseBeads([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestFilterStale(t *testing.T) {
	now := time.Now()
	threshold := now.Add(-7 * 24 * time.Hour)

	fresh := beadEntry{ID: "fresh", UpdatedAt: now.Add(-1 * 24 * time.Hour)}
	old := beadEntry{ID: "old", UpdatedAt: now.Add(-10 * 24 * time.Hour)}
	ancient := beadEntry{ID: "ancient", UpdatedAt: now.Add(-30 * 24 * time.Hour)}

	stale := filterStale([]beadEntry{fresh, old, ancient}, threshold)
	if len(stale) != 2 {
		t.Fatalf("filterStale returned %d items, want 2", len(stale))
	}
	if stale[0].ID != "old" || stale[1].ID != "ancient" {
		t.Errorf("unexpected stale IDs: %v", stale)
	}
}

func TestFilterStaleNone(t *testing.T) {
	now := time.Now()
	threshold := now.Add(-7 * 24 * time.Hour)
	recent := beadEntry{ID: "recent", UpdatedAt: now.Add(-2 * 24 * time.Hour)}
	if got := filterStale([]beadEntry{recent}, threshold); len(got) != 0 {
		t.Errorf("filterStale returned %d, want 0", len(got))
	}
}

func TestFilterStaleAll(t *testing.T) {
	threshold := time.Now()
	b := beadEntry{ID: "x", UpdatedAt: time.Now().Add(-1 * time.Hour)}
	if got := filterStale([]beadEntry{b}, threshold); len(got) != 1 {
		t.Errorf("filterStale returned %d, want 1", len(got))
	}
}

// TestBeadEntryJSONRoundtrip ensures the struct matches bd's JSON schema.
func TestBeadEntryJSONRoundtrip(t *testing.T) {
	ts := time.Date(2026, 6, 9, 19, 27, 55, 0, time.UTC)
	b := beadEntry{ID: "vbonnet-ai-apq", Title: "test", Status: "open", UpdatedAt: ts}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var b2 beadEntry
	if err := json.Unmarshal(data, &b2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !b2.UpdatedAt.Equal(ts) {
		t.Errorf("UpdatedAt = %v, want %v", b2.UpdatedAt, ts)
	}
}
