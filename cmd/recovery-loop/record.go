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
	"sort"
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
	if status == recoveryloop.StatusHealthy && job.Pulse != "" && !job.PulseIsStructural {
		if evidenceReason, admissible := currentPulseHealthReason(job, reason, truth, now); !admissible {
			recordUnverifiable(job, evidenceReason, state, rep, prev)
			return
		}
	}

	// A job that was failing and is now observed healthy has genuinely
	// recovered, but only if the observation is newer than the attempt that
	// failed. A long-window pulse from before the last action can still be
	// inside its freshness window, and clearing on that would let the failure
	// just recorded evaporate without anything new being seen (RL-39).
	if status == recoveryloop.StatusHealthy && prev.ConsecutiveFailures > 0 {
		verifiedReason := "condition cleared: " + reason
		if job.Pulse != "" && !job.PulseIsStructural {
			var admissible bool
			verifiedReason, admissible = clearingEvidenceReason(job, reason, prev, truth, now)
			if !admissible {
				recordUnverifiable(job, verifiedReason, state, rep, prev)
				return
			}
		}
		recordVerified(job, prev.LastAction, verifiedReason, prev.ConsecutiveFailures, now, state, rep, opts, stderr)
		return
	}
	if status == recoveryloop.StatusHealthy {
		state.Jobs[job.Name] = recoveryloop.JobState{
			LastAttemptTime:    now,
			LastStatus:         recoveryloop.StatusHealthy,
			PendingEscalations: prev.PendingEscalations,
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
		recoveryloop.StatusUnhealthy, recoveryloop.StatusPending,
		recoveryloop.StatusUnavailable:
		// Not reachable: PlanJob returns only healthy or snoozed alongside
		// ActionNone, and every other status is recorded by its own path.
	}
}

// currentPulseHealthReason refuses to turn an explicit-but-clock-invalid
// positive reading into current health. Planning and recovery verification
// share the same evidence-skew boundary, so the same fact cannot be HEALTHY in
// one path and unusable in another.
func currentPulseHealthReason(
	job recoveryloop.Job,
	planReason string,
	truth recoveryloop.PulseTruth,
	now time.Time,
) (string, bool) {
	fact := truth[job.Pulse]
	if !truth.Present(job.Pulse) {
		observed := "was not observed"
		if fact.Known {
			observed = fmt.Sprintf("reported status %q", fact.Status)
		}
		return fmt.Sprintf("%s, but pulse %q %s: health unverifiable",
			planReason, job.Pulse, observed), false
	}
	switch truth.ClassifyEvidence(job.Pulse, time.Time{}, now) {
	case recoveryloop.EvidenceMissing:
		return fmt.Sprintf("%s, but pulse %q carries no evidence timestamp: health unverifiable",
			planReason, job.Pulse), false
	case recoveryloop.EvidenceTooFarInFuture:
		return fmt.Sprintf("%s, but pulse %q carries evidence dated %s in the future: health unverifiable",
			planReason, job.Pulse, fact.Evidence.Sub(now).Round(time.Second)), false
	case recoveryloop.EvidenceAdmissible:
		return planReason, true
	case recoveryloop.EvidenceNotAfterBoundary:
		// No boundary is supplied above, so this case is unreachable.
	}
	return fmt.Sprintf("%s, but pulse %q evidence timing is unavailable: health unverifiable",
		planReason, job.Pulse), false
}

