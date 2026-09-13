// Outcome recording: the only places that write state, report lines, journal
// records and escalations.
//
// Every durable write in this command funnels through this file, which is what
// makes the --dry-run guarantee checkable by reading one place (RL-38).
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

func recordSnoozed(
	job recoveryloop.Job,
	sn absencealarm.Snooze,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	prev recoveryloop.JobState,
) {
	st := prev
	st.LastStatus = recoveryloop.StatusSnoozed
	st.PendingAction = ""
	st.PendingSince = time.Time{}
	st.PendingDeadline = time.Time{}
	state.Jobs[job.Name] = st
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:    job.Name,
		Status: recoveryloop.StatusSnoozed,
		Action: recoveryloop.ActionNone,
		Reason: fmt.Sprintf("snoozed until %s: %s", sn.Until.Format(time.RFC3339), sn.Reason),
	})
	rep.Snoozed++
}

// recordClear handles a job that needs no action.
func recordClear(
	job recoveryloop.Job,
	status recoveryloop.RecoveryStatus,
	reason string,
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	prev recoveryloop.JobState,
	truth recoveryloop.PulseTruth,
	stderr io.Writer,
) {
	// PlanJob reports HEALTHY on the structural checks alone when no pulse
	// truth is available -- a stale or missing absence-alarm heartbeat, or a
	// pulse nothing is configured to emit. For a job that declares a pulse,
	// that is "could not check", not health, and converting it into a cleared
	// condition is the original defect wearing a different hat: a recovery
	// announced without observing the thing that was broken.
	unverifiable := status == recoveryloop.StatusHealthy &&
		job.Pulse != "" && !truth.Present(job.Pulse)
	if unverifiable {
		recordUnverifiable(job, reason, state, rep, prev)
		return
	}

	// A job that was failing and is now observed healthy has genuinely
	// recovered, but only if the observation is newer than the attempt that
	// failed. A long-window pulse from before the last action can still be
	// inside its freshness window, and clearing on that would let the failure
	// just recorded evaporate without anything new being seen (RL-39).
	if status == recoveryloop.StatusHealthy && prev.ConsecutiveFailures > 0 {
		if job.Pulse != "" && !prev.LastAttemptTime.IsZero() {
			if ev := truth[job.Pulse].Evidence; !ev.After(prev.LastAttemptTime) {
				recordUnverifiable(job, fmt.Sprintf(
					"%s, but pulse %q was last observed %s, before the %s that failed",
					reason, job.Pulse, evidenceStampCLI(ev), prev.LastAction),
					state, rep, prev)
				return
			}
		}
		recordVerified(job, prev.LastAction, "condition cleared: "+reason, prev.ConsecutiveFailures, now, state, rep, opts, stderr)
		return
	}
	if status == recoveryloop.StatusHealthy {
		state.Jobs[job.Name] = recoveryloop.JobState{
			LastAttemptTime: now,
			LastStatus:      recoveryloop.StatusHealthy,
		}
	}
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:    job.Name,
		Status: status,
		Action: recoveryloop.ActionNone,
		Reason: reason,
	})
	switch status {
	case recoveryloop.StatusHealthy:
		rep.Healthy++
	case recoveryloop.StatusSnoozed:
		rep.Snoozed++
	case recoveryloop.StatusRecovered, recoveryloop.StatusFailed,
		recoveryloop.StatusUnhealthy, recoveryloop.StatusPending:
		// Not reachable: PlanJob returns only healthy or snoozed alongside
		// ActionNone, and every other status is recorded by its own path.
	}
}

// recordUnverifiable reports a job whose health could not be observed.
//
// It takes no action -- there is no evidence anything is wrong, and restarting
// jobs because the monitor went quiet would turn one outage into many -- but it
// refuses to call the job healthy or to reset a standing failure count. Silence
// is not an all-clear.
func recordUnverifiable(
	job recoveryloop.Job,
	reason string,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	prev recoveryloop.JobState,
) {
	st := prev
	// LastAttemptTime is the boundary a later tick compares pulse evidence
	// against. Advancing it here would move the goalposts on every tick that
	// simply could not see anything, so an outage would never accumulate a
	// comparable observation. Nothing was attempted, so nothing is stamped.
	st.LastStatus = recoveryloop.StatusPending
	state.Jobs[job.Name] = st
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:         job.Name,
		Status:      recoveryloop.StatusPending,
		Action:      recoveryloop.ActionNone,
		Attempt:     prev.ConsecutiveFailures,
		HumanNeeded: prev.HumanNeeded,
		Reason: fmt.Sprintf("%s, but pulse %q was not observed: health unverifiable",
			reason, job.Pulse),
	})
	rep.Pending++
	if prev.HumanNeeded {
		rep.HumanNeeded++
	}
}

