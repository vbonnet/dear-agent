// Package profile provides a two-tier agent profile abstraction for agent
// identity and session continuity.
//
// Engram spawns many short-lived worker agents. Each worker has a stable
// identity that is fixed at spawn time (who it is, what it can do, what it
// must not do) and a fluid, session-scoped working context that accumulates
// as the worker makes progress (what it is currently focused on and what it
// has discovered). Collapsing both into a single mutable blob loses the
// guarantee that identity is immutable and makes handoff between sessions
// ambiguous.
//
// This package separates the two concerns:
//
//   - StaticProfile  — immutable identity, capabilities, and constraints.
//     Written once at spawn time; subsequent writes are rejected.
//   - DynamicProfile — session-specific task focus and accumulated
//     discoveries. Updated repeatedly as the worker runs.
//
// A worker loads its StaticProfile at start and updates its DynamicProfile as
// it works. Because the dynamic profile is keyed by session, a later session
// (or a different agent picking up the work) can reload both tiers to resume
// with full context — better handoff and session resumption.
//
// Persistence is file-based (see Store), consistent with engram's no-database
// design.
package profile

import (
	"fmt"
	"regexp"
	"time"
)

// idPattern restricts agent and session identifiers to a safe character set.
// This doubles as path-traversal protection because identifiers become
// directory and file name components in Store.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// validID reports whether id is a safe identifier for use as a path component.
func validID(id string) bool {
	if id == ".." || id == "." {
		return false
	}
	return idPattern.MatchString(id)
}

// StaticProfile is the immutable identity of an agent, fixed at spawn time.
//
// Once persisted via Store.SaveStatic it must not change for the lifetime of
// the agent; the store enforces this by rejecting overwrites.
type StaticProfile struct {
	// AgentID uniquely identifies the agent. Required.
	AgentID string `json:"agent_id"`
	// Name is a human-readable label (e.g. "ce-ywii-worker").
	Name string `json:"name,omitempty"`
	// Role describes what the agent was spawned to do (e.g. "go-implementer").
	Role string `json:"role,omitempty"`
	// Capabilities lists what the agent is allowed/able to do.
	Capabilities []string `json:"capabilities,omitempty"`
	// Constraints lists hard rules the agent must not violate.
	Constraints []string `json:"constraints,omitempty"`
	// Model records the model the agent runs on (e.g. "claude-opus-4-8").
	Model string `json:"model,omitempty"`
	// SpawnedAt is when the agent identity was created.
	SpawnedAt time.Time `json:"spawned_at"`
}

// Validate checks that the static profile is well-formed.
func (p StaticProfile) Validate() error {
	if !validID(p.AgentID) {
		return fmt.Errorf("%w: agent_id %q", ErrInvalidID, p.AgentID)
	}
	return nil
}

// Discovery is a single thing the agent learned during a session.
type Discovery struct {
	// Timestamp is when the discovery was recorded.
	Timestamp time.Time `json:"timestamp"`
	// Summary is a one-line description of the discovery.
	Summary string `json:"summary"`
	// Detail is optional supporting context.
	Detail string `json:"detail,omitempty"`
}

// DynamicProfile is the session-scoped working context of an agent. Unlike
// StaticProfile it is expected to change many times over a session.
type DynamicProfile struct {
	// AgentID links this dynamic profile to its StaticProfile. Required.
	AgentID string `json:"agent_id"`
	// SessionID scopes the profile to a single working session. Required.
	SessionID string `json:"session_id"`
	// TaskFocus is what the agent is currently working on.
	TaskFocus string `json:"task_focus,omitempty"`
	// Discoveries accumulate as the agent learns things worth carrying forward.
	Discoveries []Discovery `json:"discoveries,omitempty"`
	// UpdatedAt is when the dynamic profile was last written.
	UpdatedAt time.Time `json:"updated_at"`
}

// Validate checks that the dynamic profile is well-formed.
func (p DynamicProfile) Validate() error {
	if !validID(p.AgentID) {
		return fmt.Errorf("%w: agent_id %q", ErrInvalidID, p.AgentID)
	}
	if !validID(p.SessionID) {
		return fmt.Errorf("%w: session_id %q", ErrInvalidID, p.SessionID)
	}
	return nil
}

// AgentProfile bundles both tiers for a single agent + session. It is the
// natural unit for context handoff and session resumption: load it once and a
// worker has both who it is and where it left off.
type AgentProfile struct {
	Static  StaticProfile  `json:"static"`
	Dynamic DynamicProfile `json:"dynamic"`
}
