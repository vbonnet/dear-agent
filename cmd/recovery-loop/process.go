// Per-job lifecycle for one tick: decide, act, and decide again.
//
// Split from main.go to keep each file about one thing; the outcome-writing
// half lives in record.go.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

func processJob(
	ctx context.Context,
	job recoveryloop.Job,
	opts *options,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	snoozes map[string]absencealarm.Snooze,
	truth recoveryloop.PulseTruth,
	launchdJobs map[string]recoveryloop.LaunchdJobInfo,
	launchdErr error,
	host recoveryloop.HostOps,
	notifyFn notifier,
	now time.Time,
	stderr io.Writer,
) {
	prev := state.Jobs[job.Name]

	// A snooze silences a job wherever it sits in the lifecycle, including
	// while a verification is still open. Settling first would escalate over
	// an outage an operator has explicitly acknowledged (RL-05, RL-22).
	if sn, snoozed := recoveryloop.IsJobSnoozed(job, snoozes, now); snoozed {
		recordSnoozed(job, sn, state, rep, prev)
		return
	}

	// A verification left open by an earlier tick is always conclusive: it
	// either confirms the pulse returned, converts to a counted failure, or
	// reports that the grace window is still open. In every case this job is
	// done for this tick and firing another action would only reset the clock.
	if !prev.PendingDeadline.IsZero() {
		// A listing that failed leaves an empty map, which VerifyRecovery
		// would read as "the service is not loaded": a transient launchctl
		// failure would convert every pending recovery into a counted failure
		// and escalate. Not observed is not observed absent (RL-41).
		// Hold only when the verdict genuinely depends on the listing. A
		// pulse-backed job whose grace window has expired with the pulse
		// still absent has a conclusive answer from the heartbeat alone, and
		// holding it would postpone a real failure indefinitely every time
		// launchctl hiccups.
		if launchdErr != nil && job.LaunchdLabel != "" && !pulseVerdictIsConclusive(job, truth, prev, now) {
			holdPending(job, prev, launchdErr, state, rep)
			return
		}
		settlePending(job, opts, state, rep, truth, launchdJobs, host, prev, now, notifyFn, stderr)
		return
	}

	action, plannedStatus, reason := recoveryloop.PlanJob(job, snoozes, truth, launchdJobs, host, now)

	if action == recoveryloop.ActionNone {
		recordClear(job, plannedStatus, reason, now, state, rep, opts, prev, truth, stderr)
		return
	}

	// The give-up decision comes first, so a dry run previews what a real tick
	// would do. Reporting a planned remediation for a job past the threshold
	// would describe an action the loop is specifically not going to take.
	givenUp := opts.giveUpAfter > 0 && prev.ConsecutiveFailures >= opts.giveUpAfter

	if opts.dryRun {
		reportDryRun(job, action, reason, givenUp, prev, rep)
		return
	}

	// A job that has failed this many times will not be fixed by firing the
	// same action again. Stay loud instead of thrashing.
	if givenUp {
		recordGivenUp(job, action, reason, now, state, rep, opts, truth, prev, notifyFn, stderr)
		return
	}

	// The boundary that post-action evidence must beat is when the action
	// actually ran, not when the tick started. `now` was captured before
	// LoadPulseTruth, and absence-alarm runs on the same 10-minute schedule as
	// this loop, so a heartbeat published between the two would make
	// pre-action evidence look post-action and verify a recovery that had not
	// happened (RL-39).
	actionAt := host.Now()
	actionCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	execErr := recoveryloop.ExecuteRecovery(actionCtx, job, action, host)
	cancel()

	if execErr != nil {
		recordFailure(job, action, reason, execErr, now, state, rep, opts, truth, notifyFn, stderr)
		return
	}

	// The command exited zero. That is not recovery. Re-observe the host and
	// let the condition itself say whether it cleared.
	verifyCtx, cancelVerify := context.WithTimeout(ctx, opts.timeout)
	freshLaunchd, listErr := host.LaunchdList(verifyCtx)
	cancelVerify()
	if listErr != nil {
		// Falling back to the pre-action snapshot would feed VerifyRecovery
		// the very defect that triggered the action and score the remediation
		// a failure without ever observing the current host. A transient
		// listing failure means structural truth is unavailable, which is
		// pending, not failed.
		fmt.Fprintf(stderr, "recovery-loop: re-list launchd for verification: %v\n", listErr)
		recordPending(job, action, fmt.Sprintf(
			"%s ran; launchd could not be re-observed to verify it (%v)", action, listErr),
			now, actionAt, state, rep, opts, prev, stderr)
		return
	}
	outcome := recoveryloop.VerifyRecovery(job, action, truth, freshLaunchd, host, now, actionAt)

	switch {
	case outcome.Verified:
		recordVerified(job, action, outcome.Reason, prev.ConsecutiveFailures+1, now, state, rep, opts, stderr)
	case outcome.Status == recoveryloop.StatusPending:
		recordPending(job, action, outcome.Reason, now, actionAt, state, rep, opts, prev, stderr)
	default:
		recordFailure(job, action, outcome.Reason, errors.New(outcome.Reason), now, state, rep, opts, truth, notifyFn, stderr)
	}
}

