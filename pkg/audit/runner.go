package audit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Runner orchestrates one audit invocation. Construct with NewRunner,
// configure its Registry, Store, and optional clock, and call Run with a
// Plan describing what to execute.
//
// One Runner instance is safe for concurrent Run calls — each call
// owns its own Plan and its own audit_runs row. The Runner does not
// own the SQLite connection; the Store does.
type Runner struct {
	Registry *Registry
	Store    Store
	Logger   *slog.Logger

	// Remediator is the dormant StrategyAuto compatibility seam. Runner drops
	// Status and Reference, and drops Note unless State changes.
	//
	// Deprecated: Leave this set to NewNoopRemediator until an idempotent
	// remediation-event persistence and legacy-migration contract exists.
	Remediator Remediator

	// Now is overridable in tests. Production callers leave it nil
	// and the runner uses time.Now.
	Now func() time.Time

	// IDGen produces audit_run_id and finding_id values. Overridable
	// in tests so golden output stays stable.
	IDGen func() string

	// CheckTimeout caps wall-clock time for a single check. Zero
	// inherits the per-call ctx deadline; checks that exceed the
	// timeout produce a StatusTimeout result and are skipped for
	// finding extraction.
	CheckTimeout time.Duration
}

// NewRunner constructs a Runner with sensible defaults: Default
// registry, slog.Default logger, NoopRemediator, time.Now,
// uuid.NewString. The caller MUST set Store before calling Run; the
// runner panics on a nil store rather than silently dropping
// findings.
func NewRunner() *Runner {
	return &Runner{
		Registry:   Default,
		Logger:     slog.Default(),
		Remediator: NewNoopRemediator(),
		Now:        time.Now,
		IDGen:      uuid.NewString,
	}
}

// Plan describes one audit invocation: which checks to run, what
// inputs they receive, what cadence label to record on the audit_runs
// row, and how to remediate. A Plan is the data shape produced by
// the .dear-agent.yml config loader and consumed by Runner.Run.
type Plan struct {
	Repo     string
	Cadence  Cadence
	RepoRoot string
	// Trees lists the per-tree subdivisions of this repo. Single-tree
	// audits use one Tree with WorkingDir == RepoRoot.
	Trees []TreePlan
	// TriggeredBy is recorded on the audit_runs row. "cli", "cron",
	// "workflow:audit-daily", etc.
	TriggeredBy string
	// SeverityPolicy maps each Severity to a per-severity policy.
	// nil falls back to the package default in DefaultSeverityPolicy.
	SeverityPolicy map[Severity]SeverityRule
	// DryRun, when true, records findings but does not invoke the
	// Remediator. Used by workflow-audit run --dry-run.
	DryRun bool
}

// TreePlan describes one subtree of a Plan. Each tree has its own
// working directory and its own list of checks; per-monorepo configs
// produce one Plan with multiple Trees.
type TreePlan struct {
	WorkingDir string
	Checks     []ScheduledCheck
}

// ScheduledCheck pairs a check id with its config block. The runner
// resolves the id against the Registry at execution time so removing
// a check is observable as a "checked id missing from registry"
// error rather than a silent skip.
type ScheduledCheck struct {
	CheckID string
	Config  map[string]any
}

// SeverityRule is the per-severity policy from .dear-agent.yml
// > audits.severity-policy. The runner consults it to decide whether
// a Finding fails the audit run, what default Strategy to apply when
// the check did not specify one, and whether to notify.
type SeverityRule struct {
	FailRun         bool
	DefaultStrategy Strategy
	Notify          bool
}

// DefaultSeverityPolicy returns the package's recommended defaults.
// Operators may override per-repo; this is the fallback
// when no policy block exists.
func DefaultSeverityPolicy() map[Severity]SeverityRule {
	return map[Severity]SeverityRule{
		SeverityP0: {FailRun: true, DefaultStrategy: StrategyAuto, Notify: true},
		SeverityP1: {FailRun: true, DefaultStrategy: StrategyPR, Notify: true},
		SeverityP2: {FailRun: false, DefaultStrategy: StrategyIssue},
		SeverityP3: {FailRun: false, DefaultStrategy: StrategyNoop},
	}
}

