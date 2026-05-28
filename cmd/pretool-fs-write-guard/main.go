// Command pretool-fs-write-guard is a Claude Code PreToolUse hook that
// enforces a worktree-only write policy for the Edit / Write / MultiEdit
// tools.
//
// It reads the PreToolUse JSON envelope from stdin, inspects the target
// file_path, and either allows the write (exit 0) or blocks it (exit 2) with
// positive-guidance text on stderr explaining the right way to proceed. See
// package fsguard for the policy.
//
// The hook fails open: any parse error or unexpected input exits 0, so a bug
// here can never brick the editing tools. The settings.json deny rules are the
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
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
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

	switch env.ToolName {
	case "Edit", "Write", "MultiEdit":
	default:
		return 0
	}

	path := env.ToolInput.FilePath
	if path == "" {
		path = env.ToolInput.Path
	}
	if path == "" {
		return 0
	}

	g := fsguard.New()
	allowed, message := g.Classify(path, env.CWD)
	if allowed {
		return 0
	}
	_, _ = fmt.Fprintln(errOut, message+fsguard.Escalation)
	return 2
}
