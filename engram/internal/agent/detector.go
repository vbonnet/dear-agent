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
// Detection strategy (see SPEC.md):
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
	inputs      *detectorInputs
}

type detectorInputs struct {
	getenv      func(string) string
	userHomeDir func() (string, error)
	pathExists  func(string) bool
}

// NewDetector creates a new agent detector
func NewDetector() *Detector {
	return newDetector(systemDetectorInputs())
}

func newDetector(inputs detectorInputs) *Detector {
	return &Detector{inputs: &inputs}
}

// Detect identifies the current AI agent platform
//
// Detection strategy (see SPEC.md):
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
	inputs := d.activeInputs()

	// Environment variable detection (primary method)
	if inputs.getenv("CLAUDECODE") == "1" || inputs.getenv("CLAUDE_CODE_ENTRYPOINT") != "" {
		return AgentClaudeCode
	}

	if inputs.getenv("CURSOR") == "1" || inputs.getenv("CURSOR_SESSION_ID") != "" {
		return AgentCursor
	}

	if inputs.getenv("WINDSURF") == "1" {
		return AgentWindsurf
	}

	// Aider sets multiple environment variables
	if inputs.getenv("AIDER_MODEL") != "" || inputs.getenv("AIDER_ARCHITECT") != "" {
		return AgentAider
	}

	// File-based detection (fallback method)
	homeDir, err := inputs.userHomeDir()
	if err != nil {
		return AgentUnknown
	}

	// Claude Code creates ~/.claude/ directory
	if inputs.pathExists(filepath.Join(homeDir, ".claude")) {
		return AgentClaudeCode
	}

	// Cursor creates .cursorrules file
	if inputs.pathExists(".cursorrules") {
		return AgentCursor
	}

	// Windsurf creates .windsurfrules file
	if inputs.pathExists(".windsurfrules") {
		return AgentWindsurf
	}

	// Aider creates .aider* files
	if inputs.pathExists(".aider.conf.yml") || inputs.pathExists(".aiderignore") {
		return AgentAider
	}

	return AgentUnknown
}

// ClearCache clears the cached detection result, forcing re-detection on next Detect() call
// This is primarily useful for testing scenarios where environment or files change
func (d *Detector) ClearCache() {
	d.cachedAgent = nil
}

func (d *Detector) activeInputs() detectorInputs {
	if d.inputs != nil {
		return *d.inputs
	}
	return systemDetectorInputs()
}

func systemDetectorInputs() detectorInputs {
	return detectorInputs{
		getenv:      os.Getenv,
		userHomeDir: os.UserHomeDir,
		pathExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
	}
}
