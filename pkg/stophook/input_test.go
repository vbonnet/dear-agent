package stophook

import (
	"strings"
	"testing"
)

func TestReadInput(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantID  string
		wantCwd string
		wantErr bool
	}{
		{
			name:    "Claude Code input",
			json:    `{"harness":"claude-code","session_id":"abc","cwd":"/tmp/proj","stop_reason":"user"}`,
			wantID:  "abc",
			wantCwd: "/tmp/proj",
		},
		{name: "Codex aliases", json: `{"harness":"codex-cli","thread_id":"codex-1","workspace_dir":"/tmp/codex","reason":"complete"}`, wantID: "codex-1", wantCwd: "/tmp/codex"},
		{name: "Antigravity aliases", json: `{"harness":"agy","conversation_id":"agy-1","project_dir":"/tmp/agy"}`, wantID: "agy-1", wantCwd: "/tmp/agy"},
		{name: "OpenCode normalized input", json: `{"harness":"opencode-cli","session_id":"oc-1","cwd":"/tmp/opencode"}`, wantID: "oc-1", wantCwd: "/tmp/opencode"},
		{
			name:    "empty cwd",
			json:    `{"session_id":"abc"}`,
			wantID:  "abc",
			wantCwd: "",
		},
		{
			name:    "invalid json",
			json:    `not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := ReadInput(strings.NewReader(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if input.Cwd != tt.wantCwd {
				t.Errorf("Cwd = %q, want %q", input.Cwd, tt.wantCwd)
			}
			if input.SessionID != tt.wantID {
				t.Errorf("SessionID = %q, want %q", input.SessionID, tt.wantID)
			}
		})
	}
}
