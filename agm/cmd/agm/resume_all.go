package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	resumeAllDetached    bool
	resumeAllIncludeArch bool
	resumeAllWorkspace   string
	resumeAllDryRun      bool
	resumeAllContinue    bool
)

var resumeAllCmd = &cobra.Command{
	Use:   "resume-all",
	Short: "Resume all stopped Claude sessions",
	Long: `Resume all non-active (stopped) sessions in batch.

Behavior:
  • Filters sessions: active (skip) vs stopped (resume) vs archived (skip by default)
  • Resumes sessions sequentially to avoid resource contention
  • Collects errors per session and reports summary
  • Updates manifest timestamps for all resumed sessions

Flags:
  --detached              Resume all sessions without attaching (default: true)
  --include-archived      Also attempt to resume archived sessions
  --workspace-filter=STR  Only resume sessions in specific workspace
  --dry-run              Preview which sessions would be resumed
  --continue-on-error    Continue resuming even if some sessions fail (default: true)

Examples:
  agm sessions resume-all                          # Resume all stopped sessions
  agm sessions resume-all --workspace-filter=proj  # Filter by workspace
  agm sessions resume-all --dry-run                # Preview without resuming
  agm sessions resume-all --include-archived       # Also resume archived`,
	RunE: runResumeAll,
}

func init() {
	resumeAllCmd.Flags().BoolVar(&resumeAllDetached, "detached", true, "Resume without attaching")
	resumeAllCmd.Flags().BoolVar(&resumeAllIncludeArch, "include-archived", false, "Also resume archived sessions")
	resumeAllCmd.Flags().StringVar(&resumeAllWorkspace, "workspace-filter", "", "Filter by workspace")
	resumeAllCmd.Flags().BoolVar(&resumeAllDryRun, "dry-run", false, "Preview without executing")
	resumeAllCmd.Flags().BoolVar(&resumeAllContinue, "continue-on-error", true, "Continue on failures")
	sessionCmd.AddCommand(resumeAllCmd)
}

func runResumeAll(cmd *cobra.Command, args []string) error {
	// Get Dolt storage adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// 1. Load all manifests from Dolt
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// 2. Filter archived sessions (unless --include-archived)
	if !resumeAllIncludeArch {
		manifests = filterNonArchived(manifests)
	}

	// 3. Apply workspace filter if specified
	if resumeAllWorkspace != "" {
		manifests = filterByWorkspace(manifests, resumeAllWorkspace)
	}

	// 4. Compute status batch (efficient single tmux query)
	statuses := session.ComputeStatusBatch(manifests, tmuxClient)

	// 5. Filter to only "stopped" sessions
	var stoppedSessions []*manifest.Manifest
	for _, m := range manifests {
		if statuses[m.Name] == "stopped" {
			stoppedSessions = append(stoppedSessions, m)
		}
	}

	// 6. Display preview and count
	fmt.Printf("Found %d stopped session(s) to resume:\n\n", len(stoppedSessions))
	for _, m := range stoppedSessions {
		fmt.Printf("  • %s (%s)\n", m.Name, m.Context.Project)
	}
	fmt.Println()

	// 7. Dry-run mode check
	if resumeAllDryRun {
		fmt.Println("ℹ️  Dry-run mode - no sessions resumed")
		return nil
	}

	// 8. No sessions to resume
	if len(stoppedSessions) == 0 {
		fmt.Println("ℹ️  No stopped sessions found")
		return nil
	}

	// 9. Resume sessions with progress
	return resumeSessionsBatch(adapter, stoppedSessions)
}

