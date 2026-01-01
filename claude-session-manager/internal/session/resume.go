package session

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/config"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/transcript"
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

	sendCommands := false
	if !exists {
		// Create new tmux session (v2: use Context.Project for working directory)
		if err := tmux.NewSession(m.Tmux.SessionName, m.Context.Project); err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
		sendCommands = true
	} else {
		// Check if Claude is already running
		claudeRunning, err := tmux.IsProcessRunning(m.Tmux.SessionName, "claude")
		if err != nil {
			// Detection failed - skip commands for safety
			sendCommands = false
		} else if claudeRunning {
			// Claude already running - skip commands
			sendCommands = false
		} else {
			// Claude not running - send commands
			sendCommands = true
		}
	}

	// 5. Send commands to tmux only if needed
	if sendCommands {
		// Change directory
		cdCmd := fmt.Sprintf("cd %s", m.Context.Project)
		if err := tmux.SendCommand(m.Tmux.SessionName, cdCmd); err != nil {
			return fmt.Errorf("failed to send cd command: %w", err)
		}

		// Resume Claude (v2: use Claude.UUID for the actual Claude session UUID)
		var resumeCmd string
		if m.Claude.UUID != "" {
			resumeCmd = fmt.Sprintf("claude --resume %s; exit", m.Claude.UUID)
		} else {
			// Fallback to starting a new Claude session if UUID is not set
			resumeCmd = "claude; exit"
		}
		if err := tmux.SendCommand(m.Tmux.SessionName, resumeCmd); err != nil {
			return fmt.Errorf("failed to send claude resume command: %w", err)
		}

		// Wait for Claude to be ready
		_ = tmux.WaitForProcessReady(m.Tmux.SessionName, "claude", 5*time.Second)
	}

	// 6. Update manifest metadata (v2: only UpdatedAt is auto-updated by Write)
	// No need to set Status or LastActivity (not in v2 schema)

	if err := manifest.Write(manifestPath, m); err != nil {
		return fmt.Errorf("failed to update manifest: %w", err)
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
