package session

import (
	"fmt"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

// Resume orchestrates the full resume workflow
func Resume(identifier string, cfg *config.Config) error {
	// 1. Resolve identifier to manifest
	m, manifestPath, err := ResolveIdentifier(identifier, cfg.SessionsDir)
	if err != nil {
		return err
	}

	// 2. Check health
	health, err := CheckHealth(m)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	// 3. If unhealthy, offer recovery (for now, just fail)
	if !health.IsHealthy() {
		return fmt.Errorf("session health check failed:\n%s", health.Summary())
	}

	// 4. Ensure tmux session exists
	exists, err := tmux.HasSession(m.Tmux.SessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		// Create new tmux session
		if err := tmux.NewSession(m.Tmux.SessionName, m.Worktree.Path); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// 5. Send commands to tmux
	// Change directory
	cdCmd := fmt.Sprintf("cd %s", m.Worktree.Path)
	if err := tmux.SendCommand(m.Tmux.SessionName, cdCmd); err != nil {
		return fmt.Errorf("failed to send cd command: %w", err)
	}

	// Resume Claude
	resumeCmd := fmt.Sprintf("claude --resume %s", m.Claude.SessionID)
	if err := tmux.SendCommand(m.Tmux.SessionName, resumeCmd); err != nil {
		return fmt.Errorf("failed to send claude resume command: %w", err)
	}

	// 6. Update manifest metadata
	m.Status = manifest.StatusActive
	m.LastActivity = time.Now()
	m.Claude.LastActivity = time.Now()

	if err := manifest.Write(manifestPath, m); err != nil {
		return fmt.Errorf("failed to update manifest: %w", err)
	}

	// 7. Attach to tmux session
	if err := tmux.AttachSession(m.Tmux.SessionName); err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}

	return nil
}
