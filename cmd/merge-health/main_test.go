package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func testDeps(tipAge time.Duration, gitErr error) deps {
	return deps{
		now: func() time.Time { return t0 },
		gitOutput: func(_ context.Context, repo string, args ...string) (string, error) {
			if gitErr != nil {
				return "", gitErr
			}
			return fmt.Sprintf("abcdef0123456789 %d", t0.Add(-tipAge).Unix()), nil
		},
		fetchMtime: func(context.Context, string) (time.Time, bool) { return t0.Add(-10 * time.Minute), true },
	}
}

// MH-01: a commit inside the window is healthy, exit 0.
func TestRun_FreshTipIsHealthy(t *testing.T) {
	if code := run([]string{"--lookback", "96h"}, testDeps(2*time.Hour, nil)); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// MH-02: a tip older than the window is degraded, exit 1.
func TestRun_StaleTipIsDegraded(t *testing.T) {
	if code := run([]string{"--lookback", "96h"}, testDeps(5*24*time.Hour, nil)); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
}

// MH-03: an unresolvable ref or unreadable repo is down, exit 2.
func TestRun_GitErrorIsDown(t *testing.T) {
	d := testDeps(0, errors.New("fatal: not a git repository"))
	if code := run(nil, d); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// MH-03: malformed git output is down, not a crash or a false healthy.
func TestRun_MalformedOutputIsDown(t *testing.T) {
	d := testDeps(0, nil)
	d.gitOutput = func(_ context.Context, _ string, _ ...string) (string, error) { return "what even", nil }
	if code := run(nil, d); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// MH-04: an invalid or non-positive lookback is a usage error, exit 3.
func TestRun_BadLookbackIsUsage(t *testing.T) {
	for _, lb := range []string{"soon", "-4h", "0s"} {
		if code := run([]string{"--lookback", lb}, testDeps(time.Hour, nil)); code != 3 {
			t.Fatalf("lookback %q: exit = %d, want 3", lb, code)
		}
	}
}

// MH-05: a tip commit from the future is down, never healthy.
func TestRun_FutureTipIsDown(t *testing.T) {
	if code := run([]string{"--lookback", "96h"}, testDeps(-time.Hour, nil)); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// MH-05 boundary: skew inside the tolerance still counts as healthy.
func TestRun_SmallSkewIsHealthy(t *testing.T) {
	if code := run([]string{"--lookback", "96h"}, testDeps(-time.Minute, nil)); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// MH-07: a missing FETCH_HEAD does not change the status.
func TestRun_NoFetchHeadStillHealthy(t *testing.T) {
	d := testDeps(time.Hour, nil)
	d.fetchMtime = func(context.Context, string) (time.Time, bool) { return time.Time{}, false }
	if code := run([]string{"--lookback", "96h"}, d); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// MH-08: the probe runs only read-only git commands, and default fetchMtime delegates to gitOutput.
func TestRun_ReadOnly(t *testing.T) {
	var calls [][]string
	d := testDeps(time.Hour, nil)
	d.fetchMtime = nil // exercise default fetchMtime resolution through gitOutput
	inner := d.gitOutput
	d.gitOutput = func(ctx context.Context, repo string, args ...string) (string, error) {
		calls = append(calls, args)
		return inner(ctx, repo, args...)
	}
	run([]string{"--lookback", "96h"}, d)
	if len(calls) != 2 || calls[0][0] != "rev-parse" || calls[1][0] != "log" {
		t.Fatalf("git calls = %v, want rev-parse and log invocations", calls)
	}
}

// TestRun_BoundsEverySubprocessWithOneDeadline covers MH-09. merge-health is
// invoked by a scheduler (absence-alarm) that runs other pulses after it, so a
// git call that never returns must not stall the tick. Both injected host
// observations must therefore receive a context that already carries a
// deadline, and it must be the same one -- a second, later deadline would let
// the probe outlive its own budget.
func TestRun_BoundsEverySubprocessWithOneDeadline(t *testing.T) {
	var fetchDeadline, gitDeadline time.Time
	var fetchOK, gitOK bool

	d := testDeps(1*time.Hour, nil)
	d.fetchMtime = func(ctx context.Context, _ string) (time.Time, bool) {
		fetchDeadline, fetchOK = ctx.Deadline()
		return t0.Add(-10 * time.Minute), true
	}
	inner := d.gitOutput
	d.gitOutput = func(ctx context.Context, repo string, args ...string) (string, error) {
		gitDeadline, gitOK = ctx.Deadline()
		return inner(ctx, repo, args...)
	}

	if code := run([]string{"--json"}, d); code != 0 {
		t.Fatalf("run exit = %d, want 0", code)
	}
	if !fetchOK {
		t.Error("fetchMtime received a context with no deadline (MH-09)")
	}
	if !gitOK {
		t.Error("gitOutput received a context with no deadline (MH-09)")
	}
	if fetchOK && gitOK && !fetchDeadline.Equal(gitDeadline) {
		t.Errorf("subprocesses ran under different deadlines %v vs %v; want one tick budget (MH-09)",
			fetchDeadline, gitDeadline)
	}
}
