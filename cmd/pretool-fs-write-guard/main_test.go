package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		envelope string
		wantCode int
		wantErr  string // substring expected on stderr when blocked
	}{
		{
			name:     "non-edit tool ignored",
			envelope: `{"tool_name":"Bash","tool_input":{"command":"rm /etc/x"}}`,
			wantCode: 0,
		},
		{
			name:     "write to worktree allowed",
			envelope: `{"tool_name":"Write","tool_input":{"file_path":"~/worktrees/x/f"}}`,
			wantCode: 0,
		},
		{
			name:     "write to src blocked",
			envelope: `{"tool_name":"Write","tool_input":{"file_path":"~/src/dear-agent/f"}}`,
			wantCode: 2,
			wantErr:  "PERMISSION_ESCALATION",
		},
		{
			name:     "edit dotfile blocked",
			envelope: `{"tool_name":"Edit","tool_input":{"file_path":"~/.gitconfig"}}`,
			wantCode: 2,
			wantErr:  "dotfile",
		},
		{
			name:     "missing path ignored",
			envelope: `{"tool_name":"Edit","tool_input":{}}`,
			wantCode: 0,
		},
		{
			name:     "garbage fails open",
			envelope: `not json`,
			wantCode: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var errBuf bytes.Buffer
			code := run(strings.NewReader(tc.envelope), &errBuf)
			if code != tc.wantCode {
				t.Fatalf("run() code=%d, want %d (stderr=%q)", code, tc.wantCode, errBuf.String())
			}
			if tc.wantErr != "" && !strings.Contains(errBuf.String(), tc.wantErr) {
				t.Fatalf("run() stderr=%q, want substring %q", errBuf.String(), tc.wantErr)
			}
		})
	}
}