// clearingEvidenceReason applies the shared temporal evidence policy to the
// no-action clearing path. It returns either a verified recovery narrative or
// the fail-closed reason that must be reported as observation-unavailable.
func clearingEvidenceReason(
	job recoveryloop.Job,
	planReason string,
	prev recoveryloop.JobState,
	truth recoveryloop.PulseTruth,
	now time.Time,
) (string, bool) {
	ev := truth[job.Pulse].Evidence
	switch truth.ClassifyEvidence(job.Pulse, prev.LastAttemptTime, now) {
	case recoveryloop.EvidenceMissing:
		return fmt.Sprintf("%s, but pulse %q carries no evidence timestamp: health unverifiable",
			planReason, job.Pulse), false
	case recoveryloop.EvidenceTooFarInFuture:
		return fmt.Sprintf("%s, but pulse %q carries evidence dated %s in the future: health unverifiable",
			planReason, job.Pulse, ev.Sub(now).Round(time.Second)), false
	case recoveryloop.EvidenceNotAfterBoundary:
		return fmt.Sprintf("%s, but pulse %q was last observed %s, before the %s that failed: health unverifiable",
			planReason, job.Pulse, evidenceStampCLI(ev), prev.LastAction), false
	case recoveryloop.EvidenceAdmissible:
		reason := "condition cleared: " + planReason
		if !job.PulseIsStructural && !prev.MissedVerificationDeadline.IsZero() &&
			ev.After(prev.MissedVerificationDeadline) {
			reason += fmt.Sprintf("; pulse evidence arrived %s after the verification deadline",
				ev.Sub(prev.MissedVerificationDeadline).Round(time.Second))
		}
		return reason, true
	}
	return "pulse evidence classification unavailable", false
}

