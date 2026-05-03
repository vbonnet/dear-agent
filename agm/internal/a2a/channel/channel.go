// Package channel provides channel-related functionality.
package channel

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/protocol"
)

// Channel represents an A2A communication channel
type Channel struct {
	Path    string
	Topic   string
	Created time.Time
}

// NewChannel creates a new channel instance
func NewChannel(channelPath string) *Channel {
	topic := extractTopicFromPath(channelPath)
	return &Channel{
		Path:  channelPath,
		Topic: topic,
	}
}

// Create creates a new channel file
func (c *Channel) Create() error {
	if _, err := os.Stat(c.Path); err == nil {
		return fmt.Errorf("channel already exists: %s", c.Path)
	}

	dir := filepath.Dir(c.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(c.Path)
	if err != nil {
		return fmt.Errorf("failed to create channel file: %w", err)
	}
	defer file.Close()

	header := fmt.Sprintf("# A2A Channel: %s\n\n", c.Topic)
	header += fmt.Sprintf("**Created**: %s\n", time.Now().Format("2006-01-02 15:04"))
	header += "**Protocol**: Agent-to-Agent Communication v1.0\n\n"
	header += "---\n\n"

	if _, err := file.WriteString(header); err != nil {
		return fmt.Errorf("failed to write channel header: %w", err)
	}

	c.Created = time.Now()
	return nil
}

// Exists checks if the channel file exists
func (c *Channel) Exists() bool {
	_, err := os.Stat(c.Path)
	return err == nil
}

// AppendMessage appends a message to the channel
func (c *Channel) AppendMessage(msg *protocol.Message) error {
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("invalid message: %w", err)
	}

	file, err := os.OpenFile(c.Path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open channel file: %w", err)
	}
	defer file.Close()

	content := msg.Format()
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// Read reads the entire channel content
func (c *Channel) Read() (string, error) {
	content, err := os.ReadFile(c.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read channel: %w", err)
	}
	return string(content), nil
}

// GetLatestMessage returns the most recent message in the channel
func (c *Channel) GetLatestMessage() (*protocol.Message, error) {
	content, err := c.Read()
	if err != nil {
		return nil, err
	}

	messages, err := parseMessages(content)
	if err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages in channel")
	}

	return messages[len(messages)-1], nil
}

// GetAllMessages returns all messages in the channel
func (c *Channel) GetAllMessages() ([]*protocol.Message, error) {
	content, err := c.Read()
	if err != nil {
		return nil, err
	}

	return parseMessages(content)
}

// GetMessageCount returns the number of messages in the channel
func (c *Channel) GetMessageCount() (int, error) {
	messages, err := c.GetAllMessages()
	if err != nil {
		return 0, err
	}
	return len(messages), nil
}

func parseMessages(content string) ([]*protocol.Message, error) {
	var messages []*protocol.Message

	messagePattern := regexp.MustCompile(`(?s)---\s*\n\*\*Agent ID\*\*:([^\n]+)\n\*\*Timestamp\*\*:([^\n]+)\n\*\*Status\*\*:([^\n]+)\n\*\*Message #\*\*:\s*(\d+)\s*\n---\s*\n(.+?)\n---`)

	matches := messagePattern.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) != 6 {
			continue
		}

		agentID := strings.TrimSpace(match[1])
		timestampStr := strings.TrimSpace(match[2])
		statusStr := strings.TrimSpace(match[3])
		messageNum, _ := strconv.Atoi(strings.TrimSpace(match[4]))
		body := match[5]

		timestamp, err := time.Parse("2006-01-02 15:04", timestampStr)
		if err != nil {
			timestamp, err = time.Parse("2006-01-02", timestampStr)
			if err != nil {
				timestamp = time.Now()
			}
		}

		status, err := protocol.ValidateStatus(statusStr)
		if err != nil {
			continue
		}

		msg := &protocol.Message{
			AgentID:       agentID,
			Timestamp:     timestamp,
			Status:        status,
			MessageNumber: messageNum,
		}

		msg.Context = extractSection(body, "### Context")
		msg.Proposal = extractSection(body, "### Proposal/Response")
		msg.Questions = extractListSection(body, "### Questions for Other Agent")
		msg.Blockers = extractListSection(body, "### Blockers/Dependencies")
		msg.NextSteps = extractListSection(body, "### Proposed Next Steps")

		messages = append(messages, msg)
	}

	return messages, nil
}

func extractSection(content, sectionName string) string {
	lines := strings.Split(content, "\n")
	var section []string
	inSection := false

	for _, line := range lines {
		if strings.HasPrefix(line, "###") {
			if strings.Contains(line, sectionName) {
				inSection = true
				continue
			} else if inSection {
				break
			}
		}

		if inSection && strings.TrimSpace(line) != "" {
			section = append(section, line)
		}
	}

	return strings.TrimSpace(strings.Join(section, "\n"))
}

func extractListSection(content, sectionName string) []string {
	section := extractSection(content, sectionName)
	if section == "" || strings.Contains(section, "None") {
		return []string{}
	}

	var items []string
	scanner := bufio.NewScanner(strings.NewReader(section))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "None") {
			continue
		}

		line = regexp.MustCompile(`^[\d]+\.\s*`).ReplaceAllString(line, "")
		line = regexp.MustCompile(`^[-*]\s*`).ReplaceAllString(line, "")
		line = strings.TrimSpace(line)

		if line != "" {
			items = append(items, line)
		}
	}

	return items
}

func extractTopicFromPath(path string) string {
	filename := filepath.Base(path)
	filename = strings.TrimSuffix(filename, ".md")
	datePattern := regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)
	topic := datePattern.ReplaceAllString(filename, "")
	return topic
}
