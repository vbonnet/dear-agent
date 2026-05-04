// posttool-worktree-tracker is a PostToolUse hook that detects git worktree
// add/remove commands in Bash tool output and tracks them in the AGM database.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// HookInput represents the JSON payload from Claude Code PostToolUse hooks
type HookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	ToolResult struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode int    `json:"exitCode"`
	} `json:"tool_result"`
}

// WorktreeEvent represents a detected worktree operation
type WorktreeEvent struct {
	Operation    string // "add" or "remove"
	WorktreePath string
	Branch       string
	RepoPath     string
}

// parseWorktreeAdd detects `git worktree add` commands and extracts the path and branch.
//
// Supported formats:
//
//	git worktree add <path>
//	git worktree add <path> -b <branch>
//	git worktree add <path> <branch>
//	git -C <repo> worktree add <path> -b <branch>
func parseWorktreeAdd(command string) *WorktreeEvent {
	// Normalize: strip leading/trailing whitespace
	command = strings.TrimSpace(command)

	// Extract -C <repo> if present
	repoPath := ""
	cFlagRe := regexp.MustCompile(`git\s+-C\s+(\S+)\s+`)
	if m := cFlagRe.FindStringSubmatch(command); len(m) > 1 {
		repoPath = m[1]
	}

	// Match the worktree add portion
	// Pattern: worktree add [--flags] <path> [-b <branch> | <branch>]
	addRe := regexp.MustCompile(`worktree\s+add\s+(?:--\S+\s+)*(\S+)(?:\s+-b\s+(\S+)|\s+(\S+))?`)
	m := addRe.FindStringSubmatch(command)
	if len(m) < 2 {
		return nil
	}

	wtPath := m[1]
	branch := ""
	if len(m) > 2 && m[2] != "" {
		branch = m[2]
	} else if len(m) > 3 && m[3] != "" {
		branch = m[3]
	}

	return &WorktreeEvent{
		Operation:    "add",
		WorktreePath: wtPath,
		Branch:       branch,
		RepoPath:     repoPath,
	}
}

// parseWorktreeRemove detects `git worktree remove` commands.
//
// Supported formats:
//
//	git worktree remove <path>
//	git worktree remove --force <path>
//	git -C <repo> worktree remove <path>
func parseWorktreeRemove(command string) *WorktreeEvent {
	command = strings.TrimSpace(command)

	repoPath := ""
	cFlagRe := regexp.MustCompile(`git\s+-C\s+(\S+)\s+`)
	if m := cFlagRe.FindStringSubmatch(command); len(m) > 1 {
		repoPath = m[1]
	}

	removeRe := regexp.MustCompile(`worktree\s+remove\s+(?:--\S+\s+)*(\S+)`)
	m := removeRe.FindStringSubmatch(command)
	if len(m) < 2 {
		return nil
	}

	return &WorktreeEvent{
		Operation:    "remove",
		WorktreePath: m[1],
		RepoPath:     repoPath,
	}
}

// detectWorktreeEvent parses a command string for worktree operations
func detectWorktreeEvent(command string) *WorktreeEvent {
	if !strings.Contains(command, "worktree") {
		return nil
	}

	if strings.Contains(command, "worktree add") {
		return parseWorktreeAdd(command)
	}
	if strings.Contains(command, "worktree remove") {
		return parseWorktreeRemove(command)
	}

	return nil
}

// log writes a timestamped message to stderr when debug mode is enabled
func log(debug bool, level, message string) {
	if debug || level == "ERROR" {
		timestamp := time.Now().Format(time.RFC3339)
		fmt.Fprintf(os.Stderr, "[%s] posttool-worktree-tracker %s: %s\n", timestamp, level, message)
	}
}

// findAGMSession finds the AGM session name from the Claude session manifest
func findAGMSession(sessionID string) string {
	if sessionID == "" {
		return ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	manifestPath := filepath.Join(homeDir, ".claude", "sessions", sessionID, "manifest.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}

	pattern := regexp.MustCompile(`agm_session_name:\s*(.+)`)
	match := pattern.FindStringSubmatch(string(data))
	if len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func run() {
	debug := os.Getenv("AGM_HOOK_DEBUG") == "1"

	// Read hook input from stdin
	var input HookInput
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		log(debug, "INFO", fmt.Sprintf("Failed to decode stdin: %v", err))
		return // Non-fatal: stdin may not be JSON
	}

	// Only process Bash tool calls
	if input.ToolName != "Bash" {
		return
	}

	// Only process successful commands
	if input.ToolResult.ExitCode != 0 {
		return
	}

	command := input.ToolInput.Command
	event := detectWorktreeEvent(command)
	if event == nil {
		return
	}

	log(debug, "INFO", fmt.Sprintf("Detected worktree %s: path=%s branch=%s", event.Operation, event.WorktreePath, event.Branch))

	// Find AGM session
	sessionID := os.Getenv("CLAUDE_SESSION_ID")
	sessionName := findAGMSession(sessionID)
	if sessionName == "" {
		log(debug, "INFO", "Not an AGM session, skipping worktree tracking")
		return
	}

	// Record the event via agm CLI
	switch event.Operation {
	case "add":
		log(debug, "INFO", fmt.Sprintf("Tracking worktree add for session %s", sessionName))
		// Fire-and-forget: record in database via agm admin command
		// For now, log the event. Full DB integration requires the agm binary.
		log(debug, "INFO", fmt.Sprintf("WORKTREE_ADD session=%s path=%s branch=%s repo=%s",
			sessionName, event.WorktreePath, event.Branch, event.RepoPath))
	case "remove":
		log(debug, "INFO", fmt.Sprintf("Tracking worktree remove for session %s", sessionName))
		log(debug, "INFO", fmt.Sprintf("WORKTREE_REMOVE session=%s path=%s",
			sessionName, event.WorktreePath))
	}
}

func main() {
	run()
}
