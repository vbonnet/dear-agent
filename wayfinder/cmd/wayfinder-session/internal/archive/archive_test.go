package archive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/retrospective"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
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
	if err := os.WriteFile(statusPath, []byte("# Status\nphase: PROBLEM\n"), 0644); err != nil {
		t.Fatalf("failed to create STATUS file: %v", err)
	}

	historyPath := filepath.Join(tmpDir, "WAYFINDER-HISTORY.jsonl")
	if err := os.WriteFile(historyPath, []byte("{\"event\":\"test\"}\n"), 0644); err != nil {
		t.Fatalf("failed to create HISTORY file: %v", err)
	}

	retroContent := "# Retrospective\n\nPrior evidence.\n"
	if err := os.WriteFile(filepath.Join(tmpDir, retrospective.RetroFilename), []byte(retroContent), 0o644); err != nil {
		t.Fatalf("failed to create RETRO file: %v", err)
	}

	// Archive phase
	ref, err := a.ArchivePhase("PROBLEM")
	if err != nil {
		t.Fatalf("ArchivePhase() error = %v", err)
	}
	if got := ref.RelativePath(); !strings.HasPrefix(got, ".wayfinder/archives/PROBLEM-") {
		t.Fatalf("ArchivePhase() ref = %q, want project-relative PROBLEM archive", got)
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

	expected := "# Status\nphase: PROBLEM\n"
	if string(statusData) != expected {
		t.Errorf("archived STATUS content = %q, want %q", string(statusData), expected)
	}

	// Verify archive contains HISTORY file
	archivedHistory := filepath.Join(archivePath, "WAYFINDER-HISTORY.jsonl")
	historyData, err := os.ReadFile(archivedHistory)
	if err != nil {
		t.Fatalf("failed to read archived HISTORY: %v", err)
	}

	expectedHistory := "{\"event\":\"test\"}\n"
	if string(historyData) != expectedHistory {
		t.Errorf("archived HISTORY content = %q, want %q", string(historyData), expectedHistory)
	}

	archivedRetro := filepath.Join(archivePath, retrospective.RetroFilename)
	retroData, err := os.ReadFile(archivedRetro)
	if err != nil {
		t.Fatalf("failed to read archived RETRO: %v", err)
	}
	if string(retroData) != retroContent {
		t.Errorf("archived RETRO content = %q, want %q", string(retroData), retroContent)
	}
}

func TestArchivePhaseMigratesLegacyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, status.StatusFilename), []byte("status"), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
	legacyPath := filepath.Join(tmpDir, history.LegacyHistoryFilename)
	if err := os.WriteFile(legacyPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write legacy history: %v", err)
	}

	if _, err := New(tmpDir).ArchivePhase("BUILD"); err != nil {
		t.Fatalf("ArchivePhase() error: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy history stat error = %v, want not exists", err)
	}
	archives, err := filepath.Glob(filepath.Join(tmpDir, ".wayfinder", "archives", "BUILD-*", history.HistoryFilename))
	if err != nil || len(archives) != 1 {
		t.Fatalf("archived histories = %v, %v; want one", archives, err)
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
	if _, err := a.ArchivePhase("PROBLEM"); err != nil {
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
		if _, err := a.ArchivePhase("PROBLEM"); err != nil {
			t.Fatalf("ArchivePhase() error = %v", err)
		}
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

func TestArchivePhaseRejectsNonCanonicalPhase(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, status.StatusFilename), []byte("status"), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}

	if _, err := New(tmpDir).ArchivePhase("../BUILD"); err == nil || !strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("ArchivePhase() error = %v, want non-canonical phase rejection", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".wayfinder")); !os.IsNotExist(err) {
		t.Fatalf("invalid phase created archive state: %v", err)
	}
}

func TestArchivePhaseRejectsSymlinkArchiveRoot(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, status.StatusFilename), []byte("status"), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
	wayfinderDir := filepath.Join(tmpDir, ".wayfinder")
	if err := os.Mkdir(wayfinderDir, 0o700); err != nil {
		t.Fatalf("create Wayfinder directory: %v", err)
	}
	externalDir := t.TempDir()
	if err := os.Symlink(externalDir, filepath.Join(wayfinderDir, "archives")); err != nil {
		t.Fatalf("create archive-root symlink: %v", err)
	}

	if _, err := New(tmpDir).ListArchives(); err == nil || !strings.Contains(err.Error(), "symbolic-link") {
		t.Fatalf("ListArchives() error = %v, want symlink rejection", err)
	}
	if _, err := New(tmpDir).ArchivePhase("BUILD"); err == nil || !strings.Contains(err.Error(), "symbolic-link") {
		t.Fatalf("ArchivePhase() error = %v, want symlink rejection", err)
	}
	entries, err := os.ReadDir(externalDir)
	if err != nil {
		t.Fatalf("read external directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("archive escaped through symlink: %v", entries)
	}
}

func TestArchivePhaseRejectsNonRegularRetrospective(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, status.StatusFilename), []byte("status"), 0o600); err != nil {
		t.Fatalf("write status: %v", err)
	}
	if err := os.Mkdir(filepath.Join(tmpDir, retrospective.RetroFilename), 0o700); err != nil {
		t.Fatalf("create invalid RETRO directory: %v", err)
	}

	if _, err := New(tmpDir).ArchivePhase("RETRO"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ArchivePhase() error = %v, want non-regular RETRO rejection", err)
	}
	entries, err := os.ReadDir(filepath.Join(tmpDir, ".wayfinder", "archives"))
	if err != nil {
		t.Fatalf("read archive root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed archive left published or temporary state: %v", entries)
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
	if _, err := a.ArchivePhase("PROBLEM"); err != nil {
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
	if !strings.HasPrefix(archive.Name, "PROBLEM") {
		t.Errorf("archive.Name = %q, want to start with PROBLEM", archive.Name)
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
	_, err := a.ArchivePhase("PROBLEM")
	if err == nil {
		t.Error("ArchivePhase() should error when STATUS file is missing")
	}
	archiveBase := filepath.Join(tmpDir, ".wayfinder", "archives")
	entries, readErr := os.ReadDir(archiveBase)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read archive root: %v", readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			t.Errorf("failed archive left temporary residue %q", entry.Name())
		}
	}
}
