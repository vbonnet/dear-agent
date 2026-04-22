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
		SchemaVersion: "2.0",
		SessionID:     "test-session",
		Name:          "test-session",
		CreatedAt:     testTime(),
		UpdatedAt:     testTime(),
		Lifecycle:     "",
		Context: Context{
			Project: "~/code",
			Purpose: "Test session",
			Tags:    []string{"test", "dev"},
			Notes:   "Test notes",
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
			name: "missing name",
			modify: func(m *Manifest) {
				m.Name = ""
			},
			wantErr: true,
		},
		{
			name: "missing schema_version",
			modify: func(m *Manifest) {
				m.SchemaVersion = ""
			},
			wantErr: true,
		},
		{
			name: "missing context.project",
			modify: func(m *Manifest) {
				m.Context.Project = ""
			},
			wantErr: true,
		},
		{
			name: "invalid lifecycle",
			modify: func(m *Manifest) {
				m.Lifecycle = "invalid"
			},
			wantErr: true,
		},
		{
			name: "valid lifecycle reaping",
			modify: func(m *Manifest) {
				m.Lifecycle = LifecycleReaping
			},
			wantErr: false,
		},
		{
			name: "valid lifecycle archived",
			modify: func(m *Manifest) {
				m.Lifecycle = LifecycleArchived
			},
			wantErr: false,
		},
		{
			name: "purpose too long",
			modify: func(m *Manifest) {
				m.Context.Purpose = string(make([]byte, MaxPurposeLen+1))
			},
			wantErr: true,
		},
		{
			name: "too many tags",
			modify: func(m *Manifest) {
				m.Context.Tags = make([]string, MaxTagsCount+1)
				for i := range m.Context.Tags {
					m.Context.Tags[i] = "tag"
				}
			},
			wantErr: true,
		},
		{
			name: "tag too long",
			modify: func(m *Manifest) {
				m.Context.Tags = []string{string(make([]byte, MaxTagLen+1))}
			},
			wantErr: true,
		},
		{
			name: "notes too long",
			modify: func(m *Manifest) {
				m.Context.Notes = string(make([]byte, MaxNotesLen+1))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := *validManifest
			tt.modify(&m)
			err := m.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidate_ImportedHistoricalSession verifies that manifests with
// tmux.session_name set pass validation even when no real tmux session exists.
// This is the "imported historical session" pattern used when creating
// manifests for past Claude Code conversations (e.g., brain-v2 personal
// workspace sessions imported from projects.json).
func TestValidate_ImportedHistoricalSession(t *testing.T) {
	m := &Manifest{
		SchemaVersion: "2.0",
		SessionID:     "chezmoi-dotfiles-setup",
		Name:          "chezmoi-dotfiles-setup",
		CreatedAt:     testTime(),
		UpdatedAt:     testTime(),
		Lifecycle:     LifecycleArchived,
		Context: Context{
			Project: "/Users/testuser",
		},
		Tmux: Tmux{
			SessionName: "chezmoi-dotfiles-setup",
		},
		Harness: "claude-code",
	}

	err := m.Validate()
	assert.NoError(t, err, "Imported historical session with tmux.session_name should pass validation")

	// Also verify that removing tmux.session_name causes failure
	m.Tmux.SessionName = ""
	err = m.Validate()
	assert.Error(t, err, "Missing tmux.session_name should fail validation")
	assert.Contains(t, err.Error(), "tmux.session_name")
}
