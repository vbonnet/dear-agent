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
		fetchMtime: func(string) (time.Time, bool) { return t0.Add(-10 * time.Minute), true },
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
	d.fetchMtime = func(string) (time.Time, bool) { return time.Time{}, false }
	if code := run([]string{"--lookback", "96h"}, d); code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
}

// MH-08: the probe runs exactly one read-only git command.
func TestRun_ReadOnly(t *testing.T) {
	var calls [][]string
	d := testDeps(time.Hour, nil)
	inner := d.gitOutput
	d.gitOutput = func(ctx context.Context, repo string, args ...string) (string, error) {
		calls = append(calls, args)
		return inner(ctx, repo, args...)
	}
	run([]string{"--lookback", "96h"}, d)
	if len(calls) != 1 || calls[0][0] != "log" {
		t.Fatalf("git calls = %v, want exactly one log invocation", calls)
	}
}
