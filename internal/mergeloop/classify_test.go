package mergeloop

import "testing"

// reqCheck builds a required check with a verdict.
func reqCheck(name string, v CheckVerdict) Check {
	return Check{Name: name, Verdict: v, Required: true}
}

func TestClassify(t *testing.T) {
	p := NewPolicy()
	cases := []struct {
		name        string
		pr          PR
		attempts    int
		agentActive bool
		want        State
	}{
		{
			name: "draft is skipped",
			pr:   PR{Number: 1, IsDraft: true, Mergeable: "MERGEABLE"},
			want: StateDraft,
		},
		{
			name: "draft via merge state",
			pr:   PR{Number: 1, MergeStateStatus: "DRAFT"},
			want: StateDraft,
		},
		{
			name: "abandon label wins over green",
			pr:   PR{Number: 2, Labels: []string{"abandoned"}, Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
			want: StateAbandoned,
		},
		{
			name: "policy block label beats conflict",
			pr:   PR{Number: 3, Labels: []string{"needs-security-review"}, Mergeable: "CONFLICTING"},
			want: StateBlockedPolicy,
		},
		{
			name: "sensitive path forces policy block",
			pr:   PR{Number: 4, Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN", ChangedFiles: []string{"internal/auth/token.go"}},
			want: StateBlockedPolicy,
		},
		{
			name: "human changes-requested is a policy block",
			pr:   PR{Number: 5, ReviewDecision: "CHANGES_REQUESTED", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
			want: StateBlockedPolicy,
		},
		{
			name:        "agent in flight skips even when failing",
			pr:          PR{Number: 6, MergeStateStatus: "CLEAN", Checks: []Check{reqCheck("ci", CheckFail)}},
			agentActive: true,
			want:        StateAgentInFlight,
		},
		{
			name: "conflicting goes to conflicted",
			pr:   PR{Number: 7, Mergeable: "CONFLICTING"},
			want: StateConflicted,
		},
		{
			name: "dirty merge state is conflicted",
			pr:   PR{Number: 8, MergeStateStatus: "DIRTY"},
			want: StateConflicted,
		},
		{
			name: "behind beats failing CI (rebase re-runs CI)",
			pr:   PR{Number: 9, MergeStateStatus: "BEHIND", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckFail)}},
			want: StateBehind,
		},
		{
			name: "failing required check on up-to-date head",
			pr:   PR{Number: 10, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckFail)}},
			want: StateCIFailing,
		},
		{
			name: "non-required failure does NOT trigger agent",
			pr:   PR{Number: 11, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{{Name: "doc-proximity", Verdict: CheckFail, Required: false}, reqCheck("ci", CheckPass)}},
			want: StateGreen,
		},
		{
			name: "pending required check waits",
			pr:   PR{Number: 12, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckPending)}},
			want: StateCIPending,
		},
		{
			name: "unavailable required-check projection waits",
			pr:   PR{Number: 18, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", CheckProjectionError: "provider timeout"},
			want: StateCIPending,
		},
		{
			name: "all required pass and mergeable is green",
			pr:   PR{Number: 13, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckPass)}},
			want: StateGreen,
		},
		{
			name: "no required checks + mergeable is green (safe-merge is authority)",
			pr:   PR{Number: 14, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE"},
			want: StateGreen,
		},
		{
			name: "unknown mergeability waits",
			pr:   PR{Number: 15, MergeStateStatus: "UNKNOWN", Mergeable: "UNKNOWN", Checks: []Check{reqCheck("ci", CheckPass)}},
			want: StateCIPending,
		},
		{
			name:     "exhausted attempts abandons a failing PR",
			pr:       PR{Number: 16, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckFail)}},
			attempts: 2,
			want:     StateAbandoned,
		},
		{
			name:     "one attempt still tries to fix",
			pr:       PR{Number: 17, MergeStateStatus: "CLEAN", Mergeable: "MERGEABLE", Checks: []Check{reqCheck("ci", CheckFail)}},
			attempts: 1,
			want:     StateCIFailing,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Classify(tc.pr, tc.attempts, tc.agentActive)
			if got.State != tc.want {
				t.Fatalf("Classify() = %s (%s), want %s", got.State, got.Reason, tc.want)
			}
			if got.Reason == "" {
				t.Errorf("Classify() returned empty reason for %s", got.State)
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"**/auth/**", "internal/auth/token.go", true},
		{"**/auth/**", "auth/token.go", true},
		{"**/auth/**", "internal/authz/token.go", false},
		{"infra/**", "infra/tf/main.tf", true},
		{"infra/**", "cmd/infra/main.go", false},
		{"**/*.tf", "infra/tf/main.tf", true},
		{"**/*.tf", "main.tf", true},
		{"**/*.tf", "main.go", false},
		{"**/billing/**", "pkg/billing/charge.go", true},
		{"a/*/c", "a/b/c", true},
		{"a/*/c", "a/b/d/c", false},
		{"**", "anything/at/all", true},
		{"x/**/y", "x/y", true},
		{"x/**/y", "x/a/b/y", true},
		{"x/**/y", "x/a/b/z", false},
	}
	for _, tc := range cases {
		if got := matchGlob(tc.pattern, tc.path); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestFailureSignatureStableAndSorted(t *testing.T) {
	pr := PR{Checks: []Check{
		reqCheck("zeta", CheckFail),
		reqCheck("alpha", CheckFail),
		reqCheck("mid", CheckPass),
	}}
	if got, want := failureSignature(pr), "alpha,zeta"; got != want {
		t.Fatalf("failureSignature = %q, want %q", got, want)
	}
	none := PR{Checks: []Check{reqCheck("ci", CheckPass)}}
	if got := failureSignature(none); got != "unknown" {
		t.Errorf("failureSignature(no fails) = %q, want unknown", got)
	}
}
