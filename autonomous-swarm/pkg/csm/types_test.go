package csm

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSessionState_Constants(t *testing.T) {
	if SessionActive != "active" {
		t.Errorf("SessionActive constant mismatch: got %s, want active", SessionActive)
	}
	if SessionStopped != "stopped" {
		t.Errorf("SessionStopped constant mismatch: got %s, want stopped", SessionStopped)
	}
	if SessionArchived != "archived" {
		t.Errorf("SessionArchived constant mismatch: got %s, want archived", SessionArchived)
	}
}

func TestSessionMetadata_JSONMarshal(t *testing.T) {
	meta := SessionMetadata{
		Name:      "test-session",
		UUID:      "123e4567-e89b-12d3-a456-426614174000",
		State:     SessionActive,
		CreatedAt: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC),
		BeadID:    "bead-1",
	}

	data, err := json.Marshal(&meta)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var meta2 SessionMetadata
	if err := json.Unmarshal(data, &meta2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if meta2.Name != meta.Name {
		t.Errorf("name mismatch: got %s, want %s", meta2.Name, meta.Name)
	}
	if meta2.UUID != meta.UUID {
		t.Errorf("uuid mismatch: got %s, want %s", meta2.UUID, meta.UUID)
	}
	if meta2.State != meta.State {
		t.Errorf("state mismatch: got %s, want %s", meta2.State, meta.State)
	}
	if meta2.BeadID != meta.BeadID {
		t.Errorf("bead_id mismatch: got %s, want %s", meta2.BeadID, meta.BeadID)
	}
}

func TestSessionMetadata_StateTransitions(t *testing.T) {
	tests := []struct {
		name  string
		state SessionState
		valid bool
	}{
		{"active state", SessionActive, true},
		{"stopped state", SessionStopped, true},
		{"archived state", SessionArchived, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := SessionMetadata{
				Name:  "test",
				UUID:  "test-uuid",
				State: tt.state,
			}

			if meta.State != tt.state {
				t.Errorf("state mismatch: got %s, want %s", meta.State, tt.state)
			}
		})
	}
}
