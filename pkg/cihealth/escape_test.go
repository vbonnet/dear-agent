package cihealth

import (
	"math"
	"strings"
	"testing"
)

const buildCheck = "Build & Test (ubuntu-latest)"

func TestClassify(t *testing.T) {
	required := []RequiredContext{{Name: buildCheck}, {Name: "govulncheck"}, {Name: "Vulnerability Scan"}}

	tests := []struct {
		name            string
		escape          Escape
		wantClass       Class
		wantRefinable   bool
		wantSummaryHas  string
		wantActionCount int
	}{
		{
			name: "no pull request means the commit bypassed pre-merge entirely",
			escape: Escape{
				PreMergeCapable:  true,
				FailingCheck:     buildCheck,
				MainSHA:          "0123456789abcdef",
				PRNumber:         0,
				RequiredContexts: required,
				RequiredKnown:    true,
				PRChecksKnown:    true,
			},
			wantClass:      ClassBypassed,
			wantSummaryHas: "0123456",
		},
		{
			name: "check absent from the pull request is a workflow-level path filter",
			escape: Escape{
				PreMergeCapable:  true,
				FailingCheck:     buildCheck,
				PRNumber:         42,
				PRChecks:         []CheckRun{{Name: "govulncheck", Conclusion: ConclusionSuccess}},
				RequiredContexts: required,
				RequiredKnown:    true,
				PRChecksKnown:    true,
			},
			wantClass:      ClassNeverRan,
			wantRefinable:  true,
			wantSummaryHas: "never reported",
		},
		{
			name: "skipped on the pull request but red on main is a selection bug",
			escape: Escape{
				PreMergeCapable:  true,
				FailingCheck:     buildCheck,
				PRNumber:         42,
				PRChecks:         []CheckRun{{Name: buildCheck, Conclusion: ConclusionSkipped}},
				RequiredContexts: required,
				RequiredKnown:    true,
				PRChecksKnown:    true,
			},
			wantClass:      ClassSelectionGap,
			wantRefinable:  true,
			wantSummaryHas: "SKIPPED",
		},
		{
			name: "failed on the pull request and not required is a gating gap",
			escape: Escape{
				PreMergeCapable:  true,
				FailingCheck:     "Structural Health (baselined)",
				PRNumber:         42,
				PRChecks:         []CheckRun{{Name: "Structural Health (baselined)", Conclusion: ConclusionFailure}},
				RequiredContexts: required,
				RequiredKnown:    true,
				PRChecksKnown:    true,
			},
			wantClass:      ClassGatingGap,
			wantSummaryHas: "not a required status check",
		},
		{
			name: "failed on the pull request while required points at a bypass",
			escape: Escape{
				PreMergeCapable:  true,
				FailingCheck:     buildCheck,
				PRNumber:         42,
				PRChecks:         []CheckRun{{Name: buildCheck, Conclusion: ConclusionFailure}},
				RequiredContexts: required,
				RequiredKnown:    true,
				PRChecksKnown:    true,
			},
			wantClass:      ClassGatingGap,
			wantSummaryHas: "administrative bypass",
		},
		{
			name: "passed pre-merge on a deliberately narrower scope is not a hole",
			escape: Escape{
				PreMergeCapable:  true,
				FailingCheck:     "Vulnerability Scan",
				PRNumber:         42,
				PRChecks:         []CheckRun{{Name: "Vulnerability Scan", Conclusion: ConclusionSuccess}},
				RequiredContexts: required,
				RequiredKnown:    true,
				PRChecksKnown:    true,
				DiffScoped:       true,
			},
			wantClass:      ClassScopeGap,
			wantSummaryHas: "working as designed",
		},
		{
			name: "passed pre-merge at the same scope is flake or merge skew",
			escape: Escape{
				PreMergeCapable:  true,
				FailingCheck:     buildCheck,
				PRNumber:         42,
				PRChecks:         []CheckRun{{Name: buildCheck, Conclusion: ConclusionSuccess}},
				RequiredContexts: required,
				RequiredKnown:    true,
				PRChecksKnown:    true,
			},
			wantClass:      ClassMergeSkew,
			wantSummaryHas: "non-deterministic",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Classify(test.escape)
			if got.Class != test.wantClass {
				t.Errorf("Class = %q, want %q", got.Class, test.wantClass)
			}
			if got.FilterRefinable != test.wantRefinable {
				t.Errorf("FilterRefinable = %v, want %v", got.FilterRefinable, test.wantRefinable)
			}
			if !strings.Contains(got.Summary, test.wantSummaryHas) {
				t.Errorf("Summary %q does not contain %q", got.Summary, test.wantSummaryHas)
			}
			if len(got.SuggestedActions) == 0 {
				t.Error("want at least one suggested action; a retro with no prevention is not a retro")
			}
		})
	}
}

