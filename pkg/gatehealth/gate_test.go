package gatehealth

import (
	"testing"
)

// pr is a terse constructor so table cases stay readable.
func pr(number int, failing ...string) PullRequest {
	return PullRequest{Number: number, FailingChecks: failing}
}

// spread builds n pull requests that all fail the same check, numbered from 1.
func spread(n int, check string) []PullRequest {
	out := make([]PullRequest, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, pr(i, check))
	}
	return out
}

// fill builds n evaluated pull requests with nothing failing, numbered after
// whatever spread produced, so a case can set an exact denominator.
func fill(n int) []PullRequest {
	out := make([]PullRequest, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, pr(1000+i))
	}
	return out
}

func TestDetectReportsHealthyWhenNoCheckIsShared(t *testing.T) {
	// GH-01: unrelated per-PR churn is the normal state and must not alarm.
	prs := []PullRequest{
		pr(1, "Build & Test (ubuntu-latest)"),
		pr(2, "Shell lint"),
		pr(3),
		pr(4, "Bats (zsh)"),
	}
	got := Detect(prs, DefaultConfig())
	if got.Status != StatusHealthy {
		t.Fatalf("status = %q, want %q (report: %+v)", got.Status, StatusHealthy, got)
	}
	if len(got.Systemic) != 0 {
		t.Fatalf("Systemic = %+v, want none", got.Systemic)
	}
}

func TestDetectFlagsACheckFailingAcrossTheFleet(t *testing.T) {
	// GH-02: the govulncheck deadlock shape. 19 of 44 PRs failed the same
	// required check while main stayed green.
	prs := spread(19, "govulncheck")
	for i := 20; i <= 44; i++ {
		prs = append(prs, pr(i))
	}
	got := Detect(prs, DefaultConfig())
	if got.Status != StatusSystemic {
		t.Fatalf("status = %q, want %q", got.Status, StatusSystemic)
	}
	if got.Dominant == nil {
		t.Fatal("Dominant = nil, want the govulncheck failure")
	}
	if got.Dominant.Check != "govulncheck" {
		t.Errorf("Dominant.Check = %q, want %q", got.Dominant.Check, "govulncheck")
	}
	if got.Dominant.PRCount != 19 {
		t.Errorf("Dominant.PRCount = %d, want 19", got.Dominant.PRCount)
	}
	if got.EvaluatedPRs != 44 {
		t.Errorf("EvaluatedPRs = %d, want 44", got.EvaluatedPRs)
	}
	wantFraction := 19.0 / 44.0
	if diff := got.Dominant.Fraction - wantFraction; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("Dominant.Fraction = %v, want %v", got.Dominant.Fraction, wantFraction)
	}
}

func TestDetectRequiresBothAbsoluteAndFractionalThresholds(t *testing.T) {
	// GH-03: a tiny repo where two of three PRs fail the same check is a high
	// fraction but too few PRs to call systemic. Guards against alarming on a
	// nearly empty queue.
	prs := []PullRequest{pr(1, "govulncheck"), pr(2, "govulncheck"), pr(3)}
	got := Detect(prs, DefaultConfig())
	if got.Status != StatusHealthy {
		t.Fatalf("status = %q, want %q: 2 PRs is below MinPRs", got.Status, StatusHealthy)
	}

	// And the mirror: many PRs but a small fraction stays quiet.
	many := spread(6, "flaky-check")
	for i := 7; i <= 100; i++ {
		many = append(many, pr(i))
	}
	got = Detect(many, DefaultConfig())
	if got.Status != StatusHealthy {
		t.Fatalf("status = %q, want %q: 6%% is below MinFraction", got.Status, StatusHealthy)
	}
}

