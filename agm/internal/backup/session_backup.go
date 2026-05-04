package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// BackupInfo contains metadata about a session backup
type BackupInfo struct {
	SessionName string    // Original session name
	Timestamp   time.Time // Backup creation time
	Path        string    // Full path to backup file
	Size        int64     // Backup file size in bytes
}

// BackupSession creates a tarball backup of a session directory
// sessionID: UUID of session to backup
// Returns: backup path, error
func BackupSession(sessionID string) (string, error) {
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Session directory: $HOME/.claude/sessions/{sessionID}
	sessionDir := filepath.Join(homeDir, ".claude", "sessions", sessionID)
	return backupSessionFromDir(sessionID, sessionDir)
}

// BackupSessionFromDir creates a tarball backup from a custom sessions directory
// sessionID: UUID of session to backup
// sessionsDir: Base directory containing sessions
// Returns: backup path, error
func BackupSessionFromDir(sessionID string, sessionsDir string) (string, error) {
	sessionDir := filepath.Join(sessionsDir, sessionID)
	return backupSessionFromDir(sessionID, sessionDir)
}

// backupSessionFromDir is the internal implementation
func backupSessionFromDir(sessionID string, sessionDir string) (string, error) {
	// Get home directory for backup location
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		return "", fmt.Errorf("session directory not found: %s", sessionDir)
	}

	// Read session manifest to get session name
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	sessionName, err := readSessionName(manifestPath)
	if err != nil {
		// Fallback to UUID if manifest can't be read
		sessionName = sessionID
	}
	if sessionName == "" {
		sessionName = sessionID // Fallback to UUID if name not set
	}

	// Sanitize session name for filename (replace special chars with dash)
	sanitizedName := sanitizeFilename(sessionName)

	// Create backup directory: $HOME/.agm/backups/sessions/
	backupDir := filepath.Join(homeDir, ".agm", "backups", "sessions")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate filename: {session-name}-{timestamp}.tar.gz
	// Timestamp format: 20260317-143022 (YYYYMMdd-HHMMSS)
	timestamp := time.Now().Format("20060102-150405")
	backupFilename := fmt.Sprintf("%s-%s.tar.gz", sanitizedName, timestamp)
	backupPath := filepath.Join(backupDir, backupFilename)

	// Create tar.gz archive
	if err := createTarGz(sessionDir, backupPath); err != nil {
		return "", fmt.Errorf("failed to create backup archive: %w", err)
	}

	return backupPath, nil
}

// RestoreSession restores a session from a backup tarball
// backupPath: Full path to backup .tar.gz file
// Returns: error
func RestoreSession(backupPath string) error {
	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found: %s", backupPath)
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Extract to ~/.claude/sessions/
	sessionsDir := filepath.Join(homeDir, ".claude", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Extract tar.gz archive
	if err := extractTarGz(backupPath, sessionsDir); err != nil {
		return fmt.Errorf("failed to extract backup: %w", err)
	}

	return nil
}

// ListAllSessionBackups lists available session backups
// Returns: slice of BackupInfo, error
func ListAllSessionBackups() ([]BackupInfo, error) {
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Backup directory
	backupDir := filepath.Join(homeDir, ".agm", "backups", "sessions")

	// Check if backup directory exists
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		return []BackupInfo{}, nil // Return empty list if no backups directory
	}

	// List files in backup directory
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .tar.gz files
		name := entry.Name()
		if !strings.HasSuffix(name, ".tar.gz") {
			continue
		}

		// Parse filename: {name}-{timestamp}.tar.gz
		parts := strings.Split(strings.TrimSuffix(name, ".tar.gz"), "-")
		if len(parts) < 2 {
			continue // Skip malformed filenames
		}

		// Last part is timestamp (YYYYMMdd-HHMMSS)
		timestampStr := parts[len(parts)-1]
		// Second to last might also be part of timestamp
		if len(parts) >= 3 {
			timestampStr = parts[len(parts)-2] + "-" + parts[len(parts)-1]
		}

		// Parse timestamp. Backups are written via time.Now().Format(...)
		// which produces a local-time string; parsing must round-trip in the
		// same zone or "time since backup" is skewed by the local offset.
		timestamp, err := time.ParseInLocation("20060102-150405", timestampStr, time.Local)
		if err != nil {
			continue // Skip if timestamp can't be parsed
		}

		// Session name is everything before the timestamp
		sessionName := strings.Join(parts[:len(parts)-2], "-")
		if sessionName == "" && len(parts) >= 3 {
			sessionName = strings.Join(parts[:len(parts)-2], "-")
		}
		if sessionName == "" {
			sessionName = parts[0] // Fallback
		}

		// Get file info for size
		fullPath := filepath.Join(backupDir, name)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			SessionName: sessionName,
			Timestamp:   timestamp,
			Path:        fullPath,
			Size:        info.Size(),
		})
	}

	return backups, nil
}

// readSessionName reads the session name from manifest.yaml
func readSessionName(manifestPath string) (string, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", err
	}

	// Parse only the name field
	var manifest struct {
		Name string `yaml:"name"`
	}

	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return "", err
	}

	return manifest.Name, nil
}

// sanitizeFilename replaces special characters with dashes for safe filenames
func sanitizeFilename(name string) string {
	// Replace characters that are problematic in filenames
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "-",
	)
	sanitized := replacer.Replace(name)

	// Collapse multiple dashes
	for strings.Contains(sanitized, "--") {
		sanitized = strings.ReplaceAll(sanitized, "--", "-")
	}

	// Trim leading/trailing dashes
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}

// createTarGz creates a gzipped tar archive of a directory
func createTarGz(sourceDir, targetPath string) error {
	// Create target file
	file, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create gzip writer
	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Get base directory name
	baseDir := filepath.Base(sourceDir)

	// Walk directory and add files to tar
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Calculate relative path from source directory
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Set header name to include base directory
		header.Name = filepath.Join(baseDir, relPath)

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// If it's a directory, we're done
		if info.IsDir() {
			return nil
		}

		// Write file content
		file, err := os.Open(path) //nolint:gosec // G122: trusted local paths, symlink TOCTOU not in threat model
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})
}

// extractTarGz extracts a gzipped tar archive to a target directory
func extractTarGz(archivePath, targetDir string) error {
	// Open archive file
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzReader)

	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return err
		}

		// Check for directory traversal (zip slip) before joining
		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("invalid file path in archive: %s", header.Name)
		}
		// Calculate target path
		targetPath := filepath.Join(targetDir, header.Name) //nolint:gosec // G305: header.Name validated above
		if !strings.HasPrefix(targetPath, filepath.Clean(targetDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in archive: %s", header.Name)
		}

		// Mask to permission bits only — tar header mode is int64, but
		// only the lower 12 bits are meaningful for os.FileMode here.
		mode := os.FileMode(header.Mode & 0o7777) //nolint:gosec // masked to perm bits
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(targetPath, mode); err != nil {
				return err
			}
		case tar.TypeReg:
			// Create parent directories
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}

			// Create file
			outFile, err := os.Create(targetPath)
			if err != nil {
				return err
			}

			// Copy content (limit to 1GB per file to prevent decompression bomb)
			const maxFileSize = 1 << 30 // 1 GiB
			if _, err := io.Copy(outFile, io.LimitReader(tarReader, maxFileSize)); err != nil {
				outFile.Close()
				return err
			}

			// Set permissions
			if err := outFile.Chmod(mode); err != nil {
				outFile.Close()
				return err
			}

			outFile.Close()
		default:
			// Skip other types (symlinks, etc.)
		}
	}

	return nil
}
