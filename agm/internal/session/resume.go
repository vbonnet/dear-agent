// Package session provides session functionality.
package session

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/transcript"
)

var (
	resumeIsClaudeRunning      = tmux.IsClaudeRunning
	resumeNewSession           = tmux.NewSession
	resumeSendCommand          = tmux.SendCommand
	resumeWaitForClaudeReady   = tmux.WaitForClaudeReady
	resumeKillSession          = tmux.KillSessionChecked
	resumePrepareClaudeCommand = harnessexec.PrepareClaudeCommand
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

	if err := ensureClaudeResumeProcess(m, exists); err != nil {
		return err
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

func ensureClaudeResumeProcess(m *manifest.Manifest, exists bool) error {
	if exists && !claudeResumeNeedsDelivery(m.Tmux.SessionName) {
		return nil
	}

	prepared, err := resumePrepareClaudeCommand(harnessexec.ClaudeLaunch{
		SessionName: m.Tmux.SessionName, SessionID: m.SessionID,
		ResumeID: m.Claude.UUID, WorkDir: m.Context.Project, ForwardTelemetry: true,
	}, os.Environ())
	if err != nil {
		return fmt.Errorf("prepare Claude resume command: %w", err)
	}

	created := false
	if !exists {
		// Stage the private handoff before allocating tmux so preparation failure
		// cannot leave an untracked shell session behind.
		if err := resumeNewSession(m.Tmux.SessionName, m.Context.Project); err != nil {
			return errors.Join(fmt.Errorf("failed to create tmux session: %w", err), cancelPreparedResume(prepared))
		}
		created = true
	}

	if submissionErr := resumeSendCommand(m.Tmux.SessionName, prepared.Command); submissionErr != nil {
		uncertain, err := harnessexec.ResolveSubmission(submissionErr, prepared.Cancel)
		if uncertain {
			fmt.Fprintf(os.Stderr,
				"Warning: Claude resume submission acknowledgement was lost; preserving the private handoff because the command may be queued: %v\n",
				submissionErr)
		}
		if err != nil {
			primaryErr := fmt.Errorf("failed to send resume command: %w", err)
			if !created {
				return primaryErr
			}
			if cleanupErr := resumeKillSession(m.Tmux.SessionName); cleanupErr != nil {
				return errors.Join(primaryErr, fmt.Errorf("clean up created tmux session: %w", cleanupErr))
			}
			return primaryErr
		}
	}

	if err := resumeWaitForClaudeReady(m.Tmux.SessionName, contracts.Load().SessionLifecycle.ResumeReadyTimeout.Duration); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Claude prompt not detected after resume: %v\n", err)
	}
	return nil
}

func claudeResumeNeedsDelivery(sessionName string) bool {
	running, err := resumeIsClaudeRunning(sessionName)
	// Detection failure proves nothing, so preserve the existing pane.
	return err == nil && !running
}

func cancelPreparedResume(prepared harnessexec.PreparedCommand) error {
	if err := prepared.Cancel(); err != nil {
		return fmt.Errorf("cancel undelivered Claude resume handoff: %w", err)
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