// Only a genuinely mis-scoped filter should tell the reader that refining
// filters will help. Saying so for a flake or a gating gap sends the next
// person to edit the wrong file.
func TestFilterRefinableOnlyForSelectionClasses(t *testing.T) {
	required := []RequiredContext{{Name: buildCheck}}
	cases := map[Class]Escape{
		ClassBypassed:  {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 0, PRChecksKnown: true},
		ClassGatingGap: {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 1, PRChecks: []CheckRun{{Name: buildCheck, Conclusion: ConclusionFailure}}, RequiredContexts: required, RequiredKnown: true, PRChecksKnown: true},
		ClassScopeGap:  {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 1, PRChecks: []CheckRun{{Name: buildCheck, Conclusion: ConclusionSuccess}}, DiffScoped: true, PRChecksKnown: true},
		ClassMergeSkew: {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 1, PRChecks: []CheckRun{{Name: buildCheck, Conclusion: ConclusionSuccess}}, PRChecksKnown: true},
	}
	for class, escape := range cases {
		if got := Classify(escape); got.FilterRefinable {
			t.Errorf("%s: FilterRefinable = true, want false", class)
		}
	}
}

func TestROIRatioAndVerdict(t *testing.T) {
	tests := []struct {
		name        string
		roi         ROI
		wantRatio   float64
		wantVerdict string
	}{
		{
			name:        "cheap check catching frequent expensive escapes",
			roi:         ROI{CureMinutes: 90, Escapes: 4, PreventionMinutes: 30, PreventionMeasured: true},
			wantRatio:   12,
			wantVerdict: "ALWAYS PREVENT",
		},
		{
			name:        "middling ratio lands in the usually-prevent band",
			roi:         ROI{CureMinutes: 60, Escapes: 2, PreventionMinutes: 30, PreventionMeasured: true},
			wantRatio:   4,
			wantVerdict: "USUALLY PREVENT",
		},
		{
			name:        "expensive check catching rare cheap escapes stays post-merge",
			roi:         ROI{CureMinutes: 30, Escapes: 1, PreventionMinutes: 600, PreventionMeasured: true},
			wantRatio:   0.05,
			wantVerdict: "CASE-BY-CASE",
		},
		{
			name:        "no escapes in the window is no signal, not a recommendation",
			roi:         ROI{CureMinutes: 90, Escapes: 0, PreventionMinutes: 30, PreventionMeasured: true},
			wantRatio:   0,
			wantVerdict: "NO SIGNAL",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.roi.Ratio(); math.Abs(got-test.wantRatio) > 1e-9 {
				t.Errorf("Ratio() = %v, want %v", got, test.wantRatio)
			}
			if got := test.roi.Verdict(); !strings.HasPrefix(got, test.wantVerdict) {
				t.Errorf("Verdict() = %q, want prefix %q", got, test.wantVerdict)
			}
		})
	}
}

