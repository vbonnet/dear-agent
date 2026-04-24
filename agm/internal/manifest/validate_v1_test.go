package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func validV1() *ManifestV1 {
	return &ManifestV1{
		SchemaVersion: "1.0",
		SessionID:     "test-session",
		Status:        StatusActive,
		CreatedAt:     testTime(),
		LastActivity:  testTime(),
		Worktree:      WorktreeV1{Path: "~/code"},
		Claude: ClaudeV1{
			SessionID:       "c86ffd41-cbcc-4bfa-8b1f-4da7c83fc3d2",
			SessionEnvPath:  "/tmp/env",
			FileHistoryPath: "/tmp/history",
			StartedAt:       testTime(),
			LastActivity:    testTime(),
		},
		Tmux: TmuxV1{
			SessionName: "claude-1",
			WindowName:  "main",
			CreatedAt:   testTime(),
		},
	}
}

func TestValidateV1(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ManifestV1)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid v1 manifest",
			modify:  func(m *ManifestV1) {},
			wantErr: false,
		},
		{
			name: "missing session_id",
			modify: func(m *ManifestV1) {
				m.SessionID = ""
			},
			wantErr: true,
			errMsg:  "session_id is required",
		},
		{
			name: "invalid session_id chars",
			modify: func(m *ManifestV1) {
				m.SessionID = "has spaces!"
			},
			wantErr: true,
			errMsg:  "invalid session_id",
		},
		{
			name: "session_id too long",
			modify: func(m *ManifestV1) {
				b := make([]byte, 101)
				for i := range b {
					b[i] = 'a'
				}
				m.SessionID = string(b)
			},
			wantErr: true,
			errMsg:  "session_id too long",
		},
		{
			name: "missing status",
			modify: func(m *ManifestV1) {
				m.Status = ""
			},
			wantErr: true,
			errMsg:  "status is required",
		},
		{
			name: "invalid status",
			modify: func(m *ManifestV1) {
				m.Status = "running"
			},
			wantErr: true,
			errMsg:  "invalid status",
		},
		{
			name: "status active",
			modify: func(m *ManifestV1) {
				m.Status = StatusActive
			},
			wantErr: false,
		},
		{
			name: "status discovered",
			modify: func(m *ManifestV1) {
				m.Status = StatusDiscovered
			},
			wantErr: false,
		},
		{
			name: "status archived",
			modify: func(m *ManifestV1) {
				m.Status = StatusArchived
			},
			wantErr: false,
		},
		{
			name: "zero created_at",
			modify: func(m *ManifestV1) {
				m.CreatedAt = time.Time{}
			},
			wantErr: true,
			errMsg:  "created_at is required",
		},
		{
			name: "zero last_activity",
			modify: func(m *ManifestV1) {
				m.LastActivity = time.Time{}
			},
			wantErr: true,
			errMsg:  "last_activity is required",
		},
		{
			name: "missing worktree path",
			modify: func(m *ManifestV1) {
				m.Worktree.Path = ""
			},
			wantErr: true,
			errMsg:  "worktree.path is required",
		},
		{
			name: "missing claude session_id",
			modify: func(m *ManifestV1) {
				m.Claude.SessionID = ""
			},
			wantErr: true,
			errMsg:  "claude.session_id is required",
		},
		{
			name: "invalid claude UUID",
			modify: func(m *ManifestV1) {
				m.Claude.SessionID = "not-a-uuid"
			},
			wantErr: true,
			errMsg:  "invalid claude.session_id",
		},
		{
			name: "missing tmux session_name",
			modify: func(m *ManifestV1) {
				m.Tmux.SessionName = ""
			},
			wantErr: true,
			errMsg:  "tmux.session_name is required",
		},
		{
			name: "invalid tmux session_name",
			modify: func(m *ManifestV1) {
				m.Tmux.SessionName = "has spaces"
			},
			wantErr: true,
			errMsg:  "invalid tmux.session_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validV1()
			tt.modify(m)
			err := ValidateV1(m)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "path outside home - /etc",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "path outside home - /tmp",
			path:    "/tmp/something",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSessionID_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "empty string",
			id:      "",
			wantErr: true,
		},
		{
			name:    "single character",
			id:      "a",
			wantErr: false,
		},
		{
			name:    "dots and dashes",
			id:      "my.session-name_v2",
			wantErr: false,
		},
		{
			name:    "slash not allowed",
			id:      "path/session",
			wantErr: true,
		},
		{
			name:    "colon not allowed",
			id:      "session:1",
			wantErr: true,
		},
		{
			name:    "exactly 100 chars",
			id:      string(make([]byte, 100)),
			wantErr: true, // null bytes not in pattern
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
