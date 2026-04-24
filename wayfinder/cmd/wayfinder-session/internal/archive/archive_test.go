package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	a := New("/tmp/test")
	if a.projectDir != "/tmp/test" {
		t.Errorf("New() projectDir = %q, want %q", a.projectDir, "/tmp/test")
	}
}

func TestArchivePhase(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir)

	// Create mock STATUS and HISTORY files
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\nphase: D1\n"), 0644); err != nil {
		t.Fatalf("failed to create STATUS file: %v", err)
	}

	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.md")
	if err := os.WriteFile(historyPath, []byte("{\"event\":\"test\"}\n"), 0644); err != nil {
		t.Fatalf("failed to create HISTORY file: %v", err)
	}

	// Archive phase
	if err := a.ArchivePhase("D1"); err != nil {
		t.Fatalf("ArchivePhase() error = %v", err)
	}

	// Verify archive directory was created
	archiveBasePath := filepath.Join(tmpDir, ".wayfinder", "archives")
	entries, err := os.ReadDir(archiveBasePath)
	if err != nil {
		t.Fatalf("failed to read archives directory: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 archive, got %d", len(entries))
	}

	// Verify archive contains STATUS file
	archivePath := filepath.Join(archiveBasePath, entries[0].Name())
	archivedStatus := filepath.Join(archivePath, "WAYFINDER-STATUS.md")
	statusData, err := os.ReadFile(archivedStatus)
	if err != nil {
		t.Fatalf("failed to read archived STATUS: %v", err)
	}

	expected := "# Status\nphase: D1\n"
	if string(statusData) != expected {
		t.Errorf("archived STATUS content = %q, want %q", string(statusData), expected)
	}

	// Verify archive contains HISTORY file
	archivedHistory := filepath.Join(archivePath, "WAYFINDER-HISTORY.md")
	historyData, err := os.ReadFile(archivedHistory)
	if err != nil {
		t.Fatalf("failed to read archived HISTORY: %v", err)
	}

	expectedHistory := "{\"event\":\"test\"}\n"
	if string(historyData) != expectedHistory {
		t.Errorf("archived HISTORY content = %q, want %q", string(historyData), expectedHistory)
	}
}

func TestArchivePhase_MissingHistory(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir)

	// Create only STATUS file (no HISTORY)
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to create STATUS file: %v", err)
	}

	// Archive should succeed even without HISTORY file
	if err := a.ArchivePhase("D1"); err != nil {
		t.Fatalf("ArchivePhase() error = %v", err)
	}

	// Verify archive was created
	archiveBasePath := filepath.Join(tmpDir, ".wayfinder", "archives")
	entries, err := os.ReadDir(archiveBasePath)
	if err != nil {
		t.Fatalf("failed to read archives directory: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 archive, got %d", len(entries))
	}
}

func TestArchivePhase_MultipleArchives(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir)

	// Create STATUS file
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to create STATUS file: %v", err)
	}

	// Create multiple archives
	for i := 0; i < 3; i++ {
		if err := a.ArchivePhase("D1"); err != nil {
			t.Fatalf("ArchivePhase() error = %v", err)
		}
		time.Sleep(2 * time.Millisecond) // Ensure different timestamps
	}

	// Verify 3 archives were created
	archives, err := a.ListArchives()
	if err != nil {
		t.Fatalf("ListArchives() error = %v", err)
	}

	if len(archives) != 3 {
		t.Errorf("ListArchives() returned %d archives, want 3", len(archives))
	}
}

func TestListArchives_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir)

	// List archives before creating any
	archives, err := a.ListArchives()
	if err != nil {
		t.Fatalf("ListArchives() error = %v", err)
	}

	if len(archives) != 0 {
		t.Errorf("ListArchives() returned %d archives, want 0", len(archives))
	}
}

func TestListArchives(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir)

	// Create STATUS file
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if err := os.WriteFile(statusPath, []byte("# Status\n"), 0644); err != nil {
		t.Fatalf("failed to create STATUS file: %v", err)
	}

	// Create archive
	if err := a.ArchivePhase("D1"); err != nil {
		t.Fatalf("ArchivePhase() error = %v", err)
	}

	// List archives
	archives, err := a.ListArchives()
	if err != nil {
		t.Fatalf("ListArchives() error = %v", err)
	}

	if len(archives) != 1 {
		t.Fatalf("ListArchives() returned %d archives, want 1", len(archives))
	}

	archive := archives[0]

	// Verify archive has name
	if archive.Name == "" {
		t.Error("archive.Name is empty")
	}

	// Verify archive name contains phase
	if archive.Name[:2] != "D1" {
		t.Errorf("archive.Name = %q, want to start with D1", archive.Name)
	}

	// Verify archive has timestamp
	if archive.Timestamp.IsZero() {
		t.Error("archive.Timestamp is zero")
	}

	// Verify archive has path
	if archive.Path == "" {
		t.Error("archive.Path is empty")
	}

	// Verify path exists
	if _, err := os.Stat(archive.Path); os.IsNotExist(err) {
		t.Errorf("archive.Path does not exist: %s", archive.Path)
	}
}

func TestArchivePhase_MissingStatusFile(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir)

	// Try to archive without STATUS file
	err := a.ArchivePhase("D1")
	if err == nil {
		t.Error("ArchivePhase() should error when STATUS file is missing")
	}
}
