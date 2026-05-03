// Package discovery provides discovery-related functionality.
package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/config"
)

// Checker checks A2A channels for updates and notifies agents
type Checker struct {
	channelsDir string
	stateFile   string
	verbose     bool
	useAGM      bool
}

// NewChecker creates a new channel checker
func NewChecker(options *CheckerOptions) *Checker {
	if options == nil {
		options = &CheckerOptions{}
	}

	channelsDir := options.ChannelsDir
	if channelsDir == "" {
		channelsDir = config.GetChannelsDir()
	}

	stateFile := options.StateFile
	if stateFile == "" {
		home, _ := os.UserHomeDir()
		stateFile = filepath.Join(home, ".engram", "a2a-last-check.json")
	}

	return &Checker{
		channelsDir: channelsDir,
		stateFile:   stateFile,
		verbose:     options.Verbose,
		useAGM:      options.UseAGM,
	}
}

// CheckerOptions contains options for channel checker
type CheckerOptions struct {
	ChannelsDir string
	StateFile   string
	Verbose     bool
	UseAGM      bool
}

// State represents the checker's persistent state
type State struct {
	LastCheckTime   string                  `json:"last_check_time"`
	ChannelsChecked map[string]ChannelState `json:"channels_checked"`
}

// ChannelState represents the state of a single channel
type ChannelState struct {
	LastSeenMessageTimestamp string `json:"last_seen_message_timestamp"`
	LastStatus               string `json:"last_status"`
}

// MessageHeader represents parsed message metadata
type MessageHeader struct {
	AgentID         string
	Timestamp       string
	Status          string
	MessageNumber   string
	ProposalPreview string
}

// Notification represents a channel update notification
type Notification struct {
	ChannelName     string
	ChannelFile     string
	AgentID         string
	Timestamp       string
	Status          string
	MessageNumber   string
	ProposalPreview string
}

// LoadState loads last check state from JSON file
func (c *Checker) LoadState() (*State, error) {
	if _, err := os.Stat(c.stateFile); os.IsNotExist(err) {
		return &State{
			LastCheckTime:   time.Now().UTC().Format(time.RFC3339),
			ChannelsChecked: make(map[string]ChannelState),
		}, nil
	}

	data, err := os.ReadFile(c.stateFile)
	if err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Could not load state file: %v\n", err)
		}
		return &State{
			LastCheckTime:   time.Now().UTC().Format(time.RFC3339),
			ChannelsChecked: make(map[string]ChannelState),
		}, nil
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Could not parse state file: %v\n", err)
		}
		return &State{
			LastCheckTime:   time.Now().UTC().Format(time.RFC3339),
			ChannelsChecked: make(map[string]ChannelState),
		}, nil
	}

	if state.ChannelsChecked == nil {
		state.ChannelsChecked = make(map[string]ChannelState)
	}

	return &state, nil
}

// SaveState saves check state to JSON file
func (c *Checker) SaveState(state *State) error {
	dir := filepath.Dir(c.stateFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(c.stateFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// ParseMessageHeader parses the latest message header from channel content
func (c *Checker) ParseMessageHeader(content string) *MessageHeader {
	lines := strings.Split(strings.TrimSpace(content), "\n")

	var separatorIndices []int
	for i, line := range lines {
		if strings.TrimSpace(line) == "---" {
			separatorIndices = append(separatorIndices, i)
		}
	}

	if len(separatorIndices) < 4 {
		return nil
	}

	headerStart := separatorIndices[len(separatorIndices)-3]
	headerEnd := separatorIndices[len(separatorIndices)-2]

	if headerStart >= len(lines) || headerEnd >= len(lines) {
		return nil
	}

	headerBlock := lines[headerStart+1 : headerEnd]

	message := &MessageHeader{}
	for _, line := range headerBlock {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "**Agent ID**:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				message.AgentID = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "**Timestamp**:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				message.Timestamp = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "**Status**:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				message.Status = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "**Message #**:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				message.MessageNumber = strings.TrimSpace(parts[1])
			}
		}
	}

	proposalStart := -1
	for i := headerEnd; i < len(lines); i++ {
		if strings.Contains(lines[i], "### Proposal") || strings.Contains(lines[i], "### Response") {
			proposalStart = i + 1
			break
		}
	}

	if proposalStart >= 0 && proposalStart < len(lines) {
		var proposalLines []string
		for i := proposalStart; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "###") {
				break
			}
			proposalLines = append(proposalLines, lines[i])
		}
		preview := strings.Join(proposalLines, " ")
		if len(preview) > 200 {
			preview = preview[:200]
		}
		message.ProposalPreview = strings.TrimSpace(preview)
	}

	return message
}

