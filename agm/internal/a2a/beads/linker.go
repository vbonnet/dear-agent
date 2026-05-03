// Package beads provides beads-related functionality.
package beads

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/config"
)

// Linker links A2A channels to bead IDs
type Linker struct {
	channelsDir string
	activeDir   string
}

// NewLinker creates a new bead linker
func NewLinker(channelsDir string) *Linker {
	if channelsDir == "" {
		channelsPath := config.GetChannelsDir()
		if filepath.Base(channelsPath) == "active" {
			channelsDir = filepath.Dir(channelsPath)
		} else {
			channelsDir = channelsPath
		}
	}
	activeDir := filepath.Join(channelsDir, "active")
	return &Linker{
		channelsDir: channelsDir,
		activeDir:   activeDir,
	}
}

// ValidateBeadExists checks if bead exists and returns path if found
func (l *Linker) ValidateBeadExists(beadID string) (bool, string) {
	projectBead := filepath.Join(".beads", beadID+".md")
	if _, err := os.Stat(projectBead); err == nil {
		absPath, _ := filepath.Abs(projectBead)
		return true, absPath
	}
	home, _ := os.UserHomeDir()
	globalBead := filepath.Join(home, ".engram", "core", "beads", beadID+".md")
	if _, err := os.Stat(globalBead); err == nil {
		return true, globalBead
	}
	dbPaths := []string{
		filepath.Join(".", ".beads", "beads.db"),
		filepath.Join(home, "src", ".beads", "beads.db"),
		filepath.Join(home, ".engram", "core", "beads.db"),
	}
	for _, dbPath := range dbPaths {
		if _, err := os.Stat(dbPath); err == nil {
			absDBPath, _ := filepath.Abs(dbPath)
			cmd := exec.Command("bd", "--db", absDBPath, "show", beadID)
			output, err := cmd.CombinedOutput()
			if err == nil && len(strings.TrimSpace(string(output))) > 0 {
				return true, fmt.Sprintf("bd://%s", beadID)
			}
		}
	}
	return false, ""
}

// ExtractMetadata extracts metadata from channel header
func (l *Linker) ExtractMetadata(channelContent string) map[string]string {
	metadata := make(map[string]string)
	re := regexp.MustCompile(`(?s)---\s*\n(.*?)\n---`)
	matches := re.FindStringSubmatch(channelContent)
	if len(matches) < 2 {
		return metadata
	}
	headerText := matches[1]
	lineRe := regexp.MustCompile(`\*\*(.+?)\*\*:\s*(.+)`)
	for _, line := range strings.Split(headerText, "\n") {
		lineMatches := lineRe.FindStringSubmatch(line)
		if len(lineMatches) == 3 {
			key := strings.TrimSpace(lineMatches[1])
			value := strings.TrimSpace(lineMatches[2])
			metadata[key] = value
		}
	}
	return metadata
}

// UpdateMetadata updates channel metadata
func (l *Linker) UpdateMetadata(channelFile string, metadata map[string]string) error {
	content, err := os.ReadFile(channelFile)
	if err != nil {
		return fmt.Errorf("failed to read channel: %w", err)
	}
	contentStr := string(content)
	var headerLines []string
	for key, value := range metadata {
		headerLines = append(headerLines, fmt.Sprintf("**%s**: %s", key, value))
	}
	newHeader := "---\n" + strings.Join(headerLines, "\n") + "\n---"
	re := regexp.MustCompile(`(?s)---\s*\n.*?\n---`)
	updatedContent := re.ReplaceAllString(contentStr, newHeader)
	if err := os.WriteFile(channelFile, []byte(updatedContent), 0o600); err != nil {
		return fmt.Errorf("failed to write channel: %w", err)
	}
	return nil
}

// LinkChannelToBead links channel to bead
func (l *Linker) LinkChannelToBead(channelID, beadID string) error {
	channelFile := filepath.Join(l.activeDir, channelID+".md")
	if _, err := os.Stat(channelFile); os.IsNotExist(err) {
		return fmt.Errorf("channel not found: %s", channelID)
	}
	beadExists, beadPath := l.ValidateBeadExists(beadID)
	if !beadExists {
		return fmt.Errorf("bead not found: %s", beadID)
	}
	content, err := os.ReadFile(channelFile)
	if err != nil {
		return fmt.Errorf("failed to read channel: %w", err)
	}
	metadata := l.ExtractMetadata(string(content))
	metadata["Bead-ID"] = beadID
	metadata["Bead-Link"] = beadPath
	return l.UpdateMetadata(channelFile, metadata)
}

// UnlinkChannelFromBead unlinks channel from bead
func (l *Linker) UnlinkChannelFromBead(channelID string) error {
	channelFile := filepath.Join(l.activeDir, channelID+".md")
	if _, err := os.Stat(channelFile); os.IsNotExist(err) {
		return fmt.Errorf("channel not found: %s", channelID)
	}
	content, err := os.ReadFile(channelFile)
	if err != nil {
		return fmt.Errorf("failed to read channel: %w", err)
	}
	metadata := l.ExtractMetadata(string(content))
	delete(metadata, "Bead-ID")
	delete(metadata, "Bead-Link")
	return l.UpdateMetadata(channelFile, metadata)
}

// GetLinkedBead returns linked bead ID, or empty string if not linked
func (l *Linker) GetLinkedBead(channelID string) (string, error) {
	channelFile := filepath.Join(l.activeDir, channelID+".md")
	if _, err := os.Stat(channelFile); os.IsNotExist(err) {
		return "", fmt.Errorf("channel not found: %s", channelID)
	}
	content, err := os.ReadFile(channelFile)
	if err != nil {
		return "", fmt.Errorf("failed to read channel: %w", err)
	}
	metadata := l.ExtractMetadata(string(content))
	return metadata["Bead-ID"], nil
}

// RunBeadCommand runs a bd command with optional timeout
func RunBeadCommand(args []string, timeout time.Duration) (string, error) {
	cmd := exec.Command("bd", args...)
	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
		defer timer.Stop()
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bd command failed: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}
