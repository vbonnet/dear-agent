// Command pretool-bash-write-guard is a Claude Code PreToolUse hook that
// enforces the worktree-only write policy for the Bash tool.
//
// It reads the PreToolUse JSON envelope from stdin, parses tool_input.command
// for filesystem-mutating operations (redirections, cp/mv/mkdir/touch/rm/tee/
// sed -i/dd/ln/chmod/chown/truncate, and git writes), and either allows the
// command (exit 0) or blocks it (exit 2) with positive-guidance text on stderr.
// Pure reads (cat, grep, ls, git log/diff/status, ...) trip no write pattern
// and are always allowed. See package fsguard for the policy.
//
// Graduated enforcement (FSGUARD_ENFORCEMENT=deny|warn|ask|defer):
//
//	deny  — exit 2, guidance to stderr (default, hard block)
//	warn  — exit 0, JSON {"permissionDecision":"allow","message":"..."} to stdout
//	ask   — exit 0, JSON {"permissionDecision":"ask","message":"..."} to stdout
//	defer — exit 0, JSON {"permissionDecision":"defer"} to stdout
//
// The hook fails open: any parse error or unexpected input exits 0, so a bug
// here can never brick the Bash tool. The settings.json deny rules are the
// backstop.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/vbonnet/dear-agent/internal/fsguard"
)

type envelope struct {
	ToolName  string `json:"tool_name"`
	CWD       string `json:"cwd"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

func main() {
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr))
}

// run reads an envelope, applies the policy, and returns the process exit
// code: 0 to allow (or for non-deny enforcement levels), 2 to hard block.
func run(in io.Reader, out, errOut io.Writer) int {
	var env envelope
	if err := json.NewDecoder(in).Decode(&env); err != nil {
		return 0 // unparseable -> fail open
	}
	if env.ToolName != "Bash" {
		return 0
	}

	// A malformed FSGUARD_CONFIG degrades to safe defaults rather than failing
	// open, so we deliberately ignore the construction error and keep enforcing.
	itc, _ := fsguard.NewInterceptor()
	d := itc.CheckCommand(env.ToolInput.Command, env.CWD)
	if d.Allowed {
		return 0
	}
	return emitDecision(d, out, errOut)
}

// emitDecision writes the appropriate hook output for a blocked Decision and
// returns the exit code. For EnforceDeny it writes to stderr and exits 2; for
// all other levels it writes a JSON hook response to stdout and exits 0.
func emitDecision(d fsguard.Decision, out, errOut io.Writer) int {
	switch d.Enforcement {
	case fsguard.EnforceDeny:
		_, _ = fmt.Fprintln(errOut, d.Message)
		return 2
	case fsguard.EnforceWarn:
		resp := fsguard.HookResponse{PermissionDecision: "allow", Message: d.Message}
		_ = json.NewEncoder(out).Encode(resp)
		return 0
	case fsguard.EnforceAsk:
		resp := fsguard.HookResponse{PermissionDecision: "ask", Message: d.Message}
		_ = json.NewEncoder(out).Encode(resp)
		return 0
	case fsguard.EnforceDefer:
		resp := fsguard.HookResponse{PermissionDecision: "defer"}
		_ = json.NewEncoder(out).Encode(resp)
		return 0
	}
	_, _ = fmt.Fprintln(errOut, d.Message)
	return 2
}