// AuditRunRecord is the in-memory shape of one row in audit_runs.
// The store owns persistence; the runner builds and updates this
// value over the lifetime of one Plan.
type AuditRunRecord struct {
	AuditRunID       string
	Repo             string
	Cadence          Cadence
	StartedAt        time.Time
	FinishedAt       time.Time
	State            AuditRunState
	TriggeredBy      string
	FindingsNew      int
	FindingsResolved int
	FindingsOpen     int
}

// AuditRunState mirrors the CHECK constraint on audit_runs.state.
type AuditRunState string

// Audit run lifecycle states.
const (
	AuditRunRunning   AuditRunState = "running"
	AuditRunSucceeded AuditRunState = "succeeded"
	AuditRunFailed    AuditRunState = "failed"
	AuditRunPartial   AuditRunState = "partial"
)

// IsValid reports whether s names a known audit-run state.
func (s AuditRunState) IsValid() bool {
	switch s {
	case AuditRunRunning, AuditRunSucceeded, AuditRunFailed, AuditRunPartial:
		return true
	}
	return false
}

// CheckOutcome is the per-check observation produced by Runner.Run:
// the Result returned by the check plus the list of upserted Finding
// rows the store assigned. Caller-facing diagnostics in the CLI
// render against this slice.
type CheckOutcome struct {
	CheckID    string
	WorkingDir string
	Result     Result
	Findings   []Finding
	// Err is the error returned by Check.Run, if any. Non-nil Err
	// transitions the audit run state to AuditRunPartial; the rest
	// of the plan continues to run.
	Err error
}

// VerifierOutcome is the per-verifier observation produced by
// Runner.Run. Parallel in shape to CheckOutcome so the CLI rendering
// code can treat the two uniformly. Findings have been upserted via
// the Store and carry the runner-stamped verifier_role / review_depth
// keys in Evidence (Phase 6.5 trust-inversion contract).
type VerifierOutcome struct {
	VerifierName string
	WorkingDir   string
	Findings     []Finding
	// Err is the error returned by Verifier.Verify, if any. Non-nil
	// Err transitions the audit run state to AuditRunPartial; remaining
	// verifiers continue to run.
	Err error
}

// RunReport is what Run returns. The audit_runs row mirrors AuditRun;
// CheckOutcomes lists per-check detail; VerifierOutcomes lists per-
// verifier detail. Proposals is the (possibly empty) list emitted by
// registered Refiners.
type RunReport struct {
	AuditRun         AuditRunRecord
	CheckOutcomes    []CheckOutcome
	VerifierOutcomes []VerifierOutcome
	Proposals        []Proposal
}

// Run executes plan and returns a RunReport. Blocking. Honors
// ctx for cancellation; on cancel the audit_runs row is closed with
// state AuditRunFailed and the error is returned. Run does NOT
// concurrently execute checks in v1 — sequential keeps the resource
// footprint predictable; concurrency lands in a follow-up PR per
// open question 4.
func (r *Runner) Run(ctx context.Context, plan Plan) (*RunReport, error) {
	if err := r.preflight(plan); err != nil {
		return nil, err
	}
	policy := plan.SeverityPolicy
	if policy == nil {
		policy = DefaultSeverityPolicy()
	}

	auditRun := AuditRunRecord{
		AuditRunID:  r.IDGen(),
		Repo:        plan.Repo,
		Cadence:     plan.Cadence,
		StartedAt:   r.Now(),
		State:       AuditRunRunning,
		TriggeredBy: plan.TriggeredBy,
	}
	if err := r.Store.BeginAuditRun(ctx, auditRun); err != nil {
		return nil, fmt.Errorf("audit: BeginAuditRun: %w", err)
	}

	report := &RunReport{AuditRun: auditRun}
	allFindings, checkErr, policyFail := r.executePlanChecks(ctx, plan, policy, report)
	verifierFindings, verifierErr, verifierPolicyFail := r.executeVerifiers(ctx, plan, policy, report)
	allFindings = append(allFindings, verifierFindings...)
	checkErr = checkErr || verifierErr
	policyFail = policyFail || verifierPolicyFail
	r.executeRefiners(ctx, auditRun.AuditRunID, allFindings, report)

	auditRun = r.finalizeAuditRun(ctx, auditRun, plan.Repo, ctx.Err(), checkErr, policyFail)
	if err := r.Store.FinishAuditRun(ctx, auditRun); err != nil {
		return report, fmt.Errorf("audit: FinishAuditRun: %w", err)
	}
	report.AuditRun = auditRun

	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	return report, nil
}

