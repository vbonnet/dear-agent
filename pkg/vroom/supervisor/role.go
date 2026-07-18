// Package supervisor implements the VROOM supervisory mesh: three long-lived
// supervisor agents (Meta-Orchestrator, Orchestrator, Overseer) each running a
// constant loop with the mutual-unblock-first invariant.
//
// Vocabulary is normative in /CONTEXT.md; architectural rationale is in
// /docs/adr/ADR-002. If this package and those documents ever disagree,
// CONTEXT.md wins for definitions and ADR-002 wins for rationale.
package supervisor

import "fmt"

// Role names the three supervisor roles in the canonical VROOM mesh.
//
// These are the *supervisors* — long-lived agents in a peer mesh — not the
// per-task Primary/Secondary/Tertiary ownership roles (which are
// responsibilities held by *any* agent for a *specific* task).
type Role string

const (
	// RoleMetaOrchestrator is the CTO-analogue supervisor: roadmap,
	// prioritization, anti-duplication, technology consistency. The
	// Meta-Orchestrator is the *sole* agent permitted to add items to the
	// roadmap (everyone else proposes via a Work Order).
	RoleMetaOrchestrator Role = "meta-orchestrator"

	// RoleOrchestrator is the COO-analogue supervisor: enqueue/dequeue work,
	// monitor active workers, keep steady progress, never sit idle.
	RoleOrchestrator Role = "orchestrator"

	// RoleOverseer is the CRO-analogue supervisor: resource usage
	// (CPU/disk/memory/quota), leak detection, session cleanup.
	RoleOverseer Role = "overseer"
)

// AllRoles returns the three supervisor roles in canonical order.
// Order is stable so tests and decision-trail records are deterministic.
func AllRoles() []Role {
	roles := make([]Role, 0, len(canonicalTopology))
	for _, member := range canonicalTopology {
		roles = append(roles, member.Role)
	}
	return roles
}

// Valid reports whether r is one of the three canonical roles.
func (r Role) Valid() bool {
	_, ok := MemberForRole(r)
	return ok
}

// Peers returns the two peers this supervisor watches at the start of every
// loop iteration. Per CONTEXT.md § "The three supervisors":
//
//	Meta-Orchestrator | Secondary: Overseer       | Tertiary: Orchestrator
//	Orchestrator      | Secondary: Meta-O         | Tertiary: Overseer
//	Overseer          | Secondary: Orchestrator   | Tertiary: Meta-O
//
// The CONTEXT.md columns name *who fills the Secondary/Tertiary role for this
// supervisor's own work*. The cyclic inverse — *whom this supervisor watches*
// — is what the loop needs, and is what Peers returns:
//
//	verifies = the peer this supervisor is Secondary *for* (verifies their work)
//	unsticks = the peer this supervisor is Tertiary *for* (unsticks them)
//
// Together, verifies and unsticks are "the other two supervisors" per the
// mutual-unblock-first invariant — every supervisor watches both, with a
// distinguished responsibility for each.
func (r Role) Peers() (verifies, unsticks Role, err error) {
	member, ok := MemberForRole(r)
	if !ok {
		return "", "", fmt.Errorf("supervisor: invalid role %q", string(r))
	}
	primary, err := member.PrimaryPeer()
	if err != nil {
		return "", "", err
	}
	tertiary, err := member.TertiaryPeer()
	if err != nil {
		return "", "", err
	}
	return primary.Role, tertiary.Role, nil
}

// PeerList returns this supervisor's two peers as a slice, in (verifies,
// unsticks) order. Convenience for loop drivers that iterate.
func (r Role) PeerList() ([]Role, error) {
	v, u, err := r.Peers()
	if err != nil {
		return nil, err
	}
	return []Role{v, u}, nil
}

// SecondaryFor returns the supervisor that holds the Secondary responsibility
// for target — the peer that *verifies* target's work and is therefore first
// in line to assume target's role under failover (retro R.1: "Only the
// designated Secondary may attempt promotion").
//
// It is the inverse of Peers: SecondaryFor(target) is the unique role r whose
// r.Peers() verifies-slot is target. Concretely:
//
//	SecondaryFor(meta-orchestrator) = overseer
//	SecondaryFor(orchestrator)      = meta-orchestrator
//	SecondaryFor(overseer)          = orchestrator
func SecondaryFor(target Role) (Role, error) {
	if !target.Valid() {
		return "", fmt.Errorf("supervisor: SecondaryFor invalid role %q", string(target))
	}
	for _, r := range AllRoles() {
		v, _, err := r.Peers()
		if err != nil {
			return "", err
		}
		if v == target {
			return r, nil
		}
	}
	return "", fmt.Errorf("supervisor: no Secondary found for role %q", string(target))
}

// TertiaryFor returns the supervisor that holds the Tertiary responsibility
// for target — the peer that *unsticks* target and may promote itself to
// target's role only if the Secondary is also dark (retro R.1: "The Tertiary
// defers unless the Secondary is also dead, then it promotes itself").
//
// It is the inverse of Peers: TertiaryFor(target) is the unique role r whose
// r.Peers() unsticks-slot is target. Concretely:
//
//	TertiaryFor(meta-orchestrator) = orchestrator
//	TertiaryFor(orchestrator)      = overseer
//	TertiaryFor(overseer)          = meta-orchestrator
func TertiaryFor(target Role) (Role, error) {
	if !target.Valid() {
		return "", fmt.Errorf("supervisor: TertiaryFor invalid role %q", string(target))
	}
	for _, r := range AllRoles() {
		_, u, err := r.Peers()
		if err != nil {
			return "", err
		}
		if u == target {
			return r, nil
		}
	}
	return "", fmt.Errorf("supervisor: no Tertiary found for role %q", string(target))
}
