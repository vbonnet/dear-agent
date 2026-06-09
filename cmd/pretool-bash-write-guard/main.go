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
	os.Exit(run(os.Stdin, os.Stderr))
}

// run reads an envelope, applies the policy, and returns the process exit
// code: 0 to allow, 2 to block (writing guidance to errOut).
func run(in io.Reader, errOut io.Writer) int {
	var env envelope
	if err := json.NewDecoder(in).Decode(&env); err != nil {
		return 0 // unparseable -> fail open
	}
	if env.ToolName != "Bash" {
		return 0
	}

	g := fsguard.New()
	allowed, message := g.InspectCommand(env.ToolInput.Command, env.CWD)
	if allowed {
		return 0
	}
	_, _ = fmt.Fprintln(errOut, message+fsguard.Escalation)
	return 2
}
