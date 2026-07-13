// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a

import (
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
)

// SessionCard is a convenience builder for the A2A Agent Card a Server
// publishes. It mirrors the fields callers customise most often when
// exposing an agent session and applies dear-agent defaults for the rest.
//
// The Server fills in URL, ProtocolVersion, and PreferredTransport at
// bind time, so leaving them zero here is fine.
type SessionCard struct {
	// Harness is the canonical runtime identifier advertised by the default
	// card, such as "claude-code", "codex-cli", "agy", or "opencode-cli".
	// Empty keeps the card harness-neutral.
	Harness string

	// SessionID uniquely identifies the session. It is appended to the
	// agent name if Name is empty so each card-document is
	// distinguishable in a multi-session registry.
	SessionID string

	// Name overrides the default name. The default includes the configured
	// harness display name and SessionID.
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
	harness := strings.TrimSpace(c.Harness)
	harnessName := harnessDisplayName(harness)
	name := c.Name
	if name == "" {
		if c.SessionID == "" {
			name = harnessName + " Session"
		} else {
			name = harnessName + " Session " + c.SessionID
		}
	}

	skills := c.Skills
	if len(skills) == 0 {
		tags := []string{"general"}
		if harness != "" {
			tags = append(tags, harness)
		}
		skills = []a2a.AgentSkill{{
			ID:          "general",
			Name:        "general",
			Description: "Drive a " + harnessName + " session as an A2A task.",
			Tags:        tags,
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

func harnessDisplayName(harness string) string {
	switch harness {
	case "claude-code":
		return "Claude Code"
	case "codex-cli":
		return "Codex"
	case "agy":
		return "Antigravity"
	case "opencode-cli":
		return "OpenCode"
	case "":
		return "Agent"
	default:
		return harness
	}
}
