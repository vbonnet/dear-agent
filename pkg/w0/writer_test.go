package w0

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Helper functions to reduce test complexity

func verifyCharterFile(t *testing.T, path, expectedContent string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read charter file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "---") {
		t.Error("expected frontmatter in content")
	}
	if !strings.Contains(content, expectedContent) {
		t.Errorf("expected %q in file", expectedContent)
	}
}

func verifyFrontmatterField(t *testing.T, path, field, value string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read charter file: %v", err)
	}

	content := string(data)
	expected := field + ": " + value
	if !strings.Contains(content, expected) {
		t.Errorf("expected %q in frontmatter", expected)
	}
}

func findBackupFile(t *testing.T, dir string) string {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), "W0-project-charter.backup-") {
			return filepath.Join(dir, file.Name())
		}
	}
	return ""
}

func verifyNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".tmp") {
			t.Error("temporary file should not remain after successful write")
		}
	}
}

func TestSaveCharter(t *testing.T) {
	t.Run("saves charter with default metadata", func(t *testing.T) {
		tmpDir := t.TempDir()
		charter := "# Project Charter\n\nThis is a test charter."

		result := SaveCharter(tmpDir, charter, nil)

		if !result.Success {
			t.Fatalf("expected success, got error: %v", result.Error)
		}

		expectedPath := filepath.Join(tmpDir, CharterFilename)
		if result.Path != expectedPath {
			t.Errorf("expected path %s, got %s", expectedPath, result.Path)
		}

		verifyCharterFile(t, result.Path, charter)
		verifyFrontmatterField(t, result.Path, "status", "approved")
	})

	t.Run("saves charter with custom metadata", func(t *testing.T) {
		tmpDir := t.TempDir()
		charter := "# Custom Charter"
		version := "1.0.0"
		metadata := &Metadata{
			Created: "2024-01-15",
			Status:  StatusDraft,
			Version: &version,
		}

		result := SaveCharter(tmpDir, charter, metadata)

		if !result.Success {
			t.Fatalf("expected success, got error: %v", result.Error)
		}

		verifyFrontmatterField(t, result.Path, "created", "2024-01-15")
		verifyFrontmatterField(t, result.Path, "status", "draft")
		verifyFrontmatterField(t, result.Path, "version", "1.0.0")
	})

	t.Run("creates backup of existing charter", func(t *testing.T) {
		tmpDir := t.TempDir()
		charter1 := "# Original Charter"
		charter2 := "# Updated Charter"

		// Write first charter
		result1 := SaveCharter(tmpDir, charter1, nil)
		if !result1.Success {
			t.Fatalf("failed to save first charter: %v", result1.Error)
		}

		// Wait a bit to ensure different timestamp
		time.Sleep(10 * time.Millisecond)

		// Write second charter (should create backup)
		result2 := SaveCharter(tmpDir, charter2, nil)
		if !result2.Success {
			t.Fatalf("failed to save second charter: %v", result2.Error)
		}

		// Check for backup file
		backupPath := findBackupFile(t, tmpDir)
		if backupPath == "" {
			t.Fatal("expected backup file to be created")
		}

		// Verify backup contains original content
		verifyCharterFile(t, backupPath, charter1)

		// Verify current file has new content
		verifyCharterFile(t, result2.Path, charter2)
	})

	t.Run("returns error for empty project path", func(t *testing.T) {
		result := SaveCharter("", "charter content", nil)

		if result.Success {
			t.Error("expected failure for empty project path")
		}
		if result.Error == nil {
			t.Error("expected error to be set")
		}
		if !strings.Contains(result.Error.Error(), "project path is required") {
			t.Errorf("unexpected error message: %v", result.Error)
		}
	})

	t.Run("returns error for empty charter content", func(t *testing.T) {
		tmpDir := t.TempDir()
		result := SaveCharter(tmpDir, "", nil)

		if result.Success {
			t.Error("expected failure for empty charter content")
		}
		if result.Error == nil {
			t.Error("expected error to be set")
		}
		if !strings.Contains(result.Error.Error(), "charter content is required") {
			t.Errorf("unexpected error message: %v", result.Error)
		}
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		result := SaveCharter("/nonexistent/path/to/project", "charter", nil)

		if result.Success {
			t.Error("expected failure for non-existent directory")
		}
		if result.Error == nil {
			t.Error("expected error to be set")
		}
		if !strings.Contains(result.Error.Error(), "does not exist") {
			t.Errorf("unexpected error message: %v", result.Error)
		}
	})

	t.Run("returns error for non-writable directory", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("skipping test when running as root")
		}

		tmpDir := t.TempDir()
		// Make directory read-only
		// #nosec G302 -- intentionally read-only for test
		if err := os.Chmod(tmpDir, 0o555); err != nil {
			t.Fatalf("failed to chmod directory: %v", err)
		}
		defer func() { _ = os.Chmod(tmpDir, 0o755) }() // Restore for cleanup

		result := SaveCharter(tmpDir, "charter", nil)

		if result.Success {
			t.Error("expected failure for non-writable directory")
		}
		if result.Error == nil {
			t.Error("expected error to be set")
		}
	})

	t.Run("atomic write using temp file", func(t *testing.T) {
		tmpDir := t.TempDir()
		charter := "# Test Charter"

		result := SaveCharter(tmpDir, charter, nil)

		if !result.Success {
			t.Fatalf("expected success, got error: %v", result.Error)
		}

		verifyNoTempFiles(t, tmpDir)
	})
}

