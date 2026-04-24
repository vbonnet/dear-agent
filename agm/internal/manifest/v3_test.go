package manifest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func validV3() *ManifestV3 {
	return &ManifestV3{
		SchemaVersion:  "3.0",
		SessionID:      "v3-session",
		Name:           "v3-session",
		CreatedAt:      testTime(),
		UpdatedAt:      testTime(),
		Lifecycle:      "",
		Context:        Context{Project: "~/code"},
		Claude:         Claude{},
		Tmux:           Tmux{SessionName: "claude-1"},
		Harness:        "claude-code",
		HarnessHistory: []HarnessSwitch{},
	}
}

func TestManifestV3_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ManifestV3)
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid v3 manifest",
			modify:  func(m *ManifestV3) {},
			wantErr: false,
		},
		{
			name: "missing harness",
			modify: func(m *ManifestV3) {
				m.Harness = ""
			},
			wantErr: true,
			errMsg:  "harness field cannot be empty",
		},
		{
			name: "nil harness history",
			modify: func(m *ManifestV3) {
				m.HarnessHistory = nil
			},
			wantErr: true,
			errMsg:  "harness_history cannot be nil",
		},
		{
			name: "unknown harness allowed",
			modify: func(m *ManifestV3) {
				m.Harness = "future-harness"
			},
			wantErr: false,
		},
		{
			name: "valid harness history",
			modify: func(m *ManifestV3) {
				m.HarnessHistory = []HarnessSwitch{
					{
						Timestamp:   time.Now(),
						FromHarness: "claude-code",
						ToHarness:   "gemini-cli",
					},
				}
			},
			wantErr: false,
		},
		{
			name: "harness history with zero timestamp",
			modify: func(m *ManifestV3) {
				m.HarnessHistory = []HarnessSwitch{
					{
						Timestamp:   time.Time{},
						FromHarness: "claude-code",
						ToHarness:   "gemini-cli",
					},
				}
			},
			wantErr: true,
			errMsg:  "timestamp cannot be zero",
		},
		{
			name: "harness history with empty from_harness",
			modify: func(m *ManifestV3) {
				m.HarnessHistory = []HarnessSwitch{
					{
						Timestamp:   time.Now(),
						FromHarness: "",
						ToHarness:   "gemini-cli",
					},
				}
			},
			wantErr: true,
			errMsg:  "from_harness cannot be empty",
		},
		{
			name: "harness history with empty to_harness",
			modify: func(m *ManifestV3) {
				m.HarnessHistory = []HarnessSwitch{
					{
						Timestamp:   time.Now(),
						FromHarness: "claude-code",
						ToHarness:   "",
					},
				}
			},
			wantErr: true,
			errMsg:  "to_harness cannot be empty",
		},
		{
			name: "inherits v2 validation - missing session_id",
			modify: func(m *ManifestV3) {
				m.SessionID = ""
			},
			wantErr: true,
			errMsg:  "session_id is required",
		},
		{
			name: "inherits v2 validation - missing name",
			modify: func(m *ManifestV3) {
				m.Name = ""
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "inherits v2 validation - missing context.project",
			modify: func(m *ManifestV3) {
				m.Context.Project = ""
			},
			wantErr: true,
			errMsg:  "context.project is required",
		},
		{
			name: "all known harnesses",
			modify: func(m *ManifestV3) {
				// Test each known harness validates
				for _, h := range []string{"claude-code", "gemini-cli", "codex-cli", "opencode-cli"} {
					m.Harness = h
				}
			},
			wantErr: false,
		},
		{
			name: "multiple harness history entries",
			modify: func(m *ManifestV3) {
				now := time.Now()
				m.HarnessHistory = []HarnessSwitch{
					{Timestamp: now.Add(-2 * time.Hour), FromHarness: "claude-code", ToHarness: "gemini-cli"},
					{Timestamp: now.Add(-1 * time.Hour), FromHarness: "gemini-cli", ToHarness: "codex-cli"},
					{Timestamp: now, FromHarness: "codex-cli", ToHarness: "claude-code"},
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validV3()
			tt.modify(m)
			err := m.Validate()
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
