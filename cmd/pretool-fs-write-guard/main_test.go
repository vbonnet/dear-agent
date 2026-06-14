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
			name:     "notebook in worktree allowed",
			envelope: `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"~/worktrees/x/notebook.ipynb"}}`,
			wantCode: 0,
		},
		{
			name:     "notebook in src blocked",
			envelope: `{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"~/src/dear-agent/nb.ipynb"}}`,
			wantCode: 2,
			wantErr:  "PERMISSION_ESCALATION",
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
// hook response with permissionDecision "allow" rather than exiting 2.
func TestRunWarnEnforcement(t *testing.T) {
	t.Setenv(fsguard.EnvEnforcement, "warn")
	var outBuf, errBuf bytes.Buffer
	code := run(
		strings.NewReader(`{"tool_name":"Write","tool_input":{"file_path":"~/src/repo/f"}}`),
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
		strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"~/.gitconfig"}}`),
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
// hook response with permissionDecision "defer" and no message.
func TestRunDeferEnforcement(t *testing.T) {
	t.Setenv(fsguard.EnvEnforcement, "defer")
	var outBuf, errBuf bytes.Buffer
	code := run(
		strings.NewReader(`{"tool_name":"Write","tool_input":{"file_path":"~/src/repo/f"}}`),
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

// TestRunAllowedWriteNeverTouchesOut verifies that allowed writes produce no
// stdout or stderr regardless of enforcement level.
func TestRunAllowedWriteNeverTouchesOut(t *testing.T) {
	for _, level := range []string{"deny", "warn", "ask", "defer"} {
		t.Run(level, func(t *testing.T) {
			t.Setenv(fsguard.EnvEnforcement, level)
			var outBuf, errBuf bytes.Buffer
			code := run(
				strings.NewReader(`{"tool_name":"Write","tool_input":{"file_path":"~/worktrees/x/f"}}`),
				&outBuf, &errBuf,
			)
			if code != 0 {
				t.Fatalf("allowed write exited %d", code)
			}
			if outBuf.Len() > 0 {
				t.Errorf("stdout non-empty for allowed write: %q", outBuf.String())
			}
		})
	}
}

func TestEmitDecision(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		enforcement  fsguard.Enforcement
		wantCode     int
		wantDecision string // JSON permissionDecision, empty means stderr path
	}{
		{"deny", fsguard.EnforceDeny, 2, ""},
		{"warn", fsguard.EnforceWarn, 0, "allow"},
		{"ask", fsguard.EnforceAsk, 0, "ask"},
		{"defer", fsguard.EnforceDefer, 0, "defer"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := fsguard.Decision{
				Allowed:     false,
				Message:     "test guidance message",
				Enforcement: tc.enforcement,
			}
			var outBuf, errBuf bytes.Buffer
			code := emitDecision(d, &outBuf, &errBuf)
			if code != tc.wantCode {
				t.Fatalf("emitDecision code=%d, want %d", code, tc.wantCode)
			}
			if tc.wantDecision == "" {
				// Deny path: message goes to stderr, stdout is empty.
				if !strings.Contains(errBuf.String(), "test guidance") {
					t.Errorf("stderr=%q missing guidance message", errBuf.String())
				}
				if outBuf.Len() > 0 {
					t.Errorf("stdout non-empty in deny mode: %q", outBuf.String())
				}
			} else {
				var resp fsguard.HookResponse
				if err := json.NewDecoder(&outBuf).Decode(&resp); err != nil {
					t.Fatalf("stdout not JSON: %v (%q)", err, outBuf.String())
				}
				if resp.PermissionDecision != tc.wantDecision {
					t.Errorf("permissionDecision=%q, want %q", resp.PermissionDecision, tc.wantDecision)
				}
			}
		})
	}
}
