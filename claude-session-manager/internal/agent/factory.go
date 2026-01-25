package agent

import "fmt"

// AgentInfo contains metadata about an AI agent
type AgentInfo struct {
	Name         string       `json:"name"`
	Status       string       `json:"status"` // "available" | "unavailable"
	Model        string       `json:"model"`
	Capabilities Capabilities `json:"capabilities"`
}

// Agent registry maps agent names to constructor functions
var agentRegistry = map[string]func() (Agent, error){
	"claude": func() (Agent, error) { return NewClaudeAdapter(nil) },
	"gemini": func() (Agent, error) { return NewGeminiAdapter(), nil },
	"gpt":    func() (Agent, error) { return NewGPTAdapter(), nil },
}

// GetAgent returns an agent adapter instance by name
func GetAgent(name string) (Agent, error) {
	constructor, ok := agentRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", name)
	}
	return constructor()
}

// GetAllAgents returns metadata for all known agents
func GetAllAgents() []AgentInfo {
	agents := []string{"claude", "gemini", "gpt"}
	result := []AgentInfo{}

	for _, name := range agents {
		// Check availability (API key presence)
		status := "available"
		if err := ValidateAgentAvailability(name); err != nil {
			status = "unavailable"
		}

		// Get agent adapter
		adapter, err := GetAgent(name)
		if err != nil {
			// Should not happen for known agents, but handle gracefully
			continue
		}

		// Get capabilities
		caps := adapter.Capabilities()

		result = append(result, AgentInfo{
			Name:         name,
			Status:       status,
			Model:        caps.ModelName,
			Capabilities: caps,
		})
	}

	return result
}
