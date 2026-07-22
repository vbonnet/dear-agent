package manifest

import (
	"testing"
	"time"
)

func TestManifestAcceptsPiMetadata(t *testing.T) {
	t.Parallel()
	m := &Manifest{
		SchemaVersion: SchemaVersion,
		SessionID:     "pi-session",
		Name:          "pi-session",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context:       Context{Project: "/tmp/project"},
		Tmux:          Tmux{SessionName: "pi-session"},
		Harness:       "pi-cli",
		Pi: &Pi{
			SessionID:      "pi-session",
			SessionDir:     "/tmp/pi-sessions",
			TranscriptPath: "/tmp/pi-sessions/session.jsonl",
			CodingAgentDir: "/tmp/pi-agent",
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
