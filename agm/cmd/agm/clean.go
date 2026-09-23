package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Batch cleanup sessions (archive/delete)",
	Long: `Interactive cleanup tool for batch operations on sessions.

Shows multi-select lists of:
  • Stopped sessions >30 days (suggested for archival)
  • Archived sessions >90 days (suggested for deletion)

Thresholds can be customized in ~/.config.agm/config.yaml

Examples:
  agm admin clean                 # Interactive cleanup with smart suggestions
  agm admin clean --dry-run       # Preview what would be cleaned`,
	RunE: func(cmd *cobra.Command, args []string) error {
		uiCfg := ui.GetGlobalConfig()

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer func() { _ = adapter.Close() }()

		// List all sessions from Dolt
		manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			return fmt.Errorf("failed to list sessions: %w", err)
		}

		// Convert to UI sessions with status
		uiSessions := uiSessionsFromManifests(manifests, tmuxClient)

		// Show multi-select cleanup UI
		result, err := ui.CleanupMultiSelect(uiSessions, uiCfg)
		if err != nil {
			return fmt.Errorf("cleanup cancelled: %w", err)
		}

		if len(result.ToArchive) == 0 && len(result.ToDelete) == 0 {
			fmt.Println("No sessions selected for cleanup.")
			return nil
		}

		// Confirm cleanup
		confirmed, err := ui.ConfirmCleanup(
			cleanupConfirmationLabels(result.ToArchive),
			cleanupConfirmationLabels(result.ToDelete),
			uiCfg,
		)
		if err != nil {
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
		strictTmux, ok := tmuxClient.(session.StrictSessionExistenceChecker)
		if !ok {
			return fmt.Errorf("cleanup requires a strict tmux session checker")
		}

		// Perform cleanup operations
		archived := 0
		deleted := 0

		// Archive stopped sessions
		for _, s := range result.ToArchive {
			applied, reason, err := applyCleanupSelection(cmd.Context(), adapter, s,
				cleanupArchive, uiCfg.Defaults.CleanupThresholdDays, strictTmux)
			// A completed mutation is counted before cancellation is reported.
			// archiveSessionManifest is not context-aware, so a signal arriving
			// mid-call still returns applied=true, and reporting the interrupt
			// first would understate what actually happened and claim the
			// current target was untouched when it was not.
			switch {
			case err == nil && applied:
				archived++
				fmt.Printf("📦 Archived: %s\n", s.Name)
			case cleanupCanceled(cmd.Context(), err):
				return cleanupCancellation(cmd.Context(), err, archived, deleted)
			case err != nil:
				ui.PrintWarning(fmt.Sprintf("Failed to archive %s: %v", s.Name, err))
			default:
				ui.PrintWarning(fmt.Sprintf("Skipped archive of %s: %s", s.Name, reason))
			}
			if cleanupCanceled(cmd.Context(), nil) {
				return cleanupCancellation(cmd.Context(), nil, archived, deleted)
			}
		}

		// Delete archived sessions
		for _, s := range result.ToDelete {
			applied, reason, err := applyCleanupSelection(cmd.Context(), adapter, s,
				cleanupDelete, uiCfg.Defaults.ArchiveThresholdDays, strictTmux)
			// Counted before the interrupt, for the same reason as the archive
			// loop above: os.RemoveAll is not context-aware.
			switch {
			case err == nil && applied:
				deleted++
				fmt.Printf("🗑️  Deleted: %s\n", s.Name)
			case cleanupCanceled(cmd.Context(), err):
				return cleanupCancellation(cmd.Context(), err, archived, deleted)
			case err != nil:
				ui.PrintWarning(fmt.Sprintf("Failed to delete %s: %v", s.Name, err))
			default:
				ui.PrintWarning(fmt.Sprintf("Skipped deletion of %s: %s", s.Name, reason))
			}
			if cleanupCanceled(cmd.Context(), nil) {
				return cleanupCancellation(cmd.Context(), nil, archived, deleted)
			}
		}

		// Summary
		fmt.Println()
		ui.PrintSuccess(fmt.Sprintf("Cleanup complete: %d archived, %d deleted", archived, deleted))
		return nil
	},
}

