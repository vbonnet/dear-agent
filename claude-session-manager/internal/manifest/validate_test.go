package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid session ID",
			id:      "github.com-user-repo-main",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			id:      "my_session_123",
			wantErr: false,
		},
		{
			name:    "valid with dashes",
			id:      "my-session-123",
			wantErr: false,
		},
		{
			name:    "invalid with spaces",
			id:      "my session",
			wantErr: true,
		},
		{
			name:    "invalid with special chars",
			id:      "my@session",
			wantErr: true,
		},
		{
			name:    "too long",
			id:      "a" + string(make([]byte, 100)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSessionID(tt.id)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name    string
		uuid    string
		wantErr bool
	}{
		{
			name:    "valid UUID",
			uuid:    "c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2",
			wantErr: false,
		},
		{
			name:    "invalid format",
			uuid:    "not-a-uuid",
			wantErr: true,
		},
		{
			name:    "uppercase (invalid)",
			uuid:    "C86FFD41-CBCC-4BFA-8B1F-4DA7C83FC3D2",
			wantErr: true,
		},
		{
			name:    "missing dashes",
			uuid:    "c86ffd41cbcc4bfa8b1f4da7c83fc3d2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUUID(tt.uuid)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	validManifest := &Manifest{
		SessionID:    "test-session",
		Status:       StatusActive,
		CreatedAt:    testTime(),
		LastActivity: testTime(),
		Worktree: Worktree{
			Path:   "/home/user/code",
			Branch: "main",
		},
		Claude: Claude{
			SessionID: "c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2",
		},
		Tmux: Tmux{
			SessionName: "claude-1",
		},
	}

	tests := []struct {
		name    string
		modify  func(*Manifest)
		wantErr bool
	}{
		{
			name:    "valid manifest",
			modify:  func(m *Manifest) {},
			wantErr: false,
		},
		{
			name: "missing session_id",
			modify: func(m *Manifest) {
				m.SessionID = ""
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			modify: func(m *Manifest) {
				m.Status = "invalid"
			},
			wantErr: true,
		},
		{
			name: "missing worktree path",
			modify: func(m *Manifest) {
				m.Worktree.Path = ""
			},
			wantErr: true,
		},
		{
			name: "invalid claude UUID",
			modify: func(m *Manifest) {
				m.Claude.SessionID = "not-a-uuid"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := *validManifest
			tt.modify(&m)
			err := Validate(&m)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
