package channel

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/config"
)

// Creator creates new A2A channels from templates
type Creator struct {
	channelsDir string
}

// NewCreator creates a new channel creator
func NewCreator(channelsDir string) *Creator {
	if channelsDir == "" {
		channelsDir = config.GetChannelsDir()
	}
	return &Creator{
		channelsDir: channelsDir,
	}
}

// CreateOptions contains options for channel creation
type CreateOptions struct {
	AgentID          string
	WayfinderProject string
	Participants     string
}

// TemplateParams contains parameters for template generation
type TemplateParams struct {
	Topic            string
	ChannelID        string
	AgentID          string
	Timestamp        string
	CreatedDate      string
	WayfinderProject string
	Participants     string
}

// CreateChannel creates a new A2A channel from template
func (c *Creator) CreateChannel(topic string, options *CreateOptions) (string, error) {
	if options == nil {
		options = &CreateOptions{}
	}

	agentID := options.AgentID
	if agentID == "" {
		agentID = detectAgentID()
	}

	if options.WayfinderProject != "" {
		wayfinderPath := config.ExpandPath(options.WayfinderProject)
		if _, err := os.Stat(wayfinderPath); os.IsNotExist(err) {
			return "", fmt.Errorf("wayfinder project path does not exist: %s", options.WayfinderProject)
		}
	}

	participants := options.Participants
	if participants == "" {
		participants = agentID
	}

	channelID := generateChannelID(topic)

	channelPath := filepath.Join(c.channelsDir, channelID+".md")
	if _, err := os.Stat(channelPath); err == nil {
		return "", fmt.Errorf("channel already exists: %s", channelID)
	}

	content := generateTemplate(TemplateParams{
		Topic:            topic,
		ChannelID:        channelID,
		AgentID:          agentID,
		Timestamp:        time.Now().Format("2006-01-02 15:04"),
		CreatedDate:      time.Now().Format("2006-01-02"),
		WayfinderProject: options.WayfinderProject,
		Participants:     participants,
	})

	if err := os.MkdirAll(c.channelsDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create channels directory: %w", err)
	}

	if err := os.WriteFile(channelPath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to create channel file: %w", err)
	}

	return channelPath, nil
}

func detectAgentID() string {
	if agentID := os.Getenv("A2A_AGENT_ID"); agentID != "" {
		return agentID
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "unknown-agent"
}

func generateChannelID(topic string) string {
	normalized := strings.ToLower(topic)
	normalized = strings.ReplaceAll(normalized, " ", "-")
	re := regexp.MustCompile(`[^a-z0-9-]`)
	normalized = re.ReplaceAllString(normalized, "")
	re = regexp.MustCompile(`-+`)
	normalized = re.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	dateSuffix := time.Now().Format("2006-01-02")
	return fmt.Sprintf("%s-%s", normalized, dateSuffix)
}

func generateTemplate(params TemplateParams) string {
	var metadataLines []string
	metadataLines = append(metadataLines, fmt.Sprintf("**Created**: %s", params.CreatedDate))
	metadataLines = append(metadataLines, fmt.Sprintf("**Topic**: %s", params.Topic))
	metadataLines = append(metadataLines, fmt.Sprintf("**Participants**: %s", params.Participants))
	if params.WayfinderProject != "" {
		metadataLines = append(metadataLines, fmt.Sprintf("**Wayfinder-Project**: %s", params.WayfinderProject))
	}
	metadataSection := strings.Join(metadataLines, "\n")

	return fmt.Sprintf(`# A2A Channel: %s

---
%s
---

## Message #1

---
**Agent ID**: %s
**Timestamp**: %s
**Status**: pending
**Message #**: 1
---

### Context

_Describe the context or problem that needs coordination._

### Proposal

_Your proposal, question, or initial thoughts._

---
`, params.ChannelID, metadataSection, params.AgentID, params.Timestamp)
}

// CreateChannelSimple creates a new A2A channel (simplified API)
func CreateChannelSimple(topic string, options *CreateOptions) (string, error) {
	creator := NewCreator("")
	return creator.CreateChannel(topic, options)
}