func TestDetectReportsEverySystemicCheckRankedDeterministically(t *testing.T) {
	// GH-04: the real fleet had several checks over threshold at once. All are
	// reported, ordered by PR count descending then name ascending so repeated
	// runs produce byte-identical output.
	var prs []PullRequest
	for i := 1; i <= 10; i++ {
		prs = append(prs, pr(i, "govulncheck", "zebra-check", "alpha-check"))
	}
	for i := 11; i <= 20; i++ {
		prs = append(prs, pr(i, "alpha-check"))
	}
	got := Detect(prs, DefaultConfig())
	if len(got.Systemic) != 3 {
		t.Fatalf("len(Systemic) = %d, want 3: %+v", len(got.Systemic), got.Systemic)
	}
	want := []string{"alpha-check", "govulncheck", "zebra-check"}
	for i, w := range want {
		if got.Systemic[i].Check != w {
			t.Errorf("Systemic[%d].Check = %q, want %q", i, got.Systemic[i].Check, w)
		}
	}
	if got.Dominant.Check != "alpha-check" {
		t.Errorf("Dominant.Check = %q, want alpha-check (20 PRs)", got.Dominant.Check)
	}
}

func TestDetectExcludesUnknownRollupsFromTheDenominator(t *testing.T) {
	// GH-05: a PR whose checks have not reported yet is not evidence of health.
	// Counting it as a passing PR would dilute the fraction and mask an outage.
	prs := spread(5, "govulncheck")
	for i := 6; i <= 10; i++ {
		p := pr(i)
		p.ChecksUnknown = true
		prs = append(prs, p)
	}
	got := Detect(prs, DefaultConfig())
	if got.EvaluatedPRs != 5 {
		t.Fatalf("EvaluatedPRs = %d, want 5: unknown rollups must not count", got.EvaluatedPRs)
	}
	if got.SkippedPRs != 5 {
		t.Errorf("SkippedPRs = %d, want 5", got.SkippedPRs)
	}
	if got.Status != StatusSystemic {
		t.Errorf("status = %q, want %q: 5/5 is fully systemic", got.Status, StatusSystemic)
	}
}

func TestDetectCountsEachPullRequestOncePerCheck(t *testing.T) {
	// GH-06: GitHub reports a check per matrix leg and re-runs leave duplicate
	// contexts on the rollup. A PR listing the same failing check twice must
	// not count as two PRs, or the fraction can exceed 1.
	prs := []PullRequest{
		pr(1, "govulncheck", "govulncheck"),
		pr(2, "govulncheck"),
		pr(3, "govulncheck"),
		pr(4, "govulncheck"),
		pr(5, "govulncheck"),
	}
	got := Detect(prs, DefaultConfig())
	if got.Dominant.PRCount != 5 {
		t.Fatalf("Dominant.PRCount = %d, want 5", got.Dominant.PRCount)
	}
	if got.Dominant.Fraction > 1.0 {
		t.Errorf("Fraction = %v, must never exceed 1.0", got.Dominant.Fraction)
	}
}

func TestDetectCountsDraftPullRequestsByDefault(t *testing.T) {
	// GH-07: drafts are counted. This inverts the detector's first calibration,
	// which excluded them and consequently read healthy through the live
	// 2026-09-03 deadlock: 33 of 42 open PRs were drafts and the govulncheck
	// failure lived in them. Draft status describes review intent, not whether
	// a branch inherits a broken gate from main.
	prs := spread(19, "govulncheck")
	for i := range prs {
		prs[i].Draft = true
	}
	for i := 20; i <= 42; i++ {
		prs = append(prs, pr(i))
	}
	got := Detect(prs, DefaultConfig())
	if got.Status != StatusSystemic {
		t.Fatalf("status = %q, want %q: drafts must be counted", got.Status, StatusSystemic)
	}
	if got.EvaluatedPRs != 42 {
		t.Errorf("EvaluatedPRs = %d, want 42", got.EvaluatedPRs)
	}
}

func TestDetectExcludeDraftsFlagStillWorks(t *testing.T) {
	// GH-07b: the escape hatch remains for a repo whose drafts really are
	// unrelated to gate health, and it demonstrably changes the verdict.
	prs := spread(19, "govulncheck")
	for i := range prs {
		prs[i].Draft = true
	}
	for i := 20; i <= 42; i++ {
		prs = append(prs, pr(i))
	}
	cfg := DefaultConfig()
	cfg.ExcludeDrafts = true
	got := Detect(prs, cfg)
	if got.Status != StatusHealthy {
		t.Fatalf("status = %q, want %q with ExcludeDrafts", got.Status, StatusHealthy)
	}
	if got.EvaluatedPRs != 23 {
		t.Errorf("EvaluatedPRs = %d, want 23", got.EvaluatedPRs)
	}
	if got.SkippedPRs != 19 {
		t.Errorf("SkippedPRs = %d, want 19", got.SkippedPRs)
	}
}

