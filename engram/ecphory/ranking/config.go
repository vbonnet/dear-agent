package ranking

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds factory configuration
type Config struct {
	Ecphory struct {
		Ranking struct {
			Provider string `yaml:"provider"` // "auto", "anthropic", "vertexai-gemini", etc.
			Fallback string `yaml:"fallback"` // Fallback provider name
		} `yaml:"ranking"`

		Providers ProvidersConfig `yaml:"providers"`
	} `yaml:"ecphory"`
}

// ProvidersConfig holds per-provider configuration
type ProvidersConfig struct {
	Anthropic      AnthropicConfig      `yaml:"anthropic"`
	VertexAI       VertexAIConfig       `yaml:"vertexai"`
	VertexAIClaude VertexAIClaudeConfig `yaml:"vertexai-claude"`
	Local          LocalConfig          `yaml:"local"`
}

// AnthropicConfig for Anthropic provider
type AnthropicConfig struct {
	APIKeyEnv string `yaml:"api_key_env"` // Environment variable name
	Model     string `yaml:"model"`
	MaxTokens int    `yaml:"max_tokens"`
}

// VertexAIConfig for Vertex AI Gemini provider
type VertexAIConfig struct {
	ProjectIDEnv string `yaml:"project_id_env"`
	Location     string `yaml:"location"`
	Model        string `yaml:"model"`
}

// VertexAIClaudeConfig for Vertex AI Claude provider
type VertexAIClaudeConfig struct {
	ProjectIDEnv string `yaml:"project_id_env"`
	Location     string `yaml:"location"` // Must be us-east5 for Claude
	Model        string `yaml:"model"`
}

// LocalConfig for local fallback provider
type LocalConfig struct {
	// No configuration needed - pure local computation
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	c := &Config{}

	// Set defaults
	c.Ecphory.Ranking.Provider = "auto"
	c.Ecphory.Ranking.Fallback = "local"

	c.Ecphory.Providers.Anthropic = AnthropicConfig{
		APIKeyEnv: "ANTHROPIC_API_KEY",
		Model:     "claude-3-5-haiku-20241022",
		MaxTokens: 4096,
	}

	c.Ecphory.Providers.VertexAI = VertexAIConfig{
		ProjectIDEnv: "GOOGLE_CLOUD_PROJECT",
		Location:     "us-central1",
		Model:        "gemini-2.0-flash-exp",
	}

	c.Ecphory.Providers.VertexAIClaude = VertexAIClaudeConfig{
		ProjectIDEnv: "GOOGLE_CLOUD_PROJECT",
		Location:     "us-east5", // Claude only available in us-east5
		Model:        "claude-sonnet-4-5@20250929",
	}

	return c
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
	// If path empty, use default location
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".engram", "config.yaml")
	}

	// If file doesn't exist, use defaults
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read and parse config
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return config, nil
}
