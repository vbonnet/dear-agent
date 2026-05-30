package audit

import (
	"context"
	"fmt"
)

// EvidenceVerifierRole is the canonical Evidence map key under which a
// Finding records *which* verifier produced it. The value is the
// Verifier's Name(). Built-in audit Checks may also set this — for an
// internal Check the value is the check id, conventionally distinguishing
// "casual" (same-family) review from "adversarial" (cross-family or
// external) review via EvidenceReviewDepth.
//
// This key is part of the Phase 6.5 trust-inversion contract: once a
// code path has been verified by a different verifier family, downstream
// consumers can compute "verified set" membership by querying findings
// with `evidence_json -> '$.verifier_role'` and `review_depth`.
const EvidenceVerifierRole = "verifier_role"

// EvidenceReviewDepth is the canonical Evidence map key describing
// *how deeply* the verifier inspected the artifact. Two values are
// defined; consumers should treat unknown values as ReviewDepthCasual.
const EvidenceReviewDepth = "review_depth"

// EvidenceVerifiedAt is the canonical Evidence map key under which the
// runner stamps the wall-clock timestamp at which an adversarial
// verifier last produced this Finding. Value is an RFC3339-encoded
// string so it round-trips through encoding/json without loss; consume
// via Finding.VerifiedAt to reify a time.Time.
//
// Phase 6.5 trust-inversion contract: a Finding with EvidenceVerifiedAt
// set is in the *verified set* — adversarially reviewed by a different
// family from the implementer. Casual-depth verifiers do NOT stamp
// this key. The complementary "subsequent edits reset the timestamp"
// half of the contract lives in pkg/workflow (the writer that touches
// the source) and is tracked in ROADMAP §6.5 — this seam is just the
// place to read and write the timestamp.
const EvidenceVerifiedAt = "verified_at"

// Review depth values. Casual is the default for internal Checks
// (static analysis, linters); Adversarial is reserved for verifiers
// that exercise the artifact dynamically (fuzzers, property checkers,
// cross-model review) and is what Phase 6.5 promotes a finding into
// the "verified" set with.
const (
	ReviewDepthCasual      = "casual"
	ReviewDepthAdversarial = "adversarial"
)

// VerifyTarget is the per-call context a Verifier receives. The shape
// mirrors audit.Env so verifiers and checks can share helpers; the type
// is distinct because verifiers may grow target-specific fields
// (artifact path, schema id, …) without bloating Env.
type VerifyTarget struct {
	// RepoRoot is the absolute path of the working tree under audit.
	RepoRoot string
	// WorkingDir is the subtree the verifier should operate on. Equal
	// to RepoRoot for single-tree audits.
	WorkingDir string
	// Config carries verifier-specific knobs from .dear-agent.yml >
	// audits.verifiers. Verifiers must tolerate an empty / nil Config
	// without panicking.
	Config map[string]any
}

// Verifier is the audit-side surface of the Phase 6.6 external
// verification seam (ROADMAP §Phase 6 6.6). A Verifier inspects a
// target produced by an upstream phase and emits Findings the same
// way a Check does — but the dispatch layer marks every returned
// Finding's Evidence with verifier_role and review_depth so the
// trust-inversion accounting in Phase 6.5 can tell internal review
// apart from external adversarial review.
//
// dear-agent does not ship a real Verifier — that is the point of the
// interface. First parties (Mythos, llm-judge) plug in via
// plugin.VerifierProvider; the no-op verifier in pkg/audit/verifiers
// exists only to prove the dispatch end-to-end.
type Verifier interface {
	// Name uniquely identifies the verifier within a registry. The
	// recommended convention is "<provider>.<verifier>" (e.g.
	// "mythos.fuzz"); the registry enforces non-empty but does not
	// constrain the alphabet.
	Name() string

	// Description is a one-line human-readable summary used by
	// `workflow audit verifiers list`. May be empty.
	Description() string

	// ReviewDepth is the depth the verifier claims its findings are
	// produced at. The runner stamps this into Evidence when the
	// finding does not already carry a review_depth key. Defaults to
	// ReviewDepthAdversarial when the verifier returns "" — verifiers
	// exist because external tooling is more thorough than internal
	// checks, so adversarial is the right default.
	ReviewDepth() string

	// Verify runs the verifier against target. Findings have the same
	// shape Checks return: CheckID empty (the runner stamps the
	// verifier name into Evidence; CheckID stays empty for verifier
	// findings so downstream filters can tell them apart), valid
	// Fingerprint, Severity, Title. Error paths follow audit Check
	// conventions: a non-nil error means "verifier could not run",
	// not "verifier ran and found a defect". Defects are findings.
	Verify(ctx context.Context, target VerifyTarget) ([]Finding, error)
}

// RegisterVerifier adds a Verifier to r. Returns an error when v is
// nil, v.Name is empty, or a verifier with the same name is already
// registered. Mirrors the (Check) Register contract; idempotency is
// at the per-name level so an init() race between two callers fails
// loudly rather than silently overwriting.
func (r *Registry) RegisterVerifier(v Verifier) error {
	if v == nil {
		return fmt.Errorf("audit: Registry.RegisterVerifier: nil verifier")
	}
	name := v.Name()
	if name == "" {
		return fmt.Errorf("audit: Registry.RegisterVerifier: empty verifier name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.verifiers == nil {
		r.verifiers = make(map[string]Verifier)
	}
	if _, ok := r.verifiers[name]; ok {
		return fmt.Errorf("audit: Registry.RegisterVerifier: verifier %q already registered", name)
	}
	r.verifiers[name] = v
	return nil
}

// Verifiers returns a snapshot of registered verifiers, sorted by
// Name. The returned slice is owned by the caller. Empty when none
// are registered — the runner treats an empty verifier list as
// "verification phase skipped", not as an error.
func (r *Registry) Verifiers() []Verifier {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Verifier, 0, len(r.verifiers))
	for _, v := range r.verifiers {
		out = append(out, v)
	}
	sortVerifiersByName(out)
	return out
}

// LookupVerifier returns the verifier registered under name, or
// (nil, false). Used by CLI `workflow audit verifiers describe NAME`.
func (r *Registry) LookupVerifier(name string) (Verifier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.verifiers[name]
	return v, ok
}

// sortVerifiersByName sorts in place using the same lightweight pattern
// the rest of the package uses for snapshot returns. Pulled into a
// helper so the registry methods stay short.
func sortVerifiersByName(vs []Verifier) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && vs[j-1].Name() > vs[j].Name(); j-- {
			vs[j-1], vs[j] = vs[j], vs[j-1]
		}
	}
}
