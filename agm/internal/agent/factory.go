package agent

import (
	"context"
	"fmt"
)

// HarnessInfo contains metadata about an AI harness
type HarnessInfo struct {
	Name         string       `json:"name"`
	Status       string       `json:"status"` // "available" | "unavailable"
	Model        string       `json:"model"`
	Capabilities Capabilities `json:"capabilities"`
}

// Harness registry maps harness names to constructor functions
var agentRegistry = map[string]func() (Agent, error){
	"claude-code": func() (Agent, error) { return NewClaudeAdapter(nil) },
	"gemini-cli":  func() (Agent, error) { return NewGeminiCLIAdapter(nil) },
	"codex-cli": func() (Agent, error) {
		return NewOpenAIAdapter(context.Background(), nil)
	},
	"opencode-cli": func() (Agent, error) {
		return NewOpenCodeAdapter(nil)
	},
}

// GetHarness returns a harness adapter instance by name
func GetHarness(name string) (Agent, error) {
	constructor, ok := agentRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown harness: %s", name)
	}
	return constructor()
}

// GetAllHarnesses returns metadata for all known harnesses
func GetAllHarnesses() []HarnessInfo {
	harnesses := []string{"claude-code", "gemini-cli", "codex-cli", "opencode-cli"}
	result := []HarnessInfo{}

	for _, name := range harnesses {
		// Check availability (API key presence)
		status := "available"
		if err := ValidateHarnessAvailability(name); err != nil {
			status = "unavailable"
		}

		// Get harness adapter
		adapter, err := GetHarness(name)
		if err != nil {
			// Should not happen for known harnesses, but handle gracefully
			continue
		}

		// Get capabilities
		caps := adapter.Capabilities()

		result = append(result, HarnessInfo{
			Name:         name,
			Status:       status,
			Model:        caps.ModelName,
			Capabilities: caps,
		})
	}

	return result
}
