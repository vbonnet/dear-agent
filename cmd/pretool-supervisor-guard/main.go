// Command pretool-supervisor-guard is a Claude Code PreToolUse hook that keeps
// VROOM supervisors out of implementation work.
//
// A supervisor (Meta-Orchestrator, Orchestrator, Overseer) coordinates workers.
// When it does the work itself, two things go wrong. It can raise a permission
// modal that a detached session cannot answer, which wedges the supervisor and
// stalls dispatch for the whole mesh; and it spends on implementation detail
// the context window its coordination role depends on. The role prompts already
// asked supervisors not to do this and it happened anyway, so the rule is
// enforced here instead of being left to prose.
//
// The hook reads the PreToolUse JSON envelope from stdin. It acts only when the
// session is a supervisor (see fsguard.DetectSupervisor); every worker session
// is passed through untouched, so worker capability is unchanged. For a
// supervisor it refuses the file-mutation tools (Edit / Write / MultiEdit /
// NotebookEdit) outside the control-plane roots, and refuses Bash commands that
// mutate the filesystem or a git repository. Read, inspect, dispatch, and
// comment tools are untouched.
//
// Unlike the write guards, this hook has no graduated enforcement. It always
// hard-blocks with exit 2, and never emits permissionDecision "ask": an "ask"
// would raise exactly the modal the rule exists to prevent. FSGUARD_ENFORCEMENT
// is deliberately not consulted.
//
// The hook fails open on anything it cannot read: a malformed envelope, an
// unknown tool, or a command the tokenizer cannot parse all exit 0, so a bug
// here can never brick a supervisor's read and dispatch path.
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
		Command      string `json:"command"`
		FilePath     string `json:"file_path"`
		Path         string `json:"path"`
		NotebookPath string `json:"notebook_path"`
	} `json:"tool_input"`
}

func main() {
	os.Exit(run(os.Stdin, os.Stderr, os.Getenv))
}

// run reads an envelope, applies the supervisor role policy, and returns the
// process exit code: 0 to allow, 2 to block.
func run(in io.Reader, errOut io.Writer, getenv func(string) string) int {
	var env envelope
	if err := json.NewDecoder(in).Decode(&env); err != nil {
		return 0 // unparseable -> fail open
	}

	identity, isSupervisor := fsguard.DetectSupervisor(getenv)
	if !isSupervisor {
		return 0 // worker session: not this hook's concern
	}

	g := fsguard.New()

	switch env.ToolName {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		path := firstNonEmpty(
			env.ToolInput.FilePath,
			env.ToolInput.Path,
			env.ToolInput.NotebookPath,
		)
		if path == "" {
			return 0 // nothing to judge
		}
		if allowed, msg := g.CheckSupervisorWrite(identity, env.ToolName, path, env.CWD); !allowed {
			return block(errOut, msg)
		}
	case "Bash":
		if env.ToolInput.Command == "" {
			return 0
		}
		if allowed, msg := g.CheckSupervisorCommand(identity, env.ToolInput.Command, env.CWD); !allowed {
			return block(errOut, msg)
		}
	}
	return 0
}

// block writes the delegation guidance and returns the hard-block exit code.
// There is deliberately no softer outcome: see the package comment.
func block(errOut io.Writer, msg string) int {
	_, _ = fmt.Fprintln(errOut, msg)
	return 2
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
