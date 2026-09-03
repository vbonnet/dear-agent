package agenticreview

import (
	"strings"
	"testing"
	"time"
)

var (
	// readyAt is the moment the pull request became reviewable. Every test
	// clock below is expressed relative to it so a dispatch window and a
	// verdict window are never confused with one another.
	readyAt = time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	// insideWindow is early enough that neither the dispatch deadline nor the
	// verdict deadline has expired.
	insideWindow = readyAt.Add(5 * time.Minute)
	// pastEveryWindow is late enough that both deadlines have expired.
	pastEveryWindow = readyAt.Add(6 * time.Hour)
)

// testConfig is the shipped default shape: three families, quorum two.
func testConfig() Config {
	return Config{
		Families:        DefaultFamilies,
		Quorum:          2,
		VerdictTimeout:  45 * time.Minute,
		DispatchTimeout: 30 * time.Minute,
	}
}

// lifecycle builds the label set a family emits as it walks its lifecycle,
// so a test states the story rather than a list of strings.
func lifecycle(f Family, phases ...Phase) []string {
	out := make([]string, 0, len(phases))
	for _, p := range phases {
		out = append(out, Label(f, p))
	}
	return out
}

func inputAt(now time.Time, labelSets ...[]string) Input {
	in := Input{Now: now, ReadyAt: readyAt, AppliedAt: map[string]time.Time{}}
	for _, set := range labelSets {
		for _, name := range set {
			in.Labels = append(in.Labels, name)
			// Absent a more specific clock, a label is treated as having
			// landed the moment the pull request went ready.
			in.AppliedAt[name] = readyAt
		}
	}
	return in
}

func familyState(t *testing.T, v Verdict, f Family) FamilyVerdict {
	t.Helper()
	for _, fv := range v.Families {
		if fv.Family == f {
			return fv
		}
	}
	t.Fatalf("verdict has no entry for family %q", f)
	return FamilyVerdict{}
}

// (a) All three families approved: the gate opens.
func TestEvaluateAllThreeApprovedIsMergeable(t *testing.T) {
	in := inputAt(insideWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhasePosted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted, PhasePosted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhasePosted, PhaseApproved),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !v.Mergeable() {
		t.Fatalf("verdict = %s (%s), want mergeable", v.Decision, v.Reason)
	}
	if v.Approved != 3 || v.Down != 0 {
		t.Fatalf("approved=%d down=%d, want 3/0", v.Approved, v.Down)
	}
}

// (b) One family requested changes while another approved: blocked. This is
// the masking failure the per-family schema exists to prevent — a global
// label set would have shown only the approval.
func TestEvaluateChangesRequestedBlocksDespiteOtherApprovals(t *testing.T) {
	in := inputAt(insideWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhasePosted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhasePosted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted, PhasePosted, PhaseChangesRequested),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked", v.Decision)
	}
	if v.Decision != DecisionBlock {
		t.Fatalf("decision = %s, want %s", v.Decision, DecisionBlock)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StateBlocking {
		t.Fatalf("codex state = %s, want %s", got, StateBlocking)
	}
	if !strings.Contains(v.Reason, "codex") {
		t.Fatalf("reason %q does not name the blocking family", v.Reason)
	}
}

// A requested change outranks a quorum that would otherwise be satisfied: two
// approvals plus an explicit rejection is still a rejection.
func TestEvaluateChangesRequestedOutranksSatisfiedQuorum(t *testing.T) {
	cfg := testConfig()
	cfg.Quorum = 1
	in := inputAt(insideWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted, PhaseChangesRequested),
	)

	v, err := cfg.Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked even at quorum 1", v.Decision)
	}
}

