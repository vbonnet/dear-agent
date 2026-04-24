// Package modelcard provides enhanced agent model cards for A2A protocol.
// Model cards extend the basic A2A AgentCard with capabilities, tools,
// runtime status, and role information for agent-to-agent discovery.
package modelcard

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Status represents the runtime status of an agent.
type Status string

const (
	StatusActive  Status = "active"
	StatusBusy    Status = "busy"
	StatusIdle    Status = "idle"
	StatusOffline Status = "offline"
)

// ModelCard describes an agent's identity, capabilities, and runtime state.
type ModelCard struct {
	// Identity
	AgentID     string `json:"agent_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Role        string `json:"role"` // orchestrator, worker, reviewer, researcher

	// Capabilities
	Capabilities []string `json:"capabilities"` // e.g. ["code-review", "testing", "refactoring"]
	Tools        []string `json:"tools"`         // e.g. ["Read", "Write", "Bash", "Grep"]
	InputModes   []string `json:"input_modes"`   // e.g. ["text/plain", "application/json"]
	OutputModes  []string `json:"output_modes"`  // e.g. ["text/plain", "application/json"]

	// Runtime state
	Status    Status    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`

	// Metadata
	SessionID string            `json:"session_id,omitempty"`
	Harness   string            `json:"harness,omitempty"` // claude-code, gemini-cli, etc.
	Model     string            `json:"model,omitempty"`   // sonnet, opus, etc.
	Tags      map[string]string `json:"tags,omitempty"`
}

// NewModelCard creates a model card with required fields.
func NewModelCard(agentID, name, role string) *ModelCard {
	return &ModelCard{
		AgentID:      agentID,
		Name:         name,
		Role:         role,
		Status:       StatusActive,
		UpdatedAt:    time.Now(),
		Capabilities: []string{},
		Tools:        []string{},
		InputModes:   []string{"text/plain"},
		OutputModes:  []string{"text/plain"},
		Tags:         make(map[string]string),
	}
}

// SetStatus updates the runtime status.
func (mc *ModelCard) SetStatus(status Status) {
	mc.Status = status
	mc.UpdatedAt = time.Now()
}

// HasCapability checks if the agent declares a specific capability.
func (mc *ModelCard) HasCapability(cap string) bool {
	for _, c := range mc.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// HasRole checks if the agent has the specified role.
func (mc *ModelCard) HasRole(role string) bool {
	return mc.Role == role
}

// MarshalJSON serializes the model card to JSON.
func (mc *ModelCard) MarshalJSON() ([]byte, error) {
	type Alias ModelCard
	return json.Marshal((*Alias)(mc))
}

// Registry manages model card registration and discovery.
type Registry struct {
	mu    sync.RWMutex
	cards map[string]*ModelCard // keyed by agent_id
}

// NewRegistry creates an empty model card registry.
func NewRegistry() *Registry {
	return &Registry{
		cards: make(map[string]*ModelCard),
	}
}

// Register adds or updates a model card in the registry.
func (r *Registry) Register(card *ModelCard) error {
	if card.AgentID == "" {
		return fmt.Errorf("modelcard: agent_id is required")
	}
	if card.Name == "" {
		return fmt.Errorf("modelcard: name is required")
	}
	if card.Role == "" {
		return fmt.Errorf("modelcard: role is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	card.UpdatedAt = time.Now()
	r.cards[card.AgentID] = card
	return nil
}

// Unregister removes a model card from the registry.
func (r *Registry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cards, agentID)
}

// Get returns a model card by agent ID.
func (r *Registry) Get(agentID string) (*ModelCard, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	card, ok := r.cards[agentID]
	return card, ok
}

// FindByRole returns all model cards with the specified role.
func (r *Registry) FindByRole(role string) []*ModelCard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelCard
	for _, card := range r.cards {
		if card.Role == role {
			result = append(result, card)
		}
	}
	return result
}

// FindByCapability returns all model cards that declare the given capability.
func (r *Registry) FindByCapability(capability string) []*ModelCard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelCard
	for _, card := range r.cards {
		if card.HasCapability(capability) {
			result = append(result, card)
		}
	}
	return result
}

// All returns all registered model cards.
func (r *Registry) All() []*ModelCard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ModelCard, 0, len(r.cards))
	for _, card := range r.cards {
		result = append(result, card)
	}
	return result
}

// ActiveAgents returns model cards for agents with active or busy status.
func (r *Registry) ActiveAgents() []*ModelCard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*ModelCard
	for _, card := range r.cards {
		if card.Status == StatusActive || card.Status == StatusBusy {
			result = append(result, card)
		}
	}
	return result
}

// UpdateStatus updates the status of a registered agent.
func (r *Registry) UpdateStatus(agentID string, status Status) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	card, ok := r.cards[agentID]
	if !ok {
		return fmt.Errorf("modelcard: agent %q not registered", agentID)
	}
	card.Status = status
	card.UpdatedAt = time.Now()
	return nil
}
