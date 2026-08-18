package supervisor

import (
	"fmt"
	"strings"
)

const (
	// IDMetaOrchestrator is the canonical AGM session identity for the
	// Meta-Orchestrator supervisor.
	IDMetaOrchestrator = "vroom-meta-orchestrator"
	// IDOrchestrator is the canonical AGM session identity for the
	// Orchestrator supervisor.
	IDOrchestrator = "vroom-orchestrator"
	// IDOverseer is the canonical AGM session identity for the Overseer
	// supervisor.
	IDOverseer = "vroom-overseer"

	// AliasMetaOrchestrator is the compact identity used by heartbeat files
	// and retained for compatibility with earlier AGM supervisor records.
	AliasMetaOrchestrator = "meta-o"
	// AliasOrchestrator is the compact Orchestrator identity.
	AliasOrchestrator = "orch"
	// AliasOverseer is the compact Overseer identity.
	AliasOverseer = "overseer"
)

// Member is one canonical member of the exactly-three-supervisor VROOM mesh.
//
// It deliberately contains only topology: identity, role, alias, and peer
// relationships. Harness, model, tick interval, prompts, and other launch
// policy belong to adapters such as cmd/vroom-dispatch.
type Member struct {
	ID          string
	Alias       string
	Role        Role
	PrimaryFor  string
	TertiaryFor string
}

// canonicalTopology is the single source of truth for the VROOM supervisor
// identities and peer graph. All exported accessors return values or copies so
// callers cannot mutate the topology.
var canonicalTopology = [...]Member{
	{
		ID:          IDMetaOrchestrator,
		Alias:       AliasMetaOrchestrator,
		Role:        RoleMetaOrchestrator,
		PrimaryFor:  IDOrchestrator,
		TertiaryFor: IDOverseer,
	},
	{
		ID:          IDOrchestrator,
		Alias:       AliasOrchestrator,
		Role:        RoleOrchestrator,
		PrimaryFor:  IDOverseer,
		TertiaryFor: IDMetaOrchestrator,
	},
	{
		ID:          IDOverseer,
		Alias:       AliasOverseer,
		Role:        RoleOverseer,
		PrimaryFor:  IDMetaOrchestrator,
		TertiaryFor: IDOrchestrator,
	},
}

// AllMembers returns the three canonical supervisors in stable order.
func AllMembers() []Member {
	members := make([]Member, len(canonicalTopology))
	copy(members, canonicalTopology[:])
	return members
}

// Lookup resolves a canonical ID, compact alias, or role name to one member.
// Matching is case-insensitive and ignores surrounding whitespace so CLI
// surfaces can share one parser without each growing its own alias table.
func Lookup(identity string) (Member, bool) {
	identity = strings.ToLower(strings.TrimSpace(identity))
	for _, member := range canonicalTopology {
		if identity == member.ID || identity == member.Alias || identity == string(member.Role) {
			return member, true
		}
	}
	return Member{}, false
}

// MemberForRole returns the canonical member that holds role.
func MemberForRole(role Role) (Member, bool) {
	for _, member := range canonicalTopology {
		if member.Role == role {
			return member, true
		}
	}
	return Member{}, false
}

// PrimaryPeer returns the canonical supervisor for which member is the
// primary responder (the peer whose work member verifies as Secondary).
func (member Member) PrimaryPeer() (Member, error) {
	peer, ok := Lookup(member.PrimaryFor)
	if !ok {
		return Member{}, fmt.Errorf("supervisor: %q has invalid primary peer %q", member.ID, member.PrimaryFor)
	}
	return peer, nil
}

// TertiaryPeer returns the canonical supervisor for which member is the
// tertiary backup (the peer member is responsible for unsticking).
func (member Member) TertiaryPeer() (Member, error) {
	peer, ok := Lookup(member.TertiaryFor)
	if !ok {
		return Member{}, fmt.Errorf("supervisor: %q has invalid tertiary peer %q", member.ID, member.TertiaryFor)
	}
	return peer, nil
}