// (c) One family is explicitly down and the other two approved: quorum carries
// the merge.
func TestEvaluateQuorumCarriesMergeWhenOneFamilyErrored(t *testing.T) {
	in := inputAt(insideWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhasePosted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhasePosted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted, PhaseError),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !v.Mergeable() {
		t.Fatalf("verdict = %s (%s), want mergeable on quorum", v.Decision, v.Reason)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StateDown {
		t.Fatalf("codex state = %s, want %s", got, StateDown)
	}
	if v.Approved != 2 || v.Down != 1 {
		t.Fatalf("approved=%d down=%d, want 2/1", v.Approved, v.Down)
	}
}

// A family that started and then went quiet past the verdict deadline is down
// by timeout, with no explicit error label needed. This is the quota-exhaustion
// shape where the reviewer dies before it can report anything.
func TestEvaluateSilentFamilyIsDownPastVerdictTimeout(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !v.Mergeable() {
		t.Fatalf("verdict = %s (%s), want mergeable", v.Decision, v.Reason)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StateDown {
		t.Fatalf("codex state = %s, want %s", got, StateDown)
	}
}

// The same silent family inside its window is pending, not down: the merge
// window this gate exists to close stays shut while a review may still land.
func TestEvaluateSilentFamilyIsPendingInsideVerdictTimeout(t *testing.T) {
	in := inputAt(insideWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked while codex may still report", v.Decision)
	}
	if v.Decision != DecisionPending {
		t.Fatalf("decision = %s, want %s", v.Decision, DecisionPending)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StatePending {
		t.Fatalf("codex state = %s, want %s", got, StatePending)
	}
}

// A family that posted a review body but has not yet published a verdict keeps
// its own clock: posting is evidence it ran, not evidence it passed.
func TestEvaluatePostedWithoutVerdictIsPendingAndNotApproval(t *testing.T) {
	in := inputAt(insideWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted, PhasePosted),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked; posted is not approved", v.Decision)
	}
	if v.Approved != 2 {
		t.Fatalf("approved=%d, want 2 — a posted review must not count as one", v.Approved)
	}
}

// A posted label refreshes the verdict deadline: a reviewer that got its body
// onto the pull request late still deserves its full window to publish a
// verdict.
func TestEvaluatePostedLabelExtendsVerdictDeadline(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
	)
	in.Labels = append(in.Labels,
		Label(FamilyCodex, PhaseStarted), Label(FamilyCodex, PhasePosted))
	in.AppliedAt[Label(FamilyCodex, PhaseStarted)] = readyAt
	in.AppliedAt[Label(FamilyCodex, PhasePosted)] = pastEveryWindow.Add(-1 * time.Minute)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StatePending {
		t.Fatalf("codex state = %s, want %s", got, StatePending)
	}
}

// (d) The pull request is ready but no family has started: not mergeable. This
// is the window between ready_for_review and the first reviewer dispatch.
func TestEvaluateReadyWithoutStartedIsNotMergeable(t *testing.T) {
	in := inputAt(insideWindow)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked with no review started", v.Decision)
	}
	if v.Decision != DecisionPending {
		t.Fatalf("decision = %s, want %s", v.Decision, DecisionPending)
	}
	for _, f := range DefaultFamilies {
		if got := familyState(t, v, f).State; got != StateMissing {
			t.Fatalf("%s state = %s, want %s", f, got, StateMissing)
		}
	}
}

// Two families approved but the third never started, still inside its dispatch
// window: blocked. Quorum must not paper over a reviewer that has simply not
// been given its chance yet.
func TestEvaluateUnstartedFamilyBlocksInsideDispatchWindow(t *testing.T) {
	in := inputAt(insideWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked; codex has not been dispatched yet", v.Decision)
	}
}

// Past the dispatch window an unstarted family is down, not missing: a
// reviewer that never came up at all degrades the same way as one that died
// mid-review.
func TestEvaluateUnstartedFamilyIsDownPastDispatchWindow(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !v.Mergeable() {
		t.Fatalf("verdict = %s (%s), want mergeable on quorum", v.Decision, v.Reason)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StateDown {
		t.Fatalf("codex state = %s, want %s", got, StateDown)
	}
}

// Every family down is never a pass: degradation buys a quorum, not a bypass.
func TestEvaluateAllFamiliesDownBlocks(t *testing.T) {
	in := inputAt(pastEveryWindow)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked with no reviewer alive", v.Decision)
	}
	if v.Approved != 0 || v.Down != 3 {
		t.Fatalf("approved=%d down=%d, want 0/3", v.Approved, v.Down)
	}
}

// Two down and one approved does not reach a quorum of two.
func TestEvaluateSingleApprovalBelowQuorumBlocks(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
	)

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked below quorum", v.Decision)
	}
	if !strings.Contains(v.Reason, "quorum") {
		t.Fatalf("reason %q should explain the quorum shortfall", v.Reason)
	}
}

