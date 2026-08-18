package agent

import "fmt"

// HarnessInfo contains metadata about an AI harness
type HarnessInfo struct {
	Name         string       `json:"name"`
	Status       string       `json:"status"` // "available" | "unavailable"
	Model        string       `json:"model"`
	Capabilities Capabilities `json:"capabilities"`
}

// GetHarness returns a harness adapter instance by name
func GetHarness(name string) (Harness, error) {
	return newHarnessWithStore(name, nil)
}

// newHarnessWithStore is the single finite constructor catalog used by
// discovery and conformance. A switch keeps the catalog immutable at runtime.
func newHarnessWithStore(name string, store SessionStore) (Harness, error) {
	switch name = NormalizeHarnessName(name); name {
	case "claude-code":
		return NewClaudeAdapter(store)
	case "gemini-cli":
		return NewGeminiCLIAdapter(store)
	case "codex-cli":
		return NewCodexCLIAdapter(store)
	case "opencode-cli":
		if store == nil {
			return NewOpenCodeAdapter(nil)
		}
		return NewOpenCodeAdapter(&OpenCodeConfig{SessionStore: store})
	case "agy":
		return NewAgyAdapter(store)
	case "pi-cli":
		return NewPiAdapter(store)
	default:
		return nil, fmt.Errorf("unknown harness: %s", name)
	}
}

// GetAllHarnesses returns metadata for all known harnesses
func GetAllHarnesses() []HarnessInfo {
	harnesses := ActiveHarnesses()
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