// recordVerified records a recovery that was observed to have cleared the
// condition. This is the only path that may write StatusRecovered.
func recordVerified(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	reason string,
	attempt int,
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	stderr io.Writer,
) {
	if attempt < 1 {
		attempt = 1
	}
	state.Jobs[job.Name] = recoveryloop.JobState{
		ConsecutiveFailures: 0,
		LastAttemptTime:     now,
		LastAction:          action,
		LastStatus:          recoveryloop.StatusRecovered,
		HumanNeeded:         false,
	}
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:     job.Name,
		Status:  recoveryloop.StatusRecovered,
		Action:  action,
		Attempt: attempt,
		Reason:  reason,
	})
	rep.Recovered++
	appendJournal(opts, stderr, recoveryloop.JournalRecord{
		Time:        now,
		Kind:        "recovery.verified",
		Job:         job.Name,
		Action:      action,
		Status:      recoveryloop.StatusRecovered,
		Attempt:     attempt,
		HumanNeeded: false,
		Reason:      reason,
	})
}

// recordPending records that a remediation ran and its outcome is not yet
// observable. It deliberately does not reset the failure counter: nothing has
// been proven yet.
func recordPending(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	reason string,
	now time.Time,
	actionAt time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	prev recoveryloop.JobState,
	stderr io.Writer,
) {
	if actionAt.IsZero() {
		actionAt = now
	}
	unhealthySince := prev.UnhealthySince
	if unhealthySince.IsZero() {
		unhealthySince = now
	}
	state.Jobs[job.Name] = recoveryloop.JobState{
		ConsecutiveFailures: prev.ConsecutiveFailures,
		LastAttemptTime:     now,
		LastAction:          action,
		LastStatus:          recoveryloop.StatusPending,
		HumanNeeded:         prev.HumanNeeded,
		PendingAction:       action,
		PendingSince:        actionAt,
		PendingDeadline:     actionAt.Add(opts.verifyGrace),
		UnhealthySince:      unhealthySince,
		LastEscalated:       prev.LastEscalated,
	}
	// The attempt ordinal counts this attempt, so it starts at one. Reporting
	// the pre-action counter makes the first remediation read as "attempt 0".
	attempt := prev.ConsecutiveFailures + 1
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:     job.Name,
		Status:  recoveryloop.StatusPending,
		Action:  action,
		Attempt: attempt,
		// A job that already crossed the escalation threshold stays visible
		// as needing a human while the next remediation is in flight.
		// Dropping the flag here made the process exit 0, and the fleet read
		// green, in the middle of an unresolved outage.
		HumanNeeded: prev.HumanNeeded,
		Reason:      reason,
	})
	rep.Pending++
	if prev.HumanNeeded {
		rep.HumanNeeded++
	}
	appendJournal(opts, stderr, recoveryloop.JournalRecord{
		Time:        now,
		Kind:        "recovery.pending",
		Job:         job.Name,
		Action:      action,
		Status:      recoveryloop.StatusPending,
		Attempt:     attempt,
		HumanNeeded: prev.HumanNeeded,
		Reason:      reason,
	})
}

// recordGivenUp records a job whose remediation is suppressed because repeated
// attempts have not cleared the condition.
func recordGivenUp(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
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
	full := fmt.Sprintf("%s; %d consecutive recoveries did not clear it, remediation suppressed pending a human",
		reason, prev.ConsecutiveFailures)
	st := prev
	st.LastAttemptTime = now
	st.LastStatus = recoveryloop.StatusFailed
	st.HumanNeeded = true
	st.PendingAction = ""
	st.PendingSince = time.Time{}
	st.PendingDeadline = time.Time{}
	state.Jobs[job.Name] = st

	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:    job.Name,
		Status: recoveryloop.StatusFailed,
		// The action reported is the one that last actually ran. ActionNone
		// reads as "nothing was ever tried", which is the opposite of what a
		// give-up means and hides what to investigate.
		Action:      prev.LastAction,
		Attempt:     prev.ConsecutiveFailures,
		HumanNeeded: true,
		Reason:      full,
	})
	rep.Failed++
	rep.HumanNeeded++
	// The report above is emitted every tick, so the outage stays visible.
	// The durable escalation is rate-limited: appending an identical record
	// to the human-facing sink every ten minutes forever is how that sink
	// stops being read, which is the failure this change exists to fix.
	if prev.LastEscalated.IsZero() || !now.Before(prev.LastEscalated.Add(reEscalateInterval)) {
		escalate(job, action, prev.ConsecutiveFailures, full, now, state, opts, truth, notifyFn, stderr)
	}
}

