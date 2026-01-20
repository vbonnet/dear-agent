package reaper

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

const (
	// PromptDetectionTimeout is how long to wait for Claude to return to prompt
	// before falling back to timer-based waiting. This should be generous enough
	// to handle slow responses but not so long that stuck sessions block indefinitely.
	PromptDetectionTimeout = 60 * time.Second

	// PaneCloseTimeout is how long to wait for the tmux pane to close after sending /exit.
	// Claude should exit quickly after receiving /exit command, but we allow extra time for
	// cleanup operations.
	PaneCloseTimeout = 30 * time.Second

	// FallbackWaitTime is used when prompt detection fails or times out.
	// This is a conservative estimate of how long Claude might take to finish
	// a response and return to the prompt.
	FallbackWaitTime = 5 * time.Second
)

// Reaper manages the async archival process for a CSM session
// It waits for Claude to return to prompt, sends /exit, and archives the session
type Reaper struct {
	SessionName string
	SocketPath  string
}

// New creates a new Reaper for the given session
func New(sessionName string) *Reaper {
	return &Reaper{
		SessionName: sessionName,
		SocketPath:  tmux.GetSocketPath(),
	}
}

// Run executes the full reaper sequence:
// 1. Wait for Claude to return to prompt (prompt detection)
// 2. Send /exit command to exit Claude
// 3. Wait for pane to close
// 4. Archive session (update manifest + move directory)
func (r *Reaper) Run() error {
	log.Printf("📋 Starting reaper sequence for session: %s", r.SessionName)

	// Step 1: Wait for Claude to be ready (prompt detection)
	log.Printf("⏳ Waiting for Claude to return to prompt...")
	if err := r.waitForPrompt(PromptDetectionTimeout); err != nil {
		log.Printf("⚠ Prompt detection failed: %v", err)
		log.Printf("💡 Falling back to %v timer", FallbackWaitTime)
		time.Sleep(FallbackWaitTime)
	} else {
		log.Printf("✓ Prompt detected - Claude is ready")
	}

	// Step 2: Send /exit to exit Claude
	log.Printf("📤 Sending /exit to exit Claude...")
	if err := r.sendExit(); err != nil {
		return fmt.Errorf("failed to send /exit to tmux pane (session may have already exited): %w", err)
	}
	log.Printf("✓ /exit sent successfully")

	// Step 3: Wait for pane to close
	log.Printf("⏳ Waiting for pane to close...")
	if err := r.waitForPaneClose(PaneCloseTimeout); err != nil {
		return fmt.Errorf("pane did not close within %v (check tmux session status manually): %w", PaneCloseTimeout, err)
	}
	log.Printf("✓ Pane closed successfully")

	// Step 4: Archive session
	log.Printf("📦 Archiving session...")
	if err := r.archiveSession(); err != nil {
		return fmt.Errorf("archive failed: %w", err)
	}
	log.Printf("✓ Session archived successfully")

	return nil
}

// waitForPrompt monitors output stream for Claude prompt
// Uses tmux control mode to detect when Claude is ready for input
func (r *Reaper) waitForPrompt(timeout time.Duration) error {
	return tmux.WaitForClaudePrompt(r.SessionName, timeout)
}

// sendExit sends /exit command to exit Claude Code cleanly
// Claude Code requires the /exit command (not Ctrl+D) to exit properly
func (r *Reaper) sendExit() error {
	socketPath := tmux.GetSocketPath()

	// Send /exit command followed by Enter
	// Using -l flag to send literal text (slash command)
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", r.SessionName, "-l", "/exit")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send /exit: %w", err)
	}

	// Send Enter to execute the command
	cmd = exec.Command("tmux", "-S", socketPath, "send-keys", "-t", r.SessionName, "Enter")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	return nil
}

// waitForPaneClose waits for tmux pane to close
func (r *Reaper) waitForPaneClose(timeout time.Duration) error {
	return tmux.WaitForPaneClose(r.SessionName, timeout)
}

// archiveSession updates manifest and moves directory
// This is based on cmd/csm/archive.go but without interactive prompts
func (r *Reaper) archiveSession() error {
	sessionsDir, err := r.getSessionsDir()
	if err != nil {
		return fmt.Errorf("failed to get sessions directory: %w", err)
	}

	// Resolve session identifier to manifest
	m, manifestPath, err := session.ResolveIdentifier(r.SessionName, sessionsDir)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	log.Printf("📁 Manifest path: %s", manifestPath)

	// Check if already archived
	if m.Lifecycle == manifest.LifecycleArchived {
		log.Printf("ℹ️  Session already archived, skipping")
		return nil
	}

	// Update lifecycle field
	m.Lifecycle = manifest.LifecycleArchived

	// Write manifest (automatic backup + UpdatedAt)
	if err := manifest.Write(manifestPath, m); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}
	log.Printf("✓ Manifest updated to archived")

	// Move session directory to .archive-old-format/
	sessionDir := filepath.Dir(manifestPath)
	archiveBaseDir := filepath.Join(sessionsDir, ".archive-old-format")
	archiveTargetDir := filepath.Join(archiveBaseDir, filepath.Base(sessionDir))

	// Create archive directory
	if err := os.MkdirAll(archiveBaseDir, 0700); err != nil {
		return fmt.Errorf("failed to create archive dir: %w", err)
	}

	// Handle conflicts with timestamp
	if _, err := os.Stat(archiveTargetDir); err == nil {
		timestamp := time.Now().Format("20060102T150405Z")
		archiveTargetDir = archiveTargetDir + "-" + timestamp
		log.Printf("⚠ Archive conflict - renaming to: %s", filepath.Base(archiveTargetDir))
	}

	// Move directory
	if err := os.Rename(sessionDir, archiveTargetDir); err != nil {
		return fmt.Errorf("failed to move to archive: %w", err)
	}
	log.Printf("✓ Session moved to: %s", archiveTargetDir)

	return nil
}

// getSessionsDir returns the sessions directory path
func (r *Reaper) getSessionsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, "sessions"), nil
}