// A check that costs nothing should always run — the formula must not divide
// by zero and must not report the free check as not worth running.
func TestROIFreePreventionIsAlwaysWorthIt(t *testing.T) {
	roi := ROI{CureMinutes: 45, Escapes: 1, PreventionMinutes: 0, PreventionMeasured: true}
	if got := roi.Ratio(); !math.IsInf(got, 1) {
		t.Errorf("Ratio() = %v, want +Inf", got)
	}
	if got := roi.Verdict(); !strings.HasPrefix(got, "ALWAYS PREVENT") {
		t.Errorf("Verdict() = %q, want ALWAYS PREVENT", got)
	}
	if got := roi.Explain(); !strings.Contains(got, "unbounded") {
		t.Errorf("Explain() = %q, want it to mention unbounded", got)
	}
}

// A free check that has never caught anything is not evidence of anything.
func TestROIFreePreventionWithNoEscapesIsNoSignal(t *testing.T) {
	roi := ROI{CureMinutes: 45, Escapes: 0, PreventionMinutes: 0, PreventionMeasured: true}
	if got := roi.Ratio(); got != 0 {
		t.Errorf("Ratio() = %v, want 0", got)
	}
	if got := roi.Verdict(); !strings.HasPrefix(got, "NO SIGNAL") {
		t.Errorf("Verdict() = %q, want NO SIGNAL", got)
	}
}

// The retro has to show its arithmetic; a bare verdict is not auditable.
func TestROIExplainShowsItsWork(t *testing.T) {
	got := ROI{CureMinutes: 90, Escapes: 4, PreventionMinutes: 30, PreventionMeasured: true}.Explain()
	for _, want := range []string{"ROI = (Cure x Frequency) / Prevention", "90", "4", "30", "12.0:1", "ALWAYS PREVENT"} {
		if !strings.Contains(got, want) {
			t.Errorf("Explain() missing %q:\n%s", want, got)
		}
	}
}

func TestSortedContexts(t *testing.T) {
	got := SortedContexts([]RequiredContext{{Name: "govulncheck"}, {Name: "Build & Test (macos-latest)"}, {Name: "Vulnerability Scan"}})
	want := []string{"Build & Test (macos-latest)", "Vulnerability Scan", "govulncheck"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SortedContexts() = %v, want %v", got, want)
		}
	}
}

// Regression, caught by dry-running the sweep against real data. All four
// workflows red on main were schedule-only (Tofu Drift, Security Audit, Infra
// Repo Reconciliation, Multi-Agent Orchestration). They report on no pull
// request by construction, so they landed in ClassNeverRan and the retro told
// the reader to go widen a path filter that was never involved.
func TestScheduleOnlyWorkflowIsNotAnEscape(t *testing.T) {
	got := Classify(Escape{
		PreMergeCapable: false,
		FailingCheck:    "drift",
		MainSHA:         "98dcac4aaaa",
		PRNumber:        0,
	})
	if got.Class != ClassPostMergeOnly {
		t.Errorf("Class = %q, want %q", got.Class, ClassPostMergeOnly)
	}
	if got.FilterRefinable {
		t.Error("FilterRefinable = true; a schedule-only workflow has no filter to refine")
	}
	if !strings.Contains(got.Summary, "no pull_request trigger") {
		t.Errorf("Summary should say why it never ran pre-merge, got %q", got.Summary)
	}
}

// Same regression, other half. A schedule-only workflow has no pre-merge runs
// to price either. Treating that unmeasured zero as "prevention is free" made
// every one of them come out as "ALWAYS PREVENT — block this pre-merge", which
// is exactly backwards for a drift detector that cannot run on a pull request.
func TestUnmeasuredPreventionIsNotFreePrevention(t *testing.T) {
	roi := ROI{CureMinutes: 90, Escapes: 31, PreventionMeasured: false}
	if got := roi.Ratio(); got != 0 {
		t.Errorf("Ratio() = %v, want 0 for unmeasured prevention", got)
	}
	if got := roi.Verdict(); !strings.HasPrefix(got, "INSUFFICIENT DATA") {
		t.Errorf("Verdict() = %q, want INSUFFICIENT DATA", got)
	}
	explain := roi.Explain()
	if !strings.Contains(explain, "<unmeasured>") {
		t.Errorf("Explain() should mark the denominator unmeasured, got %q", explain)
	}
	if strings.Contains(explain, "unbounded") {
		t.Errorf("Explain() must not claim prevention is free, got %q", explain)
	}
}

