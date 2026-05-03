package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBackupSession_CreatesValidTarball verifies that BackupSession creates a valid tarball
func TestBackupSession_CreatesValidTarball(t *testing.T) {
	// Setup temp directories
	tmpDir := t.TempDir()
	setupTestSession(t, tmpDir, "test-session-1", "test-backup-session")

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	sessionID := "test-session-1"

	// Create backup
	backupPath, err := BackupSession(sessionID)
	if err != nil {
		t.Fatalf("BackupSession failed: %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatalf("backup file not created: %s", backupPath)
	}

	// Verify backup is a valid gzip file
	file, err := os.Open(backupPath)
	if err != nil {
		t.Fatalf("failed to open backup: %v", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("backup is not a valid gzip file: %v", err)
	}
	defer gzReader.Close()

	// Verify it's a valid tar archive
	tarReader := tar.NewReader(gzReader)
	fileCount := 0
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("invalid tar archive: %v", err)
		}
		fileCount++
		t.Logf("Archived file: %s", header.Name)
	}

	if fileCount == 0 {
		t.Error("backup archive is empty")
	}
}

// TestBackupSession_FilenameFormat verifies the backup filename format
func TestBackupSession_FilenameFormat(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestSession(t, tmpDir, "test-session-2", "my-test-session")

	t.Setenv("HOME", tmpDir)

	sessionID := "test-session-2"

	// Create backup
	backupPath, err := BackupSession(sessionID)
	if err != nil {
		t.Fatalf("BackupSession failed: %v", err)
	}

	// Extract filename
	filename := filepath.Base(backupPath)

	// Verify format: {name}-{timestamp}.tar.gz
	if !strings.HasSuffix(filename, ".tar.gz") {
		t.Errorf("filename doesn't end with .tar.gz: %s", filename)
	}

	// Verify contains session name
	if !strings.HasPrefix(filename, "my-test-session-") {
		t.Errorf("filename doesn't start with session name: %s", filename)
	}

	// Verify timestamp format (YYYYMMdd-HHMMSS)
	parts := strings.Split(strings.TrimSuffix(filename, ".tar.gz"), "-")
	if len(parts) < 3 {
		t.Fatalf("filename doesn't have expected parts: %s", filename)
	}

	// Last two parts should be date and time
	dateStr := parts[len(parts)-2]
	timeStr := parts[len(parts)-1]

	if len(dateStr) != 8 {
		t.Errorf("date part has wrong length: %s", dateStr)
	}

	if len(timeStr) != 6 {
		t.Errorf("time part has wrong length: %s", timeStr)
	}

	// Parse timestamp to verify format
	timestampStr := dateStr + "-" + timeStr
	_, err = time.Parse("20060102-150405", timestampStr)
	if err != nil {
		t.Errorf("timestamp part is not valid: %s", timestampStr)
	}
}

// TestRestoreSession_RestoresCorrectly verifies session restoration
func TestRestoreSession_RestoresCorrectly(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-3"
	setupTestSession(t, tmpDir, sessionID, "restore-test")

	t.Setenv("HOME", tmpDir)

	// Create backup
	backupPath, err := BackupSession(sessionID)
	if err != nil {
		t.Fatalf("BackupSession failed: %v", err)
	}

	// Delete original session
	sessionDir := filepath.Join(tmpDir, ".claude", "sessions", sessionID)
	if err := os.RemoveAll(sessionDir); err != nil {
		t.Fatalf("failed to remove session: %v", err)
	}

	// Verify session is gone
	if _, err := os.Stat(sessionDir); !os.IsNotExist(err) {
		t.Fatal("session directory still exists after removal")
	}

	// Restore session
	if err := RestoreSession(backupPath); err != nil {
		t.Fatalf("RestoreSession failed: %v", err)
	}

	// Verify session directory exists
	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		t.Fatal("session directory not restored")
	}

	// Verify manifest exists
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("manifest.yaml not restored")
	}

	// Verify test file exists
	testFilePath := filepath.Join(sessionDir, "test.txt")
	if _, err := os.Stat(testFilePath); os.IsNotExist(err) {
		t.Fatal("test.txt not restored")
	}

	// Verify test file content
	content, err := os.ReadFile(testFilePath)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	expectedContent := "test content"
	if string(content) != expectedContent {
		t.Errorf("restored content = %q, want %q", string(content), expectedContent)
	}
}

// TestBackupRestore_Roundtrip verifies backup then restore works correctly
func TestBackupRestore_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-4"
	sessionName := "roundtrip-test"
	setupTestSession(t, tmpDir, sessionID, sessionName)

	t.Setenv("HOME", tmpDir)

	// Read original manifest
	sessionDir := filepath.Join(tmpDir, ".claude", "sessions", sessionID)
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	originalManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read original manifest: %v", err)
	}

	// Create backup
	backupPath, err := BackupSession(sessionID)
	if err != nil {
		t.Fatalf("BackupSession failed: %v", err)
	}

	// Delete session
	if err := os.RemoveAll(sessionDir); err != nil {
		t.Fatalf("failed to remove session: %v", err)
	}

	// Restore session
	if err := RestoreSession(backupPath); err != nil {
		t.Fatalf("RestoreSession failed: %v", err)
	}

	// Verify manifest content matches
	restoredManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read restored manifest: %v", err)
	}

	if string(restoredManifest) != string(originalManifest) {
		t.Error("restored manifest content doesn't match original")
	}
}

