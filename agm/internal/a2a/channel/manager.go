package channel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manager manages the lifecycle of A2A channels
type Manager struct {
	BaseDir    string
	ArchiveDir string
}

// ChannelInfo contains metadata about a channel
type ChannelInfo struct {
	Topic        string
	Path         string
	Created      time.Time
	LastModified time.Time
	MessageCount int
}

// NewManager creates a new channel manager
func NewManager(baseDir string) *Manager {
	if strings.HasPrefix(baseDir, "~/") {
		homeDir, _ := os.UserHomeDir()
		baseDir = filepath.Join(homeDir, baseDir[2:])
	}

	archiveDir := filepath.Join(filepath.Dir(baseDir), "archive")

	return &Manager{
		BaseDir:    baseDir,
		ArchiveDir: archiveDir,
	}
}

// CreateChannel creates a new channel with the given topic
func (m *Manager) CreateChannel(topic string) (*Channel, error) {
	date := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.md", topic, date)
	channelPath := filepath.Join(m.BaseDir, filename)

	channel := NewChannel(channelPath)
	if err := channel.Create(); err != nil {
		return nil, err
	}

	return channel, nil
}

// GetChannel returns a channel by topic (finds most recent if multiple exist)
func (m *Manager) GetChannel(topic string) (*Channel, error) {
	channels, err := m.ListChannels()
	if err != nil {
		return nil, err
	}

	var matches []*ChannelInfo
	for _, ch := range channels {
		if ch.Topic == topic {
			matches = append(matches, ch)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("channel not found: %s", topic)
	}

	mostRecent := matches[0]
	for _, ch := range matches {
		if ch.LastModified.After(mostRecent.LastModified) {
			mostRecent = ch
		}
	}

	return NewChannel(mostRecent.Path), nil
}

// ListChannels returns all channels in the base directory
func (m *Manager) ListChannels() ([]*ChannelInfo, error) {
	if err := os.MkdirAll(m.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var channels []*ChannelInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		channelPath := filepath.Join(m.BaseDir, entry.Name())
		channel := NewChannel(channelPath)

		info, err := entry.Info()
		if err != nil {
			continue
		}

		messageCount := 0
		if messages, err := channel.GetAllMessages(); err == nil {
			messageCount = len(messages)
		}

		channels = append(channels, &ChannelInfo{
			Topic:        channel.Topic,
			Path:         channelPath,
			Created:      info.ModTime(),
			LastModified: info.ModTime(),
			MessageCount: messageCount,
		})
	}

	return channels, nil
}

// ArchiveChannel moves a channel to the archive directory
func (m *Manager) ArchiveChannel(topic string) error {
	channel, err := m.GetChannel(topic)
	if err != nil {
		return err
	}

	now := time.Now()
	archiveSubDir := filepath.Join(m.ArchiveDir, now.Format("2006-01"))
	if err := os.MkdirAll(archiveSubDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	filename := filepath.Base(channel.Path)
	archivePath := filepath.Join(archiveSubDir, filename)

	if err := os.Rename(channel.Path, archivePath); err != nil {
		return fmt.Errorf("failed to move channel to archive: %w", err)
	}

	return nil
}

// ArchiveInactive archives channels that haven't been modified in N days
func (m *Manager) ArchiveInactive(daysInactive int) error {
	channels, err := m.ListChannels()
	if err != nil {
		return err
	}

	cutoff := time.Now().AddDate(0, 0, -daysInactive)

	for _, ch := range channels {
		if ch.LastModified.Before(cutoff) {
			if err := m.ArchiveChannel(ch.Topic); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to archive channel %s: %v\n", ch.Topic, err)
			}
		}
	}

	return nil
}