// A check that was cancelled, timed out, or never started did not pass. The
// classifier used to fall past the skipped and failure cases straight into the
// scope-gap and merge-skew branches, both of which assert a pre-merge pass —
// so a superseded PR run was reported as a non-deterministic check.
func TestNonSuccessConclusionIsNotTreatedAsAPass(t *testing.T) {
	for _, conclusion := range []string{
		ConclusionCancelled,
		ConclusionTimedOut,
		ConclusionStartupFailure,
		ConclusionActionRequired,
		ConclusionNeutral,
	} {
		t.Run(conclusion, func(t *testing.T) {
			base := Escape{
				PreMergeCapable: true,
				FailingCheck:    "Integration Tests (affected)",
				MainSHA:         "abc1234def",
				PRNumber:        7,
				PRChecks:        []CheckRun{{Name: "Integration Tests (affected)", Conclusion: conclusion}},
				RequiredKnown:   true,
				PRChecksKnown:   true,
			}

			if got := Classify(base); got.Class != ClassInconclusive {
				t.Errorf("Class = %q, want %q", got.Class, ClassInconclusive)
			}

			// Diff-scoping must not rescue it either: scope-gap says the check
			// passed at a narrower scope, and it did not pass at all.
			scoped := base
			scoped.DiffScoped = true
			if got := Classify(scoped); got.Class != ClassInconclusive {
				t.Errorf("diff-scoped Class = %q, want %q", got.Class, ClassInconclusive)
			}
		})
	}
}

// Reading the branch ruleset needs Administration (read), which the watchdog's
// token does not have. A denied request returns an empty list, which is
// indistinguishable from "this repository requires nothing" — and that reading
// turns every required-check failure into an advisory one.
func TestUnknownRequiredContextsDoNotAssertAdvisory(t *testing.T) {
	got := Classify(Escape{
		PreMergeCapable: true,
		FailingCheck:    "Build & Test (ubuntu-latest)",
		MainSHA:         "abc1234def",
		PRNumber:        9,
		PRChecks:        []CheckRun{{Name: "Build & Test (ubuntu-latest)", Conclusion: ConclusionFailure}},
		PRChecksKnown:   true,
		RequiredKnown:   false,
	})
	if got.Class != ClassGatingGap {
		t.Fatalf("Class = %q, want %q", got.Class, ClassGatingGap)
	}
	if strings.Contains(got.Summary, "not a required status check") {
		t.Errorf("Summary asserts an advisory context it could not establish: %q", got.Summary)
	}
	if !strings.Contains(got.Summary, "could not be determined") {
		t.Errorf("Summary should report the lookup as unresolved, got %q", got.Summary)
	}
}

// A scheduled run compares the repo against a world that moves on its own, so
// the commit at the head of main is not evidence of what caused the failure.
func TestScheduledDetectionIsNotPinnedOnTheHeadCommit(t *testing.T) {
	got := Classify(Escape{
		PreMergeCapable:    true,
		ScheduledDetection: true,
		FailingCheck:       "reconcile",
		MainSHA:            "abc1234def",
		PRNumber:           11,
		PRChecks:           nil,
		RequiredKnown:      true,
	})
	if got.Class != ClassPostMergeOnly {
		t.Errorf("Class = %q, want %q", got.Class, ClassPostMergeOnly)
	}
	if got.FilterRefinable {
		t.Error("FilterRefinable = true; a scheduled detection has no pre-merge filter to refine")
	}
}