func TestFormatCharterWithFrontmatter(t *testing.T) {
	t.Run("formats with basic metadata", func(t *testing.T) {
		charter := "# Charter Content"
		metadata := &Metadata{
			Created: "2024-01-15",
			Status:  StatusApproved,
		}

		result := FormatCharterWithFrontmatter(charter, metadata)

		expected := `---
created: 2024-01-15
status: approved
---

# Charter Content
`
		if result != expected {
			t.Errorf("expected:\n%s\n\ngot:\n%s", expected, result)
		}
	})

	t.Run("formats with version", func(t *testing.T) {
		charter := "# Charter"
		version := "2.0.0"
		metadata := &Metadata{
			Created: "2024-01-15",
			Status:  StatusRevised,
			Version: &version,
		}

		result := FormatCharterWithFrontmatter(charter, metadata)

		if !strings.Contains(result, "version: 2.0.0") {
			t.Error("expected version in output")
		}
	})

	t.Run("trims charter content", func(t *testing.T) {
		charter := "\n\n  # Charter  \n\n"
		metadata := &Metadata{
			Created: "2024-01-15",
			Status:  StatusDraft,
		}

		result := FormatCharterWithFrontmatter(charter, metadata)

		if !strings.Contains(result, "# Charter\n") {
			t.Error("expected trimmed charter content")
		}
		if strings.Contains(result, "  # Charter  ") {
			t.Error("charter should be trimmed")
		}
	})
}

func TestGenerateFrontmatter(t *testing.T) {
	t.Run("generates basic frontmatter", func(t *testing.T) {
		metadata := &Metadata{
			Created: "2024-01-15",
			Status:  StatusApproved,
		}

		result := GenerateFrontmatter(metadata)

		expected := `---
created: 2024-01-15
status: approved
---`
		if result != expected {
			t.Errorf("expected:\n%s\n\ngot:\n%s", expected, result)
		}
	})

	t.Run("includes version when present", func(t *testing.T) {
		version := "1.5.2"
		metadata := &Metadata{
			Created: "2024-01-15",
			Status:  StatusDraft,
			Version: &version,
		}

		result := GenerateFrontmatter(metadata)

		expected := `---
created: 2024-01-15
status: draft
version: 1.5.2
---`
		if result != expected {
			t.Errorf("expected:\n%s\n\ngot:\n%s", expected, result)
		}
	})
}

func TestReadCharter(t *testing.T) {
	t.Run("reads existing charter", func(t *testing.T) {
		tmpDir := t.TempDir()
		charter := "# Test Charter"
		version := "1.0"
		metadata := &Metadata{
			Created: "2024-01-15",
			Status:  StatusApproved,
			Version: &version,
		}

		// Write charter first
		writeResult := SaveCharter(tmpDir, charter, metadata)
		if !writeResult.Success {
			t.Fatalf("failed to write charter: %v", writeResult.Error)
		}

		// Read it back
		readResult := ReadCharter(tmpDir)

		if !readResult.Success {
			t.Fatalf("expected success, got error: %v", readResult.Error)
		}

		if readResult.Content != charter {
			t.Errorf("expected content %q, got %q", charter, readResult.Content)
		}

		if readResult.Metadata.Created != "2024-01-15" {
			t.Errorf("expected created date 2024-01-15, got %s", readResult.Metadata.Created)
		}

		if readResult.Metadata.Status != StatusApproved {
			t.Errorf("expected status approved, got %s", readResult.Metadata.Status)
		}

		if readResult.Metadata.Version == nil || *readResult.Metadata.Version != "1.0" {
			t.Error("expected version 1.0")
		}
	})

	t.Run("returns error for non-existent charter", func(t *testing.T) {
		tmpDir := t.TempDir()

		result := ReadCharter(tmpDir)

		if result.Success {
			t.Error("expected failure for non-existent charter")
		}
		if result.Error == nil {
			t.Error("expected error to be set")
		}
		if !strings.Contains(result.Error.Error(), "does not exist") {
			t.Errorf("unexpected error message: %v", result.Error)
		}
	})
}

