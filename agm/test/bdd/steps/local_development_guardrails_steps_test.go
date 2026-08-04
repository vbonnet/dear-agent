package steps

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSafePRBDDLockParserAcceptsUnixAndWindowsLineEndings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktree")
	porcelain := "worktree " + filepath.Join(filepath.Dir(root), "repo") + "\nHEAD abc\n\n" +
		"worktree " + root + "\nHEAD def\nlocked bdd-owner\n"

	for _, test := range []struct {
		name       string
		lineEnding string
	}{
		{name: "LF", lineEnding: "\n"},
		{name: "CRLF", lineEnding: "\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			locked, reason, err := parseSafePRBDDLockState(root, strings.ReplaceAll(porcelain, "\n", test.lineEnding))
			if err != nil {
				t.Fatal(err)
			}
			if !locked || reason != "bdd-owner" {
				t.Fatalf("worktree lock = locked:%t reason:%q", locked, reason)
			}
		})
	}
}

func TestMissingNamedGoTestRuns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name: "all exact top-level tests ran",
			output: "=== RUN   TestCommandRegression\n" +
				"--- PASS: TestCommandRegression (0.00s)\n" +
				"=== RUN   TestInternalRegression\r\n" +
				"--- PASS: TestInternalRegression (0.00s)\r\n",
		},
		{
			name:   "missing regression is reported",
			output: "=== RUN   TestCommandRegression\n",
			want:   []string{"TestInternalRegression"},
		},
		{
			name:   "subtest marker is not an exact regression run",
			output: "=== RUN   TestInternalRegression/subcase\n",
			want:   []string{"TestCommandRegression", "TestInternalRegression"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := missingNamedGoTestRuns(tt.output, "TestCommandRegression", "TestInternalRegression")
			if !slices.Equal(got, tt.want) {
				t.Fatalf("missingNamedGoTestRuns() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMinuteConstant(t *testing.T) {
	source := `const (
	goListCommandTimeout = 5 * time.Minute
)`
	minutes, err := minuteConstant(source, "goListCommandTimeout")
	if err != nil {
		t.Fatal(err)
	}
	if minutes != 5 {
		t.Fatalf("minuteConstant() = %d, want 5", minutes)
	}
	if _, err := minuteConstant(source, "goTestCommandTimeout"); err == nil {
		t.Fatal("missing minute constant should fail")
	}
}

func TestWorkflowJobTimeoutMinutesScopesToNamedJob(t *testing.T) {
	source := `jobs:
  ci:
    timeout-minutes: 30
  integration-tests:
    name: Integration Tests (affected)
    # Preserve room around nested command deadlines.
    timeout-minutes: 40
    steps:
      - run: go test ./...
  next-job:
    timeout-minutes: 15
`
	minutes, err := workflowJobTimeoutMinutes([]byte(source), "integration-tests")
	if err != nil {
		t.Fatal(err)
	}
	if minutes != 40 {
		t.Fatalf("workflowJobTimeoutMinutes() = %d, want 40", minutes)
	}
	if _, err := workflowJobTimeoutMinutes([]byte(source), "missing-job"); err == nil {
		t.Fatal("missing workflow job should fail")
	}
}

func TestWorkflowJobTimeoutMinutesStaysWithinNamedJob(t *testing.T) {
	tests := []struct {
		name     string
		workflow string
		want     int
		wantErr  bool
	}{
		{
			name: "reads named job timeout",
			workflow: "jobs:\n" +
				"  integration-tests:\n" +
				"    runs-on: ubuntu-latest\n" +
				"    timeout-minutes: 100\n" +
				"  next-job:\n" +
				"    timeout-minutes: 200\n",
			want: 100,
		},
		{
			name: "does not borrow next job timeout",
			workflow: "jobs:\n" +
				"  integration-tests:\n" +
				"    runs-on: ubuntu-latest\n" +
				"  next-job:\n" +
				"    timeout-minutes: 200\n",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := workflowJobTimeoutMinutes([]byte(tc.workflow), "integration-tests")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("workflowJobTimeoutMinutes = %d, want missing-timeout error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("workflowJobTimeoutMinutes = %d, want %d", got, tc.want)
			}
		})
	}
}
