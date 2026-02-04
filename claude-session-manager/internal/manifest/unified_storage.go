package manifest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// MigrationOptions configures unified storage migration behavior
type MigrationOptions struct {
	DryRun    bool   // Preview changes without modifying files
	Force     bool   // Overwrite existing destinations
	Workspace string // Filter to specific workspace (empty = all)
}

// MigrationReport summarizes migration results
type MigrationReport struct {
	Succeeded int
	Failed    int
	Skipped   int
	Errors    []MigrationError
}

// MigrationError captures details of a failed migration
type MigrationError struct {
	SessionID   string
	SessionName string
	Error       string
}

// MigrateToUnifiedStorage migrates sessions from workspace-specific locations
// to unified storage at ~/src/sessions/{session-name}/
func MigrateToUnifiedStorage(locations []SessionLocation, opts MigrationOptions) (*MigrationReport, error) {
	report := &MigrationReport{
		Errors: []MigrationError{},
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	unifiedBaseDir := filepath.Join(homeDir, "src", "sessions")

	// Ensure unified sessions directory exists
	if !opts.DryRun {
		if err := os.MkdirAll(unifiedBaseDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create unified sessions directory: %w", err)
		}
	}

	for _, location := range locations {
		// Filter by workspace if specified
		if opts.Workspace != "" && location.Workspace != opts.Workspace {
			continue
		}

		// Target directory: ~/src/sessions/{session-name}/
		targetDir := filepath.Join(unifiedBaseDir, location.Name)
		targetManifestPath := filepath.Join(targetDir, "manifest.yaml")

		// Check if destination exists
		if _, err := os.Stat(targetDir); err == nil {
			if !opts.Force {
				report.Skipped++
				continue // Skip if exists and not forcing
			}
		}

		if opts.DryRun {
			fmt.Printf("[DRY-RUN] Would migrate: %s → %s\n", location.ManifestPath, targetManifestPath)
			report.Succeeded++
			continue
		}

		// Acquire lock to prevent concurrent migrations
		if err := AcquireLock(location.ManifestPath); err != nil {
			report.Failed++
			report.Errors = append(report.Errors, MigrationError{
				SessionID:   location.SessionID,
				SessionName: location.Name,
				Error:       fmt.Sprintf("lock acquisition failed: %v", err),
			})
			continue
		}

		// Perform migration
		if err := migrateSession(location, targetDir); err != nil {
			ReleaseLock(location.ManifestPath)
			report.Failed++
			report.Errors = append(report.Errors, MigrationError{
				SessionID:   location.SessionID,
				SessionName: location.Name,
				Error:       err.Error(),
			})
			continue
		}

		ReleaseLock(location.ManifestPath)

		// Log successful migration
		logMigrationSuccess(location.SessionID, location.ManifestPath, targetManifestPath)
		report.Succeeded++
	}

	return report, nil
}

// migrateSession performs the actual file movement for a single session
func migrateSession(location SessionLocation, targetDir string) error {
	// Create target directory
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Move manifest
	targetManifestPath := filepath.Join(targetDir, "manifest.yaml")
	if err := moveFile(location.ManifestPath, targetManifestPath); err != nil {
		return fmt.Errorf("failed to move manifest: %w", err)
	}

	// Move conversation files (if any exist in conversation directory)
	conversationFiles, _ := filepath.Glob(filepath.Join(location.ConversationDir, "*.html"))
	for _, srcFile := range conversationFiles {
		dstFile := filepath.Join(targetDir, filepath.Base(srcFile))
		if err := moveFile(srcFile, dstFile); err != nil {
			// Non-fatal: log warning but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to move conversation %s: %v\n", srcFile, err)
		}
	}

	return nil
}

// moveFile moves a file atomically (same filesystem) or via copy+delete (cross filesystem)
func moveFile(src, dst string) error {
	// Try atomic rename first (works only on same filesystem)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fallback: copy + delete for cross-filesystem moves
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Verify copy succeeded, then remove source
	if err := dstFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync destination: %w", err)
	}

	if err := os.Remove(src); err != nil {
		return fmt.Errorf("failed to remove source after copy: %w", err)
	}

	return nil
}

// SessionLocation (redeclared here to avoid circular import with discovery package)
// In production, this would be in a shared types package
type SessionLocation struct {
	Workspace       string
	SessionID       string
	Name            string
	ManifestPath    string
	ConversationDir string
}

// logMigrationSuccess logs a successful migration to audit log
func logMigrationSuccess(sessionID, fromPath, toPath string) {
	// In production, this would write to ~/.agm/logs/migration-unified-storage.log
	// For now, log to stderr
	timestamp := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(os.Stderr, "[%s] [%s] [SUCCESS] [%s] → [%s]\n", timestamp, sessionID, fromPath, toPath)
}
