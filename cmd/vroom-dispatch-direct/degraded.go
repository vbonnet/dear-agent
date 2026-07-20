package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/notify"
)

// retryAttempts/retryBaseDelay bound retries for the fail-closed subprocess
// queries (bd ready, agm session list, gh pr list). Each is a single point of
// failure that halts the entire dispatch tick on ANY error — correct instinct
// (better to skip a tick than double-dispatch), but a transient blip (a gh
// token mid-rotation, a momentary dolt lock, a flaky agm CLI call) doesn't
// deserve to burn a whole tick. Retrying inside the same process costs
// seconds; failing the tick costs a full dispatch-loop INTERVAL (5 minutes by
// default) of the flywheel doing nothing. This is what should have absorbed
// the ~20-minute gh-auth-rotation window instead of four consecutive dead
// ticks (dispatch-loop.log, 2026-07-20T04:24:26Z–04:39:58Z).
const retryAttempts = 3

// retryBaseDelay is a var (not const) so tests can shrink it instead of
// eating the real backoff.
var retryBaseDelay = 3 * time.Second

// withRetry runs fn up to retryAttempts times with linear backoff, returning
// the last error if every attempt fails. It does not retry once ctx is done —
// a canceled/expired context means further attempts are pointless.
func withRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 1; attempt <= retryAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}
		if attempt < retryAttempts {
			select {
			case <-time.After(retryBaseDelay * time.Duration(attempt)):
			case <-ctx.Done():
				return err
			}
		}
	}
	return err
}

// execStderr extracts the stderr captured by exec.Cmd.Output() (populated on
// *exec.ExitError whenever cmd.Stderr was left nil) and collapses it to a
// bounded single-line snippet. Output() error's own Error() method returns
// only "exit status 1" — the actual cause (e.g. "gh: To use GitHub CLI in a
// headless environment, ..." or an auth failure message) was silently
// available but never surfaced, which is why diagnosing the 2026-07-20 gh
// auth incident required correlating log timestamps instead of just reading
// the error.
func execStderr(err error) string {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || len(exitErr.Stderr) == 0 {
		return ""
	}
	s := strings.Join(strings.Fields(string(exitErr.Stderr)), " ")
	const maxLen = 300
	if len(s) > maxLen {
		s = s[:maxLen] + "…"
	}
	return s
}

// wrapExecErr wraps a subprocess error with its stderr snippet (if any) so
// the fail-closed message names the actual cause instead of a bare exit code.
func wrapExecErr(context string, err error) error {
	if detail := execStderr(err); detail != "" {
		return fmt.Errorf("%s: %w: %s", context, err, detail)
	}
	return fmt.Errorf("%s: %w", context, err)
}

// alertThreshold is the number of consecutive fail-closed ticks (persisted
// across process invocations — each tick is a fresh process) after which a
// halted dispatch loop escalates instead of merely logging. One retried
// failure is treated as a normal transient blip; two in a row means the
// flywheel has been a no-op for a full extra interval and a human should
// know, not just a grep-able log line among hundreds of healthy ticks.
const alertThreshold = 2

// dispatchState is the durable record of vroom-dispatch-direct's control-
// plane health. Persisted to disk because each tick is a fresh process
// (see dispatch-loop.sh) — "consecutive failures" cannot live in memory.
type dispatchState struct {
	LastRun             time.Time `json:"last_run"`
	OK                  bool      `json:"ok"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastError           string    `json:"last_error,omitempty"`
	LastSuccess         time.Time `json:"last_success,omitzero"`
}

// loadDispatchState reads the persisted state, defaulting to a zero value
// (first run, or a corrupt/missing file — never fatal, this is diagnostic
// state, not a source of truth for dispatch decisions).
func loadDispatchState(path string) dispatchState {
	data, err := os.ReadFile(path)
	if err != nil {
		return dispatchState{}
	}
	var s dispatchState
	if err := json.Unmarshal(data, &s); err != nil {
		return dispatchState{}
	}
	return s
}

func saveDispatchState(path string, s dispatchState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dispatch state: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// recordFailure persists a fail-closed halt and, once the streak reaches
// alertThreshold, escalates loudly: a banner on stderr that no per-tick log
// summary can bury, plus a best-effort desktop notification. This is the
// fix for PROBLEM 1 — a token rotation (or any transient gh/bd/agm failure)
// must never invisibly stall the flywheel while the loop keeps ticking and
// looking alive.
func recordFailure(ctx context.Context, statePath string, cause error) {
	s := loadDispatchState(statePath)
	s.LastRun = time.Now()
	s.OK = false
	s.ConsecutiveFailures++
	s.LastError = cause.Error()
	if err := saveDispatchState(statePath, s); err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch-direct: WARNING: failed to persist degraded state: %v\n", err)
	}

	if s.ConsecutiveFailures < alertThreshold {
		return
	}

	fmt.Fprintf(os.Stderr, "\n"+
		"########################################################\n"+
		"# VROOM-DISPATCH-DIRECT DEGRADED\n"+
		"# %d consecutive fail-closed ticks — the flywheel has been\n"+
		"# a no-op for this entire streak while still \"running\".\n"+
		"# last error: %v\n"+
		"# state file: %s\n"+
		"########################################################\n\n",
		s.ConsecutiveFailures, cause, statePath)

	notifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sendDesktopAlert(notifyCtx,
		fmt.Sprintf("%d consecutive fail-closed ticks: %v", s.ConsecutiveFailures, cause)); err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch-direct: WARNING: desktop alert failed (non-fatal): %v\n", err)
	}
}

// recordSuccess clears the failure streak on a tick that completed without a
// fail-closed halt (whether or not it dispatched anything — "nothing new to
// dispatch" is a normal steady state, see the package doc).
func recordSuccess(statePath string) {
	s := loadDispatchState(statePath)
	now := time.Now()
	s.LastRun = now
	s.OK = true
	s.ConsecutiveFailures = 0
	s.LastError = ""
	s.LastSuccess = now
	if err := saveDispatchState(statePath, s); err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch-direct: WARNING: failed to persist dispatch state: %v\n", err)
	}
}

// sendDesktopAlert fires a native desktop notification via pkg/notify's
// DesktopDispatcher — the same mechanism already used elsewhere in this repo
// to surface events to the human. Best-effort: a headless/CI environment
// without a GUI session should not turn this into a second failure mode.
// Package var so tests can stub it rather than firing a real notification.
var sendDesktopAlert = func(ctx context.Context, body string) error {
	d := notify.NewDesktopDispatcher()
	defer d.Close()
	return d.Dispatch(ctx, &notify.Notification{
		Title:     "VROOM dispatch degraded",
		Body:      body,
		Source:    "vroom-dispatch-direct",
		Timestamp: time.Now(),
	})
}
