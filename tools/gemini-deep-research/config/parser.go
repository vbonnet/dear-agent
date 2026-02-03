package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// PromptConfig represents the prompts section in the YAML config
type PromptConfig struct {
	Extract  string `yaml:"extract"`
	Analyze  string `yaml:"analyze"`
	Research string `yaml:"research"`
}

// FileConfig represents the structure of the YAML config file
type FileConfig struct {
	Prompts PromptConfig `yaml:"prompts"`
}

// CLIPrompts represents prompts provided via CLI flags
type CLIPrompts struct {
	ExtractPrompt  string
	AnalyzePrompt  string
	ResearchPrompt string
}

// MergedPrompts represents the final merged configuration
type MergedPrompts struct {
	ExtractPrompt  string
	AnalyzePrompt  string
	ResearchPrompt string
}

// GetDefaultPrompts returns built-in default prompts
func GetDefaultPrompts() MergedPrompts {
	return MergedPrompts{
		ExtractPrompt: `Analyze the following content and identify the main research topics.
Return ONLY valid JSON in this format:
{"topics": ["topic1", "topic2", "topic3"]}`,
		AnalyzePrompt: `Analyze the following content and identify the main research topics.
Return ONLY valid JSON in this format:
{"topics": ["topic1", "topic2", "topic3"]}`,
		ResearchPrompt: `Research the following topics and provide a comprehensive analysis.`,
	}
}

// LoadPrompts loads and merges prompt configuration from multiple sources
// Precedence: CLI > Project > Global > Defaults
func LoadPrompts(cliPrompts CLIPrompts) (MergedPrompts, error) {
	// Start with defaults
	result := GetDefaultPrompts()

	// Load global config (~/.config/ai-tools/prompts.yaml)
	globalPath := getGlobalConfigPath()
	if globalConfig, err := loadConfigFile(globalPath); err == nil {
		mergePrompts(&result, globalConfig)
	} else if !os.IsNotExist(err) {
		return MergedPrompts{}, fmt.Errorf("error loading global config: %w", err)
	}

	// Load project config (.ai-tools/prompts.yaml)
	projectPath := getProjectConfigPath()
	if projectConfig, err := loadConfigFile(projectPath); err == nil {
		mergePrompts(&result, projectConfig)
	} else if !os.IsNotExist(err) {
		return MergedPrompts{}, fmt.Errorf("error loading project config: %w", err)
	}

	// Apply CLI flags (highest priority)
	if cliPrompts.ExtractPrompt != "" {
		result.ExtractPrompt = cliPrompts.ExtractPrompt
	}
	if cliPrompts.AnalyzePrompt != "" {
		result.AnalyzePrompt = cliPrompts.AnalyzePrompt
	}
	if cliPrompts.ResearchPrompt != "" {
		result.ResearchPrompt = cliPrompts.ResearchPrompt
	}

	return result, nil
}

// getGlobalConfigPath returns the path to the global config file
func getGlobalConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "ai-tools", "prompts.yaml")
}

// getProjectConfigPath returns the path to the project config file
func getProjectConfigPath() string {
	return filepath.Join(".ai-tools", "prompts.yaml")
}

// loadConfigFile loads a YAML config file
func loadConfigFile(path string) (PromptConfig, error) {
	if path == "" {
		return PromptConfig{}, os.ErrNotExist
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return PromptConfig{}, err
	}

	var fileConfig FileConfig
	if err := yaml.Unmarshal(data, &fileConfig); err != nil {
		return PromptConfig{}, fmt.Errorf("YAML syntax error in %s: %w", path, err)
	}

	return fileConfig.Prompts, nil
}

// mergePrompts merges prompts from a config file into the result
func mergePrompts(result *MergedPrompts, config PromptConfig) {
	if config.Extract != "" {
		result.ExtractPrompt = config.Extract
	}
	if config.Analyze != "" {
		result.AnalyzePrompt = config.Analyze
	}
	if config.Research != "" {
		result.ResearchPrompt = config.Research
	}
}
