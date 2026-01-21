package gemini

import (
	"os"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// Session represents a Gemini chat session
type Session struct {
	ID        agent.SessionID
	Context   agent.SessionContext
	Messages  []agent.Message
	Status    agent.Status
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Config holds Vertex AI configuration from environment
type Config struct {
	ProjectID string
	Location  string
	Model     string
}

// loadConfig reads configuration from environment variables
func loadConfig() Config {
	projectID := os.Getenv("GCP_PROJECT_ID")
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	location := os.Getenv("GCP_LOCATION")
	if location == "" {
		location = os.Getenv("VERTEX_AI_LOCATION")
	}
	if location == "" {
		location = "us-central1" // Default
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.0-flash-exp" // Default
	}

	return Config{
		ProjectID: projectID,
		Location:  location,
		Model:     model,
	}
}
