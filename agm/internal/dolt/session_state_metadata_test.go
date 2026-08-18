package dolt

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// State and final output must survive the metadata round trip: before they
// were persisted, every `agm session state set` (hook- or watcher-issued) was
// silently dropped, which disabled the stall detector and made completed
// sessions unreadable (ce-0zng9).
func TestSessionStateMetadataRoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 11, 10, 30, 0, 0, time.UTC)
	src := &manifest.Manifest{
		State:          manifest.StateDone,
		StateUpdatedAt: at,
		StateSource:    "completion-watcher",
		FinalOutput:    "RESULT: opened PR #99",
		FinalOutputAt:  at,
	}

	b, err := json.Marshal(buildSessionMetadata(src))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got manifest.Manifest
	if err := unmarshalSessionMetadata(&got, b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.State != manifest.StateDone {
		t.Fatalf("State = %q, want %q", got.State, manifest.StateDone)
	}
	if !got.StateUpdatedAt.Equal(at) {
		t.Fatalf("StateUpdatedAt = %v, want %v", got.StateUpdatedAt, at)
	}
	if got.StateSource != "completion-watcher" {
		t.Fatalf("StateSource = %q", got.StateSource)
	}
	if got.FinalOutput != "RESULT: opened PR #99" {
		t.Fatalf("FinalOutput = %q", got.FinalOutput)
	}
	if !got.FinalOutputAt.Equal(at) {
		t.Fatalf("FinalOutputAt = %v, want %v", got.FinalOutputAt, at)
	}
}

func TestSessionStateMetadataOmittedWhenEmpty(t *testing.T) {
	meta := buildSessionMetadata(&manifest.Manifest{})
	for _, key := range []string{"state", "state_updated_at", "state_source", "final_output", "final_output_at"} {
		if _, present := meta[key]; present {
			t.Fatalf("empty manifest serialized %q", key)
		}
	}
}
