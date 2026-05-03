package audit

import (
	"context"
	"fmt"
	"time"
)

// ProposalLayer names which DEAR layer a Proposal targets. ADR-011
// §D5 calls out only define and enforce — Audit and Resolve are the
// loops that produced the proposal, not its target.
type ProposalLayer string

// Proposal targets.
const (
	ProposalDefine  ProposalLayer = "define"
	ProposalEnforce ProposalLayer = "enforce"
)

// IsValid reports whether l names a known proposal layer.
func (l ProposalLayer) IsValid() bool {
	switch l {
	case ProposalDefine, ProposalEnforce:
		return true
	}
	return false
}

// ProposalState is the lifecycle of one row in audit_proposals.
type ProposalState string

// Proposal lifecycle states.
const (
	ProposalProposed ProposalState = "proposed"
	ProposalAccepted ProposalState = "accepted"
	ProposalRejected ProposalState = "rejected"
	ProposalExpired  ProposalState = "expired"
)

// IsValid reports whether s names a known proposal state.
func (s ProposalState) IsValid() bool {
	switch s {
	case ProposalProposed, ProposalAccepted, ProposalRejected, ProposalExpired:
		return true
	}
	return false
}

// Proposal is a Refiner's output: a suggested amendment to Define
// or Enforce that, if accepted, would prevent recurrence of the
// findings that produced it.
//
// Patch is a unified diff or a YAML fragment depending on Layer.
// For ProposalEnforce a typical patch adds a linter to .golangci.yml
// or a hook to settings.json. For ProposalDefine a typical patch
// amends a CLAUDE.md / SPEC.md / ADR rule.
//
// The runner persists Proposals into audit_proposals with state
// ProposalProposed; the operator decides via `workflow audit propose`.
type Proposal struct {
	ProposalID  string // assigned by the store
	AuditRunID  string // FK into audit_runs
	Layer       ProposalLayer
	Title       string
	Rationale   string
	Patch       string
	State       ProposalState
	ProposedAt  time.Time
	DecidedAt   time.Time
	DecidedBy   string
	Decision    string // free-form note from the reviewer
}

// Validate returns a non-nil error when the proposal is missing
// fields the store requires.
func (p Proposal) Validate() error {
	if !p.Layer.IsValid() {
		return fmt.Errorf("audit: Proposal.Layer %q invalid", p.Layer)
	}
	if p.Title == "" {
		return fmt.Errorf("audit: Proposal.Title is empty")
	}
	if p.Rationale == "" {
		return fmt.Errorf("audit: Proposal.Rationale is empty")
	}
	return nil
}

// Refiner inspects a slice of Findings (typically one audit run's
// emissions) and proposes amendments to Define / Enforce. The runner
// invokes every registered Refiner once per audit run, after all
// checks have produced their findings.
//
// Refiners are pure: they read findings, they write Proposals, they
// touch nothing else. The store decides whether a proposal is novel
// or duplicates an existing one (keyed on Layer + Title).
type Refiner interface {
	// Name is the stable identifier of this refiner ("lint-gap",
	// "vuln-rule", ...). Used for filtering in the CLI.
	Name() string
	// Propose returns zero or more Proposals for the given findings.
	// Findings is the full list from one audit run; the refiner
	// filters by CheckID / fingerprint as it sees fit.
	Propose(ctx context.Context, findings []Finding) ([]Proposal, error)
}
