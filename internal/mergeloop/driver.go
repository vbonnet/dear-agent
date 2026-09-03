package mergeloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PRLister fetches open PRs for a repo, normalized for classification.
type PRLister interface {
	// ListOpen returns up to maxOpen+1 open PRs (so the caller can detect that
	// the open count exceeds a backpressure cap of maxOpen).
	ListOpen(ctx context.Context, repo string, maxOpen int) ([]PR, error)
}

// Rebaser brings a PR's branch up to date with its base (mechanical).
type Rebaser interface {
	Rebase(ctx context.Context, repo string, pr int) error
}

// Merger performs the gated, irreversible squash-merge. The real implementation
// delegates to the vetted safe-merge wrapper (ADR-029, AGENTS.md principle 9).
// A non-nil error that is "not ready yet" (soak/bot/threads) is reported via
// ErrNotReady so the loop treats it as wait-and-retry, not a hard failure.
type Merger interface {
	Merge(ctx context.Context, repo string, pr int) error
}

// AgentSpawner launches and tracks scoped agent sessions that edit code.
type AgentSpawner interface {
	// ActiveSession reports whether an agent is already driving this PR.
	ActiveSession(ctx context.Context, repo string, pr int) (bool, error)
	// Spawn launches an agent for the request and returns the session name.
	// It returns ErrSpawnUnavailable when the agent substrate cannot run
	// (e.g. missing auth) so the loop can defer rather than block.
	Spawn(ctx context.Context, req AgentRequest) (string, error)
}

// ThreadResolver resolves review threads on a PR. In unattended dark-factory
// mode, bot review threads (e.g. from gemini-code-assist) block the merge
// gate because the repo enforces required_conversation_resolution. This
// interface lets the mergeloop resolve them before attempting the merge.
type ThreadResolver interface {
	// ResolveBotThreads resolves unresolved review threads authored by known
	// bot accounts on the given PR. Returns the number of threads resolved.
	// Human-authored threads are never touched.
	ResolveBotThreads(ctx context.Context, repo string, pr int) (int, error)
}

// AgentKind distinguishes the two code-editing tasks the loop delegates.
type AgentKind string

const (
	// AgentFixCI — diagnose and fix a failing required check, then push.
	AgentFixCI AgentKind = "fix-ci"
	// AgentResolveConflict — resolve merge conflicts against the base, push.
	AgentResolveConflict AgentKind = "resolve-conflict"
)

// AgentRequest is a scoped, self-contained instruction for a spawned agent.
type AgentRequest struct {
	Repo        string
	PRNumber    int
	Branch      string
	Kind        AgentKind
	SessionName string // e.g. "mergeloop/pr-432"
	FailureSig  string // short signature of the failure being fixed
	Prompt      string // full DoD-bearing prompt
}

// ErrNotReady signals a merge could not proceed for a soft, transient reason
// (soak window, bot review, unresolved threads). The loop retries next tick.
var ErrNotReady = fmt.Errorf("merge not ready yet")

// ErrSpawnUnavailable signals the agent substrate is unavailable (e.g. auth).
var ErrSpawnUnavailable = fmt.Errorf("agent spawn unavailable")

// AuditEvent is one JSONL line in the merge-loop audit log.
type AuditEvent struct {
	Timestamp string `json:"timestamp"`
	Repo      string `json:"repo"`
	PR        int    `json:"pr,omitempty"`
	State     State  `json:"state,omitempty"`
	Action    string `json:"action"`
	Detail    string `json:"detail,omitempty"`
}

// Deps bundles the injected collaborators. Clock and Audit default to sane
// implementations when nil; Metrics and ThreadResolver are nil-safe.
type Deps struct {
	Lister  PRLister
	Rebaser Rebaser
	Merger  Merger
	Spawner AgentSpawner
	Threads ThreadResolver // nil = no thread resolution before merge
	Clock   func() time.Time
	Audit   func(AuditEvent)
	Metrics *Metrics
}

// DefaultRebaseCooldown is the default minimum delay between two rebases of the
// same PR. It is sized above the slowest required check in this repository
// (Build & Test (ubuntu-latest), measured at ~23 minutes) so that a rebase the
// loop performs has time to produce a verdict before the loop considers
// rebasing again.
const DefaultRebaseCooldown = 30 * time.Minute

