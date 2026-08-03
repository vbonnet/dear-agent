// Package archive provides archive-related functionality.
package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/retrospective"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// ArchiveManager handles archiving phases before rewind
type ArchiveManager struct {
	projectDir string
}

// ArchiveRef identifies one complete archive relative to its project. It is
// returned only after the archive directory has been fully published.
type ArchiveRef struct {
	relativePath string
}

// RelativePath returns the archive path relative to the Wayfinder project.
func (r ArchiveRef) RelativePath() string {
	return r.relativePath
}

// New creates a new ArchiveManager for the given project directory
func New(projectDir string) *ArchiveManager {
	return &ArchiveManager{
		projectDir: projectDir,
	}
}

// ArchivePhase creates a snapshot of the current phase state before rewinding.
// It publishes the archive only after all required files are copied, returning
// a project-relative reference suitable for a scoped Git commit.
func (a *ArchiveManager) ArchivePhase(phaseName string) (ArchiveRef, error) {
	if !slices.Contains(status.AllWaypointsV2Schema(), phaseName) {
		return ArchiveRef{}, fmt.Errorf("archive phase %q is not canonical", phaseName)
	}
	wayfinderDir := filepath.Join(a.projectDir, ".wayfinder")
	if err := ensureOwnedDirectory(wayfinderDir); err != nil {
		return ArchiveRef{}, fmt.Errorf("prepare Wayfinder directory: %w", err)
	}
	archiveBase := filepath.Join(a.projectDir, ".wayfinder", "archives")
	if err := ensureOwnedDirectory(archiveBase); err != nil {
		return ArchiveRef{}, fmt.Errorf("create archive root: %w", err)
	}
	temporaryPrefix := ".tmp-" + phaseName + "-"
	temporaryDir, err := os.MkdirTemp(archiveBase, temporaryPrefix)
	if err != nil {
		return ArchiveRef{}, fmt.Errorf("create temporary archive directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(temporaryDir) }()

	timestamp := time.Now().Format("20060102-150405.000")
	randomSuffix := strings.TrimPrefix(filepath.Base(temporaryDir), temporaryPrefix)
	if randomSuffix == "" || randomSuffix == filepath.Base(temporaryDir) {
		return ArchiveRef{}, fmt.Errorf("derive unique archive suffix")
	}
	archiveName := phaseName + "-" + timestamp + "-" + randomSuffix
	archiveDir := filepath.Join(archiveBase, archiveName)

	// Archive STATUS file
	statusSrc := filepath.Join(a.projectDir, status.StatusFilename)
	statusDst := filepath.Join(temporaryDir, status.StatusFilename)
	if err := copyRequiredRegularFile(statusSrc, statusDst); err != nil {
		return ArchiveRef{}, fmt.Errorf("archive STATUS file: %w", err)
	}

	// Archive HISTORY file if it exists
	historyLog := history.New(a.projectDir)
	if err := historyLog.EnsureCurrentFile(); err != nil {
		return ArchiveRef{}, fmt.Errorf("migrate HISTORY file: %w", err)
	}
	historySrc := filepath.Join(a.projectDir, history.HistoryFilename)
	historyDst := filepath.Join(temporaryDir, history.HistoryFilename)
	if err := copyOptionalRegularFile(historySrc, historyDst); err != nil {
		return ArchiveRef{}, fmt.Errorf("archive HISTORY file: %w", err)
	}

	// Archive the existing retrospective when present. Besides preserving the
	// pre-rewind trace, this rejects directories and symlinks before status is
	// mutated, rather than discovering an invalid marker during the late append.
	retroSrc := filepath.Join(a.projectDir, retrospective.RetroFilename)
	retroDst := filepath.Join(temporaryDir, retrospective.RetroFilename)
	if err := copyOptionalRegularFile(retroSrc, retroDst); err != nil {
		return ArchiveRef{}, fmt.Errorf("archive RETRO file: %w", err)
	}

	if err := os.Rename(temporaryDir, archiveDir); err != nil {
		return ArchiveRef{}, fmt.Errorf("publish archive: %w", err)
	}
	relativePath, err := filepath.Rel(a.projectDir, archiveDir)
	if err != nil {
		return ArchiveRef{}, fmt.Errorf("make archive path relative: %w", err)
	}
	return ArchiveRef{relativePath: filepath.ToSlash(relativePath)}, nil
}

// ListArchives returns all archived phase snapshots
func (a *ArchiveManager) ListArchives() ([]ArchiveInfo, error) {
	wayfinderDir := filepath.Join(a.projectDir, ".wayfinder")
	exists, err := inspectOwnedDirectory(wayfinderDir)
	if err != nil {
		return nil, fmt.Errorf("inspect Wayfinder directory: %w", err)
	}
	if !exists {
		return []ArchiveInfo{}, nil
	}
	archiveBasePath := filepath.Join(a.projectDir, ".wayfinder", "archives")
	exists, err = inspectOwnedDirectory(archiveBasePath)
	if err != nil {
		return nil, fmt.Errorf("inspect archive root: %w", err)
	}
	if !exists {
		return []ArchiveInfo{}, nil
	}

	entries, err := os.ReadDir(archiveBasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read archives directory: %w", err)
	}

	var archives []ArchiveInfo
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't stat
		}

		archives = append(archives, ArchiveInfo{
			Name:      entry.Name(),
			Timestamp: info.ModTime(),
			Path:      filepath.Join(archiveBasePath, entry.Name()),
		})
	}

	return archives, nil
}

func ensureOwnedDirectory(path string) error {
	exists, err := inspectOwnedDirectory(path)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	exists, err = inspectOwnedDirectory(path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("directory was not created: %s", path)
	}
	return nil
}

func inspectOwnedDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("refuse symbolic-link directory %s", path)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("path is not a directory: %s", path)
	}
	return true, nil
}

func copyRequiredRegularFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}
	return copyFile(src, dst)
}

func copyOptionalRegularFile(src, dst string) error {
	info, err := os.Lstat(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}
	return copyFile(src, dst)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0o600)
}

// ArchiveInfo contains metadata about an archived phase
type ArchiveInfo struct {
	Name      string
	Timestamp time.Time
	Path      string
}