// The cure term is a standing default: nothing measures how long main sat red
// or how many people it blocked. A default that can push the ratio across the
// 3:1 and 10:1 bands must not be rendered as evidence.
func TestAssumedCureCostYieldsAProvisionalVerdict(t *testing.T) {
	roi := ROI{CureMinutes: 90, CureAssumed: true, Escapes: 4, PreventionMinutes: 30, PreventionMeasured: true}
	verdict := roi.Verdict()
	if !strings.HasPrefix(verdict, "PROVISIONAL") {
		t.Errorf("Verdict() = %q, want a PROVISIONAL prefix", verdict)
	}
	if !strings.Contains(verdict, "ALWAYS PREVENT") {
		t.Errorf("Verdict() should still report the band it computed, got %q", verdict)
	}
	if explain := roi.Explain(); !strings.Contains(explain, "ASSUMED") {
		t.Errorf("Explain() should label the assumed term, got %q", explain)
	}
}

// A truncated run history is a lower bound on the denominator, which biases the
// ratio upward — toward recommending a pre-merge gate.
func TestTruncatedPreventionIsReportedAsALowerBound(t *testing.T) {
	roi := ROI{CureMinutes: 90, Escapes: 4, PreventionMinutes: 30, PreventionMeasured: true, PreventionTruncated: true}
	if verdict := roi.Verdict(); !strings.HasPrefix(verdict, "PROVISIONAL") {
		t.Errorf("Verdict() = %q, want a PROVISIONAL prefix", verdict)
	}
	if explain := roi.Explain(); !strings.Contains(explain, "LOWER BOUND") {
		t.Errorf("Explain() should mark the denominator a lower bound, got %q", explain)
	}
}

// Fully measured terms keep the plain prescriptive band.
func TestFullyMeasuredROIIsNotProvisional(t *testing.T) {
	roi := ROI{CureMinutes: 90, Escapes: 4, PreventionMinutes: 30, PreventionMeasured: true}
	if got := roi.Verdict(); !strings.HasPrefix(got, "ALWAYS PREVENT") {
		t.Errorf("Verdict() = %q, want ALWAYS PREVENT", got)
	}
}

// The retro is the fix agent's brief. Pointing it at a selector that does not
// exist in this repository sends it to edit a file it cannot find.
func TestRetroNamesOnlyMechanismsThatExist(t *testing.T) {
	for _, escape := range []Escape{
		{PreMergeCapable: true, FailingCheck: "c", PRNumber: 1, RequiredKnown: true, PRChecksKnown: true},
		{PreMergeCapable: true, FailingCheck: "c", PRNumber: 1, RequiredKnown: true, PRChecksKnown: true, PRChecks: []CheckRun{{Name: "c", Conclusion: ConclusionSkipped}}},
	} {
		body := Retro{FailingCheck: "c", Finding: Classify(escape), RequiredKnown: true}.Body()
		for _, ghost := range []string{"changed-paths.yml", "global-inputs", "ADR-038"} {
			if strings.Contains(body, ghost) {
				t.Errorf("retro body names %q, which does not exist in this repository", ghost)
			}
		}
	}
}

