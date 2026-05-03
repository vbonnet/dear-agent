package audit

import "context"

// Store is the persistence interface for the audit subsystem. The
// production implementation is SQLiteStore; tests use NewMemoryStore.
//
// All methods are context-aware. The store must NOT block the runner
// indefinitely on a cancelled context — callers cancel ctx on shutdown
// and expect the next call to return promptly.
//
// All methods are safe for concurrent use.
type Store interface {
	// BeginAuditRun inserts a new audit_runs row in state running.
	// The caller has already populated AuditRunID via Runner.IDGen.
	BeginAuditRun(ctx context.Context, rec AuditRunRecord) error
	// FinishAuditRun updates the audit_runs row with final state and
	// counts. Idempotent: re-finishing a run is allowed (the runner
	// may retry on transient failures).
	FinishAuditRun(ctx context.Context, rec AuditRunRecord) error

	// UpsertFinding inserts a new finding or updates the existing
	// one keyed by (Repo, Fingerprint). Returns the persisted row
	// with FindingID populated. Lifecycle rules:
	//   - first time the (repo, fp) pair is seen → state=open
	//   - re-emission while open/acknowledged → bump last_seen
	//   - re-emission while resolved → flip to reopened, bump last_seen
	UpsertFinding(ctx context.Context, f Finding) (Finding, error)

	// SetFindingState transitions a finding to state. Note is
	// persisted to the finding's row for audit trail. Returns the
	// updated row. Errors when the transition is illegal (open →
	// reopened) so misuse is loud.
	SetFindingState(ctx context.Context, findingID string, state FindingState, note string) (Finding, error)

	// CountFindings returns aggregate counts for a repo. Used by the
	// runner to populate audit_runs.findings_* columns.
	CountFindings(ctx context.Context, repo string) (FindingCounts, error)

	// ListFindings returns findings matching the filter, sorted by
	// (severity asc, last_seen desc). Empty filter returns all.
	ListFindings(ctx context.Context, filter FindingFilter) ([]Finding, error)

	// GetFinding returns the row by id, or (Finding{}, ErrNotFound).
	GetFinding(ctx context.Context, findingID string) (Finding, error)

	// UpsertProposal inserts a new proposal or returns the existing
	// one keyed by (audit_run_id, layer, title). Returns the
	// proposal_id of the persisted row.
	UpsertProposal(ctx context.Context, p Proposal) (string, error)
	// ListProposals returns proposals matching the filter.
	ListProposals(ctx context.Context, filter ProposalFilter) ([]Proposal, error)
	// SetProposalState records a decision on a proposal. The store
	// stamps DecidedAt and DecidedBy from the parameters.
	SetProposalState(ctx context.Context, proposalID string, state ProposalState, decidedBy, note string) error

	// Close releases any underlying resources. Safe to call multiple
	// times.
	Close() error
}

// FindingFilter narrows a ListFindings query. Zero-valued fields
// match everything.
type FindingFilter struct {
	Repo     string
	State    FindingState
	Severity Severity
	CheckID  string
	Limit    int
}

// ProposalFilter narrows a ListProposals query.
type ProposalFilter struct {
	Repo       string
	Layer      ProposalLayer
	State      ProposalState
	AuditRunID string
	Limit      int
}

// FindingCounts is the aggregate the runner persists to
// audit_runs.findings_*.
type FindingCounts struct {
	Open     int
	Resolved int
	// New is the count of findings whose first_seen falls within the
	// last 24 hours — used as a quick proxy for "what showed up in
	// this run". A more precise per-run delta is available by
	// querying audit_findings.first_seen against the audit_run's
	// started_at directly.
	New int
}
