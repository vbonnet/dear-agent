package messaging

import (
	"fmt"
	"sync"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/modelcard"
)

// Handler processes incoming messages. Return an error to signal failure.
type Handler func(msg *Message) error

// Router delivers messages to registered handlers based on routing mode.
// It uses the modelcard.Registry for role-based routing resolution.
type Router struct {
	mu       sync.RWMutex
	handlers map[string]Handler          // agent_id -> handler
	cards    *modelcard.Registry         // for role-based lookups
	inbox    map[string][]*Message       // agent_id -> pending messages
}

// NewRouter creates a router with a model card registry for role resolution.
func NewRouter(cards *modelcard.Registry) *Router {
	return &Router{
		handlers: make(map[string]Handler),
		cards:    cards,
		inbox:    make(map[string][]*Message),
	}
}

// RegisterHandler registers a message handler for an agent.
func (r *Router) RegisterHandler(agentID string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[agentID] = handler
}

// UnregisterHandler removes the handler for an agent.
func (r *Router) UnregisterHandler(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, agentID)
}

// Send routes a message based on its routing mode.
// Returns the list of agent IDs the message was delivered to.
func (r *Router) Send(msg *Message) ([]string, error) {
	if err := msg.Validate(); err != nil {
		return nil, err
	}

	if msg.IsExpired() {
		return nil, fmt.Errorf("messaging: message %s has expired", msg.ID)
	}

	switch msg.RoutingMode {
	case RouteDirect:
		return r.sendDirect(msg)
	case RouteBroadcast:
		return r.sendBroadcast(msg)
	case RouteRole:
		return r.sendToRole(msg)
	default:
		return nil, fmt.Errorf("messaging: unknown routing mode %q", msg.RoutingMode)
	}
}

func (r *Router) sendDirect(msg *Message) ([]string, error) {
	r.mu.RLock()
	handler, ok := r.handlers[msg.Recipient]
	r.mu.RUnlock()

	if !ok {
		// No live handler — queue to inbox
		r.mu.Lock()
		r.inbox[msg.Recipient] = append(r.inbox[msg.Recipient], msg)
		r.mu.Unlock()
		return []string{msg.Recipient}, nil
	}

	if err := handler(msg); err != nil {
		return nil, fmt.Errorf("messaging: handler error for %s: %w", msg.Recipient, err)
	}
	return []string{msg.Recipient}, nil
}

func (r *Router) sendBroadcast(msg *Message) ([]string, error) {
	r.mu.RLock()
	handlers := make(map[string]Handler, len(r.handlers))
	for id, h := range r.handlers {
		if id != msg.Sender { // don't send to self
			handlers[id] = h
		}
	}
	r.mu.RUnlock()

	var delivered []string
	for id, handler := range handlers {
		if err := handler(msg); err != nil {
			continue // best-effort broadcast
		}
		delivered = append(delivered, id)
	}
	return delivered, nil
}

func (r *Router) sendToRole(msg *Message) ([]string, error) {
	targets := r.cards.FindByRole(msg.TargetRole)
	if len(targets) == 0 {
		return nil, fmt.Errorf("messaging: no agents with role %q", msg.TargetRole)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var delivered []string
	for _, card := range targets {
		if card.AgentID == msg.Sender {
			continue // don't send to self
		}
		handler, ok := r.handlers[card.AgentID]
		if !ok {
			// Queue to inbox
			r.inbox[card.AgentID] = append(r.inbox[card.AgentID], msg)
			delivered = append(delivered, card.AgentID)
			continue
		}
		if err := handler(msg); err != nil {
			continue
		}
		delivered = append(delivered, card.AgentID)
	}
	return delivered, nil
}

// DrainInbox returns and clears all queued messages for an agent.
func (r *Router) DrainInbox(agentID string) []*Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	messages := r.inbox[agentID]
	delete(r.inbox, agentID)
	return messages
}

// PeekInbox returns queued messages without removing them.
func (r *Router) PeekInbox(agentID string) []*Message {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.inbox[agentID]
}