// Driver drives one repo's open PRs toward MERGED.
type Driver struct {
	Repo           string
	Policy         Policy
	Tracker        *Tracker
	Deps           Deps
	Cap            int           // backpressure: skip the tick above this many open PRs
	StallThreshold time.Duration // a PR actionable with no action for longer is "stalled"

	// RebaseCooldown is the minimum time between two rebases of the same PR.
	//
	// Under a strict required-status-checks policy every PR must be up to date
	// with its base to merge, so a busy main leaves most PRs BEHIND. Advancing a
	// branch pushes a new head, which invalidates the whole check matrix and
	// restarts a full CI run. When the loop re-rebases a PR sooner than CI takes
	// to finish, the PR is reset before it can ever present a green, up-to-date
	// check set, and it can never merge no matter how many ticks run.
	//
	// The cooldown must therefore exceed the slowest required check. Zero
	// disables it and restores the previous rebase-every-tick behaviour.
	RebaseCooldown time.Duration
}

// TickResult summarizes one pass.
type TickResult struct {
	OpenPRs     int
	Merged      int
	Rebased     int
	AgentsSpawn int
	Escalated   int
	Stalled     int
	Skipped     int
	Actions     map[State]int
}

func (d *Driver) now() time.Time {
	if d.Deps.Clock != nil {
		return d.Deps.Clock()
	}
	return time.Now()
}

func (d *Driver) audit(ev AuditEvent) {
	ev.Timestamp = d.now().UTC().Format(time.RFC3339)
	ev.Repo = d.Repo
	if d.Deps.Audit != nil {
		d.Deps.Audit(ev)
		return
	}
	appendAudit(ev)
}

// Tick performs one idempotent pass over all open PRs. It never returns an
// error for an individual PR's failure — those are audited and the loop
// continues (anti-stall: keep going). It returns an error only when the world
// is unworkable (cannot even list PRs).
func (d *Driver) Tick(ctx context.Context) (TickResult, error) {
	ctx, endSpan := d.Deps.Metrics.StartTickSpan(ctx, d.Repo)
	defer endSpan()

	res := TickResult{Actions: map[State]int{}}
	maxOpen := d.Cap
	if maxOpen <= 0 {
		maxOpen = 50
	}
	prs, err := d.Deps.Lister.ListOpen(ctx, d.Repo, maxOpen)
	if err != nil {
		return res, fmt.Errorf("listing open PRs: %w", err)
	}
	res.OpenPRs = len(prs)

	if len(prs) > maxOpen {
		d.audit(AuditEvent{Action: "backpressure",
			Detail: fmt.Sprintf("%d open PRs exceed cap %d", len(prs), maxOpen)})
		return res, nil
	}

	for _, pr := range prs {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		st := d.drivePR(ctx, pr, &res)
		res.Actions[st]++
	}

	d.Deps.Metrics.recordTick(ctx, res)
	if err := d.Tracker.Save(); err != nil {
		d.audit(AuditEvent{Action: "state_save_error", Detail: err.Error()})
	}
	return res, nil
}

