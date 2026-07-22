package agent

import (
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
)

// Shell command construction for the legacy tmux-backed adapters.
//
// These builders are pure string -> string functions with no I/O, so the
// quoting they perform is directly assertable in tests. That is the point:
// before ce-93lw.1 the same command text was assembled inline inside functions
// that also created tmux sessions and blocked on harness readiness, which made
// the escaping untestable and let hand-written quotes drift in unnoticed.
//
// Every caller-controlled value goes through launchparity.ShellQuote — the same
// primitive the modern spawn path in internal/ops uses. Values that reach here
// are not all human-typed: working directories arrive from bead text, worktree
// paths and MCP callers, and UUIDs are read back from the session store.
//
// These commands are pasted into a live tmux pane, where a shell parses them by
// construction, so an argv builder in the style of cmd/safe-pr is not available
// here. One quoting function applied at every interpolation is.

// buildClaudeStartCommand returns the command that starts Claude in a fresh
// tmux session, pre-authorizing the workspace so Claude does not block on its
// interactive trust prompt.
func buildClaudeStartCommand(workDir string, authorizedDirs []string) string {
	var b strings.Builder
	b.WriteString("claude --add-dir ")
	b.WriteString(launchparity.ShellQuote(workDir))

	for _, dir := range authorizedDirs {
		if dir == workDir {
			continue
		}
		b.WriteString(" --add-dir ")
		b.WriteString(launchparity.ShellQuote(dir))
	}

	b.WriteString(" && exit")
	return b.String()
}

// buildClaudeResumeCommand returns the command that resumes a Claude session by
// UUID after its tmux session has been recreated.
func buildClaudeResumeCommand(workDir, sessionID string) string {
	return "cd " + launchparity.ShellQuote(workDir) +
		" && claude --resume " + launchparity.ShellQuote(sessionID) +
		" && exit"
}

// buildGeminiStartCommand returns the command that starts Gemini CLI in a fresh
// tmux session with the workspace pre-authorized.
func buildGeminiStartCommand(workDir string, authorizedDirs []string) string {
	var b strings.Builder
	b.WriteString("gemini --include-directories ")
	b.WriteString(launchparity.ShellQuote(workDir))

	for _, dir := range authorizedDirs {
		if dir == workDir {
			continue
		}
		b.WriteString(" --include-directories ")
		b.WriteString(launchparity.ShellQuote(dir))
	}

	b.WriteString(" && exit")
	return b.String()
}

// buildGeminiResumeCommand returns the command that resumes a Gemini session.
// An empty uuid falls back to Gemini's "latest" selector, which is a literal
// flag value rather than caller data and so is not quoted.
func buildGeminiResumeCommand(workDir, uuid string) string {
	resumeTarget := "latest"
	if uuid != "" {
		resumeTarget = launchparity.ShellQuote(uuid)
	}
	return "cd " + launchparity.ShellQuote(workDir) +
		" && gemini --resume " + resumeTarget +
		" && exit"
}

// buildOpenCodeResumeCommand returns the command that re-attaches to OpenCode
// after its tmux session has been recreated. OpenCode keeps session state
// server-side, so there is no per-session resume token to pass.
func buildOpenCodeResumeCommand(workDir string) string {
	return "cd " + launchparity.ShellQuote(workDir) + " && opencode attach && exit"
}

// buildSetDirCommand returns the `cd` line sent to a harness pane when a
// session's working directory changes. Callers validate the path with
// ValidateSendDirPath first; the quoting here is deliberate defense in depth so
// that relaxing that validator cannot by itself reopen an injection.
func buildSetDirCommand(path string) string {
	return "cd " + launchparity.ShellQuote(path) + "\r"
}
