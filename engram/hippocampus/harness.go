package hippocampus

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var supportedHarnesses = []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}

// SupportedHarnesses returns the canonical harness identifiers supported by
// Hippocampus transcript discovery.
func SupportedHarnesses() []string {
	return append([]string(nil), supportedHarnesses...)
}

// NewHarnessAdapter constructs a transcript adapter for a supported harness.
// dataDir overrides that harness's default application-data directory.
func NewHarnessAdapter(name, dataDir string) (HarnessAdapter, error) {
	switch normalizeHarnessName(name) {
	case "claude-code":
		return NewClaudeCodeAdapter(dataDir), nil
	case "codex-cli":
		return NewCodexCLIAdapter(dataDir), nil
	case "agy":
		return NewAgyAdapter(dataDir), nil
	case "opencode-cli":
		return NewOpenCodeAdapter(dataDir), nil
	case "pi-cli":
		return NewPiAdapter(dataDir), nil
	default:
		return nil, fmt.Errorf("unsupported hippocampus harness %q", name)
	}
}

func normalizeHarnessName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "claude", "claude-code":
		return "claude-code"
	case "codex", "codex-cli":
		return "codex-cli"
	case "agy", "agy-cli", "antigravity", "antigravity-cli", "gemini-cli":
		return "agy"
	case "opencode", "opencode-cli":
		return "opencode-cli"
	case "pi", "pi-cli":
		return "pi-cli"
	default:
		return ""
	}
}

func defaultHomeSubdir(parts ...string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(append([]string{home}, parts...)...)
}

func canonicalMemoryDir(projectPath string) (string, error) {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}
	digest := sha256.Sum256([]byte(filepath.Clean(absPath)))
	return filepath.Join(defaultHomeSubdir(".engram", "memory", "projects"), fmt.Sprintf("%x", digest[:8])), nil
}

func existingCanonicalMemoryDir(projectPath string) (string, error) {
	dir, err := canonicalMemoryDir(projectPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("memory dir not found: %w", err)
	}
	return dir, nil
}

// ResolveMemoryDir returns the harness-neutral project memory directory when
// present, then falls back to Claude Code's legacy native memory location.
func ResolveMemoryDir(projectPath string) (string, error) {
	if dir, err := existingCanonicalMemoryDir(projectPath); err == nil {
		return dir, nil
	}
	return NewClaudeCodeAdapter("").GetMemoryDir(projectPath)
}

// HarnessAdapter discovers sessions and reads transcripts from an AI coding harness.
// Implementations exist for Claude Code, Codex CLI, Gemini CLI, etc.
type HarnessAdapter interface {
	// Name returns the harness identifier (e.g., "claude-code").
	Name() string

	// DiscoverSessions finds sessions for a project since a given time.
	// Returns sessions sorted by start time (oldest first).
	DiscoverSessions(ctx context.Context, projectPath string, since time.Time) ([]SessionInfo, error)

	// ReadTranscript reads a session's user+assistant text content.
	// Tool results and progress events are skipped to reduce noise.
	ReadTranscript(ctx context.Context, session SessionInfo) (string, error)

	// GetMemoryDir returns the auto-memory directory for a project.
	// For Claude Code: ~/.claude/projects/<project-key>/memory/
	GetMemoryDir(projectPath string) (string, error)
}

// SessionInfo describes a discovered session.
type SessionInfo struct {
	ID        string
	StartTime time.Time
	EndTime   time.Time
	Project   string
	FilePath  string // path to session transcript file
}
