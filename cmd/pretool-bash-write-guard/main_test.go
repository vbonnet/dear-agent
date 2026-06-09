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
		wantErr  string
	}{
		{
			name:     "non-bash tool ignored",
			envelope: `{"tool_name":"Write","tool_input":{"command":"x"}}`,
			wantCode: 0,
		},
		{
			name:     "read command allowed",
			envelope: `{"tool_name":"Bash","tool_input":{"command":"cat ~/src/dear-agent/f"}}`,
			wantCode: 0,
		},
		{
			name:     "rm in src blocked",
			envelope: `{"tool_name":"Bash","tool_input":{"command":"rm ~/src/dear-agent/f"}}`,
			wantCode: 2,
			wantErr:  "PERMISSION_ESCALATION",
		},
		{
			name:     "git commit in src blocked",
			envelope: `{"tool_name":"Bash","tool_input":{"command":"git -C ~/src/dear-agent commit -m x"}}`,
			wantCode: 2,
			wantErr:  "worktree",
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
