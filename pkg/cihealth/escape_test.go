package cihealth

import (
	"math"
	"strings"
	"testing"
)

const buildCheck = "Build & Test (ubuntu-latest)"

func TestClassify(t *testing.T) {
	required := []string{buildCheck, "govulncheck", "Vulnerability Scan"}

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
	required := []string{buildCheck}
	cases := map[Class]Escape{
		ClassBypassed:  {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 0},
		ClassGatingGap: {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 1, PRChecks: []CheckRun{{Name: buildCheck, Conclusion: ConclusionFailure}}, RequiredContexts: required},
		ClassScopeGap:  {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 1, PRChecks: []CheckRun{{Name: buildCheck, Conclusion: ConclusionSuccess}}, DiffScoped: true},
		ClassMergeSkew: {PreMergeCapable: true, FailingCheck: buildCheck, PRNumber: 1, PRChecks: []CheckRun{{Name: buildCheck, Conclusion: ConclusionSuccess}}},
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
	got := SortedContexts([]string{"govulncheck", "Build & Test (macos-latest)", "Vulnerability Scan"})
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
