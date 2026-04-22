// Package vcs provides version control for engram memory files.
//
// It wraps git operations to automatically track changes to .ai.md and .why.md
// memory files, making every change auditable, reversible, and pushable to a
// remote for backup.
//
// Usage:
//
//	cfg := &vcs.Config{
//	    Enabled:    true,
//	    RepoPath:   "~/.engram/memories/",
//	    PushStrategy: "async",
//	}
//	m, err := vcs.New(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer m.Close()
//
//	hash, err := m.TrackChange("path/to/memory.ai.md", "update memory")
package vcs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Config holds configuration for the VCS memory system
type Config struct {
	// Enabled controls whether VCS tracking is active
	Enabled bool `yaml:"enabled"`

	// RepoPath is the directory for the memory git repository
	RepoPath string `yaml:"repo_path"`

	// PushStrategy controls when changes are pushed (immediate|async|batched|manual)
	PushStrategy string `yaml:"push_strategy"`

	// BatchInterval is the push interval for batched strategy (Go duration string)
	BatchInterval string `yaml:"batch_interval"`

	// RemoteURL is the git remote URL (empty = no remote)
	RemoteURL string `yaml:"remote_url"`

	// RemoteName is the git remote name (default: origin)
	RemoteName string `yaml:"remote_name"`

	// Branch is the git branch name (default: main)
	Branch string `yaml:"branch"`

	// Validation controls pre-commit validation
	Validation ValidationConfig `yaml:"validation"`

	// OptIn controls which optional artifacts are tracked
	OptIn OptInConfig `yaml:"opt_in"`
}

// ValidationConfig controls pre-commit validation behavior
type ValidationConfig struct {
	// RequireWhyFile enforces .ai.md -> .why.md pairing
	RequireWhyFile bool `yaml:"require_why_file"`

	// LintOnCommit runs engram linting before commit
	LintOnCommit bool `yaml:"lint_on_commit"`
}

// OptInConfig controls which optional artifacts are tracked
type OptInConfig struct {
	// ErrorMemory tracks error-memory.jsonl
	ErrorMemory bool `yaml:"error_memory"`

	// EcphoryMetadata tracks retrieval counts and timestamps
	EcphoryMetadata bool `yaml:"ecphory_metadata"`

	// Logs tracks telemetry and session logs
	Logs bool `yaml:"logs"`
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled:       true,
		RepoPath:      "~/.engram/memories/",
		PushStrategy:  "async",
		BatchInterval: "5m",
		RemoteName:    "origin",
		Branch:        "main",
		Validation: ValidationConfig{
			RequireWhyFile: true,
			LintOnCommit:   true,
		},
	}
}

// MemoryVCS is the top-level facade for version-controlled memory operations
type MemoryVCS struct {
	repo   *Repo
	pusher *Pusher
	cfg    *Config
}

// New creates a new MemoryVCS instance, initializing the repo if needed.
func New(cfg *Config) (*MemoryVCS, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if !cfg.Enabled {
		return &MemoryVCS{cfg: cfg}, nil
	}

	if cfg.RepoPath == "" {
		return nil, fmt.Errorf("vcs: repo_path is required")
	}

	// Expand ~ in path
	repoPath := expandHome(cfg.RepoPath)

	// Ensure repo exists
	repo, err := EnsureRepo(repoPath, cfg.RemoteName, cfg.RemoteURL, cfg.Branch)
	if err != nil {
		return nil, fmt.Errorf("vcs: ensure repo: %w", err)
	}

	// Install pre-commit hook if validation is enabled
	if cfg.Validation.RequireWhyFile || cfg.Validation.LintOnCommit {
		if err := InstallPreCommitHook(repoPath); err != nil {
			// Non-fatal: log but continue
			_ = err
		}
	}

	// Set up pusher
	strategy := ParsePushStrategy(cfg.PushStrategy)
	interval, _ := time.ParseDuration(cfg.BatchInterval)
	if interval == 0 {
		interval = 5 * time.Minute
	}
	pusher := NewPusher(repo, strategy, cfg.RemoteName, cfg.Branch, interval)

	return &MemoryVCS{
		repo:   repo,
		pusher: pusher,
		cfg:    cfg,
	}, nil
}

// TrackChange stages and commits a memory file change.
// Automatically includes companion files (.ai.md <-> .why.md).
// Triggers push based on configured strategy.
// Returns the commit hash.
func (m *MemoryVCS) TrackChange(path, message string) (string, error) {
	if m.repo == nil {
		return "", nil // VCS disabled
	}

	hash, err := m.repo.TrackChange(path, message)
	if err != nil {
		return "", err
	}

	if hash != "" && m.pusher != nil {
		_ = m.pusher.TriggerPush() // best-effort
	}

	return hash, nil
}

// TrackDelete stages and commits a memory file deletion.
func (m *MemoryVCS) TrackDelete(path, message string) (string, error) {
	if m.repo == nil {
		return "", nil
	}

	hash, err := m.repo.TrackDelete(path, message)
	if err != nil {
		return "", err
	}

	if hash != "" && m.pusher != nil {
		_ = m.pusher.TriggerPush()
	}

	return hash, nil
}

// Push triggers an immediate push regardless of strategy.
func (m *MemoryVCS) Push() error {
	if m.pusher == nil {
		return fmt.Errorf("vcs: no pusher configured (VCS disabled or no remote)")
	}
	return m.pusher.ForcePush()
}

// Log returns commit history for a file.
func (m *MemoryVCS) Log(path string, limit int) ([]CommitEntry, error) {
	if m.repo == nil {
		return nil, nil
	}
	return m.repo.Log(path, limit)
}

// Diff returns the diff between two refs.
func (m *MemoryVCS) Diff(path, fromRef, toRef string) (string, error) {
	if m.repo == nil {
		return "", nil
	}
	return m.repo.Diff(path, fromRef, toRef)
}

// Restore reverts a file to a specific commit and commits the revert.
func (m *MemoryVCS) Restore(path, commitHash string) error {
	if m.repo == nil {
		return fmt.Errorf("vcs: disabled")
	}

	if err := m.repo.Restore(path, commitHash); err != nil {
		return err
	}

	// Commit the restoration
	msg := fmt.Sprintf("memory: restore %s to %s", path, commitHash[:8])
	if _, err := m.repo.TrackChange(path, msg); err != nil {
		return fmt.Errorf("commit restore: %w", err)
	}

	if m.pusher != nil {
		_ = m.pusher.TriggerPush()
	}

	return nil
}

// Status returns the VCS status of tracked files.
func (m *MemoryVCS) Status() (string, error) {
	if m.repo == nil {
		return "VCS disabled", nil
	}
	return m.repo.Status()
}

// Close shuts down the pusher (flushes pending pushes).
func (m *MemoryVCS) Close() {
	if m.pusher != nil {
		m.pusher.Close()
	}
}

// Repo returns the underlying Repo (for advanced operations).
func (m *MemoryVCS) Repo() *Repo {
	return m.repo
}

// expandHome expands ~ to the user's home directory
func expandHome(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}
