package steps

import (
	"context"
	"path/filepath"
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
	minutes, err := workflowJobTimeoutMinutes(source, "integration-tests")
	if err != nil {
		t.Fatal(err)
	}
	if minutes != 40 {
		t.Fatalf("workflowJobTimeoutMinutes() = %d, want 40", minutes)
	}
	if _, err := workflowJobTimeoutMinutes(source, "missing-job"); err == nil {
		t.Fatal("missing workflow job should fail")
	}
}

func TestAffectedIntegrationDeadlineLayersRejectInvalidBudgets(t *testing.T) {
	for _, test := range []struct {
		name  string
		state localDevGuardrailState
		ok    bool
	}{
		{
			name:  "valid nesting",
			state: localDevGuardrailState{affectedPackageMins: 20, affectedListMins: 5, affectedStartupMins: 10, affectedCommandMins: 30, affectedJobMins: 40},
			ok:    true,
		},
		{
			name:  "startup grace is too small",
			state: localDevGuardrailState{affectedPackageMins: 20, affectedListMins: 5, affectedStartupMins: 1, affectedCommandMins: 21, affectedJobMins: 40},
		},
		{
			name:  "command does not compose package and startup budgets",
			state: localDevGuardrailState{affectedPackageMins: 20, affectedListMins: 5, affectedStartupMins: 10, affectedCommandMins: 31, affectedJobMins: 41},
		},
		{
			name:  "workflow headroom is too small",
			state: localDevGuardrailState{affectedPackageMins: 20, affectedListMins: 5, affectedStartupMins: 10, affectedCommandMins: 30, affectedJobMins: 39},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), localDevGuardrailStateKey{}, &test.state)
			err := affectedIntegrationDeadlineLayersShouldPreserveTheirNestedBudgets(ctx)
			if test.ok && err != nil {
				t.Fatalf("valid timeout layers failed: %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("invalid timeout layers should fail")
			}
		})
	}
}