// drivePR classifies one PR and performs at most one action. It returns the
// classified state.
func (d *Driver) drivePR(ctx context.Context, pr PR, res *TickResult) State {
	now := d.now()
	rec := d.Tracker.Get(pr.Number, now)

	agentActive := false
	if d.Deps.Spawner != nil {
		if active, err := d.Deps.Spawner.ActiveSession(ctx, d.Repo, pr.Number); err == nil {
			agentActive = active
		}
	}

	// Reset the agent-attempt budget when the failure signature has changed
	// since we last spawned, or clear the failure episode when CI has passed.
	// Projection errors do not establish a current failure signature. Preserve
	// the existing attempt budget through provider outages and ordinary pending
	// reruns; reset it when CI passes or when an actual failing check set proves
	// that a different failure is now actionable.
	if pr.CheckProjectionError == "" {
		if pr.requiredVerdict() == CheckPass {
			d.Tracker.ResetAttemptsIfPassing(pr.Number)
		} else if pr.requiredVerdict() == CheckFail {
			d.Tracker.ResetAttemptsIfSigChanged(pr.Number, failureSignature(pr))
		}
	}

	cls := d.Policy.Classify(pr, rec.AgentAttempts, agentActive)

	// Stall detection: an actionable PR untouched for longer than the
	// threshold is the failure the Define rule forbids.
	if d.isStalled(cls.State, rec, now) {
		res.Stalled++
		d.Deps.Metrics.recordStall(ctx, pr.Number, cls.State)
		d.audit(AuditEvent{PR: pr.Number, State: cls.State, Action: "stall_detected",
			Detail: fmt.Sprintf("no action since %s", rec.LastActionAt.Format(time.RFC3339))})
	}

	switch cls.State {
	case StateDraft, StateAgentInFlight, StateCIPending:
		res.Skipped++
		return cls.State

	case StateBlockedPolicy:
		res.Escalated++
		d.escalate(ctx, pr, cls, now)
		return cls.State

	case StateAbandoned:
		res.Escalated++
		d.escalate(ctx, pr, cls, now)
		return cls.State

	case StateBehind:
		d.doRebase(ctx, pr, now, res)
		return cls.State

	case StateConflicted:
		d.doSpawn(ctx, pr, AgentResolveConflict, "conflict", now, res)
		return cls.State

	case StateCIFailing:
		d.doSpawn(ctx, pr, AgentFixCI, failureSignature(pr), now, res)
		return cls.State

	case StateGreen:
		d.resolveBotThreads(ctx, pr)
		d.doMerge(ctx, pr, now, res)
		return cls.State
	}
	return cls.State
}

func (d *Driver) isStalled(st State, rec *PRRecord, now time.Time) bool {
	if d.StallThreshold <= 0 || rec.LastActionAt.IsZero() {
		return false
	}
	switch st {
	case StateBehind, StateConflicted, StateCIFailing, StateGreen:
		return now.Sub(rec.LastActionAt) > d.StallThreshold
	case StateDraft, StateAbandoned, StateBlockedPolicy, StateAgentInFlight, StateCIPending:
		return false
	}
	return false
}

// rebaseCoolingDown reports whether this PR was rebased recently enough that
// the CI run that rebase started has probably not finished yet. Rebasing again
// now would discard that in-flight run and start the wait over.
func (d *Driver) rebaseCoolingDown(rec *PRRecord, now time.Time) bool {
	if d.RebaseCooldown <= 0 || rec == nil {
		return false
	}
	if rec.LastState != StateBehind || rec.LastActionAt.IsZero() {
		return false
	}
	return now.Sub(rec.LastActionAt) < d.RebaseCooldown
}

func (d *Driver) doRebase(ctx context.Context, pr PR, now time.Time, res *TickResult) {
	if d.Deps.Rebaser == nil {
		return
	}
	if d.rebaseCoolingDown(d.Tracker.Get(pr.Number, now), now) {
		res.Skipped++
		d.audit(AuditEvent{PR: pr.Number, State: StateBehind, Action: "rebase_cooldown",
			Detail: fmt.Sprintf("rebased less than %s ago; letting CI settle", d.RebaseCooldown)})
		return
	}
	err := d.Deps.Rebaser.Rebase(ctx, d.Repo, pr.Number)
	d.Tracker.RecordAction(pr.Number, StateBehind, now)
	if err != nil {
		d.audit(AuditEvent{PR: pr.Number, State: StateBehind, Action: "rebase_error", Detail: err.Error()})
		return
	}
	res.Rebased++
	d.audit(AuditEvent{PR: pr.Number, State: StateBehind, Action: "rebased"})
}

func (d *Driver) resolveBotThreads(ctx context.Context, pr PR) {
	if d.Deps.Threads == nil {
		return
	}
	resolved, err := d.Deps.Threads.ResolveBotThreads(ctx, d.Repo, pr.Number)
	if err != nil {
		d.audit(AuditEvent{PR: pr.Number, State: StateGreen, Action: "thread_resolve_error", Detail: err.Error()})
		return
	}
	if resolved > 0 {
		d.audit(AuditEvent{PR: pr.Number, State: StateGreen, Action: "bot_threads_resolved",
			Detail: fmt.Sprintf("resolved %d bot review thread(s)", resolved)})
	}
}

