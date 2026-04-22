// Package backup provides backup functionality.
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
)

const MaxBackups = 10

// CreateBackup creates a numbered backup of a file
// Returns backup number created
func CreateBackup(sourcePath string) (int, error) {
	// Find next backup number
	backups, err := ListBackups(sourcePath)
	if err != nil {
		return 0, err
	}

	nextNum := 1
	if len(backups) > 0 {
		nextNum = backups[len(backups)-1] + 1
	}

	// Create backup
	backupPath := fmt.Sprintf("%s.%d", sourcePath, nextNum)
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read source: %w", err)
	}

	if err := fileutil.AtomicWrite(backupPath, data, 0600); err != nil {
		return 0, fmt.Errorf("failed to write backup: %w", err)
	}

	// Rotate if needed
	if err := RotateBackups(sourcePath, MaxBackups); err != nil {
		return nextNum, fmt.Errorf("backup created but rotation failed: %w", err)
	}

	return nextNum, nil
}

// ListBackups returns sorted list of backup numbers for a file
func ListBackups(sourcePath string) ([]int, error) {
	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	backups := []int{}
	for _, entry := range entries {
		name := entry.Name()
		// Match pattern: manifest.yaml.1, manifest.yaml.2, etc.
		if strings.HasPrefix(name, base+".") {
			numStr := strings.TrimPrefix(name, base+".")
			num, err := strconv.Atoi(numStr)
			if err == nil {
				backups = append(backups, num)
			}
		}
	}

	sort.Ints(backups)
	return backups, nil
}

// RotateBackups deletes oldest backups if count exceeds maxBackups
func RotateBackups(sourcePath string, maxBackups int) error {
	backups, err := ListBackups(sourcePath)
	if err != nil {
		return err
	}

	// Delete oldest backups
	for len(backups) > maxBackups {
		oldest := backups[0]
		backupPath := fmt.Sprintf("%s.%d", sourcePath, oldest)
		if err := os.Remove(backupPath); err != nil {
			return fmt.Errorf("failed to remove old backup: %w", err)
		}
		backups = backups[1:]
	}

	return nil
}

// RestoreBackup restores a backup to the source file
// Creates a backup of current state before restoring
func RestoreBackup(sourcePath string, backupNum int) error {
	backupPath := fmt.Sprintf("%s.%d", sourcePath, backupNum)

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup not found: %s", backupPath)
	}

	// Backup current state first
	if _, err := os.Stat(sourcePath); err == nil {
		if _, err := CreateBackup(sourcePath); err != nil {
			return fmt.Errorf("failed to backup current state: %w", err)
		}
	}

	// Restore backup
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup: %w", err)
	}

	if err := fileutil.AtomicWrite(sourcePath, data, 0600); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return nil
}
