package main

import (
	"encoding/json"
	"testing"
	"time"
)

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