// CheckChannel checks if channel has updates requiring notification
func (c *Checker) CheckChannel(channelFile string, state *State) *Notification {
	channelName := filepath.Base(channelFile)
	channelName = strings.TrimSuffix(channelName, ".md")

	info, err := os.Stat(channelFile)
	if err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Could not stat %s: %v\n", channelFile, err)
		}
		return nil
	}
	_ = info.ModTime()

	lastSeen, exists := state.ChannelsChecked[channelName]
	lastSeenTimeStr := ""
	if exists {
		lastSeenTimeStr = lastSeen.LastSeenMessageTimestamp
	}

	content, err := os.ReadFile(channelFile)
	if err != nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Could not read %s: %v\n", channelFile, err)
		}
		return nil
	}

	message := c.ParseMessageHeader(string(content))
	if message == nil {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Could not parse message from %s\n", channelName)
		}
		return nil
	}

	messageTimestamp := message.Timestamp
	if lastSeenTimeStr != "" && messageTimestamp == lastSeenTimeStr {
		return nil
	}

	status := message.Status
	if status != "awaiting-response" {
		state.ChannelsChecked[channelName] = ChannelState{
			LastSeenMessageTimestamp: messageTimestamp,
			LastStatus:               status,
		}
		return nil
	}

	state.ChannelsChecked[channelName] = ChannelState{
		LastSeenMessageTimestamp: messageTimestamp,
		LastStatus:               status,
	}

	return &Notification{
		ChannelName:     channelName,
		ChannelFile:     channelFile,
		AgentID:         message.AgentID,
		Timestamp:       messageTimestamp,
		Status:          status,
		MessageNumber:   message.MessageNumber,
		ProposalPreview: message.ProposalPreview,
	}
}

// FormatNotification formats notification message for display
func (c *Checker) FormatNotification(notification *Notification) string {
	return fmt.Sprintf("A2A Update: New message in channel %q\n\n- Status: %s\n- From: %s\n- Timestamp: %s\n- Message: %s...\n\nAction: Read %s and respond\n",
		notification.ChannelName, notification.Status, notification.AgentID,
		notification.Timestamp, notification.ProposalPreview, notification.ChannelFile)
}

// CheckAllChannels checks all active channels for updates
func (c *Checker) CheckAllChannels(dryRun bool) (int, []*Notification, error) {
	state, err := c.LoadState()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to load state: %w", err)
	}

	var notifications []*Notification

	if _, err := os.Stat(c.channelsDir); os.IsNotExist(err) {
		if c.verbose {
			fmt.Fprintf(os.Stderr, "Warning: Channels directory not found: %s\n", c.channelsDir)
		}
		return 0, nil, nil
	}

	matches, err := filepath.Glob(filepath.Join(c.channelsDir, "*.md"))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to glob channels: %w", err)
	}

	for _, channelFile := range matches {
		notification := c.CheckChannel(channelFile, state)
		if notification != nil {
			notifications = append(notifications, notification)
		}
	}

	state.LastCheckTime = time.Now().UTC().Format(time.RFC3339)

	if !dryRun {
		if err := c.SaveState(state); err != nil {
			return len(matches), notifications, fmt.Errorf("failed to save state: %w", err)
		}
	}

	return len(matches), notifications, nil
}
