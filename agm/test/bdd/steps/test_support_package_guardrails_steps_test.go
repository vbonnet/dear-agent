package steps

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestTrustIsolationCommandIsBoundedAndGroupCancelable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := newTrustIsolationTestCommand(ctx)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("nested trust test must run in an isolated process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("nested trust test must cancel its process group")
	}
	if cmd.WaitDelay != time.Second {
		t.Fatalf("nested trust test WaitDelay = %v, want %v", cmd.WaitDelay, time.Second)
	}
	if !slices.Contains(cmd.Args, "-timeout=90s") {
		t.Fatalf("nested trust test args %q omit the inner test timeout", cmd.Args)
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("cancel before start = %v", err)
	}
}
