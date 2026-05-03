package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// TestSessionGuard guards against test-* session creation without --test flag
type TestSessionGuard struct {
	toolName  string
	toolInput string
	debug     bool
}

// NewTestSessionGuard creates a new guard from environment variables
func NewTestSessionGuard() *TestSessionGuard {
	return &TestSessionGuard{
		toolName:  os.Getenv("CLAUDE_TOOL_NAME"),
		toolInput: os.Getenv("CLAUDE_TOOL_INPUT"),
		debug:     os.Getenv("AGM_HOOK_DEBUG") == "1",
	}
}

// log writes debug messages to stderr if debug is enabled
func (g *TestSessionGuard) log(level, message string) {
	if g.debug {
		fmt.Fprintf(os.Stderr, "[TestSessionGuard] %s: %s\n", level, message)
	}
}

// extractSessionName extracts session name from agm session new command
//
// Matches patterns:
//   - agm session new test-foo
//   - agm session new test-bar --workspace=oss
//   - agm session new --test test-baz
//
// Returns session name or empty string if not a session creation command
func (g *TestSessionGuard) extractSessionName(command string) string {
	// Pattern: agm session new [flags] <session-name> [more-flags]
	// We need to find the session name (first non-flag argument after 'new')
	pattern := regexp.MustCompile(`\bagm\s+session\s+new\s+(?:--\S+\s+)*([a-zA-Z0-9\-_]+)`)
	match := pattern.FindStringSubmatch(command)

	if len(match) > 1 {
		sessionName := match[1]
		g.log("INFO", fmt.Sprintf("Extracted session name: %s", sessionName))
		return sessionName
	}

	return ""
}

// isTestPattern checks if session name matches test-* pattern
func (g *TestSessionGuard) isTestPattern(sessionName string) bool {
	return strings.HasPrefix(strings.ToLower(sessionName), "test-")
}

// hasTestFlag checks if --test flag is present in command
func (g *TestSessionGuard) hasTestFlag(command string) bool {
	return strings.Contains(command, "--test")
}

// hasOverrideFlag checks if --allow-test-name flag is present in command
func (g *TestSessionGuard) hasOverrideFlag(command string) bool {
	return strings.Contains(command, "--allow-test-name")
}

// generateErrorMessage generates helpful error message for blocked command
func (g *TestSessionGuard) generateErrorMessage(sessionName string) string {
	return fmt.Sprintf(`
❌ Test Session Pattern Detected

You attempted to create session '%s' without --test flag.

PROBLEM:
  Test sessions should use --test flag for isolation.
  Without it, session goes to production workspace (~/.claude/sessions/).

SOLUTION:
  agm session new --test %s

WHAT --test DOES:
  • Creates session in ~/sessions-test/ (isolated from production)
  • Tmux session prefixed with agm-test-%s
  • Automatically cleaned up by test infrastructure
  • Not tracked in AGM database (ephemeral)

TO OVERRIDE (if you really need production session named test-*):
  agm session new %s --allow-test-name

For more info: agm session new --help
`, sessionName, sessionName, sessionName, sessionName)
}

// Run executes the main hook logic
//
// Returns:
//   - 0: allow execution (no violation or override present)
//   - 1: block execution (test-* pattern without --test flag)
func (g *TestSessionGuard) Run() int {
	g.log("INFO", fmt.Sprintf("Hook started for tool: %s", g.toolName))

	// Only check Bash tool invocations
	if g.toolName != "Bash" {
		g.log("INFO", fmt.Sprintf("Skipping non-Bash tool: %s", g.toolName))
		return 0
	}

	// Extract session name from command
	sessionName := g.extractSessionName(g.toolInput)
	if sessionName == "" {
		g.log("INFO", "Not a session creation command")
		return 0
	}

	// Check if it matches test-* pattern
	if !g.isTestPattern(sessionName) {
		g.log("INFO", fmt.Sprintf("Session name '%s' doesn't match test-* pattern", sessionName))
		return 0
	}

	// Check for --test flag (allowed)
	if g.hasTestFlag(g.toolInput) {
		g.log("INFO", "--test flag present, allowing")
		return 0
	}

	// Check for --allow-test-name override (allowed)
	if g.hasOverrideFlag(g.toolInput) {
		g.log("INFO", "--allow-test-name flag present, allowing")
		return 0
	}

	// BLOCK: test-* pattern without --test or --allow-test-name
	g.log("INFO", fmt.Sprintf("BLOCKING: test-* pattern '%s' without --test flag", sessionName))

	// Print error message to stderr (will be shown to user)
	fmt.Fprintln(os.Stderr, g.generateErrorMessage(sessionName))

	return 1
}

func main() {
	os.Exit(safeRun())
}

// Don't block on hook errors (graceful degradation).
func safeRun() (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[TestSessionGuard] Hook error: %v\n", r)
			exitCode = 0
		}
	}()

	guard := NewTestSessionGuard()
	return guard.Run()
}