func recordFailure(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	reason string,
	execErr error,
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	truth recoveryloop.PulseTruth,
	notifyFn notifier,
	stderr io.Writer,
) {
	prev := state.Jobs[job.Name]
	attempts := prev.ConsecutiveFailures + 1
	humanNeeded := attempts >= opts.escalateAfter
	unhealthySince := prev.UnhealthySince
	if unhealthySince.IsZero() {
		unhealthySince = now
	}

	state.Jobs[job.Name] = recoveryloop.JobState{
		ConsecutiveFailures: attempts,
		LastAttemptTime:     now,
		LastAction:          action,
		LastStatus:          recoveryloop.StatusFailed,
		HumanNeeded:         humanNeeded,
		UnhealthySince:      unhealthySince,
		LastEscalated:       prev.LastEscalated,
	}
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:         job.Name,
		Status:      recoveryloop.StatusFailed,
		Action:      action,
		Attempt:     attempts,
		HumanNeeded: humanNeeded,
		Reason:      reason,
		Error:       execErr.Error(),
	})
	rep.Failed++

	if humanNeeded {
		rep.HumanNeeded++
		escalate(job, action, attempts, reason, now, state, opts, truth, notifyFn, stderr)
	}

	appendJournal(opts, stderr, recoveryloop.JournalRecord{
		Time:        now,
		Kind:        "recovery.attempt",
		Job:         job.Name,
		Action:      action,
		Status:      recoveryloop.StatusFailed,
		Attempt:     attempts,
		HumanNeeded: humanNeeded,
		Reason:      reason,
		Error:       execErr.Error(),
	})
}

// appendJournal is the one place recovery journal records are written.
//
// Routing every append through it keeps --dry-run honest: a dry run reports
// what this tick would do and leaves no trace, including on the paths that
// settle work an earlier tick left open (RL-11).
func appendJournal(opts *options, stderr io.Writer, rec recoveryloop.JournalRecord) {
	if opts.dryRun {
		return
	}
	if err := recoveryloop.AppendJournal(opts.journalPath, rec); err != nil {
		fmt.Fprintf(stderr, "recovery-loop: append journal: %v\n", err)
	}
}

// escalate puts a failing job in front of a human on a sink that is actually
// watched (RL-30).
//
// A desktop banner is not durable and has no machine consumer: the live
// incident dispatched one on every tick for two days and nothing recorded that
// anyone had seen it. The escalation therefore also lands on the absence-alarm
// journal, which is the fleet's human-facing absence path, naming the pulse and
// how long it has really been absent.
func escalate(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	attempts int,
	reason string,
	now time.Time,
	state *recoveryloop.State,
	opts *options,
	truth recoveryloop.PulseTruth,
	notifyFn notifier,
	stderr io.Writer,
) {
	if opts.dryRun {
		return
	}
	pulse := job.Pulse
	if pulse == "" {
		pulse = job.Name
	}
	absentFor := truth.AbsentFor(job.Pulse, now)
	if absentFor == 0 {
		if st := state.Jobs[job.Name]; !st.UnhealthySince.IsZero() && now.After(st.UnhealthySince) {
			absentFor = now.Sub(st.UnhealthySince)
		}
	}
	// The reason carries the condition that actually failed to clear, which is
	// often structural rather than a missing pulse. Stating "pulse absent"
	// unconditionally sends whoever reads this to the wrong place.
	condition := fmt.Sprintf("pulse %q absent for %s", pulse, absentFor.Round(time.Minute))
	if job.Pulse != "" && truth.Present(job.Pulse) {
		condition = fmt.Sprintf("pulse %q is present; the unresolved condition is structural", pulse)
	} else if job.Pulse == "" {
		condition = fmt.Sprintf("no pulse configured; unhealthy for %s", absentFor.Round(time.Minute))
	}
	body := fmt.Sprintf(
		"recovery-loop could not restore %s. %s. %d consecutive recoveries failed to clear it (last action %s). %s",
		job.Name, condition, attempts, action, reason)

	// Delivery, not intent, is what the rate limit measures. Stamping
	// LastEscalated when no sink accepted the message would buy 24h of silence
	// for an escalation nobody received: the false-green shape applied to the
	// escalation path itself (RL-40).
	var delivered bool
	// The record's status is the pulse's real status. Stamping StatusAbsent on
	// a structural failure whose pulse is present tells a machine consumer the
	// opposite of what the reason text says.
	pulseStatus := absencealarm.StatusAbsent
	if job.Pulse != "" && truth.Present(job.Pulse) {
		pulseStatus = absencealarm.StatusPresent
	}
	if err := absencealarm.AppendJournal(opts.absenceJournal, absencealarm.JournalRecord{
		Time:   now,
		Kind:   "recovery.human_needed",
		Pulse:  pulse,
		Status: pulseStatus,
		Reason: body,
		Misses: attempts,
	}); err != nil {
		fmt.Fprintf(stderr, "recovery-loop: append absence escalation: %v\n", err)
	} else {
		delivered = true
	}

	if notifyFn != nil {
		notifyCtx, cancel := context.WithTimeout(context.Background(), defaultNotifyTimeout)
		title := fmt.Sprintf("HUMAN NEEDED: %s not recovered", job.Name)
		if err := notifyFn(notifyCtx, title, body); err != nil {
			fmt.Fprintf(stderr, "recovery-loop: notify: %v\n", err)
		} else {
			delivered = true
		}
		cancel()
	}

	st := state.Jobs[job.Name]
	st.HumanNeeded = true
	if delivered {
		st.LastEscalated = now
	} else {
		fmt.Fprintf(stderr,
			"recovery-loop: no escalation sink accepted the %s alert; will retry next tick\n", job.Name)
	}
	state.Jobs[job.Name] = st
}
