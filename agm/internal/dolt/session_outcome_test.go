package dolt

import (
	"encoding/json"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestSessionOutcome_MetadataRoundTrip verifies the outcome survives a
// marshal→unmarshal cycle through the agm_sessions.metadata JSON column, which
// is the persistence mechanism for the field (no dedicated column).
func TestSessionOutcome_MetadataRoundTrip(t *testing.T) {
	for _, want := range []manifest.SessionOutcome{
		manifest.OutcomeCompleted,
		manifest.OutcomeCrashed,
		manifest.OutcomeKilled,
		manifest.OutcomeGCStale,
	} {
		t.Run(string(want), func(t *testing.T) {
			src := &manifest.Manifest{SessionID: "sid", Name: "n", Outcome: want}
			b, err := json.Marshal(buildSessionMetadata(src))
			if err != nil {
				t.Fatalf("marshal metadata: %v", err)
			}

			var got manifest.Manifest
			if err := unmarshalEngramMetadata(&got, b); err != nil {
				t.Fatalf("unmarshal metadata: %v", err)
			}
			if got.Outcome != want {
				t.Errorf("round-trip outcome = %q, want %q", got.Outcome, want)
			}
		})
	}
}

// TestSessionOutcome_UnknownNotPersisted: an empty outcome is omitted from the
// metadata blob entirely (no "outcome" key) so legacy records stay clean.
func TestSessionOutcome_UnknownNotPersisted(t *testing.T) {
	meta := buildSessionMetadata(&manifest.Manifest{Outcome: manifest.OutcomeUnknown})
	if _, ok := meta["outcome"]; ok {
		t.Errorf("expected no 'outcome' key for unknown outcome, got %v", meta["outcome"])
	}
}

// TestSessionOutcome_ReadAlongsideEngram: outcome is read even when engram
// metadata is present and enabled (it is read before the engram early-return).
func TestSessionOutcome_ReadAlongsideEngram(t *testing.T) {
	src := &manifest.Manifest{
		Outcome:        manifest.OutcomeGCStale,
		EngramMetadata: &manifest.EngramMetadata{Enabled: true, Query: "q", Count: 2},
	}
	b, err := json.Marshal(buildSessionMetadata(src))
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var got manifest.Manifest
	if err := unmarshalEngramMetadata(&got, b); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if got.Outcome != manifest.OutcomeGCStale {
		t.Errorf("outcome = %q, want %q", got.Outcome, manifest.OutcomeGCStale)
	}
	if got.EngramMetadata == nil || got.EngramMetadata.Query != "q" {
		t.Errorf("engram metadata not preserved alongside outcome: %+v", got.EngramMetadata)
	}
}

func TestSessionMetadata_CodexRoundTrip(t *testing.T) {
	src := &manifest.Manifest{
		Codex: &manifest.Codex{
			SessionID:      "019ef2af-97e0-7443-9f07-03e40636740c",
			TranscriptPath: "/Users/vbonnet/.codex/sessions/rollout.jsonl",
		},
	}
	b, err := json.Marshal(buildSessionMetadata(src))
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var got manifest.Manifest
	if err := unmarshalEngramMetadata(&got, b); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if got.Codex == nil {
		t.Fatal("Codex metadata did not round-trip")
	}
	if got.Codex.SessionID != src.Codex.SessionID {
		t.Errorf("Codex session ID = %q, want %q", got.Codex.SessionID, src.Codex.SessionID)
	}
	if got.Codex.TranscriptPath != src.Codex.TranscriptPath {
		t.Errorf("Codex transcript path = %q, want %q", got.Codex.TranscriptPath, src.Codex.TranscriptPath)
	}
}