// cleanupCanceled reports whether the batch was interrupted rather than
// hitting a per-item problem.
//
// SIGINT and SIGTERM cancel the root command context, and applyCleanupSelection
// surfaces that as an ordinary error. Treating it as a per-item warning let
// both loops run to completion and RunE print "Cleanup complete" and return
// nil, so an interrupted batch exited successfully while silently leaving
// every remaining selection untouched.
func cleanupCanceled(ctx context.Context, err error) bool {
	return ctx.Err() != nil || errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// cleanupCancellation reports what the interrupted batch did manage to do, and
// returns a non-nil error so the command's exit status says it did not finish.
func cleanupCancellation(ctx context.Context, err error, archived, deleted int) error {
	cause := err
	if cause == nil {
		cause = ctx.Err()
	}
	ui.PrintWarning(fmt.Sprintf(
		"Cleanup interrupted after %d archived and %d deleted; remaining selections were not touched",
		archived, deleted))
	return fmt.Errorf("cleanup canceled: %w", cause)
}

type cleanupAction uint8

const (
	cleanupArchive cleanupAction = iota
	cleanupDelete
)

// applyCleanupSelection never acts on the manifest held across the interactive
// picker and confirmation. It shares the lifecycle lock with archive and resume.
func applyCleanupSelection(ctx context.Context, adapter *dolt.Adapter, selected *ui.Session,
	action cleanupAction, thresholdDays int, checker session.StrictSessionExistenceChecker) (bool, string, error) {
	if err := validateCleanupSelection(selected, checker); err != nil {
		return false, "", err
	}
	var applied bool
	var reason string
	err := ops.WithSessionLockContext(ctx, selected.SessionID, func() error {
		var operationErr error
		applied, reason, operationErr = applyLockedCleanupSelection(ctx, adapter, selected,
			action, thresholdDays, checker)
		return operationErr
	})
	return applied, reason, err
}

func validateCleanupSelection(selected *ui.Session, checker session.StrictSessionExistenceChecker) error {
	if selected == nil || selected.Manifest == nil || selected.SessionID == "" {
		return fmt.Errorf("cleanup selection has no stable session ID")
	}
	if id := selected.SessionID; id == "." || id == ".." || filepath.Base(id) != id || filepath.IsAbs(id) {
		return fmt.Errorf("cleanup session ID %q is not a single path component", id)
	}
	if checker == nil {
		return fmt.Errorf("strict tmux probe is required")
	}
	return nil
}

func applyLockedCleanupSelection(ctx context.Context, adapter *dolt.Adapter, selected *ui.Session,
	action cleanupAction, thresholdDays int, checker session.StrictSessionExistenceChecker) (bool, string, error) {
	id := selected.SessionID
	current, err := adapter.GetSession(id)
	if err != nil {
		return false, "", fmt.Errorf("reload session %s: %w", id, err)
	}
	if current == nil || current.SessionID != id {
		return false, "", fmt.Errorf("reload session %s: stable identity missing", id)
	}
	reason, err := cleanupEligibility(current, selected, action, thresholdDays)
	if err != nil || reason != "" {
		return false, reason, err
	}
	active, err := checker.HasSessionStrict(ctx, session.TmuxSessionName(current))
	if err != nil {
		return false, "", fmt.Errorf("check tmux session %s: %w", id, err)
	}
	if active {
		return false, "session has an active tmux pane", nil
	}
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	if action == cleanupArchive {
		err = archiveSessionManifest(adapter, current)
	} else {
		err = deleteSessionManifest(current)
	}
	return err == nil, "", err
}

func cleanupEligibility(current *manifest.Manifest, selected *ui.Session,
	action cleanupAction, thresholdDays int) (string, error) {
	if !current.UpdatedAt.Equal(selected.UpdatedAt) || !reflect.DeepEqual(current, selected.Manifest) {
		return "session changed after selection", nil
	}
	if thresholdDays > 0 && !current.UpdatedAt.Before(time.Now().AddDate(0, 0, -thresholdDays)) {
		return "session no longer meets the age threshold", nil
	}
	switch action {
	case cleanupArchive:
		if current.Lifecycle != manifest.LifecycleLegacy {
			return "session is no longer stopped", nil
		}
	case cleanupDelete:
		if current.Lifecycle != manifest.LifecycleArchived {
			return "session is no longer archived", nil
		}
	default:
		return "", fmt.Errorf("unknown cleanup action %d", action)
	}
	return "", nil
}

func cleanupConfirmationLabels(sessions []*ui.Session) []string {
	labels := make([]string, len(sessions))
	for i, s := range sessions {
		labels[i] = fmt.Sprintf("%q [ID: %q]", s.Name, s.SessionID)
	}
	return labels
}

func archiveSessionManifest(adapter *dolt.Adapter, m *manifest.Manifest) error {
	m.Lifecycle = manifest.LifecycleArchived
	// `agm clean` archives stopped sessions during routine cleanup — treat them
	// as completed so the archived record is triage-legible.
	if m.Outcome == manifest.OutcomeUnknown {
		m.Outcome = manifest.OutcomeCompleted
	}
	if err := adapter.UpdateSession(m); err != nil {
		return err
	}

	// Auto-commit manifest change if in git repo
	manifestPath := getManifestPath(m.SessionID)
	_ = git.CommitManifest(manifestPath, "archive", m.Name) // Errors logged internally

	return nil
}

func deleteSessionManifest(m *manifest.Manifest) error {
	manifestDir := getSessionDir(m.SessionID)
	return os.RemoveAll(manifestDir)
}

func getManifestPath(sessionID string) string {
	return filepath.Join(cfg.SessionsDir, sessionID, "manifest.yaml")
}

func getSessionDir(sessionID string) string {
	return filepath.Join(cfg.SessionsDir, sessionID)
}

func init() {
	adminCmd.AddCommand(cleanCmd)
}