// settlePending resolves a verification left open by an earlier tick.
//
// It re-runs the full verification rather than reading pulse truth alone. A
// file-mtime pulse can look present while the service that writes it has since
// been unloaded or started failing to launch, and declaring recovery from the
// pulse alone would call that job fixed.
func settlePending(
	job recoveryloop.Job,
	opts *options,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	truth recoveryloop.PulseTruth,
	launchdJobs map[string]recoveryloop.LaunchdJobInfo,
	host recoveryloop.HostOps,
	prev recoveryloop.JobState,
	now time.Time,
	notifyFn notifier,
	stderr io.Writer,
) {
	outcome := recoveryloop.VerifyRecovery(job, prev.PendingAction, truth, launchdJobs, host, now, prev.PendingSince)
	switch {
	case outcome.Verified:
		recordVerified(job, prev.PendingAction, outcome.Reason, prev.ConsecutiveFailures+1, now, state, rep, opts, stderr)
		return
	case outcome.Status == recoveryloop.StatusFailed:
		// A structural condition came back or never cleared: that is
		// observable now, so there is nothing left to wait for.
		recordFailure(job, prev.PendingAction, outcome.Reason, errors.New(outcome.Reason),
			now, state, rep, opts, truth, notifyFn, stderr)
		return
	}

	if now.After(prev.PendingDeadline) {
		absentFor := truth.AbsentFor(job.Pulse, now)
		reason := fmt.Sprintf("pulse %q did not return within %s of %s",
			job.Pulse, opts.verifyGrace, prev.PendingAction)
		if absentFor > 0 {
			reason += fmt.Sprintf("; absent for %s", absentFor.Round(time.Minute))
		}
		recordFailure(job, prev.PendingAction, reason, errors.New(reason), now, state, rep, opts, truth, notifyFn, stderr)
		return
	}
	// Still inside the grace window: the answer is not in yet, and firing
	// another action would only reset the clock.
	st := prev
	st.LastStatus = recoveryloop.StatusPending
	state.Jobs[job.Name] = st
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:    job.Name,
		Status: recoveryloop.StatusPending,
		Action: prev.PendingAction,
		// The same ordinal the recovery.pending record used for this action,
		// so a consumer following one attempt across ticks sees one number.
		Attempt:     prev.ConsecutiveFailures + 1,
		HumanNeeded: prev.HumanNeeded,
		Reason: fmt.Sprintf("awaiting pulse %q until %s",
			job.Pulse, prev.PendingDeadline.Format(time.RFC3339)),
	})
	rep.Pending++
	if prev.HumanNeeded {
		rep.HumanNeeded++
	}
}

// holdPending leaves an open verification open because the host could not be
// observed this tick. It judges nothing and counts nothing: the deadline and
// failure count are carried forward untouched.
func holdPending(
	job recoveryloop.Job,
	prev recoveryloop.JobState,
	cause error,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
) {
	st := prev
	st.LastStatus = recoveryloop.StatusPending
	state.Jobs[job.Name] = st
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:         job.Name,
		Status:      recoveryloop.StatusPending,
		Action:      prev.PendingAction,
		Attempt:     prev.ConsecutiveFailures + 1,
		HumanNeeded: prev.HumanNeeded,
		Reason: fmt.Sprintf("holding verification of %s: launchd state unavailable this tick (%v)",
			prev.PendingAction, cause),
	})
	rep.Pending++
	if prev.HumanNeeded {
		rep.HumanNeeded++
	}
}

// recordSnoozed reports a job an operator has explicitly silenced.
//
// It clears any open verification: while a job is snoozed nobody is judging
// the outcome of its last remediation, and holding a deadline that lapses
// under the snooze would convert into a counted failure the moment it lifts.
// The failure count itself is preserved -- a snooze hides a condition, it does
// not fix one.

// reportDryRun describes what a real tick would do to this job, including the
// give-up suppression, so a preview cannot advertise an action the loop is
// specifically not going to take (RL-44).
func reportDryRun(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	reason string,
	givenUp bool,
	prev recoveryloop.JobState,
	rep *recoveryloop.Heartbeat,
) {
	status := recoveryloop.StatusUnhealthy
	planned := action
	note := " (planned, dry-run)"
	if givenUp {
		status = recoveryloop.StatusFailed
		planned = recoveryloop.ActionNone
		note = fmt.Sprintf(" (suppressed: %d consecutive failures, dry-run)", prev.ConsecutiveFailures)
	}
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:         job.Name,
		Status:      status,
		Action:      planned,
		Attempt:     prev.ConsecutiveFailures,
		HumanNeeded: givenUp,
		Reason:      reason + note,
	})
	if givenUp {
		rep.Failed++
		rep.HumanNeeded++
		return
	}
	rep.Planned++
}
