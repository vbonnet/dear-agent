package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateBackup(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "manifest.yaml")

	// Create source file
	content := []byte("test content")
	if err := os.WriteFile(sourcePath, content, 0600); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Create first backup
	num1, err := CreateBackup(sourcePath)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if num1 != 1 {
		t.Errorf("first backup number = %d, want 1", num1)
	}

	// Verify backup file exists
	backupPath := filepath.Join(tmpDir, "manifest.yaml.1")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Errorf("backup file not created: %s", backupPath)
	}

	// Verify backup content
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup: %v", err)
	}

	if string(backupContent) != string(content) {
		t.Errorf("backup content = %q, want %q", backupContent, content)
	}

	// Create second backup
	num2, err := CreateBackup(sourcePath)
	if err != nil {
		t.Fatalf("CreateBackup (second) failed: %v", err)
	}

	if num2 != 2 {
		t.Errorf("second backup number = %d, want 2", num2)
	}
}

func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "manifest.yaml")

	// Create source file
	if err := os.WriteFile(sourcePath, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Initially no backups
	backups, err := ListBackups(sourcePath)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != 0 {
		t.Errorf("initial backups count = %d, want 0", len(backups))
	}

	// Create backups out of order
	for _, num := range []int{3, 1, 5, 2, 4} {
		backupPath := filepath.Join(tmpDir, fmt.Sprintf("manifest.yaml.%d", num))
		if err := os.WriteFile(backupPath, []byte("backup"), 0600); err != nil {
			t.Fatalf("failed to create backup %d: %v", num, err)
		}
	}

	// List should return sorted
	backups, err = ListBackups(sourcePath)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	expected := []int{1, 2, 3, 4, 5}
	if len(backups) != len(expected) {
		t.Fatalf("backups count = %d, want %d", len(backups), len(expected))
	}

	for i, num := range backups {
		if num != expected[i] {
			t.Errorf("backup[%d] = %d, want %d", i, num, expected[i])
		}
	}
}

func TestRotateBackups(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "manifest.yaml")

	// Create source file
	if err := os.WriteFile(sourcePath, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Create 15 backups
	for i := 1; i <= 15; i++ {
		backupPath := filepath.Join(tmpDir, fmt.Sprintf("manifest.yaml.%d", i))
		if err := os.WriteFile(backupPath, []byte("backup"), 0600); err != nil {
			t.Fatalf("failed to create backup %d: %v", i, err)
		}
	}

	// Rotate to keep max 10
	if err := RotateBackups(sourcePath, MaxBackups); err != nil {
		t.Fatalf("RotateBackups failed: %v", err)
	}

	// Verify only 10 backups remain
	backups, err := ListBackups(sourcePath)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != MaxBackups {
		t.Errorf("after rotation, backups count = %d, want %d", len(backups), MaxBackups)
	}

	// Verify oldest (1-5) were deleted, newest (6-15) remain
	expected := []int{6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	for i, num := range backups {
		if num != expected[i] {
			t.Errorf("backup[%d] = %d, want %d", i, num, expected[i])
		}
	}
}

func TestRestoreBackup(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "manifest.yaml")

	// Create source file
	originalContent := []byte("original content")
	if err := os.WriteFile(sourcePath, originalContent, 0600); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Create backup
	backupNum, err := CreateBackup(sourcePath)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Modify source
	modifiedContent := []byte("modified content")
	if err := os.WriteFile(sourcePath, modifiedContent, 0600); err != nil {
		t.Fatalf("failed to modify source: %v", err)
	}

	// Restore backup
	if err := RestoreBackup(sourcePath, backupNum); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}

	// Verify content restored
	restoredContent, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}

	if string(restoredContent) != string(originalContent) {
		t.Errorf("restored content = %q, want %q", restoredContent, originalContent)
	}

	// Verify backup of modified state was created
	backups, err := ListBackups(sourcePath)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	// Should have 2 backups now (original + backup of modified before restore)
	if len(backups) < 2 {
		t.Errorf("backups count after restore = %d, want >= 2", len(backups))
	}
}

func TestRestoreBackup_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "manifest.yaml")

	// Try to restore non-existent backup
	err := RestoreBackup(sourcePath, 999)
	if err == nil {
		t.Error("RestoreBackup should fail for non-existent backup")
	}

	if !os.IsNotExist(err) && err.Error() != "backup not found: "+sourcePath+".999" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCreateBackup_SequentialNumbering(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "manifest.yaml")

	// Create source file
	if err := os.WriteFile(sourcePath, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Create 5 backups
	for i := 1; i <= 5; i++ {
		num, err := CreateBackup(sourcePath)
		if err != nil {
			t.Fatalf("CreateBackup %d failed: %v", i, err)
		}

		if num != i {
			t.Errorf("backup %d number = %d, want %d", i, num, i)
		}
	}

	// Verify all 5 backups exist
	backups, err := ListBackups(sourcePath)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != 5 {
		t.Errorf("backups count = %d, want 5", len(backups))
	}
}

func TestBackupPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "manifest.yaml")

	// Create source file
	if err := os.WriteFile(sourcePath, []byte("test"), 0600); err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Create backup
	num, err := CreateBackup(sourcePath)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Check backup file permissions
	backupPath := filepath.Join(tmpDir, fmt.Sprintf("manifest.yaml.%d", num))
	info, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("failed to stat backup: %v", err)
	}

	mode := info.Mode()
	expectedPerm := os.FileMode(0600)

	if mode.Perm() != expectedPerm {
		t.Errorf("backup permissions = %o, want %o", mode.Perm(), expectedPerm)
	}
}
