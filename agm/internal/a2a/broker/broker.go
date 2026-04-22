// Package broker provides the MessageBroker abstraction for agent-to-agent
// communication. It integrates model card registration, message routing,
// and session lifecycle management into a single interface.
package broker

import (
	"fmt"
	"sync"

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
	mu     sync.RWMutex
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

func (b *Broker) RegisterAgent(card *modelcard.ModelCard) error {
	if err := b.cards.Register(card); err != nil {
		return fmt.Errorf("broker: %w", err)
	}
	return nil
}

func (b *Broker) UnregisterAgent(agentID string) {
	b.cards.Unregister(agentID)
	b.router.UnregisterHandler(agentID)
}

func (b *Broker) Send(msg *messaging.Message) ([]string, error) {
	return b.router.Send(msg)
}

func (b *Broker) OnMessage(agentID string, handler messaging.Handler) {
	b.router.RegisterHandler(agentID, handler)
}

func (b *Broker) DrainInbox(agentID string) []*messaging.Message {
	return b.router.DrainInbox(agentID)
}

func (b *Broker) FindAgentsByRole(role string) []*modelcard.ModelCard {
	return b.cards.FindByRole(role)
}

func (b *Broker) FindAgentsByCapability(capability string) []*modelcard.ModelCard {
	return b.cards.FindByCapability(capability)
}

func (b *Broker) GetAgent(agentID string) (*modelcard.ModelCard, bool) {
	return b.cards.Get(agentID)
}

func (b *Broker) ActiveAgents() []*modelcard.ModelCard {
	return b.cards.ActiveAgents()
}

func (b *Broker) UpdateStatus(agentID string, status modelcard.Status) error {
	return b.cards.UpdateStatus(agentID, status)
}