// recordObservationUnavailable reports a failed observation without changing
// the job's durable recovery state. No action ran, so there is no attempt to
// count, no pending verification to open, and no recovery journal entry to
// append (RL-48).
func recordObservationUnavailable(
	job recoveryloop.Job,
	cause error,
	rep *recoveryloop.Heartbeat,
	prev recoveryloop.JobState,
) {
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:         job.Name,
		Status:      recoveryloop.StatusUnavailable,
		Action:      recoveryloop.ActionNone,
		Attempt:     prev.ConsecutiveFailures,
		HumanNeeded: prev.HumanNeeded,
		Reason: fmt.Sprintf("launchd observation unavailable; no remediation attempted (%v)",
			cause),
	})
	rep.Unavailable++
	if prev.HumanNeeded {
		rep.HumanNeeded++
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
	st.LastStatus = recoveryloop.StatusUnavailable
	state.Jobs[job.Name] = st
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:         job.Name,
		Status:      recoveryloop.StatusUnavailable,
		Action:      recoveryloop.ActionNone,
		Attempt:     prev.ConsecutiveFailures,
		HumanNeeded: prev.HumanNeeded,
		Reason:      reason,
	})
	rep.Unavailable++
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
	pendingEscalations := state.Jobs[job.Name].PendingEscalations
	state.Jobs[job.Name] = recoveryloop.JobState{
		ConsecutiveFailures: 0,
		LastAttemptTime:     now,
		LastAction:          action,
		LastStatus:          recoveryloop.StatusRecovered,
		HumanNeeded:         false,
		PendingEscalations:  pendingEscalations,
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
		LastAttemptTime:     actionAt,
		LastAction:          action,
		LastStatus:          recoveryloop.StatusPending,
		HumanNeeded:         prev.HumanNeeded,
		PendingAction:       action,
		PendingSince:        actionAt,
		PendingDeadline:     actionAt.Add(opts.verifyGrace),
		UnhealthySince:      unhealthySince,
		LastEscalated:       prev.LastEscalated,
		PendingEscalations:  prev.PendingEscalations,
		PendingNotification: prev.PendingNotification,
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

// recordPendingUnavailable preserves the pending recovery lifecycle while
// marking this tick's required observation as unavailable. Pending state says
// the prior action is still awaiting a verdict; Unavailable makes the tick
// fail closed instead of returning a false-green exit code (RL-12, RL-41).
func recordPendingUnavailable(
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
	recordPending(job, action, reason, now, actionAt, state, rep, opts, prev, stderr)
	rep.Unavailable++
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
	full := fmt.Sprintf("%s; planned %s remediation suppressed after %d consecutive recoveries did not clear it, pending a human",
		reason, action, prev.ConsecutiveFailures)
	st := prev
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
	if prev.PendingNotification != nil {
		// The post-observation pass retries this incident's exact rejected
		// banner. Do not replace it with a logical duplicate every tick.
		return
	}
	due := escalationDue(prev.LastEscalated, now)
	if due {
		escalate(job, prev.LastAction, prev.ConsecutiveFailures, full, now, state, opts, truth, due, notifyFn, stderr)
	}
}

// escalationDue treats a future delivery timestamp as a corrected-clock
// artifact, not as permission to silence a standing outage until that future
// instant plus the normal rate-limit window. The timestamp is left untouched
// until a sink accepts a fresh delivery, preserving RL-40 on delivery failure.
func escalationDue(lastEscalated, now time.Time) bool {
	return lastEscalated.IsZero() || lastEscalated.After(now) ||
		!now.Before(lastEscalated.Add(reEscalateInterval))
}

func recordFailure(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	reason string,
	execErr error,
	now time.Time,
	actionAt time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	truth recoveryloop.PulseTruth,
	notifyFn notifier,
	stderr io.Writer,
) {
	if actionAt.IsZero() {
		actionAt = now
	}
	prev := state.Jobs[job.Name]
	attempts := prev.ConsecutiveFailures + 1
	humanNeeded := prev.HumanNeeded || attempts >= opts.escalateAfter
	newlyHumanNeeded := !prev.HumanNeeded && humanNeeded
	unhealthySince := prev.UnhealthySince
	if unhealthySince.IsZero() {
		unhealthySince = now
	}

	state.Jobs[job.Name] = recoveryloop.JobState{
		ConsecutiveFailures:        attempts,
		LastAttemptTime:            actionAt,
		LastAction:                 action,
		LastStatus:                 recoveryloop.StatusFailed,
		HumanNeeded:                humanNeeded,
		UnhealthySince:             unhealthySince,
		LastEscalated:              prev.LastEscalated,
		PendingEscalations:         prev.PendingEscalations,
		PendingNotification:        prev.PendingNotification,
		MissedVerificationDeadline: prev.MissedVerificationDeadline,
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
		if newlyHumanNeeded || (prev.PendingNotification == nil && escalationDue(prev.LastEscalated, now)) {
			escalate(job, action, attempts, reason, now, state, opts, truth,
				true, notifyFn, stderr)
		}
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
	notify bool,
	notifyFn notifier,
	stderr io.Writer,
) {
	if opts.dryRun {
		return
	}
	absentFor := truth.AbsentFor(job.Pulse, now)
	if absentFor == 0 {
		if st := state.Jobs[job.Name]; !st.UnhealthySince.IsZero() && now.After(st.UnhealthySince) {
			absentFor = now.Sub(st.UnhealthySince)
		}
	}
	pulse, pulseStatus, condition := escalationPulseContext(job, truth, absentFor)
	body := fmt.Sprintf(
		"recovery-loop could not restore %s. %s. %d consecutive recoveries failed to clear it (last action %s). %s",
		job.Name, condition, attempts, action, reason)

	// Delivery, not intent, is what the rate limit measures. Stamping
	// LastEscalated when no sink accepted the message would buy 24h of silence
	// for an escalation nobody received: the false-green shape applied to the
	// escalation path itself (RL-40).
	record := absencealarm.JournalRecord{
		Time:   now,
		Kind:   "recovery.human_needed",
		Pulse:  pulse,
		Status: pulseStatus,
		Reason: body,
		Misses: attempts,
	}
	var durableDelivered bool
	if err := absencealarm.AppendJournal(opts.absenceJournal, record); err != nil {
		fmt.Fprintf(stderr, "recovery-loop: append absence escalation: %v\n", err)
	} else {
		durableDelivered = true
	}

	st := state.Jobs[job.Name]
	st.HumanNeeded = true
	if !durableDelivered {
		st.PendingEscalations = append(st.PendingEscalations, record)
	}
	var notificationDelivered bool
	if notify {
		pending := record
		st.PendingNotification = &pending
		if notifyFn != nil {
			opts.notificationAttempted[job.Name] = true
			notifyCtx, cancel := context.WithTimeout(context.Background(), defaultNotifyTimeout)
			title := fmt.Sprintf("HUMAN NEEDED: %s not recovered", job.Name)
			if err := notifyFn(notifyCtx, title, body); err != nil {
				fmt.Fprintf(stderr, "recovery-loop: notify: %v\n", err)
			} else {
				notificationDelivered = true
				st.PendingNotification = nil
				st.LastEscalated = now
			}
			cancel()
		}
	}
	accepted := durableDelivered || notificationDelivered
	if !accepted && notify {
		fmt.Fprintf(stderr,
			"recovery-loop: no escalation sink accepted the %s alert; will retry next tick\n", job.Name)
	} else if !durableDelivered && !notify {
		fmt.Fprintf(stderr,
			"recovery-loop: durable escalation for %s is still pending; will retry next tick\n", job.Name)
	}
	state.Jobs[job.Name] = st
}

// retryPendingEscalations services durable delivery debt independently of the
// current registry. Exact records survive a job rename or removal, while the
// original pulse identity still participates in the snooze boundary.
func retryPendingEscalations(
	jobs []recoveryloop.Job,
	snoozes map[string]absencealarm.Snooze,
	now time.Time,
	state *recoveryloop.State,
	opts *options,
	stderr io.Writer,
) {
	jobsByName := make(map[string]recoveryloop.Job, len(jobs))
	for _, job := range jobs {
		jobsByName[job.Name] = job
	}
	names := make([]string, 0, len(state.Jobs))
	for name, st := range state.Jobs {
		if len(st.PendingEscalations) > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		job, configured := jobsByName[name]
		if !configured {
			job = recoveryloop.Job{Name: name}
		} else if _, snoozed := recoveryloop.IsJobSnoozed(job, snoozes, now); snoozed {
			// A current job-level snooze defers all of its historical debt. The
			// per-record check below additionally protects renamed pulses.
			continue
		}
		retryPendingEscalationQueue(job, snoozes, now, state, opts, stderr)
	}
}

// retryPendingEscalationQueue retries exact durable records rejected on
// earlier ticks before an unsnoozed job takes any new lifecycle path. The
// prepass snooze gate intentionally runs first; otherwise the retry is
// independent of whether this tick becomes pending, unavailable, failed, or
// recovered (RL-35, RL-59).
func retryPendingEscalationQueue(
	job recoveryloop.Job,
	snoozes map[string]absencealarm.Snooze,
	now time.Time,
	state *recoveryloop.State,
	opts *options,
	stderr io.Writer,
) {
	st := state.Jobs[job.Name]
	if len(st.PendingEscalations) == 0 || opts.dryRun {
		return
	}
	resolvedIncident := !st.HumanNeeded && st.ConsecutiveFailures == 0
	if resolvedIncident {
		// Recovery starts a new notification epoch even when historical durable
		// debt remains. A later outage in this tick must not inherit the old
		// incident's rate limit.
		st.LastEscalated = time.Time{}
	}
	queue := st.PendingEscalations
	for i, record := range queue {
		// The rejected record is the stable identity of the historical event;
		// a later config edit must not bypass a snooze on its original pulse.
		recordJob := job
		recordJob.Pulse = record.Pulse
		if _, snoozed := recoveryloop.IsJobSnoozed(recordJob, snoozes, now); snoozed {
			st.PendingEscalations = append([]absencealarm.JournalRecord(nil), queue[i:]...)
			state.Jobs[job.Name] = st
			return
		}
		if err := absencealarm.AppendJournal(opts.absenceJournal, record); err != nil {
			fmt.Fprintf(stderr, "recovery-loop: retry durable escalation: %v\n", err)
			st.PendingEscalations = append([]absencealarm.JournalRecord(nil), queue[i:]...)
			state.Jobs[job.Name] = st
			return
		}
	}
	st.PendingEscalations = nil
	state.Jobs[job.Name] = st
}

// retryPendingNotifications runs only after current lifecycle observation.
// Durable delivery is safe to retry in the prepass, but a banner saying "not
// recovered" is not: the same tick may prove recovery moments later. Deferring
// banners until a current result conclusively remains failed keeps their
// narrative aligned with current state.
func retryPendingNotifications(
	jobs []recoveryloop.Job,
	snoozes map[string]absencealarm.Snooze,
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	notifyFn notifier,
	stderr io.Writer,
) {
	if opts.dryRun || notifyFn == nil {
		return
	}
	jobsByName := configuredJobsByName(jobs)
	confirmedUnresolved := failedHumanNeededJobs(rep)
	for _, name := range pendingNotificationNames(state, confirmedUnresolved) {
		job, configured := jobsByName[name]
		if !configured {
			// Durable debt is registry-independent, but a removed job has no
			// current observation that can justify a "not recovered" banner.
			continue
		}
		retryPendingNotification(job, snoozes, now, state, opts, notifyFn, stderr)
	}
}

func configuredJobsByName(jobs []recoveryloop.Job) map[string]recoveryloop.Job {
	byName := make(map[string]recoveryloop.Job, len(jobs))
	for _, job := range jobs {
		byName[job.Name] = job
	}
	return byName
}

func failedHumanNeededJobs(rep *recoveryloop.Heartbeat) map[string]bool {
	confirmed := make(map[string]bool, len(rep.Results))
	for _, result := range rep.Results {
		if result.Status == recoveryloop.StatusFailed && result.HumanNeeded {
			confirmed[result.Job] = true
		}
	}
	return confirmed
}

func pendingNotificationNames(
	state *recoveryloop.State,
	confirmedUnresolved map[string]bool,
) []string {
	names := make([]string, 0, len(state.Jobs))
	for name, st := range state.Jobs {
		if confirmedUnresolved[name] && st.HumanNeeded && st.PendingNotification != nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func retryPendingNotification(
	job recoveryloop.Job,
	snoozes map[string]absencealarm.Snooze,
	now time.Time,
	state *recoveryloop.State,
	opts *options,
	notifyFn notifier,
	stderr io.Writer,
) {
	st := state.Jobs[job.Name]
	if opts.notificationAttempted[job.Name] || !escalationDue(st.LastEscalated, now) {
		return
	}
	if _, snoozed := recoveryloop.IsJobSnoozed(job, snoozes, now); snoozed {
		return
	}
	recordJob := job
	recordJob.Pulse = st.PendingNotification.Pulse
	if _, snoozed := recoveryloop.IsJobSnoozed(recordJob, snoozes, now); snoozed {
		return
	}
	opts.notificationAttempted[job.Name] = true
	notifyCtx, cancel := context.WithTimeout(context.Background(), defaultNotifyTimeout)
	title := fmt.Sprintf("HUMAN NEEDED: %s not recovered", job.Name)
	if err := notifyFn(notifyCtx, title, st.PendingNotification.Reason); err != nil {
		fmt.Fprintf(stderr, "recovery-loop: notify: %v\n", err)
	} else {
		st.LastEscalated = now
		st.PendingNotification = nil
		state.Jobs[job.Name] = st
	}
	cancel()
}

// escalationPulseContext returns one shared machine status and human
// description, preserving the observed pulse polarity instead of fabricating
// ABSENT for undetermined, unobserved, or pulse-less structural failures.
func escalationPulseContext(
	job recoveryloop.Job,
	truth recoveryloop.PulseTruth,
	unhealthyFor time.Duration,
) (string, absencealarm.Status, string) {
	if job.Pulse == "" {
		return job.Name, absencealarm.StatusUndetermined,
			fmt.Sprintf("no pulse configured; structural condition unhealthy for %s", unhealthyFor.Round(time.Minute))
	}
	fact, observed := truth[job.Pulse]
	if !observed || !fact.Known {
		return job.Pulse, absencealarm.StatusUndetermined,
			fmt.Sprintf("pulse %q has no current observation; unresolved condition is structural", job.Pulse)
	}
	switch fact.Status {
	case absencealarm.StatusPresent:
		return job.Pulse, fact.Status,
			fmt.Sprintf("pulse %q is present; the unresolved condition is structural", job.Pulse)
	case absencealarm.StatusAbsent:
		return job.Pulse, fact.Status,
			fmt.Sprintf("pulse %q absent for %s", job.Pulse, unhealthyFor.Round(time.Minute))
	case absencealarm.StatusUndetermined:
		return job.Pulse, fact.Status,
			fmt.Sprintf("pulse %q observation is undetermined for %s", job.Pulse, unhealthyFor.Round(time.Minute))
	case absencealarm.StatusSnoozed:
		return job.Pulse, fact.Status,
			fmt.Sprintf("pulse %q is snoozed; the unresolved condition is structural", job.Pulse)
	default:
		status := fact.Status
		if status == "" {
			status = absencealarm.StatusUndetermined
		}
		return job.Pulse, status,
			fmt.Sprintf("pulse %q reported status %q; the unresolved condition is structural", job.Pulse, fact.Status)
	}
}
