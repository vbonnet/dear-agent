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

func TestSessionMetadata_OpenAIRoundTrip(t *testing.T) {
	src := &manifest.Manifest{
		OpenAI: &manifest.OpenAI{
			SessionsDir:     "/tmp/openai-sessions",
			BaseURL:         "https://azure.example.test",
			IsAzure:         true,
			AzureAPIVersion: "2025-01-01-preview",
			Temperature:     1.2,
			MaxTokens:       2048,
		},
	}
	metadata, err := json.Marshal(buildSessionMetadata(src))
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var got manifest.Manifest
	if err := unmarshalEngramMetadata(&got, metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if got.OpenAI == nil || *got.OpenAI != *src.OpenAI {
		t.Fatalf("OpenAI metadata = %#v, want %#v", got.OpenAI, src.OpenAI)
	}
}

func TestSessionMetadata_AgyRoundTrip(t *testing.T) {
	src := &manifest.Manifest{
		Agy: &manifest.Agy{
			ConversationID: "117ff898-a964-4a9f-b460-1be4a8a49b17",
			WorkspacePath:  "/tmp/agy-probe",
			ConversationDB: "/Users/vbonnet/.gemini/antigravity-cli/conversations/117ff898-a964-4a9f-b460-1be4a8a49b17.db",
			TranscriptPath: "/Users/vbonnet/.gemini/antigravity-cli/brain/117ff898-a964-4a9f-b460-1be4a8a49b17/.system_generated/logs/transcript.jsonl",
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
	if got.Agy == nil {
		t.Fatal("AGY metadata did not round-trip")
	}
	if got.Agy.ConversationID != src.Agy.ConversationID {
		t.Errorf("AGY conversation ID = %q, want %q", got.Agy.ConversationID, src.Agy.ConversationID)
	}
	if got.Agy.WorkspacePath != src.Agy.WorkspacePath {
		t.Errorf("AGY workspace path = %q, want %q", got.Agy.WorkspacePath, src.Agy.WorkspacePath)
	}
	if got.Agy.ConversationDB != src.Agy.ConversationDB {
		t.Errorf("AGY conversation DB = %q, want %q", got.Agy.ConversationDB, src.Agy.ConversationDB)
	}
	if got.Agy.TranscriptPath != src.Agy.TranscriptPath {
		t.Errorf("AGY transcript path = %q, want %q", got.Agy.TranscriptPath, src.Agy.TranscriptPath)
	}
}

func TestSessionMetadata_PiRoundTrip(t *testing.T) {
	src := &manifest.Manifest{
		WorkingDirectory: "/tmp/pi-work",
		Pi: &manifest.Pi{
			SessionID: "pi-native-id", SessionDir: "/tmp/pi-sessions", TranscriptPath: "/tmp/pi-sessions/native.jsonl",
		},
	}
	metadata, err := json.Marshal(buildSessionMetadata(src))
	if err != nil {
		t.Fatal(err)
	}
	got := &manifest.Manifest{}
	if err := unmarshalEngramMetadata(got, metadata); err != nil {
		t.Fatal(err)
	}
	if got.Pi == nil || *got.Pi != *src.Pi {
		t.Fatalf("Pi metadata = %#v, want %#v", got.Pi, src.Pi)
	}
}
