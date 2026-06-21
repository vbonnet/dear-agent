package prchecks

import "testing"

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
			required: nil,
			want:     true,
		},
		{
			name:     "empty required, any run is not stuck",
			pr:       PR{Number: 6},
			runs:     []CheckRun{{Name: "anything"}},
			required: nil,
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
