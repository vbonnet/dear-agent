package steps

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSpecAuditGoTestCommandIsBoundedAndGroupCancelable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), specAuditGoTestDeadline)
	defer cancel()
	command := newSpecAuditGoTestCommand(ctx, packageSpecBDDRepoRoot(), "TestExample")

	if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
		t.Fatal("nested SPEC audit test must run in an isolated process group")
	}
	if command.Cancel == nil {
		t.Fatal("nested SPEC audit test must cancel its process group")
	}
	if command.WaitDelay != time.Second {
		t.Fatalf("nested SPEC audit test WaitDelay = %v, want %v", command.WaitDelay, time.Second)
	}
	if !slices.Contains(command.Args, "-timeout="+specAuditGoTestTimeout) {
		t.Fatalf("nested SPEC audit test args %q omit the inner test timeout", command.Args)
	}
	if !slices.Contains(command.Args, "-v") {
		t.Fatalf("nested SPEC audit test args %q omit verbose test output", command.Args)
	}
	if err := command.Cancel(); err != nil {
		t.Fatalf("cancel before start = %v", err)
	}
}

func TestSpecAuditGoTestRunnerRejectsZeroMatch(t *testing.T) {
	state := &specGovernanceToolingState{repoRoot: packageSpecBDDRepoRoot()}
	ctx := context.WithValue(context.Background(), specGovernanceToolingStateKey{}, state)
	if err := runSpecAuditGoTests(ctx, "TestSpecAuditGoTestRunnerMustNotExist"); err != nil {
		t.Fatalf("runSpecAuditGoTests() error = %v, want deferred BDD state failure", err)
	}
	if state.err == nil {
		t.Fatalf("zero-match run unexpectedly succeeded; output:\n%s", state.output)
	}
	if !strings.Contains(state.err.Error(), "did not run a named regression") {
		t.Fatalf("zero-match error = %v, want named-regression rejection", state.err)
	}
	if strings.Contains(state.output, "=== RUN") {
		t.Fatalf("zero-match output unexpectedly reported a running test:\n%s", state.output)
	}
}

func TestBoundedSpecAuditOutputCapsBytes(t *testing.T) {
	output := &boundedSpecAuditOutput{limit: 3}
	if written, err := output.Write([]byte("abcd")); err != nil || written != 4 {
		t.Fatalf("Write() = (%d, %v), want (4, nil)", written, err)
	}
	if got := output.String(); got != "abc" {
		t.Fatalf("String() = %q, want capped output", got)
	}
	if !output.Truncated() {
		t.Fatal("Truncated() = false, want true after the output limit")
	}
}