func TestDetectReportsNoQueueWhenEveryPRIsExcluded(t *testing.T) {
	// GH-07c: excluding everything leaves no evidence, which is no_queue rather
	// than healthy. A probe must never claim health from an empty sample.
	prs := spread(5, "govulncheck")
	for i := range prs {
		prs[i].Draft = true
	}
	cfg := DefaultConfig()
	cfg.ExcludeDrafts = true
	if got := Detect(prs, cfg); got.Status != StatusNoQueue {
		t.Fatalf("status = %q, want %q", got.Status, StatusNoQueue)
	}
}

func TestDetectFallsBackToShippedDefaultsOnInvalidConfig(t *testing.T) {
	// GH-13: a bad config must degrade to the shipped thresholds, never to a
	// silently disabled alarm. MinFraction 0 would otherwise mark every check
	// systemic and MinPRs 0 would alarm on an empty queue.
	prs := spread(19, "govulncheck")
	for i := 20; i <= 44; i++ {
		prs = append(prs, pr(i))
	}
	got := Detect(prs, Config{MinFraction: 0, MinPRs: 0})
	if got.Status != StatusSystemic {
		t.Fatalf("status = %q, want %q under a bad config", got.Status, StatusSystemic)
	}
	// The churn case must still stay quiet rather than alarming on everything.
	churn := []PullRequest{pr(1, "a"), pr(2, "b"), pr(3, "c"), pr(4), pr(5), pr(6)}
	if got := Detect(churn, Config{MinFraction: 0, MinPRs: 0}); got.Status != StatusHealthy {
		t.Errorf("status = %q, want %q: bad config must not alarm on churn", got.Status, StatusHealthy)
	}
}

func TestDetectReportsNoQueueWhenNothingIsEvaluable(t *testing.T) {
	// GH-08: an empty queue is not health and not an outage. Reporting it as
	// healthy would let a totally dead pipeline read green.
	got := Detect(nil, DefaultConfig())
	if got.Status != StatusNoQueue {
		t.Fatalf("status = %q, want %q", got.Status, StatusNoQueue)
	}
	if got.Dominant != nil {
		t.Errorf("Dominant = %+v, want nil", got.Dominant)
	}
}

func TestDetectNamesALikelyFixForTheDominantCheck(t *testing.T) {
	// GH-09: the point of the detector is collapsing diagnosis time, so the
	// report must carry the remediation rather than just the symptom.
	got := Detect(spread(19, "govulncheck"), DefaultConfig())
	if got.Remediation == "" {
		t.Fatal("Remediation is empty; the report must name a likely fix")
	}
	if got.RemediationKind != RemediationDependencyBump {
		t.Errorf("RemediationKind = %q, want %q", got.RemediationKind, RemediationDependencyBump)
	}
}

func TestDetectFallsBackToAGenericRemediation(t *testing.T) {
	// GH-10: an unmapped check still gets an actionable next step.
	got := Detect(spread(19, "Some Bespoke Gate"), DefaultConfig())
	if got.Status != StatusSystemic {
		t.Fatalf("status = %q, want systemic", got.Status)
	}
	if got.Remediation == "" {
		t.Fatal("Remediation is empty for an unmapped check")
	}
	if got.RemediationKind != RemediationInvestigate {
		t.Errorf("RemediationKind = %q, want %q", got.RemediationKind, RemediationInvestigate)
	}
}

func TestConfigRejectsNonsenseThresholds(t *testing.T) {
	// GH-11: a misconfigured fraction must fail loudly rather than silently
	// disabling the alarm, which is the failure mode this whole package exists
	// to prevent.
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"zero fraction", Config{MinFraction: 0, MinPRs: 5}},
		{"fraction above one", Config{MinFraction: 1.5, MinPRs: 5}},
		{"zero minimum PRs", Config{MinFraction: 0.3, MinPRs: 0}},
		{"negative minimum PRs", Config{MinFraction: 0.3, MinPRs: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); err == nil {
				t.Fatalf("Validate() = nil, want an error for %+v", tc.cfg)
			}
		})
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("DefaultConfig().Validate() = %v, want nil", err)
	}
}

