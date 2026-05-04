// Package artifacts manages artifact storage and memory pointers for A2A channels.
package artifacts

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultArtifactsDir is the default filesystem location for A2A artifacts.
const DefaultArtifactsDir = "~/src/a2a-artifacts"

// Artifact represents stored artifact metadata
type Artifact struct {
	Name string
	Size int64
}

// Manager manages artifact storage and memory pointer generation
type Manager struct {
	baseDir string
}

// NewManager creates a new artifact manager
func NewManager(baseDir string) (*Manager, error) {
	if baseDir == "" {
		baseDir = expandPath(DefaultArtifactsDir)
	}
	err := os.MkdirAll(baseDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("create base directory: %w", err)
	}
	return &Manager{baseDir: baseDir}, nil
}

// StoreArtifact stores an artifact in the channel directory
func (m *Manager) StoreArtifact(channelID string, artifactPath string, description string) (string, error) {
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		return "", fmt.Errorf("artifact not found: %s", artifactPath)
	}
	channelDir := filepath.Join(m.baseDir, channelID)
	err := os.MkdirAll(channelDir, 0o700)
	if err != nil {
		return "", fmt.Errorf("create channel directory: %w", err)
	}
	artifactName := filepath.Base(artifactPath)
	destPath := filepath.Join(channelDir, artifactName)
	err = copyFile(artifactPath, destPath)
	if err != nil {
		return "", fmt.Errorf("copy artifact: %w", err)
	}
	err = m.updateIndex(channelID, artifactName, description)
	if err != nil {
		return "", fmt.Errorf("update index: %w", err)
	}
	return destPath, nil
}

// GeneratePointer generates a memory pointer markdown for an artifact
func (m *Manager) GeneratePointer(channelID, artifactName, summary string, keyPoints []string) string {
	artifactPath := filepath.Join(m.baseDir, channelID, artifactName)
	var builder strings.Builder
	fmt.Fprintf(&builder, "See artifact: %s\n", artifactPath)
	if summary != "" {
		fmt.Fprintf(&builder, "\n**Summary**: %s\n", summary)
	}
	if len(keyPoints) > 0 {
		builder.WriteString("\n**Key points**:\n")
		for _, point := range keyPoints {
			fmt.Fprintf(&builder, "- %s\n", point)
		}
	}
	return builder.String()
}

// ListArtifacts lists all artifacts for a channel
func (m *Manager) ListArtifacts(channelID string) ([]Artifact, error) {
	channelDir := filepath.Join(m.baseDir, channelID)
	if _, err := os.Stat(channelDir); os.IsNotExist(err) {
		return []Artifact{}, nil
	}
	entries, err := os.ReadDir(channelDir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	var artifacts []Artifact
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "INDEX.md" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			artifacts = append(artifacts, Artifact{
				Name: entry.Name(),
				Size: info.Size(),
			})
		}
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].Name < artifacts[j].Name
	})
	return artifacts, nil
}

// GetArtifactPath gets the full path to an artifact if it exists
func (m *Manager) GetArtifactPath(channelID, artifactName string) (string, bool) {
	artifactPath := filepath.Join(m.baseDir, channelID, artifactName)
	if _, err := os.Stat(artifactPath); err == nil {
		return artifactPath, true
	}
	return "", false
}

func (m *Manager) updateIndex(channelID, artifactName, description string) error {
	indexFile := filepath.Join(m.baseDir, "INDEX.md")
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		content := "# A2A Artifact Index\n\nGlobal index of all stored artifacts.\n\n"
		err = os.WriteFile(indexFile, []byte(content), 0o600)
		if err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}
	timestamp := time.Now().Format("2006-01-02 15:04")
	entry := fmt.Sprintf("- **%s/%s**", channelID, artifactName)
	if description != "" {
		entry += fmt.Sprintf(" - %s", description)
	}
	entry += fmt.Sprintf(" (added %s)\n", timestamp)
	file, err := os.OpenFile(indexFile, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer file.Close()
	_, err = file.WriteString(entry)
	if err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}
	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}
	return os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())
}

func expandPath(path string) string {
	expanded := os.ExpandEnv(path)
	if len(expanded) > 0 && expanded[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			if len(expanded) == 1 {
				return home
			}
			return filepath.Join(home, expanded[1:])
		}
	}
	return expanded
}

// FormatSize formats file size in human-readable format
func FormatSize(sizeBytes int64) string {
	size := float64(sizeBytes)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	for _, unit := range units {
		if size < 1024.0 {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
		size /= 1024.0
	}
	return fmt.Sprintf("%.1f PB", size)
}

// ParseKeyPoints parses key points from a string (comma-separated)
func ParseKeyPoints(keyPointsStr string) []string {
	if keyPointsStr == "" {
		return nil
	}
	points := strings.Split(keyPointsStr, ",")
	var result []string
	for _, point := range points {
		trimmed := strings.TrimSpace(point)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
