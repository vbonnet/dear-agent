// Package broker provides the MessageBroker abstraction for agent-to-agent
// communication. It integrates model card registration, message routing,
// and session lifecycle management into a single interface.
package broker

import (
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/messaging"
	"github.com/vbonnet/dear-agent/agm/internal/a2a/modelcard"
)

// MessageBroker is the central interface for A2A communication.
// Agents interact with this to register themselves, discover peers,
// and send/receive structured messages.
type MessageBroker interface {
	// RegisterAgent publishes a model card and makes the agent discoverable.
	RegisterAgent(card *modelcard.ModelCard) error

	// UnregisterAgent removes the agent's model card and handler.
	UnregisterAgent(agentID string)

	// Send routes a message to its intended recipient(s).
	Send(msg *messaging.Message) ([]string, error)

	// OnMessage registers a handler for incoming messages.
	OnMessage(agentID string, handler messaging.Handler)

	// DrainInbox retrieves and clears queued messages for an agent.
	DrainInbox(agentID string) []*messaging.Message

	// FindAgentsByRole returns model cards for agents with the given role.
	FindAgentsByRole(role string) []*modelcard.ModelCard

	// FindAgentsByCapability returns model cards for agents with the given capability.
	FindAgentsByCapability(capability string) []*modelcard.ModelCard

	// GetAgent returns the model card for a specific agent.
	GetAgent(agentID string) (*modelcard.ModelCard, bool)

	// ActiveAgents returns all currently active agents.
	ActiveAgents() []*modelcard.ModelCard

	// UpdateStatus updates an agent's runtime status.
	UpdateStatus(agentID string, status modelcard.Status) error
}

// Broker is the default in-process implementation of MessageBroker.
type Broker struct {
	cards  *modelcard.Registry
	router *messaging.Router
}

// New creates a new Broker with fresh registries.
func New() *Broker {
	cards := modelcard.NewRegistry()
	router := messaging.NewRouter(cards)
	return &Broker{
		cards:  cards,
		router: router,
	}
}

// RegisterAgent registers an agent's model card with the broker.
func (b *Broker) RegisterAgent(card *modelcard.ModelCard) error {
	if err := b.cards.Register(card); err != nil {
		return fmt.Errorf("broker: %w", err)
	}
	return nil
}

// UnregisterAgent removes an agent's card and any registered handler.
func (b *Broker) UnregisterAgent(agentID string) {
	b.cards.Unregister(agentID)
	b.router.UnregisterHandler(agentID)
}

// Send routes a message via the router and returns the recipient agent IDs.
func (b *Broker) Send(msg *messaging.Message) ([]string, error) {
	return b.router.Send(msg)
}

// OnMessage registers a handler that receives messages addressed to agentID.
func (b *Broker) OnMessage(agentID string, handler messaging.Handler) {
	b.router.RegisterHandler(agentID, handler)
}

// DrainInbox returns and clears the buffered messages for agentID.
func (b *Broker) DrainInbox(agentID string) []*messaging.Message {
	return b.router.DrainInbox(agentID)
}

// FindAgentsByRole returns all registered agents matching the given role.
func (b *Broker) FindAgentsByRole(role string) []*modelcard.ModelCard {
	return b.cards.FindByRole(role)
}

// FindAgentsByCapability returns all registered agents that advertise the capability.
func (b *Broker) FindAgentsByCapability(capability string) []*modelcard.ModelCard {
	return b.cards.FindByCapability(capability)
}

// GetAgent returns the model card for agentID and whether it was found.
func (b *Broker) GetAgent(agentID string) (*modelcard.ModelCard, bool) {
	return b.cards.Get(agentID)
}

// ActiveAgents returns model cards for all currently active agents.
func (b *Broker) ActiveAgents() []*modelcard.ModelCard {
	return b.cards.ActiveAgents()
}

// UpdateStatus updates the registered status of agentID.
func (b *Broker) UpdateStatus(agentID string, status modelcard.Status) error {
	return b.cards.UpdateStatus(agentID, status)
}