func TestDefaultConfigWouldHaveCaughtTheGovulncheckDeadlock(t *testing.T) {
	// GH-12: pins the shipped thresholds against the outage as actually
	// measured on 2026-09-03 by running this detector against the live repo:
	// govulncheck failing on 19 of 42 evaluated open pull requests (45.2%), of
	// which 33 were drafts. This is the regression guard for the whole package.
	// If someone loosens a threshold or re-excludes drafts, this test fails.
	prs := spread(19, "govulncheck")
	for i := range prs {
		prs[i].Draft = true // as in the real queue: the failure lived in drafts
	}
	for i := 20; i <= 42; i++ {
		prs = append(prs, pr(i))
	}
	got := Detect(prs, DefaultConfig())
	if got.Status != StatusSystemic {
		t.Fatalf("shipped defaults no longer catch the 2026-09-03 deadlock: %+v", got)
	}
	if got.Dominant.Check != "govulncheck" || got.Dominant.PRCount != 19 {
		t.Fatalf("Dominant = %+v, want govulncheck on 19 PRs", got.Dominant)
	}
	if got.RemediationKind != RemediationDependencyBump {
		t.Errorf("RemediationKind = %q, want %q", got.RemediationKind, RemediationDependencyBump)
	}
}

func TestDefaultConfigPinsItsExactThresholds(t *testing.T) {
	// GH-13: the outage regression above proves only that the 2026-09-03 queue
	// still reads systemic, which a wide band of thresholds satisfies. Defaults
	// of MinFraction 0.45 / MinPRs 19 would pass it while consuming all the
	// headroom, and much lower ones would pass it while making the alarm noisy.
	// These are the shipped numbers, asserted exactly, plus the boundary cases
	// that fail on drift in either direction.
	cfg := DefaultConfig()
	if cfg.MinFraction != 0.30 {
		t.Errorf("DefaultConfig().MinFraction = %v, want 0.30", cfg.MinFraction)
	}
	if cfg.MinPRs != 5 {
		t.Errorf("DefaultConfig().MinPRs = %d, want 5", cfg.MinPRs)
	}
	if cfg.ExcludeDrafts {
		t.Error("DefaultConfig().ExcludeDrafts = true, want false; drafts are counted (GH-12)")
	}

	// Fraction boundary, with MinPRs comfortably cleared so only the fraction
	// is under test: 6 of 20 is exactly 0.30 and systemic; 6 of 21 is 0.286 and
	// is not. Raising MinFraction breaks the first case, lowering it breaks the
	// second.
	atFraction := append(spread(6, "shared"), fill(14)...)
	if got := Detect(atFraction, cfg); got.Status != StatusSystemic {
		t.Errorf("6 of 20 (30.0%%) = %v, want systemic at the 0.30 boundary", got.Status)
	}
	belowFraction := append(spread(6, "shared"), fill(15)...)
	if got := Detect(belowFraction, cfg); got.Status == StatusSystemic {
		t.Errorf("6 of 21 (28.6%%) = systemic, want healthy below the 0.30 boundary")
	}

	// Count boundary: 5 of 10 clears MinPRs, 4 of 8 is 50% but only 4 PRs.
	// Raising MinPRs breaks the first case, lowering it breaks the second.
	atCount := append(spread(5, "shared"), fill(5)...)
	if got := Detect(atCount, cfg); got.Status != StatusSystemic {
		t.Errorf("5 of 10 = %v, want systemic at the MinPRs boundary", got.Status)
	}
	belowCount := append(spread(4, "shared"), fill(4)...)
	if got := Detect(belowCount, cfg); got.Status == StatusSystemic {
		t.Errorf("4 of 8 (50%%, 4 PRs) = systemic, want healthy below MinPRs=5")
	}
}