// TestListSessionBackups_ReturnsCorrectInfo verifies ListSessionBackups returns correct information
func TestListSessionBackups_ReturnsCorrectInfo(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)

	// Initially no backups
	backups, err := ListAllSessionBackups()
	if err != nil {
		t.Fatalf("ListSessionBackups failed: %v", err)
	}

	if len(backups) != 0 {
		t.Errorf("expected 0 backups initially, got %d", len(backups))
	}

	// Create test sessions and backups
	sessions := []struct {
		id   string
		name string
	}{
		{"session-1", "first-session"},
		{"session-2", "second-session"},
		{"session-3", "third-session"},
	}

	for _, s := range sessions {
		setupTestSession(t, tmpDir, s.id, s.name)
		_, err := BackupSession(s.id)
		if err != nil {
			t.Fatalf("BackupSession failed for %s: %v", s.id, err)
		}
		// Sleep briefly to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// List backups
	backups, err = ListAllSessionBackups()
	if err != nil {
		t.Fatalf("ListSessionBackups failed: %v", err)
	}

	if len(backups) != len(sessions) {
		t.Fatalf("expected %d backups, got %d", len(sessions), len(backups))
	}

	// Verify each backup has correct info
	for _, backup := range backups {
		// Verify session name is set
		if backup.SessionName == "" {
			t.Error("backup has empty session name")
		}

		// Verify timestamp is recent (within last minute)
		if time.Since(backup.Timestamp) > time.Minute {
			t.Errorf("backup timestamp is too old: %v", backup.Timestamp)
		}

		// Verify path exists
		if _, err := os.Stat(backup.Path); os.IsNotExist(err) {
			t.Errorf("backup path doesn't exist: %s", backup.Path)
		}

		// Verify size is positive
		if backup.Size <= 0 {
			t.Errorf("backup size is not positive: %d", backup.Size)
		}

		t.Logf("Backup: name=%s, timestamp=%s, path=%s, size=%d",
			backup.SessionName, backup.Timestamp, backup.Path, backup.Size)
	}
}

// TestBackupSession_CreatesDirectory verifies mkdir -p behavior
func TestBackupSession_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	sessionID := "test-session-5"
	setupTestSession(t, tmpDir, sessionID, "mkdir-test")

	t.Setenv("HOME", tmpDir)

	// Verify backup directory doesn't exist yet
	backupDir := filepath.Join(tmpDir, ".agm", "backups", "sessions")
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatal("backup directory already exists")
	}

	// Create backup (should create directory)
	_, err := BackupSession(sessionID)
	if err != nil {
		t.Fatalf("BackupSession failed: %v", err)
	}

	// Verify backup directory was created
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		t.Fatal("backup directory not created")
	}

	// Verify it's a directory
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("failed to stat backup directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("backup path is not a directory")
	}
}

// TestBackupSession_NonExistentSession verifies error handling for missing session
func TestBackupSession_NonExistentSession(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)

	// Try to backup non-existent session
	_, err := BackupSession("non-existent-session")
	if err == nil {
		t.Fatal("BackupSession should fail for non-existent session")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestRestoreSession_NonExistentBackup verifies error handling for missing backup
func TestRestoreSession_NonExistentBackup(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)

	// Try to restore non-existent backup
	err := RestoreSession("/path/to/non-existent-backup.tar.gz")
	if err == nil {
		t.Fatal("RestoreSession should fail for non-existent backup")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestSanitizeFilename verifies filename sanitization
func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple-name", "simple-name"},
		{"name with spaces", "name-with-spaces"},
		{"name/with/slashes", "name-with-slashes"},
		{"name:with:colons", "name-with-colons"},
		{"name*with?special<chars>", "name-with-special-chars"},
		{"name--with--double--dashes", "name-with-double-dashes"},
		{"--leading-and-trailing--", "leading-and-trailing"},
	}

	for _, tt := range tests {
		result := sanitizeFilename(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// setupTestSession creates a test session directory with manifest and test file
func setupTestSession(t *testing.T, homeDir, sessionID, sessionName string) {
	t.Helper()

	// Create session directory
	sessionDir := filepath.Join(homeDir, ".claude", "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session directory: %v", err)
	}

	// Create manifest.yaml
	manifestContent := []byte(`schema_version: "2.0"
session_id: "` + sessionID + `"
name: "` + sessionName + `"
created_at: 2026-03-17T14:00:00Z
updated_at: 2026-03-17T14:00:00Z
lifecycle: ""
context:
  project: "test-project"
claude:
  uuid: "test-claude-uuid"
tmux:
  session_name: "test-tmux-session"
`)

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, manifestContent, 0600); err != nil {
		t.Fatalf("failed to create manifest: %v", err)
	}

	// Create a test file
	testFilePath := filepath.Join(sessionDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Create a subdirectory with a file
	subDir := filepath.Join(sessionDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}

	subFilePath := filepath.Join(subDir, "subfile.txt")
	if err := os.WriteFile(subFilePath, []byte("subfile content"), 0600); err != nil {
		t.Fatalf("failed to create subfile: %v", err)
	}
}