// The threshold is configurable, and raising it to unanimity withdraws the
// degradation allowance.
func TestEvaluateQuorumThresholdIsConfigurable(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted, PhaseError),
	)

	strict := testConfig()
	strict.Quorum = 3
	v, err := strict.Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked at quorum 3", v.Decision)
	}

	lenient := testConfig()
	lenient.Quorum = 1
	v, err = lenient.Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !v.Mergeable() {
		t.Fatalf("verdict = %s, want mergeable at quorum 1", v.Decision)
	}
}

// A quorum larger than the family set can never be satisfied, so it is a
// configuration error rather than a permanently red gate nobody can explain.
func TestEvaluateRejectsUnsatisfiableConfig(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"quorum above family count": func(c *Config) { c.Quorum = 4 },
		"zero quorum":               func(c *Config) { c.Quorum = 0 },
		"negative quorum":           func(c *Config) { c.Quorum = -1 },
		"no families":               func(c *Config) { c.Families = nil },
		"duplicate family":          func(c *Config) { c.Families = []Family{FamilyClaude, FamilyClaude} },
		"blank family":              func(c *Config) { c.Families = []Family{FamilyClaude, ""} },
		"zero verdict timeout":      func(c *Config) { c.VerdictTimeout = 0 },
		"zero dispatch timeout":     func(c *Config) { c.DispatchTimeout = 0 },
	} {
		cfg := testConfig()
		mutate(&cfg)
		if _, err := cfg.Evaluate(inputAt(insideWindow)); err == nil {
			t.Errorf("%s: Evaluate returned no error, want a configuration error", name)
		}
	}
}

// Without a readiness clock the dispatch deadline cannot be evaluated, so an
// unstarted family stays missing rather than silently degrading to down.
func TestEvaluateWithoutReadyClockKeepsUnstartedFamiliesMissing(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
	)
	in.ReadyAt = time.Time{}

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked without a readiness clock", v.Decision)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StateMissing {
		t.Fatalf("codex state = %s, want %s", got, StateMissing)
	}
}

// A started label with no recorded timestamp cannot be aged out, so it stays
// pending. Missing evidence never becomes permission to merge.
func TestEvaluateStartedWithoutTimestampStaysPending(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
		lifecycle(FamilyGemini, PhaseStarted, PhaseApproved),
		lifecycle(FamilyCodex, PhaseStarted),
	)
	delete(in.AppliedAt, Label(FamilyCodex, PhaseStarted))

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked without a start timestamp", v.Decision)
	}
	if got := familyState(t, v, FamilyCodex).State; got != StatePending {
		t.Fatalf("codex state = %s, want %s", got, StatePending)
	}
}

// Labels belonging to a family outside the configured set are reported but
// never counted: an unconfigured reviewer cannot vote itself into the quorum.
func TestEvaluateIgnoresUnconfiguredFamilyApprovals(t *testing.T) {
	in := inputAt(pastEveryWindow,
		lifecycle(FamilyClaude, PhaseStarted, PhaseApproved),
	)
	in.Labels = append(in.Labels, "agentic-review:llama:approved")

	v, err := testConfig().Evaluate(in)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Approved != 1 {
		t.Fatalf("approved=%d, want 1 — llama is not a configured family", v.Approved)
	}
	if len(v.Unconfigured) != 1 || v.Unconfigured[0] != Family("llama") {
		t.Fatalf("unconfigured = %v, want [llama]", v.Unconfigured)
	}
	if v.Mergeable() {
		t.Fatalf("verdict = %s, want blocked", v.Decision)
	}
}

// Family verdicts come back in configured order so the gate summary, the merge
// loop reason, and the audit trail all read the same way every run.
func TestEvaluateReportsFamiliesInConfiguredOrder(t *testing.T) {
	v, err := testConfig().Evaluate(inputAt(insideWindow))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(v.Families) != len(DefaultFamilies) {
		t.Fatalf("got %d family verdicts, want %d", len(v.Families), len(DefaultFamilies))
	}
	for i, f := range DefaultFamilies {
		if v.Families[i].Family != f {
			t.Fatalf("family %d = %q, want %q", i, v.Families[i].Family, f)
		}
	}
}