// A post-merge-only finding is not an escape, so "should this move pre-merge?"
// has no answer to price. Rendering the ratio anyway produced a retro headed
// post-merge-only that closed with "ALWAYS PREVENT — block this pre-merge",
// because the workflow had measurable pre-merge runs from its other trigger.
func TestPostMergeOnlyRetroDoesNotPricePlacement(t *testing.T) {
	finding := Classify(Escape{
		PreMergeCapable:    true,
		ScheduledDetection: true,
		FailingCheck:       "reconcile",
		MainSHA:            "abc1234def",
		RequiredKnown:      true,
	})
	if finding.PricesPlacement() {
		t.Fatal("PricesPlacement() = true for post-merge-only")
	}

	body := Retro{
		FailingCheck:  "reconcile",
		Finding:       finding,
		RequiredKnown: true,
		ROI:           ROI{CureMinutes: 90, Escapes: 40, PreventionMinutes: 5, PreventionMeasured: true},
	}.Body()

	if !strings.Contains(body, "not a placement decision") {
		t.Errorf("body should decline to price placement, got:\n%s", body)
	}
	if !strings.Contains(body, "not an escape") {
		t.Errorf("body should say why placement does not apply to this class, got:\n%s", body)
	}
	for _, banned := range []string{"ALWAYS PREVENT", "USUALLY PREVENT", "CASE-BY-CASE"} {
		if strings.Contains(body, banned) {
			t.Errorf("post-merge-only retro renders a placement verdict %q", banned)
		}
	}
	// The footer still has to be there; skipping the section must not skip the
	// provenance line.
	if !strings.Contains(body, "main-health-watchdog.yml") {
		t.Error("body lost its provenance footer")
	}
}

// Placement is priced only where widening pre-merge selection or scope is
// actually the remedy. Pricing it elsewhere produced verdicts that contradicted
// the finding directly above them — a `merge-skew` brief that had just
// established the check passed pre-merge, closing with "block this pre-merge".
func TestPlacementIsPricedOnlyWhereItIsTheDecision(t *testing.T) {
	for _, class := range []Class{ClassNeverRan, ClassSelectionGap, ClassScopeGap} {
		if !(Finding{Class: class}).PricesPlacement() {
			t.Errorf("PricesPlacement() = false for %q, want true", class)
		}
	}
	for _, class := range []Class{ClassMergeSkew, ClassInconclusive, ClassBypassed, ClassGatingGap, ClassPostMergeOnly} {
		if (Finding{Class: class}).PricesPlacement() {
			t.Errorf("PricesPlacement() = true for %q, want false", class)
		}
	}
}

// A ruleset pins a required context to a producing App. Matching on the name
// alone lets any other App publish a check called "Build & Test
// (ubuntu-latest)" and be mistaken for the required one, which turns an
// ordinary merge into a reported administrative bypass.
func TestRequiredContextMatchesOnProducingApp(t *testing.T) {
	const (
		githubActions int64 = 15368
		someOtherApp  int64 = 99999
	)
	required := []RequiredContext{{Name: buildCheck, IntegrationID: githubActions}}

	base := Escape{
		PreMergeCapable:  true,
		FailingCheck:     buildCheck,
		MainSHA:          "abc1234def",
		PRNumber:         5,
		RequiredContexts: required,
		RequiredKnown:    true,
		PRChecksKnown:    true,
	}

	// A foreign App's same-named failing check is not the required context, so
	// the merge is a gating gap on an advisory check, not a bypass.
	foreign := base
	foreign.PRChecks = []CheckRun{{Name: buildCheck, Conclusion: ConclusionFailure, AppID: someOtherApp}}
	if got := Classify(foreign); strings.Contains(got.Summary, "administrative bypass") {
		t.Errorf("a foreign app's check was treated as the required one: %q", got.Summary)
	}

	// The pinned App's failing check is the required context.
	genuine := base
	genuine.PRChecks = []CheckRun{{Name: buildCheck, Conclusion: ConclusionFailure, AppID: githubActions}}
	if got := Classify(genuine); !strings.Contains(got.Summary, "administrative bypass") {
		t.Errorf("the pinned app's check should be required, got %q", got.Summary)
	}

	// An unknown producer still matches: refusing would turn every failed
	// identity lookup into a false "not required".
	unknown := base
	unknown.PRChecks = []CheckRun{{Name: buildCheck, Conclusion: ConclusionFailure}}
	if got := Classify(unknown); !strings.Contains(got.Summary, "administrative bypass") {
		t.Errorf("an unknown producer should not flip the gating verdict, got %q", got.Summary)
	}
}