func TestParseCharterWithFrontmatter(t *testing.T) {
	t.Run("parses valid frontmatter", func(t *testing.T) {
		content := `---
created: 2024-01-15
status: approved
version: 1.0
---

# Charter Content

This is the charter body.`

		charter, metadata := ParseCharterWithFrontmatter(content)

		verifyParsedCharter(t, charter, "# Charter Content")
		verifyParsedMetadata(t, metadata, "2024-01-15", StatusApproved, "1.0")
	})

	t.Run("handles missing frontmatter", func(t *testing.T) {
		content := "# Just a charter\n\nNo frontmatter here."
		charter, metadata := ParseCharterWithFrontmatter(content)

		if charter != content {
			t.Error("expected full content returned when no frontmatter")
		}

		if metadata.Status != StatusDraft {
			t.Errorf("expected default status draft, got %s", metadata.Status)
		}

		today := time.Now().Format("2006-01-02")
		if metadata.Created != today {
			t.Errorf("expected today's date %s, got %s", today, metadata.Created)
		}
	})

	t.Run("handles malformed frontmatter", func(t *testing.T) {
		content := `---
created: 2024-01-15
status: approved
# Missing closing marker

Charter content here.`

		charter, metadata := ParseCharterWithFrontmatter(content)

		if charter != content {
			t.Error("expected full content returned for malformed frontmatter")
		}

		if metadata.Status != StatusDraft {
			t.Error("expected default status for malformed frontmatter")
		}
	})

	t.Run("parses frontmatter without version", func(t *testing.T) {
		content := `---
created: 2024-02-01
status: revised
---

Charter body.`

		charter, metadata := ParseCharterWithFrontmatter(content)

		verifyParsedCharter(t, charter, "Charter body.")
		verifyParsedMetadataNoVersion(t, metadata, "2024-02-01", StatusRevised)
	})
}

func verifyParsedCharter(t *testing.T, charter, expectedContent string) {
	t.Helper()
	if !strings.Contains(charter, expectedContent) {
		t.Errorf("expected charter to contain %q", expectedContent)
	}
	if strings.Contains(charter, "---") {
		t.Error("charter should not contain frontmatter markers")
	}
}

func verifyParsedMetadata(t *testing.T, metadata *Metadata, created string, status Status, version string) {
	t.Helper()
	if metadata.Created != created {
		t.Errorf("expected created %s, got %s", created, metadata.Created)
	}
	if metadata.Status != status {
		t.Errorf("expected status %s, got %s", status, metadata.Status)
	}
	if metadata.Version == nil || *metadata.Version != version {
		t.Errorf("expected version %s, got %v", version, metadata.Version)
	}
}

func verifyParsedMetadataNoVersion(t *testing.T, metadata *Metadata, created string, status Status) {
	t.Helper()
	if metadata.Created != created {
		t.Errorf("expected created %s, got %s", created, metadata.Created)
	}
	if metadata.Status != status {
		t.Errorf("expected status %s, got %s", status, metadata.Status)
	}
	if metadata.Version != nil {
		t.Error("expected version to be nil")
	}
}

func TestCharterExists(t *testing.T) {
	t.Run("returns true when charter exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a charter
		result := SaveCharter(tmpDir, "# Test", nil)
		if !result.Success {
			t.Fatalf("failed to save charter: %v", result.Error)
		}

		if !CharterExists(tmpDir) {
			t.Error("expected CharterExists to return true")
		}
	})

	t.Run("returns false when charter does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()

		if CharterExists(tmpDir) {
			t.Error("expected CharterExists to return false")
		}
	})
}

func TestDeleteCharter(t *testing.T) {
	t.Run("deletes existing charter", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a charter
		writeResult := SaveCharter(tmpDir, "# Test", nil)
		if !writeResult.Success {
			t.Fatalf("failed to save charter: %v", writeResult.Error)
		}

		// Delete it
		deleteResult := DeleteCharter(tmpDir)

		if !deleteResult.Success {
			t.Fatalf("expected success, got error: %v", deleteResult.Error)
		}

		// Verify it's gone
		if CharterExists(tmpDir) {
			t.Error("charter should not exist after deletion")
		}
	})

	t.Run("returns error for non-existent charter", func(t *testing.T) {
		tmpDir := t.TempDir()

		result := DeleteCharter(tmpDir)

		if result.Success {
			t.Error("expected failure for non-existent charter")
		}
		if result.Error == nil {
			t.Error("expected error to be set")
		}
		if !strings.Contains(result.Error.Error(), "does not exist") {
			t.Errorf("unexpected error message: %v", result.Error)
		}
	})
}

func TestCopyFile(t *testing.T) {
	t.Run("copies file successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "source.txt")
		dstPath := filepath.Join(tmpDir, "dest.txt")

		content := "test content"
		// #nosec G306 -- test file, 0644 is appropriate
		if err := os.WriteFile(srcPath, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write source file: %v", err)
		}

		err := copyFile(srcPath, dstPath)
		if err != nil {
			t.Fatalf("copyFile failed: %v", err)
		}

		// Verify destination exists and has same content
		dstData, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("failed to read destination: %v", err)
		}

		if string(dstData) != content {
			t.Errorf("expected %q, got %q", content, string(dstData))
		}
	})

	t.Run("returns error for non-existent source", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := copyFile("/nonexistent/file", filepath.Join(tmpDir, "dest.txt"))

		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})
}
