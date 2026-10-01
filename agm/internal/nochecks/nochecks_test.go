package nochecks

import (
	"errors"
	"reflect"
	"testing"
)

func TestNeedsRetrigger(t *testing.T) {
	required := map[string]bool{
		"Build & Test (ubuntu-latest)": true,
		"govulncheck":                  true,
	}

	cases := []struct {
		name     string
		pr       PR
		runs     []CheckRun
		required map[string]bool
		want     bool
	}{
		{
			name:     "no runs with required set is stuck",
			pr:       PR{Number: 1},
			runs:     nil,
			required: required,
			want:     true,
		},
		{
			name:     "one required run present is not stuck",
			pr:       PR{Number: 2},
			runs:     []CheckRun{{Name: "Build & Test (ubuntu-latest)"}},
			required: required,
			want:     false,
		},
		{
			name: "only non-required runs present is still stuck",
			pr:   PR{Number: 3},
			// A run exists, but none of it satisfies a required context — the
			// required workflow never fired, so the PR can never go green.
			runs:     []CheckRun{{Name: "Some Optional Lint"}},
			required: required,
			want:     true,
		},
		{
			name:     "draft is never flagged even with zero runs",
			pr:       PR{Number: 4, IsDraft: true},
			runs:     nil,
			required: required,
			want:     false,
		},
		{
			name:     "empty required, zero runs falls back to stuck",
			pr:       PR{Number: 5},
			runs:     nil,
			required: map[string]bool{},
			want:     true,
		},
		{
			name:     "empty required, any run is not stuck",
			pr:       PR{Number: 6},
			runs:     []CheckRun{{Name: "anything"}},
			required: map[string]bool{},
			want:     false,
		},
		{
			name: "partial required set is not stuck (CI is in progress)",
			pr:   PR{Number: 7},
			// Only one of two required checks has reported; the other is still
			// queued. CI fired, so this is the merge loop's problem, not ours.
			runs:     []CheckRun{{Name: "govulncheck"}},
			required: required,
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NeedsRetrigger(tc.pr, tc.runs, tc.required)
			if got != tc.want {
				t.Errorf("NeedsRetrigger(%+v, %v, required=%v) = %v, want %v",
					tc.pr, tc.runs, tc.required, got, tc.want)
			}
		})
	}
}

func TestScan_FlagsStuckPRsSortedByNumber(t *testing.T) {
	prs := []PR{
		{Number: 582, Title: "stuck b", BaseRefName: "main", HeadRefName: "feat/b", HeadSHA: "bbb"},
		{Number: 579, Title: "stuck a", BaseRefName: "main", HeadRefName: "feat/a", HeadSHA: "aaa"},
		{Number: 600, Title: "healthy", BaseRefName: "main", HeadRefName: "feat/c", HeadSHA: "ccc"},
	}
	runsOf := func(sha string) ([]CheckRun, error) {
		if sha == "ccc" {
			return []CheckRun{{Name: "Build & Test (ubuntu-latest)"}}, nil
		}
		return nil, nil // aaa, bbb have no runs → stuck
	}
	required := RequiredChecksByBase{byBase: map[string]map[string]bool{
		"main": {"Build & Test (ubuntu-latest)": true},
	}}

	stuck, readErrs, err := Scan(prs, required, runsOf)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(readErrs) != 0 {
		t.Fatalf("unexpected read errors: %v", readErrs)
	}
	gotNums := []int{stuck[0].Number, stuck[1].Number}
	if !reflect.DeepEqual(gotNums, []int{579, 582}) {
		t.Errorf("stuck numbers = %v, want sorted [579 582]", gotNums)
	}
}

func TestScan_ReadErrorNeverFlagsAndIsReported(t *testing.T) {
	prs := []PR{
		{Number: 1, BaseRefName: "main", HeadSHA: "boom"},
		{Number: 2, BaseRefName: "main", HeadSHA: "ok"},
	}
	runsOf := func(sha string) ([]CheckRun, error) {
		if sha == "boom" {
			return nil, errors.New("github 502")
		}
		return nil, nil // #2 genuinely has no runs → stuck
	}

	policies := RequiredChecksByBase{byBase: map[string]map[string]bool{"main": {}}}
	stuck, readErrs, err := Scan(prs, policies, runsOf)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(stuck) != 1 || stuck[0].Number != 2 {
		t.Fatalf("expected only #2 flagged, got %+v", stuck)
	}
	if readErrs == nil || readErrs[1] == nil {
		t.Errorf("expected a read error recorded for PR #1, got %v", readErrs)
	}
	// PR #1's read failed, so it must never be flagged even though it has no runs.
	for _, s := range stuck {
		if s.Number == 1 {
			t.Error("PR #1 was flagged despite an unresolved check-runs read")
		}
	}
}

