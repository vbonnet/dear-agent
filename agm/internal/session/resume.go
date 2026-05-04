// Package session provides session functionality.
package session

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/transcript"
)

// shellQuote quotes a string for safe use in shell commands
// This prevents command injection by escaping special characters
func shellQuote(s string) string {
	// Simple but secure: wrap in single quotes and escape any single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// Resume orchestrates the full resume workflow
func Resume(identifier string, cfg *config.Config, adapter *dolt.Adapter) error {
	// 1. Resolve identifier to manifest
	m, _, err := ResolveIdentifier(identifier, cfg.SessionsDir, adapter)
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

	sendCommands := false
	if !exists {
		// Create new tmux session (v2: use Context.Project for working directory)
		if err := tmux.NewSession(m.Tmux.SessionName, m.Context.Project); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		sendCommands = true
	} else {
		// Check if Claude is already running
		claudeRunning, err := tmux.IsClaudeRunning(m.Tmux.SessionName)
		switch {
		case err != nil:
			// Detection failed - skip commands for safety
			sendCommands = false
		case claudeRunning:
			// Claude already running - skip commands
			sendCommands = false
		default:
			// Claude not running - send commands
			sendCommands = true
		}
	}

	// 5. Send commands to tmux only if needed
	if sendCommands {
		// Build combined command: cd <project-dir> && claude --resume <uuid> && exit
		// This ensures the directory change happens in the same shell as the Claude command
		var fullCmd string
		if m.Claude.UUID != "" {
			fullCmd = fmt.Sprintf("cd %s && claude --resume %s && exit",
				shellQuote(m.Context.Project),
				shellQuote(m.Claude.UUID))
		} else {
			// Fallback to starting a new Claude session if UUID is not set
			fullCmd = fmt.Sprintf("cd %s && claude && exit", shellQuote(m.Context.Project))
		}

		// Send combined command to tmux
		if err := tmux.SendCommand(m.Tmux.SessionName, fullCmd); err != nil {
			return fmt.Errorf("failed to send resume command: %w", err)
		}

		// Wait for Claude to be ready
		_ = tmux.WaitForClaudeReady(m.Tmux.SessionName, contracts.Load().SessionLifecycle.ResumeReadyTimeout.Duration)
	}

	// 6. Update manifest metadata (v2: only UpdatedAt is auto-updated by Write)
	// No need to set Status or LastActivity (not in v2 schema)
	m.UpdatedAt = time.Now()

	// Write to Dolt database
	if adapter == nil {
		return fmt.Errorf("dolt adapter required")
	}
	if err := adapter.UpdateSession(m); err != nil {
		return fmt.Errorf("failed to update session in Dolt: %w", err)
	}

	// 7. Extract and display transcript context (if available)
	displayTranscriptContext(m)

	// 8. Attach to tmux session
	if err := tmux.AttachSession(m.Tmux.SessionName); err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}

	return nil
}

// displayTranscriptContext extracts and displays context from previous session transcript
func displayTranscriptContext(m *manifest.Manifest) {
	// Only attempt if UUID is set
	if m.Claude.UUID == "" {
		return
	}

	// Extract context (last 3 exchanges = 6 messages)
	ctx, err := transcript.ExtractContext(m.Context.Project, m.Claude.UUID, 3)
	if err != nil {
		// Silently skip if transcript not available (not an error)
		return
	}

	// Check if we're in Desktop (TMUX set) or Web environment
	isDesktop := os.Getenv("TMUX") != ""

	if isDesktop {
		// Desktop: Print context to terminal (boxed UI)
		fmt.Println()
		fmt.Println(ctx.FormatForDisplay())
		fmt.Println()
	} else {
		// Web: Print instructions (clipboard copy not implemented in v1)
		fmt.Println()
		fmt.Println("📝 Transcript context available from previous session:")
		fmt.Println("   (Copy the following to resume context)")
		fmt.Println()
		fmt.Println(ctx.FormatForDisplay())
		fmt.Println()
	}
}
