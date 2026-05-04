// Package agent provides automatic detection of AI coding agent platforms.
//
// Engram supports multiple AI coding assistants and automatically detects which
// agent is currently running to provide agent-specific optimizations and behavior.
//
// Supported agents:
//   - Claude Code: Anthropic's official CLI for Claude
//   - Cursor: AI-powered code editor
//   - Windsurf: Code navigation and exploration tool
//   - Aider: Terminal-based AI pair programming
//   - Unknown: Fallback for unsupported or manual usage
//
// Detection strategy (see ADR-008):
//  1. Check environment variables (primary detection method)
//  2. Check for agent-specific files in working directory (fallback)
//  3. Return AgentUnknown if no agent detected
//
// Example usage:
//
//	detector := agent.NewDetector()
//	currentAgent := detector.Detect()
//
//	if currentAgent == agent.AgentClaudeCode {
//	    // Optimize for Claude Code
//	}
//
// Detection is automatic during platform initialization and influences:
//   - Plugin selection and prioritization
//   - Engram retrieval filtering
//   - Telemetry tagging
package agent

import (
	"os"
	"path/filepath"
)

// Agent represents a detected AI coding agent platform
type Agent string

// Recognized AI agent platform values.
const (
	AgentClaudeCode Agent = "claude-code"
	AgentCursor     Agent = "cursor"
	AgentWindsurf   Agent = "windsurf"
	AgentAider      Agent = "aider"
	AgentUnknown    Agent = "unknown"
)

// Detector detects which AI agent is currently running
type Detector struct {
	cachedAgent *Agent // Cache detection result after first call
}

// NewDetector creates a new agent detector
func NewDetector() *Detector {
	return &Detector{
		cachedAgent: nil,
	}
}

// Detect identifies the current AI agent platform
//
// Detection strategy (see ADR-008):
// 1. Check environment variables (primary)
// 2. Check for agent-specific files (fallback)
//
// Detection result is cached after the first call for performance.
// Environment variables and files are not expected to change during runtime.
func (d *Detector) Detect() Agent {
	// Return cached result if available
	if d.cachedAgent != nil {
		return *d.cachedAgent
	}

	// Perform detection
	agent := d.detect()

	// Cache the result
	d.cachedAgent = &agent

	return agent
}

// detect performs the actual detection logic
func (d *Detector) detect() Agent {
	// Environment variable detection (primary method)
	if os.Getenv("CLAUDECODE") == "1" || os.Getenv("CLAUDE_CODE_ENTRYPOINT") != "" {
		return AgentClaudeCode
	}

	if os.Getenv("CURSOR") == "1" || os.Getenv("CURSOR_SESSION_ID") != "" {
		return AgentCursor
	}

	if os.Getenv("WINDSURF") == "1" {
		return AgentWindsurf
	}

	// Aider sets multiple environment variables
	if os.Getenv("AIDER_MODEL") != "" || os.Getenv("AIDER_ARCHITECT") != "" {
		return AgentAider
	}

	// File-based detection (fallback method)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return AgentUnknown
	}

	// Claude Code creates ~/.claude/ directory
	if d.fileExists(filepath.Join(homeDir, ".claude")) {
		return AgentClaudeCode
	}

	// Cursor creates .cursorrules file
	if d.fileExists(".cursorrules") {
		return AgentCursor
	}

	// Windsurf creates .windsurfrules file
	if d.fileExists(".windsurfrules") {
		return AgentWindsurf
	}

	// Aider creates .aider* files
	if d.fileExists(".aider.conf.yml") || d.fileExists(".aiderignore") {
		return AgentAider
	}

	return AgentUnknown
}

// ClearCache clears the cached detection result, forcing re-detection on next Detect() call
// This is primarily useful for testing scenarios where environment or files change
func (d *Detector) ClearCache() {
	d.cachedAgent = nil
}

// fileExists checks if a file or directory exists
func (d *Detector) fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
