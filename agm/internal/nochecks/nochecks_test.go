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
		{Number: 582, Title: "stuck b", HeadRefName: "feat/b", HeadSHA: "bbb"},
		{Number: 579, Title: "stuck a", HeadRefName: "feat/a", HeadSHA: "aaa"},
		{Number: 600, Title: "healthy", HeadRefName: "feat/c", HeadSHA: "ccc"},
	}
	runsOf := func(sha string) ([]CheckRun, error) {
		if sha == "ccc" {
			return []CheckRun{{Name: "Build & Test (ubuntu-latest)"}}, nil
		}
		return nil, nil // aaa, bbb have no runs → stuck
	}
	required := map[string]bool{"Build & Test (ubuntu-latest)": true}

	stuck, readErrs := Scan(prs, required, runsOf)
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
		{Number: 1, HeadSHA: "boom"},
		{Number: 2, HeadSHA: "ok"},
	}
	runsOf := func(sha string) ([]CheckRun, error) {
		if sha == "boom" {
			return nil, errors.New("github 502")
		}
		return nil, nil // #2 genuinely has no runs → stuck
	}

	stuck, readErrs := Scan(prs, nil, runsOf)
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
	stuck, _ := Scan(prs, nil, func(string) ([]CheckRun, error) { return nil, nil })
	if len(stuck) != 0 {
		t.Errorf("draft PR should not be flagged, got %+v", stuck)
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