// filterNonArchived removes archived sessions from list
func filterNonArchived(manifests []*manifest.Manifest) []*manifest.Manifest {
	filtered := make([]*manifest.Manifest, 0, len(manifests))
	for _, m := range manifests {
		if m.Lifecycle != manifest.LifecycleArchived {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// filterByWorkspace filters sessions by workspace field
func filterByWorkspace(manifests []*manifest.Manifest, workspace string) []*manifest.Manifest {
	filtered := make([]*manifest.Manifest, 0, len(manifests))
	for _, m := range manifests {
		if m.Workspace == workspace {
			filtered = append(filtered, m)
		}
	}
	return filtered
}

// resumeSessionsBatch resumes sessions sequentially with progress UI
func resumeSessionsBatch(adapter *dolt.Adapter, sessions []*manifest.Manifest) error {
	var successCount, failCount int
	var errors []string

	// Initialize progress UI
	prog := progress.New(progress.WithDefaultGradient())
	spin := spinner.New()
	spin.Spinner = spinner.Dot

	total := len(sessions)

	for i, m := range sessions {
		// Display progress
		percent := float64(i) / float64(total)
		fmt.Printf("\r%s [%d/%d] %s Resuming: %s\n",
			spin.View(),
			i+1,
			total,
			prog.ViewAs(percent),
			m.Name)

		// Find manifest path
		manifestPath := filepath.Join(cfg.SessionsDir, m.SessionID, "manifest.yaml")

		// Check session health
		health, err := checkSessionHealth(adapter, m.SessionID, manifestPath)
		if err != nil || !health.CanResume {
			errMsg := fmt.Sprintf("%s: health check failed", m.Name)
			errors = append(errors, errMsg)
			failCount++

			if !resumeAllContinue {
				break
			}
			continue
		}

		// Auto-detect harness from manifest
		harnessName := m.Harness
		if harnessName == "" {
			harnessName = "claude-code" // Default for backward compatibility
		}

		// Resume session (force detached mode for bulk)
		originalDetached := resumeDetached
		resumeDetached = resumeAllDetached

		err = resumeSession(adapter, m.SessionID, manifestPath, harnessName, health)

		resumeDetached = originalDetached

		if err != nil {
			errMsg := fmt.Sprintf("%s: %v", m.Name, err)
			errors = append(errors, errMsg)
			failCount++

			if !resumeAllContinue {
				break
			}
		} else {
			successCount++

			// Write resume timestamp for orchestrator coordination (ADR-010)
			// Non-critical: Log warning but don't fail if timestamp write fails
			if err := writeResumeTimestamp(m.SessionID); err != nil {
				ui.PrintWarning(fmt.Sprintf("Warning: failed to write resume timestamp for %s: %v", m.Name, err))
			}
		}

		// Delay between operations to avoid tmux overload
		if i < total-1 { // Skip delay after last session
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Clear progress line
	fmt.Print("\r" + strings.Repeat(" ", 80) + "\r")

	// Display summary
	fmt.Println(strings.Repeat("=", 60))
	if successCount > 0 {
		ui.PrintSuccess(fmt.Sprintf("Successfully resumed %d session(s)", successCount))
	}
	if failCount > 0 {
		ui.PrintWarning(fmt.Sprintf("Failed to resume %d session(s)", failCount))
		fmt.Println("\nErrors:")
		for _, errMsg := range errors {
			fmt.Printf("  • %s\n", errMsg)
		}
	}

	return nil
}

// writeResumeTimestamp creates .agm/resume-timestamp file for orchestrator coordination
// This enables orchestrator v2 to detect recently resumed sessions and send restart prompts
// See ADR-010 for integration details: docs/adr/ADR-010-orchestrator-resume-detection.md
func writeResumeTimestamp(sessionID string) error {
	// Create .agm directory if it doesn't exist
	agmDir := filepath.Join(cfg.SessionsDir, sessionID, ".agm")
	if err := os.MkdirAll(agmDir, 0755); err != nil {
		return fmt.Errorf("failed to create .agm directory: %w", err)
	}

	// Write current timestamp in RFC3339 format
	timestampFile := filepath.Join(agmDir, "resume-timestamp")
	timestamp := time.Now().Format(time.RFC3339)

	if err := os.WriteFile(timestampFile, []byte(timestamp), 0644); err != nil {
		return fmt.Errorf("failed to write resume timestamp: %w", err)
	}

	return nil
}