// The watchdog files this at detection time, when nothing has been fixed. DEAR
// is Define, Execute, Audit, Retro; calling a document with no execution and no
// outcome a completed retrospective is a claim the policy can check.
func TestBriefDoesNotClaimToBeACompletedRetro(t *testing.T) {
	r := Retro{
		FailingCheck:  "Build & Test (ubuntu-latest)",
		Finding:       Classify(Escape{PreMergeCapable: true, FailingCheck: "Build & Test (ubuntu-latest)", PRNumber: 1, RequiredKnown: true, PRChecksKnown: true}),
		RequiredKnown: true,
	}

	if strings.Contains(r.Title(), "DEAR retro") {
		t.Errorf("Title() = %q claims to be a completed retrospective", r.Title())
	}

	body := r.Body()
	for _, want := range []string{
		"Incident brief",
		"## Define",
		"## Execute",
		"## Audit",
		"## Retro — prevention (provisional)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

// A merge-skew brief established two paragraphs earlier that the check DID run
// and pass pre-merge. Closing it with a placement verdict contradicts that, and
// a generic "this is not an escape" disclaimer is false for this class.
func TestMergeSkewDeclinesPlacementWithoutCallingItANonEscape(t *testing.T) {
	body := Retro{
		FailingCheck: buildCheck,
		Finding: Classify(Escape{
			PreMergeCapable: true,
			FailingCheck:    buildCheck,
			PRNumber:        3,
			PRChecks:        []CheckRun{{Name: buildCheck, Conclusion: ConclusionSuccess}},
			RequiredKnown:   true,
			PRChecksKnown:   true,
		}),
		RequiredKnown: true,
		ROI:           ROI{CureMinutes: 90, Escapes: 40, PreventionMinutes: 5, PreventionMeasured: true},
	}.Body()

	if !strings.Contains(body, "not a placement decision") {
		t.Errorf("merge-skew should decline to price placement:\n%s", body)
	}
	if strings.Contains(body, "not an escape") {
		t.Error("merge-skew IS an escape; the disclaimer must not claim otherwise")
	}
	for _, banned := range []string{"ALWAYS PREVENT", "USUALLY PREVENT", "CASE-BY-CASE"} {
		if strings.Contains(body, banned) {
			t.Errorf("merge-skew brief renders a placement verdict %q", banned)
		}
	}
}

// A denied or timed-out check-runs query returns an empty slice, which reads
// exactly like "the check genuinely never reported" — and that reading produces
// `never-ran` plus advice to widen a path filter, off the back of a transient
// API error.
func TestFailedCheckLookupIsNotEvidenceOfNeverRan(t *testing.T) {
	got := Classify(Escape{
		PreMergeCapable: true,
		FailingCheck:    buildCheck,
		MainSHA:         "abc1234def",
		PRNumber:        12,
		PRChecks:        nil,
		PRChecksKnown:   false,
		RequiredKnown:   true,
	})
	if got.Class != ClassUnknown {
		t.Errorf("Class = %q, want %q", got.Class, ClassUnknown)
	}
	if got.FilterRefinable {
		t.Error("FilterRefinable = true on an unread lookup")
	}
	if got.PricesPlacement() {
		t.Error("PricesPlacement() = true with no evidence to price")
	}
}

// A truncated failure history undercounts escapes, and the numerator moving is
// what crosses the placement thresholds.
func TestTruncatedEscapeCountIsReportedAsALowerBound(t *testing.T) {
	roi := ROI{CureMinutes: 90, Escapes: 100, EscapesTruncated: true, PreventionMinutes: 30, PreventionMeasured: true}
	if verdict := roi.Verdict(); !strings.HasPrefix(verdict, "PROVISIONAL") {
		t.Errorf("Verdict() = %q, want a PROVISIONAL prefix", verdict)
	}
	if explain := roi.Explain(); !strings.Contains(explain, "LOWER BOUND") {
		t.Errorf("Explain() should mark the numerator a lower bound, got %q", explain)
	}
}
