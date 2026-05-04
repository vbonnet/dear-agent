package phaseisolation

import "os"

// Platform identifies an AI coding platform.
type Platform string

// Recognized AI coding platform values.
const (
	PlatformClaudeCode Platform = "claude-code"
	PlatformCursor     Platform = "cursor"
	PlatformWindsurf   Platform = "windsurf"
	PlatformAider      Platform = "aider"
	PlatformUnknown    Platform = "unknown"
)

// DetectPlatform detects the current AI coding platform from environment variables.
func DetectPlatform() Platform {
	if os.Getenv("CLAUDECODE") == "1" || os.Getenv("CLAUDE_CODE_ENTRYPOINT") != "" {
		return PlatformClaudeCode
	}
	if os.Getenv("CURSOR") != "" {
		return PlatformCursor
	}
	if os.Getenv("WINDSURF") != "" {
		return PlatformWindsurf
	}
	if os.Getenv("AIDER_MODEL") != "" || os.Getenv("AIDER") != "" {
		return PlatformAider
	}
	return PlatformUnknown
}

// IsTaskToolAvailable returns true if the Task tool is available (Claude Code only).
func IsTaskToolAvailable() bool {
	return DetectPlatform() == PlatformClaudeCode
}

// AreSubAgentsAvailable returns true if sub-agents are available (Claude Code only).
func AreSubAgentsAvailable() bool {
	return DetectPlatform() == PlatformClaudeCode
}