// executePlanChecks loops every (tree, check) pair, populates report
// in place, and returns the rolled-up findings + signal flags the
// caller needs to compute the audit_runs final state.
func (r *Runner) executePlanChecks(
	ctx context.Context,
	plan Plan,
	policy map[Severity]SeverityRule,
	report *RunReport,
) (allFindings []Finding, checkErr, policyFail bool) {
	allFindings = make([]Finding, 0, 64)
	for _, tree := range plan.Trees {
		for _, sc := range tree.Checks {
			outcome := r.runOneCheck(ctx, plan, tree, sc, policy)
			if outcome.Err != nil {
				checkErr = true
			}
			for i := range outcome.Findings {
				if rule, ok := policy[outcome.Findings[i].Severity]; ok && rule.FailRun {
					policyFail = true
				}
				allFindings = append(allFindings, outcome.Findings[i])
			}
			report.CheckOutcomes = append(report.CheckOutcomes, outcome)
		}
	}
	return allFindings, checkErr, policyFail
}

// executeVerifiers walks every registered Verifier over each tree of
// the plan, persists their findings via the store, and stamps the
// runner-owned Evidence keys (verifier_role, review_depth). Mirrors
// executePlanChecks shape so the runner state machine treats the two
// uniformly.
//
// A verifier returning an error is recorded as VerifierOutcome.Err
// and triggers the AuditRunPartial transition the same way a Check
// error does. A verifier that returns Findings whose Severity meets
// the policy.FailRun bar triggers policyFail just like a Check.
func (r *Runner) executeVerifiers(
	ctx context.Context,
	plan Plan,
	policy map[Severity]SeverityRule,
	report *RunReport,
) (allFindings []Finding, verifierErr, policyFail bool) {
	verifiers := r.Registry.Verifiers()
	if len(verifiers) == 0 {
		return nil, false, false
	}
	allFindings = make([]Finding, 0, len(verifiers))
	for _, tree := range plan.Trees {
		for _, v := range verifiers {
			outcome := r.runOneVerifier(ctx, plan, tree, v, policy)
			if outcome.Err != nil {
				verifierErr = true
			}
			for i := range outcome.Findings {
				if rule, ok := policy[outcome.Findings[i].Severity]; ok && rule.FailRun {
					policyFail = true
				}
				allFindings = append(allFindings, outcome.Findings[i])
			}
			report.VerifierOutcomes = append(report.VerifierOutcomes, outcome)
		}
	}
	return allFindings, verifierErr, policyFail
}

// runOneVerifier executes one Verifier against one tree, stamps the
// verifier-role / review-depth Evidence keys, and persists findings
// via the store. Pure with respect to runner aggregation state.
func (r *Runner) runOneVerifier(
	ctx context.Context,
	plan Plan,
	tree TreePlan,
	v Verifier,
	policy map[Severity]SeverityRule,
) VerifierOutcome {
	logger := r.Logger.With("verifier", v.Name(), "tree", tree.WorkingDir)

	target := VerifyTarget{
		RepoRoot:   plan.RepoRoot,
		WorkingDir: tree.WorkingDir,
	}

	verifyCtx := ctx
	var cancel context.CancelFunc
	if r.CheckTimeout > 0 {
		verifyCtx, cancel = context.WithTimeout(ctx, r.CheckTimeout)
		defer cancel()
	}

	findings, err := v.Verify(verifyCtx, target)
	outcome := VerifierOutcome{VerifierName: v.Name(), WorkingDir: tree.WorkingDir, Err: err}
	if err != nil {
		logger.Warn("audit: verifier returned error", "err", err)
		return outcome
	}

	depth := v.ReviewDepth()
	if depth == "" {
		depth = ReviewDepthAdversarial
	}

	for i := range findings {
		stored, ok := r.persistVerifierFinding(ctx, findings[i], plan, v, depth, policy, logger)
		if !ok {
			continue
		}
		outcome.Findings = append(outcome.Findings, stored)
	}
	return outcome
}

