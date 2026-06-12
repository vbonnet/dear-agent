package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/fsguard"
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
			var outBuf, errBuf bytes.Buffer
			code := run(strings.NewReader(tc.envelope), &outBuf, &errBuf)
			if code != tc.wantCode {
				t.Fatalf("run() code=%d, want %d (stderr=%q)", code, tc.wantCode, errBuf.String())
			}
			if tc.wantErr != "" && !strings.Contains(errBuf.String(), tc.wantErr) {
				t.Fatalf("run() stderr=%q, want substring %q", errBuf.String(), tc.wantErr)
			}
		})
	}
}

// TestRunWarnEnforcement verifies that FSGUARD_ENFORCEMENT=warn emits a JSON
// hook response with permissionDecision "allow" for Bash write blocks.
func TestRunWarnEnforcement(t *testing.T) {
	t.Setenv(fsguard.EnvEnforcement, "warn")
	var outBuf, errBuf bytes.Buffer
	code := run(
		strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"rm ~/src/repo/f"}}`),
		&outBuf, &errBuf,
	)
	if code != 0 {
		t.Fatalf("warn enforcement exited %d, want 0", code)
	}
	if errBuf.Len() > 0 {
		t.Errorf("unexpected stderr in warn mode: %q", errBuf.String())
	}
	var resp fsguard.HookResponse
	if err := json.NewDecoder(&outBuf).Decode(&resp); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (got %q)", err, outBuf.String())
	}
	if resp.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %q, want %q", resp.PermissionDecision, "allow")
	}
	if resp.Message == "" {
		t.Error("message is empty in warn response")
	}
}

// TestRunAskEnforcement verifies that FSGUARD_ENFORCEMENT=ask emits a JSON
// hook response with permissionDecision "ask".
func TestRunAskEnforcement(t *testing.T) {
	t.Setenv(fsguard.EnvEnforcement, "ask")
	var outBuf, errBuf bytes.Buffer
	code := run(
		strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"rm ~/src/repo/f"}}`),
		&outBuf, &errBuf,
	)
	if code != 0 {
		t.Fatalf("ask enforcement exited %d, want 0", code)
	}
	var resp fsguard.HookResponse
	if err := json.NewDecoder(&outBuf).Decode(&resp); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (got %q)", err, outBuf.String())
	}
	if resp.PermissionDecision != "ask" {
		t.Errorf("permissionDecision = %q, want %q", resp.PermissionDecision, "ask")
	}
}

// TestRunDeferEnforcement verifies that FSGUARD_ENFORCEMENT=defer emits a JSON
// hook response with permissionDecision "defer".
func TestRunDeferEnforcement(t *testing.T) {
	t.Setenv(fsguard.EnvEnforcement, "defer")
	var outBuf, errBuf bytes.Buffer
	code := run(
		strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"rm ~/src/repo/f"}}`),
		&outBuf, &errBuf,
	)
	if code != 0 {
		t.Fatalf("defer enforcement exited %d, want 0", code)
	}
	var resp fsguard.HookResponse
	if err := json.NewDecoder(&outBuf).Decode(&resp); err != nil {
		t.Fatalf("stdout is not valid JSON: %v (got %q)", err, outBuf.String())
	}
	if resp.PermissionDecision != "defer" {
		t.Errorf("permissionDecision = %q, want %q", resp.PermissionDecision, "defer")
	}
}
