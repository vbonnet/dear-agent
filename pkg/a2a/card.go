// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a

import (
	"github.com/a2aproject/a2a-go/a2a"
)

// SessionCard is a convenience builder for the A2A Agent Card a Server
// publishes. It mirrors the fields callers customise most often when
// exposing a Claude Code session and applies dear-agent defaults for
// the rest.
//
// The Server fills in URL, ProtocolVersion, and PreferredTransport at
// bind time, so leaving them zero here is fine.
type SessionCard struct {
	// SessionID uniquely identifies the session. It is appended to the
	// agent name if Name is empty so each card-document is
	// distinguishable in a multi-session registry.
	SessionID string

	// Name overrides the default name. Defaults to
	// "Claude Code Session <SessionID>".
	Name string

	// Description is the human-readable summary advertised in the card.
	Description string

	// Provider is the optional organisation block.
	Provider *a2a.AgentProvider

	// Skills overrides the inferred skill list. When empty, a single
	// "general" skill is advertised; callers with richer manifest data
	// (e.g. agm/internal/a2a) typically supply their own list here.
	Skills []a2a.AgentSkill

	// Capabilities overrides the default capabilities block. The default
	// declares StateTransitionHistory=true so clients can see the
	// input-required→working transitions that drive the supervisor loop.
	Capabilities *a2a.AgentCapabilities
}

// Build returns a *a2a.AgentCard populated from c and the dear-agent
// defaults. The returned card is safe to pass to NewServer; its URL,
// ProtocolVersion and PreferredTransport fields are overwritten by the
// Server once the listener is bound.
func (c SessionCard) Build() *a2a.AgentCard {
	name := c.Name
	if name == "" {
		if c.SessionID == "" {
			name = "Claude Code Session"
		} else {
			name = "Claude Code Session " + c.SessionID
		}
	}

	skills := c.Skills
	if len(skills) == 0 {
		skills = []a2a.AgentSkill{{
			ID:          "general",
			Name:        "general",
			Description: "Drive a Claude Code session as an A2A task.",
			Tags:        []string{"general", "claude-code"},
		}}
	}

	caps := a2a.AgentCapabilities{
		Streaming:              false,
		StateTransitionHistory: true,
	}
	if c.Capabilities != nil {
		caps = *c.Capabilities
	}

	return &a2a.AgentCard{
		Name:               name,
		Description:        c.Description,
		Provider:           c.Provider,
		Skills:             skills,
		Capabilities:       caps,
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		// URL / ProtocolVersion / PreferredTransport intentionally left
		// zero: NewServer fills them in once the listener is bound.
	}
}