func TestScan_DraftsExcluded(t *testing.T) {
	prs := []PR{{Number: 9, IsDraft: true, HeadSHA: "x"}}
	runCalls := 0
	stuck, _, err := Scan(prs, RequiredChecksByBase{byBase: map[string]map[string]bool{}}, func(string) ([]CheckRun, error) {
		runCalls++
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(stuck) != 0 {
		t.Errorf("draft PR should not be flagged, got %+v", stuck)
	}
	if runCalls != 0 {
		t.Fatalf("draft caused %d check-run read(s), want zero", runCalls)
	}
}

func TestScan_UsesEachPullRequestBasePolicy(t *testing.T) {
	prs := []PR{
		{Number: 20, BaseRefName: "main", HeadRefName: "feature/main", HeadSHA: "main-sha"},
		{Number: 10, BaseRefName: "stack-base", HeadRefName: "feature/stack", HeadSHA: "stack-sha"},
	}
	policies := RequiredChecksByBase{byBase: map[string]map[string]bool{
		"main":       {"Main Required": true},
		"stack-base": {"Stack Required": true},
	}}
	runsOf := func(string) ([]CheckRun, error) {
		return []CheckRun{{Name: "Stack Required"}}, nil
	}

	stuck, readErrs, err := Scan(prs, policies, runsOf)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(readErrs) != 0 {
		t.Fatalf("unexpected read errors: %v", readErrs)
	}
	if len(stuck) != 1 || stuck[0].Number != 20 || stuck[0].BaseRefName != "main" {
		t.Fatalf("stuck = %+v, want only main-based PR #20", stuck)
	}
}

func TestScan_CapturesAnIsolatedCloneOfTheClassifyingPolicy(t *testing.T) {
	policy := map[string]bool{"Build": true}
	policies := RequiredChecksByBase{byBase: map[string]map[string]bool{"main": policy}}

	stuck, readErrs, err := Scan(
		[]PR{{Number: 7, BaseRefName: "main", HeadSHA: "abc123"}},
		policies,
		func(string) ([]CheckRun, error) { return nil, nil },
	)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(readErrs) != 0 || len(stuck) != 1 {
		t.Fatalf("Scan() = stuck %#v, read errors %v; want one candidate", stuck, readErrs)
	}
	if !stuck[0].requiredChecks["Build"] || len(stuck[0].requiredChecks) != 1 {
		t.Fatalf("captured policy = %#v, want Build only", stuck[0].requiredChecks)
	}

	delete(policy, "Build")
	policy["Mutated owner"] = true
	if !stuck[0].requiredChecks["Build"] || stuck[0].requiredChecks["Mutated owner"] {
		t.Fatalf("captured policy changed through owner alias: %#v", stuck[0].requiredChecks)
	}

	stuck[0].requiredChecks["Candidate only"] = true
	if policy["Candidate only"] {
		t.Fatalf("owner policy changed through candidate alias: %#v", policy)
	}
}

func TestScan_RejectsUninitializedOrMissingBasePolicyBeforeRunReads(t *testing.T) {
	cases := []struct {
		name     string
		pr       PR
		policies RequiredChecksByBase
	}{
		{name: "zero value", pr: PR{Number: 1, BaseRefName: "main", HeadSHA: "abc"}},
		{
			name:     "missing policy for PR base",
			pr:       PR{Number: 1, BaseRefName: "main", HeadSHA: "abc"},
			policies: RequiredChecksByBase{byBase: map[string]map[string]bool{"other": {}}},
		},
		{
			name:     "nil inner policy",
			pr:       PR{Number: 1, BaseRefName: "main", HeadSHA: "abc"},
			policies: RequiredChecksByBase{byBase: map[string]map[string]bool{"main": nil}},
		},
		{
			name:     "missing PR base identity",
			pr:       PR{Number: 1, HeadSHA: "abc"},
			policies: RequiredChecksByBase{byBase: map[string]map[string]bool{}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runCalls := 0
			stuck, readErrs, err := Scan(
				[]PR{tc.pr},
				tc.policies,
				func(string) ([]CheckRun, error) {
					runCalls++
					return nil, nil
				},
			)
			if err == nil {
				t.Fatal("Scan() succeeded with incomplete policy owner")
			}
			if stuck != nil || readErrs != nil {
				t.Fatalf("Scan() returned partial results: stuck=%v readErrs=%v", stuck, readErrs)
			}
			if runCalls != 0 {
				t.Fatalf("incomplete policy owner caused %d check-run read(s), want zero", runCalls)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	if got := ShortSHA("cebb82eb05bea83"); got != "cebb82e" {
		t.Errorf("ShortSHA long = %q, want cebb82e", got)
	}
	if got := ShortSHA("abc"); got != "abc" {
		t.Errorf("ShortSHA short = %q, want abc", got)
	}
}