// persistVerifierFinding stamps the runner-owned Evidence keys onto a
// Finding produced by a Verifier and upserts via the Store. Mirrors
// persistFinding but treats the verifier name as the CheckID so the
// (repo, fingerprint) uniqueness contract on audit_findings carries a
// stable namespace. Verifier-emitted CheckIDs are conventionally
// prefixed "verify." to distinguish them in CLI rendering.
func (r *Runner) persistVerifierFinding(
	ctx context.Context,
	f Finding,
	plan Plan,
	v Verifier,
	depth string,
	policy map[Severity]SeverityRule,
	logger *slog.Logger,
) (Finding, bool) {
	f.Repo = plan.Repo
	f.CheckID = "verify." + v.Name()
	if f.Evidence == nil {
		f.Evidence = make(map[string]any, 2)
	}
	if _, ok := f.Evidence[EvidenceVerifierRole]; !ok {
		f.Evidence[EvidenceVerifierRole] = v.Name()
	}
	if _, ok := f.Evidence[EvidenceReviewDepth]; !ok {
		f.Evidence[EvidenceReviewDepth] = depth
	}
	// Trust-inversion contract (ROADMAP §6.5): an *adversarial* review
	// promotes the finding into the verified set. We resolve the depth
	// the same way the runner did above — caller-set overrides win —
	// and only stamp verified_at when the effective depth is
	// adversarial. Casual-depth verifiers (and Checks, which never run
	// through this path) do not earn the timestamp.
	if effectiveDepth, _ := f.Evidence[EvidenceReviewDepth].(string); effectiveDepth == ReviewDepthAdversarial {
		if _, ok := f.Evidence[EvidenceVerifiedAt]; !ok {
			f.Evidence[EvidenceVerifiedAt] = r.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	if !f.Severity.IsValid() {
		// Default to P2 — verifier findings are drift / hypothesis-grade
		// unless the verifier explicitly raises severity.
		f.Severity = SeverityP2
	}
	if err := f.Validate(); err != nil {
		logger.Warn("audit: invalid verifier finding emitted; skipping", "err", err)
		return Finding{}, false
	}
	if f.Suggested.Strategy == StrategyUnspecified {
		if rule, ok := policy[f.Severity]; ok {
			f.Suggested.Strategy = rule.DefaultStrategy
		}
	}
	stored, upErr := r.Store.UpsertFinding(ctx, f)
	if upErr != nil {
		logger.Warn("audit: UpsertFinding (verifier) failed", "err", upErr, "fp", f.Fingerprint)
		return Finding{}, false
	}
	return stored, true
}

// executeRefiners walks every registered Refiner over the pooled
// findings and persists their Proposals to the store, appending
// successfully-stored proposals onto report.
func (r *Runner) executeRefiners(ctx context.Context, auditRunID string, findings []Finding, report *RunReport) {
	for _, refiner := range r.Registry.Refiners() {
		props, err := refiner.Propose(ctx, findings)
		if err != nil {
			r.Logger.Warn("audit: refiner proposal failed", "refiner", refiner.Name(), "err", err)
			continue
		}
		for i := range props {
			props[i].AuditRunID = auditRunID
			props[i].State = ProposalProposed
			props[i].ProposedAt = r.Now()
			if vErr := props[i].Validate(); vErr != nil {
				r.Logger.Warn("audit: refiner produced invalid proposal", "refiner", refiner.Name(), "err", vErr)
				continue
			}
			id, upErr := r.Store.UpsertProposal(ctx, props[i])
			if upErr != nil {
				r.Logger.Warn("audit: persist proposal failed", "refiner", refiner.Name(), "err", upErr)
				continue
			}
			props[i].ProposalID = id
			report.Proposals = append(report.Proposals, props[i])
		}
	}
}

// finalizeAuditRun computes the final audit_runs row state and counts.
// Pure: takes everything it needs as parameters, returns the updated
// record. Caller persists.
func (r *Runner) finalizeAuditRun(
	ctx context.Context,
	rec AuditRunRecord,
	repo string,
	ctxErr error,
	checkErr, policyFail bool,
) AuditRunRecord {
	counts, err := r.Store.CountFindings(ctx, repo)
	if err != nil {
		r.Logger.Warn("audit: CountFindings failed; counts may be stale", "err", err)
	}
	rec.FinishedAt = r.Now()
	rec.FindingsOpen = counts.Open
	rec.FindingsNew = counts.New
	rec.FindingsResolved = counts.Resolved

	switch {
	case ctxErr != nil:
		rec.State = AuditRunFailed
	case policyFail:
		rec.State = AuditRunFailed
	case checkErr:
		rec.State = AuditRunPartial
	default:
		rec.State = AuditRunSucceeded
	}
	return rec
}

// preflight validates plan and runner configuration before any
// state changes. Returns the first error encountered. Splits into
// two halves so each stays under the gocyclo threshold.
func (r *Runner) preflight(plan Plan) error {
	if err := r.preflightRunner(); err != nil {
		return err
	}
	return preflightPlan(plan, r.Registry)
}

// preflightRunner validates the runner's own wiring. Side effect:
// populates Remediator and Logger with safe defaults when callers
// left them nil.
func (r *Runner) preflightRunner() error {
	if r.Store == nil {
		return errors.New("audit: Runner.Store is nil")
	}
	if r.Registry == nil {
		return errors.New("audit: Runner.Registry is nil")
	}
	if r.IDGen == nil {
		return errors.New("audit: Runner.IDGen is nil")
	}
	if r.Now == nil {
		return errors.New("audit: Runner.Now is nil")
	}
	if r.Remediator == nil {
		r.Remediator = NewNoopRemediator()
	}
	if r.Logger == nil {
		r.Logger = slog.Default()
	}
	return nil
}

// preflightPlan validates a Plan against the registry. Pure: no
// runner state mutated.
func preflightPlan(plan Plan, registry *Registry) error {
	if plan.Repo == "" {
		return errors.New("audit: Plan.Repo is empty")
	}
	if plan.RepoRoot == "" {
		return errors.New("audit: Plan.RepoRoot is empty")
	}
	if !plan.Cadence.IsValid() {
		return fmt.Errorf("audit: Plan.Cadence %q invalid", plan.Cadence)
	}
	if len(plan.Trees) == 0 {
		return errors.New("audit: Plan.Trees is empty")
	}
	for ti, tree := range plan.Trees {
		if tree.WorkingDir == "" {
			return fmt.Errorf("audit: Plan.Trees[%d].WorkingDir is empty", ti)
		}
		for ci, sc := range tree.Checks {
			if sc.CheckID == "" {
				return fmt.Errorf("audit: Plan.Trees[%d].Checks[%d].CheckID is empty", ti, ci)
			}
			if _, ok := registry.Lookup(sc.CheckID); !ok {
				return fmt.Errorf("audit: Plan.Trees[%d].Checks[%d]: unknown check %q", ti, ci, sc.CheckID)
			}
		}
	}
	return nil
}

// runOneCheck executes one ScheduledCheck against one tree. Pure with
// respect to state — caller handles aggregation. Errors from the
// Check are captured on outcome.Err so the runner can downgrade the
// audit_runs.state to "partial" without losing other checks.
func (r *Runner) runOneCheck(
	ctx context.Context,
	plan Plan,
	tree TreePlan,
	sc ScheduledCheck,
	policy map[Severity]SeverityRule,
) CheckOutcome {
	check, _ := r.Registry.Lookup(sc.CheckID) // existence verified in preflight
	meta := check.Meta()
	logger := r.Logger.With("check", meta.ID, "tree", tree.WorkingDir)

	env := Env{
		RepoRoot:   plan.RepoRoot,
		WorkingDir: tree.WorkingDir,
		Config:     sc.Config,
		Logger:     logger,
		Now:        r.Now,
	}

	checkCtx := ctx
	var cancel context.CancelFunc
	if r.CheckTimeout > 0 {
		checkCtx, cancel = context.WithTimeout(ctx, r.CheckTimeout)
		defer cancel()
	}

	start := r.Now()
	result, err := check.Run(checkCtx, env)
	result.Duration = r.Now().Sub(start)

	outcome := CheckOutcome{CheckID: meta.ID, WorkingDir: tree.WorkingDir, Result: result, Err: err}
	if err != nil {
		logger.Warn("audit: check returned error", "err", err)
		return outcome
	}
	if result.Status == StatusSkipped || result.Status == StatusTimeout {
		return outcome
	}

	// Persist findings via the store. Apply the per-check severity
	// ceiling and resolve unspecified strategies via policy.
	for i := range result.Findings {
		stored, ok := r.persistFinding(ctx, result.Findings[i], plan, meta, policy, env, logger)
		if !ok {
			continue
		}
		outcome.Findings = append(outcome.Findings, stored)
	}
	// Sort findings by severity (most severe first) for stable CLI
	// rendering — checks may emit in any order.
	sort.Slice(outcome.Findings, func(i, j int) bool {
		return outcome.Findings[i].Severity.Rank() < outcome.Findings[j].Severity.Rank()
	})
	return outcome
}

// persistFinding handles the per-finding pipeline pulled out of
// runOneCheck: stamp metadata, clamp severity, validate, upsert, and
// (optionally) trigger inline auto-remediation. Returns the stored
// finding plus an ok=false signal when the finding was dropped.
func (r *Runner) persistFinding(
	ctx context.Context,
	f Finding,
	plan Plan,
	meta CheckMeta,
	policy map[Severity]SeverityRule,
	env Env,
	logger *slog.Logger,
) (Finding, bool) {
	f.Repo = plan.Repo
	f.CheckID = meta.ID
	f.Severity = ClampSeverity(f.Severity, meta.SeverityCeiling)
	if err := f.Validate(); err != nil {
		logger.Warn("audit: invalid finding emitted; skipping", "err", err)
		return Finding{}, false
	}
	if f.Suggested.Strategy == StrategyUnspecified {
		if rule, ok := policy[f.Severity]; ok {
			f.Suggested.Strategy = rule.DefaultStrategy
		}
	}
	stored, upErr := r.Store.UpsertFinding(ctx, f)
	if upErr != nil {
		logger.Warn("audit: UpsertFinding failed", "err", upErr, "fp", f.Fingerprint)
		return Finding{}, false
	}
	if !plan.DryRun && stored.Suggested.Strategy == StrategyAuto && r.Remediator != nil {
		r.applyInlineRemediation(ctx, stored, env, logger)
	}
	return stored, true
}

// applyInlineRemediation runs the dormant remediator seam and, when it reports
// a valid state change, passes only the state and note to the store. Status and
// Reference are ignored; Note is also ignored for an invalid or unchanged
// state. The outcome is therefore not durable evidence. Errors are logged;
// nothing here is allowed to fail the audit run because the finding is already
// persisted.
func (r *Runner) applyInlineRemediation(ctx context.Context, stored Finding, env Env, logger *slog.Logger) {
	if remErr := stored.Suggested.Validate(); remErr != nil {
		logger.Warn("audit: remediation invalid; skipping", "err", remErr)
		return
	}
	out, applyErr := r.Remediator.Apply(ctx, stored, env)
	if applyErr != nil {
		logger.Warn("audit: remediation failed", "err", applyErr)
		return
	}
	if !out.State.IsValid() || out.State == stored.State {
		return
	}
	if _, err := r.Store.SetFindingState(ctx, stored.FindingID, out.State, out.Note); err != nil {
		logger.Warn("audit: SetFindingState failed", "err", err)
	}
}