func (d *Driver) doMerge(ctx context.Context, pr PR, now time.Time, res *TickResult) {
	if d.Deps.Merger == nil {
		return
	}
	err := d.Deps.Merger.Merge(ctx, d.Repo, pr.Number)
	d.Tracker.RecordAction(pr.Number, StateGreen, now)
	if err != nil {
		// "Not ready" (soak/bot/threads) is a wait, not a failure.
		action := "merge_error"
		if isNotReady(err) {
			action = "merge_deferred"
		}
		d.audit(AuditEvent{PR: pr.Number, State: StateGreen, Action: action, Detail: err.Error()})
		return
	}
	res.Merged++
	d.Deps.Metrics.recordMerge(ctx, pr.Number, now.Sub(d.Tracker.Get(pr.Number, now).FirstSeenAt))
	d.audit(AuditEvent{PR: pr.Number, State: StateGreen, Action: "merged"})
	d.Tracker.Forget(pr.Number)
}

func (d *Driver) doSpawn(ctx context.Context, pr PR, kind AgentKind, failSig string, now time.Time, res *TickResult) {
	if d.Deps.Spawner == nil {
		return
	}
	req := AgentRequest{
		Repo:        d.Repo,
		PRNumber:    pr.Number,
		Branch:      pr.HeadRefName,
		Kind:        kind,
		SessionName: SessionName(pr.Number),
		FailureSig:  failSig,
		Prompt:      BuildPrompt(d.Repo, pr, kind),
	}
	session, err := d.Deps.Spawner.Spawn(ctx, req)
	if err != nil {
		if isSpawnUnavailable(err) {
			// Defer-don't-block: record a handoff, escalate, keep going.
			d.Tracker.RecordEscalation(pr.Number, "agent substrate unavailable: "+err.Error(), now)
			res.Escalated++
			d.Deps.Metrics.recordEscalation(ctx, pr.Number, "spawn_unavailable")
			d.audit(AuditEvent{PR: pr.Number, Action: "spawn_deferred", Detail: err.Error()})
			return
		}
		d.audit(AuditEvent{PR: pr.Number, Action: "spawn_error", Detail: err.Error()})
		return
	}
	d.Tracker.RecordAgentSpawn(pr.Number, failSig, session, now)
	res.AgentsSpawn++
	d.Deps.Metrics.recordAgentSpawn(ctx, pr.Number, string(kind))
	d.audit(AuditEvent{PR: pr.Number, Action: "agent_spawned",
		Detail: fmt.Sprintf("kind=%s session=%s sig=%s", kind, session, failSig)})
}

func (d *Driver) escalate(ctx context.Context, pr PR, cls Classification, now time.Time) {
	d.Tracker.RecordEscalation(pr.Number, cls.Reason, now)
	d.Deps.Metrics.recordEscalation(ctx, pr.Number, string(cls.State))
	d.audit(AuditEvent{PR: pr.Number, State: cls.State, Action: "escalated", Detail: cls.Reason})
}

// SessionName is the deterministic AGM session name for a PR's agent, used both
// to spawn and to detect an in-flight agent.
func SessionName(pr int) string { return fmt.Sprintf("mergeloop/pr-%d", pr) }

// failureSignature is a short, stable key for the current failure so repeated
// identical failures count toward the attempt cap but a changed failure resets
// it. It is the sorted set of failing required check names.
func failureSignature(pr PR) string {
	var names []string
	for _, c := range pr.requiredChecks() {
		if c.Verdict == CheckFail {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return "unknown"
	}
	// Simple insertion sort to avoid importing sort for a tiny slice.
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j-1] > names[j]; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return strings.Join(names, ",")
}

func isNotReady(err error) bool         { return errors.Is(err, ErrNotReady) }
func isSpawnUnavailable(err error) bool { return errors.Is(err, ErrSpawnUnavailable) }

// appendAudit writes one JSONL line to the merge-loop audit log, mirroring the
// safe-merge audit convention.
func appendAudit(ev AuditEvent) {
	dir := StateDir()
	if d := os.Getenv("MERGELOOP_AUDIT_DIR"); d != "" {
		dir = d
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "mergeloop-audit.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	if data, err := json.Marshal(ev); err == nil {
		if _, werr := f.Write(append(data, '\n')); werr != nil {
			_ = f.Close()
			return
		}
	}
	// Explicitly check the Close error on this writable handle rather than
	// deferring an error-ignoring close: a failed Close can mask data loss
	// (flushed buffers, delayed write errors). Nothing is actionable for a
	// best-effort audit log beyond returning, but the error must be observed.
	if cerr := f.Close(); cerr != nil {
		return
	}
}
