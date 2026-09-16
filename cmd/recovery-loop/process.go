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
	pulseSource recoveryloop.PulseSourceStatus,
	launchdJobs map[string]recoveryloop.LaunchdJobInfo,
	launchdErr error,
	host recoveryloop.HostOps,
	notifyFn notifier,
	now time.Time,
	stderr io.Writer,
) {
	prev := state.Jobs[job.Name]

	// A snooze silences a job wherever it sits in the lifecycle, including
	// while a verification or durable escalation retry is still open. Settling
	// first would escalate over an outage an operator explicitly acknowledged
	// (RL-05, RL-22, RL-35).
	if sn, snoozed := recoveryloop.IsJobSnoozed(job, snoozes, now); snoozed {
		recordSnoozed(job, sn, state, rep, prev)
		return
	}

	prev = boundPendingDeadline(job.Name, prev, state, now, opts.verifyGrace)

	// A verification left open by an earlier tick is always conclusive: it
	// either confirms the pulse returned, converts to a counted failure, or
	// reports that the grace window is still open. In every case this job is
	// done for this tick and firing another action would only reset the clock.
	if handleOpenVerification(job, opts, state, rep, truth, pulseSource, launchdJobs, launchdErr, host, prev, now, notifyFn, stderr) {
		return
	}

	// The initial launchd listing is an observation boundary, not an empty
	// collection. Treating its nil map as authoritative makes a transient
	// launchctl failure look like every service is unloaded and can trigger a
	// fleet-wide bootstrap. Unknown state permits no action (RL-48).
	if launchdErr != nil && job.LaunchdLabel != "" {
		recordObservationUnavailable(job, launchdErr, rep, prev)
		return
	}

	action, plannedStatus, reason := recoveryloop.PlanJob(job, snoozes, truth, pulseSource, launchdJobs, host, now)

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
	// A new action supersedes any deadline provenance from the prior attempt.
	// If this action fails before opening a new verification, a later recovery
	// must not be annotated against the older attempt's deadline.
	prev.MissedVerificationDeadline = time.Time{}
	state.Jobs[job.Name] = prev
	actionAt := host.Now()
	actionCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	execErr := recoveryloop.ExecuteRecovery(actionCtx, job, action, host)
	cancel()

	if execErr != nil {
		recordFailure(job, action, reason, execErr, now, actionAt, state, rep, opts, truth, notifyFn, stderr)
		return
	}
	if outcome, failed := recoveryloop.VerifyIndependentStructure(job, action, host); failed {
		recordFailure(job, action, outcome.Reason, errors.New(outcome.Reason), now, actionAt,
			state, rep, opts, truth, notifyFn, stderr)
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
		recordPendingUnavailable(job, action, fmt.Sprintf(
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
		recordFailure(job, action, outcome.Reason, errors.New(outcome.Reason), now, actionAt, state, rep, opts, truth, notifyFn, stderr)
	}
}

// boundPendingDeadline limits the damage from a host clock that jumped ahead
// when a pending action was recorded and was later corrected. It never moves
// PendingSince, so evidence still has to post-date the original action.
func boundPendingDeadline(
	jobName string,
	prev recoveryloop.JobState,
	state *recoveryloop.State,
	now time.Time,
	grace time.Duration,
) recoveryloop.JobState {
	if prev.PendingDeadline.IsZero() {
		return prev
	}
	latestDeadline := now.Add(grace)
	if prev.PendingDeadline.After(latestDeadline) {
		prev.PendingDeadline = latestDeadline
		state.Jobs[jobName] = prev
	}
	return prev
}

// handleOpenVerification owns the one-action-at-a-time boundary. Once a prior
// action is pending, this tick either settles it or holds it for more evidence;
// it never plans a second action.
func handleOpenVerification(
	job recoveryloop.Job,
	opts *options,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	truth recoveryloop.PulseTruth,
	pulseSource recoveryloop.PulseSourceStatus,
	launchdJobs map[string]recoveryloop.LaunchdJobInfo,
	launchdErr error,
	host recoveryloop.HostOps,
	prev recoveryloop.JobState,
	now time.Time,
	notifyFn notifier,
	stderr io.Writer,
) bool {
	if prev.PendingDeadline.IsZero() {
		return false
	}
	// A launchd listing failure cannot hide an independently observable
	// structural failure. In particular, a zero-exit reinstall that leaves its
	// binary missing is conclusively failed even while launchd is unavailable.
	if outcome, failed := recoveryloop.VerifyIndependentStructure(job, prev.PendingAction, host); failed {
		recordSettledPendingFailure(job, outcome.Reason, now, state, rep, opts, truth, prev, notifyFn, stderr)
		return true
	}
	// A listing that failed leaves an empty map, which VerifyRecovery would
	// read as "the service is not loaded": a transient launchctl failure would
	// convert every pending recovery into a counted failure. Hold only when the
	// heartbeat does not already settle the outcome (RL-41).
	if launchdErr != nil && job.LaunchdLabel != "" {
		if pulseVerdictIsConclusive(job, truth, pulseSource, prev, now) {
			failExpiredPulseVerification(job, opts, state, rep, truth, prev, now, notifyFn, stderr)
			return true
		}
		holdPendingUnavailable(job, prev, fmt.Sprintf(
			"holding verification of %s: launchd state unavailable this tick (%v)",
			prev.PendingAction, launchdErr), state, rep)
		return true
	}
	settlePending(job, opts, state, rep, truth, pulseSource, launchdJobs, host, prev, now, notifyFn, stderr)
	return true
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
	pulseSource recoveryloop.PulseSourceStatus,
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
		if job.Pulse != "" && !job.PulseIsStructural {
			ev := truth[job.Pulse].Evidence
			if !prev.PendingDeadline.IsZero() && ev.After(prev.PendingDeadline) {
				outcome.Reason += fmt.Sprintf("; pulse evidence arrived %s after the verification deadline",
					ev.Sub(prev.PendingDeadline).Round(time.Second))
			}
		}
		recordVerified(job, prev.PendingAction, outcome.Reason, prev.ConsecutiveFailures+1, now, state, rep, opts, stderr)
		return
	case outcome.Status == recoveryloop.StatusFailed:
		// A structural condition came back or never cleared: that is
		// observable now, so there is nothing left to wait for.
		recordSettledPendingFailure(job, outcome.Reason, now, state, rep, opts, truth, prev, notifyFn, stderr)
		return
	}
	sourceOwnerDown := job.Pulse == recoveryloop.AbsenceAlarmHeartbeatPulse &&
		pulseSource.RequiresRecovery()
	if job.Pulse != "" && !job.PulseIsStructural && !sourceOwnerDown &&
		!truth.VerificationObservationAvailable(job.Pulse, prev.PendingSince, now) {
		holdPendingUnavailable(job, prev, fmt.Sprintf(
			"holding verification of %s: pulse %q observation unavailable (%s)",
			prev.PendingAction, job.Pulse, outcome.Reason), state, rep)
		return
	}

	if now.After(prev.PendingDeadline) {
		failExpiredPulseVerification(job, opts, state, rep, truth, prev, now, notifyFn, stderr)
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

// failExpiredPulseVerification settles an open attempt from pulse evidence
// alone. This remains safe when launchd cannot be listed: a missed pulse
// deadline is independently conclusive, while a nil launchd map is not proof
// that the service is unloaded.
func failExpiredPulseVerification(
	job recoveryloop.Job,
	opts *options,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	truth recoveryloop.PulseTruth,
	prev recoveryloop.JobState,
	now time.Time,
	notifyFn notifier,
	stderr io.Writer,
) {
	absentFor := truth.AbsentFor(job.Pulse, now)
	reason := fmt.Sprintf("pulse %q did not return within %s of %s",
		job.Pulse, opts.verifyGrace, prev.PendingAction)
	if absentFor > 0 {
		reason += fmt.Sprintf("; absent for %s", absentFor.Round(time.Minute))
	}
	recordSettledPendingFailure(job, reason, now, state, rep, opts, truth, prev, notifyFn, stderr)
}

// recordSettledPendingFailure records failure of an open attempt and captures
// its deadline only when that attempt actually outlived the deadline. A newer
// action clears this provenance before it runs.
func recordSettledPendingFailure(
	job recoveryloop.Job,
	reason string,
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	truth recoveryloop.PulseTruth,
	prev recoveryloop.JobState,
	notifyFn notifier,
	stderr io.Writer,
) {
	if !prev.PendingDeadline.IsZero() && now.After(prev.PendingDeadline) {
		prev.MissedVerificationDeadline = prev.PendingDeadline
		state.Jobs[job.Name] = prev
	}
	actionBoundary := prev.PendingSince
	if actionBoundary.After(now) {
		// The pending deadline was already bounded against a corrected clock.
		// Once that quarantined attempt settles, do not copy its still-future
		// action timestamp into the next proof boundary: current evidence would
		// otherwise remain unusable until wall time caught up.
		actionBoundary = now
	}
	recordFailure(job, prev.PendingAction, reason, errors.New(reason), now, actionBoundary, state, rep, opts, truth, notifyFn, stderr)
}

// holdPendingUnavailable leaves an open verification open because a required
// observation was unavailable this tick. It judges no recovery and counts no
// failure: the deadline and failure count are carried forward, while the tick
// fails closed instead of reading green.
func holdPendingUnavailable(
	job recoveryloop.Job,
	prev recoveryloop.JobState,
	reason string,
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
		Reason:      reason,
	})
	rep.Pending++
	rep.Unavailable++
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
	humanNeeded := prev.HumanNeeded || givenUp
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
		HumanNeeded: humanNeeded,
		Reason:      reason + note,
	})
	if humanNeeded {
		rep.HumanNeeded++
	}
	if givenUp {
		rep.Failed++
		return
	}
	rep.Planned++
}
