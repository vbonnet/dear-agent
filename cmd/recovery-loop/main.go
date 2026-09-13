// Command recovery-loop self-heals dead or wedged fleet background jobs (ce-a1uqr).
//
// It consumes the absence-alarm journal, evaluates critical launchd services
// and binaries against a declarative registry, enforces expiring snooze policies,
// executes bounded remediation actions (reinstall, bootstrap, kickstart),
// tracks consecutive failure escalation, and journals every recovery attempt.
//
// Usage:
//
//	recovery-loop                       # evaluate and recover jobs; exit 0 if clean
//	recovery-loop --dry-run             # plan and report without executing actions
//	recovery-loop --json                # machine-readable report on stdout
//	recovery-loop --config PATH         # critical jobs configuration file (JSON)
//	recovery-loop --snooze PATH         # shared snooze file (JSON)
//	recovery-loop --state PATH          # recovery lifecycle state file (JSON)
//	recovery-loop --journal PATH        # recovery journal file (JSONL, append)
//	recovery-loop --heartbeat PATH      # self-liveness heartbeat file (JSON)
//
// Exit codes: 0 = all jobs healthy, snoozed, or successfully recovered;
// 1 = at least one recovery failed or requires human intervention;
// 2 = usage or configuration error.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
	"github.com/vbonnet/dear-agent/pkg/notify"
	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

const (
	defaultActionTimeout = 60 * time.Second
	defaultNotifyTimeout = 15 * time.Second
	// defaultVerifyGrace is how long a restarted job has to emit its pulse
	// before the remediation is judged to have failed. It must exceed one
	// absence-alarm tick, or a working recovery would be scored a failure.
	defaultVerifyGrace = 30 * time.Minute
	// defaultEscalateAfter matches RL-09: two consecutive failures reach a
	// human. The failures are now failures to CLEAR the condition, not
	// failures to run a command.
	defaultEscalateAfter = 2
	// defaultGiveUpAfter bounds pointless remediation. Past it the job stays
	// loudly escalated instead of being restarted every tick forever; the
	// live incident ran 555 consecutive no-op kickstarts on one job.
	defaultGiveUpAfter = 5
	// defaultMaxHeartbeatAge is how stale the absence-alarm heartbeat may be
	// before its pulse truth is refused. Stale evidence must not confirm a
	// recovery.
	//
	// It is deliberately just over the deployed absence-alarm-heartbeat pulse
	// window (30m), not a round couple of hours: the heartbeat is how a
	// stopped monitor is detected, so a threshold far beyond that window lets
	// the monitor's own last "present" self-report vouch for it long after it
	// died. The slack covers one missed tick and no more.
	defaultMaxHeartbeatAge = 45 * time.Minute
	// reEscalateInterval bounds how often one standing, given-up outage may
	// re-alarm. A sink that receives the same record every tick forever stops
	// being read, which is the failure mode this whole change exists to fix.
	reEscalateInterval = 24 * time.Hour
)

// notifier delivers an escalation message to the operator (RL-09, RL-17).
type notifier func(ctx context.Context, title, body string) error

func desktopNotifier() notifier {
	d := notify.NewDesktopDispatcher()
	return func(ctx context.Context, title, body string) error {
		return d.Dispatch(ctx, &notify.Notification{
			ID:        fmt.Sprintf("recovery-loop-%d", time.Now().UnixNano()),
			Title:     title,
			Body:      body,
			Level:     slog.LevelError,
			Source:    "recovery-loop",
			Timestamp: time.Now(),
		})
	}
}

type options struct {
	configPath     string
	defaultConfig  string
	snoozePath     string
	statePath      string
	journalPath    string
	absenceJournal string
	absenceHB      string
	absenceState   string
	heartbeatPath  string
	dryRun         bool
	jsonOut        bool
	timeout        time.Duration
	verifyGrace    time.Duration
	escalateAfter  int
	giveUpAfter    int
	maxHBAge       time.Duration
}

func parseFlags(args []string, stderr io.Writer) (*options, int) {
	fs := flag.NewFlagSet("recovery-loop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	home, _ := os.UserHomeDir()
	defaultCfg := filepath.Join(home, ".config", "dear-agent", "recovery-loop-jobs.json")
	opts := options{
		defaultConfig: defaultCfg,
	}
	fs.StringVar(&opts.configPath, "config", defaultCfg, "critical jobs configuration file (JSON)")
	fs.StringVar(&opts.snoozePath, "snooze", filepath.Join(home, ".config", "dear-agent", "absence-alarm-snooze.json"), "shared snooze file (JSON)")
	fs.StringVar(&opts.statePath, "state", filepath.Join(home, ".local", "state", "dear-agent", "recovery-loop-state.json"), "recovery state file (JSON)")
	fs.StringVar(&opts.journalPath, "journal", filepath.Join(home, ".agm", "escalation", "recovery-loop.jsonl"), "recovery journal (JSONL, append)")
	fs.StringVar(&opts.absenceJournal, "absence-journal", filepath.Join(home, ".agm", "escalation", "absence-alarm.jsonl"), "absence-alarm journal (JSONL)")
	fs.StringVar(&opts.absenceHB, "absence-heartbeat", filepath.Join(home, ".local", "state", "dear-agent", "absence-alarm.heartbeat.json"), "absence-alarm heartbeat (JSON): the source of present/absent pulse truth")
	fs.StringVar(&opts.absenceState, "absence-state", filepath.Join(home, ".local", "state", "dear-agent", "absence-alarm-state.json"), "absence-alarm state (JSON): dates each standing alarm")
	fs.DurationVar(&opts.verifyGrace, "verify-grace", defaultVerifyGrace, "how long a pulse has to return before a remediation counts as failed")
	fs.IntVar(&opts.escalateAfter, "escalate-after", defaultEscalateAfter, "consecutive unverified recoveries before escalating to a human")
	fs.IntVar(&opts.giveUpAfter, "give-up-after", defaultGiveUpAfter, "stop retrying a job after this many consecutive failures (0 = never stop)")
	fs.DurationVar(&opts.maxHBAge, "max-heartbeat-age", defaultMaxHeartbeatAge, "refuse absence-alarm pulse truth older than this")
	fs.StringVar(&opts.heartbeatPath, "heartbeat", filepath.Join(home, ".local", "state", "dear-agent", "recovery-loop.heartbeat.json"), "self-liveness heartbeat file")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "plan and report recovery actions without executing or writing state")
	fs.BoolVar(&opts.jsonOut, "json", false, "emit report as JSON on stdout")
	fs.DurationVar(&opts.timeout, "timeout", defaultActionTimeout, "per-action execution deadline")

	if err := fs.Parse(args); err != nil {
		return nil, 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "recovery-loop: unexpected positional argument(s): %v\n", fs.Args())
		return nil, 2
	}
	if opts.timeout <= 0 {
		fmt.Fprintf(stderr, "recovery-loop: --timeout must be positive\n")
		return nil, 2
	}
	if opts.verifyGrace <= 0 {
		fmt.Fprintf(stderr, "recovery-loop: --verify-grace must be positive\n")
		return nil, 2
	}
	if opts.escalateAfter < 1 {
		fmt.Fprintf(stderr, "recovery-loop: --escalate-after must be at least 1\n")
		return nil, 2
	}
	if opts.giveUpAfter < 0 {
		fmt.Fprintf(stderr, "recovery-loop: --give-up-after must not be negative\n")
		return nil, 2
	}
	return &opts, 0
}

func resolveJobs(configPath, defaultConfig string) ([]recoveryloop.Job, error) {
	var jobs []recoveryloop.Job
	if _, err := os.Stat(configPath); err == nil {
		cfg, err := recoveryloop.LoadConfig(configPath)
		if err != nil {
			return nil, err
		}
		jobs = cfg.Jobs
	} else if os.IsNotExist(err) && configPath == defaultConfig {
		jobs = recoveryloop.DefaultJobs()
	} else {
		return nil, fmt.Errorf("read config %s: %w", configPath, err)
	}

	seen := make(map[string]bool, len(jobs))
	for i := range jobs {
		j := &jobs[i]
		if j.Name == "" {
			return nil, fmt.Errorf("job at index %d has empty name", i)
		}
		if seen[j.Name] {
			return nil, fmt.Errorf("duplicate job name %q", j.Name)
		}
		seen[j.Name] = true
		j.PlistPath = recoveryloop.ExpandHome(j.PlistPath)
		j.BinaryPath = recoveryloop.ExpandHome(j.BinaryPath)
		for k, arg := range j.InstallCmd {
			j.InstallCmd[k] = recoveryloop.ExpandHome(arg)
		}
	}
	return jobs, nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, recoveryloop.DefaultHostOps(), desktopNotifier()))
}

func run(args []string, stdout, stderr io.Writer, host recoveryloop.HostOps, notifyFn notifier) int {
	opts, exitCode := parseFlags(args, stderr)
	if exitCode != 0 {
		return exitCode
	}

	now := host.Now()

	jobs, err := resolveJobs(opts.configPath, opts.defaultConfig)
	if err != nil {
		fmt.Fprintf(stderr, "recovery-loop: %v\n", err)
		return 2
	}

	snoozes, err := absencealarm.LoadSnoozes(opts.snoozePath, now)
	if err != nil {
		fmt.Fprintf(stderr, "recovery-loop: %v\n", err)
		return 2
	}

	state, stateErr := recoveryloop.LoadState(opts.statePath)
	if stateErr != nil {
		fmt.Fprintf(stderr, "recovery-loop: %v (proceeding with empty state)\n", stateErr)
	}

	// Pulse truth comes from the absence-alarm heartbeat, the only source
	// that reports presence as well as absence. The escalation journal is
	// append-only and records absences only, so a pulse that recovered would
	// stay "alarming" in it forever.
	truth, err := recoveryloop.LoadPulseTruth(opts.absenceHB, opts.absenceState, now, opts.maxHBAge)
	if err != nil {
		fmt.Fprintf(stderr, "recovery-loop: load pulse truth: %v\n", err)
	}

	ctx := context.Background()
	launchdJobs, listErr := host.LaunchdList(ctx)
	if listErr != nil {
		fmt.Fprintf(stderr, "recovery-loop: launchd list: %v\n", listErr)
	}

	rep := recoveryloop.Heartbeat{
		TickTime: now,
	}

	for _, job := range jobs {
		processJob(ctx, job, opts, &state, &rep, snoozes, truth, launchdJobs, host, notifyFn, now, stderr)
	}

	if !opts.dryRun {
		if err := recoveryloop.SaveState(opts.statePath, state); err != nil {
			fmt.Fprintf(stderr, "recovery-loop: save state: %v\n", err)
		}
		if err := recoveryloop.WriteHeartbeat(opts.heartbeatPath, rep); err != nil {
			fmt.Fprintf(stderr, "recovery-loop: write heartbeat: %v\n", err)
		}
	}

	emitReport(stdout, stderr, rep, opts.jsonOut)

	if rep.Failed > 0 || rep.HumanNeeded > 0 {
		return 1
	}
	return 0
}

// processJob evaluates, remediates and VERIFIES one job for this tick.
//
// The order matters. A verification left pending by an earlier tick is settled
// first, because the question "did the last remediation actually work?" has to
// be answered before another one is fired. Skipping that step is what let the
// loop restart the same job on every tick for days while reporting success.
func processJob(
	ctx context.Context,
	job recoveryloop.Job,
	opts *options,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	snoozes map[string]absencealarm.Snooze,
	truth recoveryloop.PulseTruth,
	launchdJobs map[string]recoveryloop.LaunchdJobInfo,
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
		recordSnoozed(job, sn, now, state, rep, prev)
		return
	}

	// A verification left open by an earlier tick is always conclusive: it
	// either confirms the pulse returned, converts to a counted failure, or
	// reports that the grace window is still open. In every case this job is
	// done for this tick and firing another action would only reset the clock.
	if !prev.PendingDeadline.IsZero() {
		settlePending(job, opts, state, rep, truth, launchdJobs, host, prev, now, notifyFn, stderr)
		return
	}

	action, plannedStatus, reason := recoveryloop.PlanJob(job, snoozes, truth, launchdJobs, host, now)

	if action == recoveryloop.ActionNone {
		recordClear(job, plannedStatus, reason, now, state, rep, opts, prev, truth, stderr)
		return
	}

	if opts.dryRun {
		rep.Results = append(rep.Results, recoveryloop.Result{
			Job:    job.Name,
			Status: recoveryloop.StatusUnhealthy,
			Action: action,
			Reason: reason + " (planned, dry-run)",
		})
		rep.Planned++
		return
	}

	// A job that has failed this many times will not be fixed by firing the
	// same action again. Stay loud instead of thrashing.
	if opts.giveUpAfter > 0 && prev.ConsecutiveFailures >= opts.giveUpAfter {
		recordGivenUp(job, action, reason, now, state, rep, opts, truth, prev, notifyFn, stderr)
		return
	}

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
		fmt.Fprintf(stderr, "recovery-loop: re-list launchd for verification: %v\n", listErr)
		freshLaunchd = launchdJobs
	}
	outcome := recoveryloop.VerifyRecovery(job, action, truth, freshLaunchd, host, now)

	switch {
	case outcome.Verified:
		recordVerified(job, action, outcome.Reason, now, state, rep, opts, stderr)
	case outcome.Status == recoveryloop.StatusPending:
		recordPending(job, action, outcome.Reason, now, state, rep, opts, prev, stderr)
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
	outcome := recoveryloop.VerifyRecovery(job, prev.PendingAction, truth, launchdJobs, host, now)
	switch {
	case outcome.Verified:
		recordVerified(job, prev.PendingAction, outcome.Reason, now, state, rep, opts, stderr)
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
		Job:         job.Name,
		Status:      recoveryloop.StatusPending,
		Action:      prev.PendingAction,
		Attempt:     prev.ConsecutiveFailures,
		HumanNeeded: prev.HumanNeeded,
		Reason: fmt.Sprintf("awaiting pulse %q until %s",
			job.Pulse, prev.PendingDeadline.Format(time.RFC3339)),
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
func recordSnoozed(
	job recoveryloop.Job,
	sn absencealarm.Snooze,
	now time.Time,
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
		recordUnverifiable(job, reason, now, state, rep, prev)
		return
	}

	// A job that was failing and is now observed healthy has genuinely
	// recovered.
	if status == recoveryloop.StatusHealthy && prev.ConsecutiveFailures > 0 {
		recordVerified(job, prev.LastAction, "condition cleared: "+reason, now, state, rep, opts, stderr)
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
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	prev recoveryloop.JobState,
) {
	st := prev
	st.LastAttemptTime = now
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
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	stderr io.Writer,
) {
	state.Jobs[job.Name] = recoveryloop.JobState{
		ConsecutiveFailures: 0,
		LastAttemptTime:     now,
		LastAction:          action,
		LastStatus:          recoveryloop.StatusRecovered,
		HumanNeeded:         false,
	}
	rep.Results = append(rep.Results, recoveryloop.Result{
		Job:    job.Name,
		Status: recoveryloop.StatusRecovered,
		Action: action,
		Reason: reason,
	})
	rep.Recovered++
	appendJournal(opts, stderr, recoveryloop.JournalRecord{
		Time:        now,
		Kind:        "recovery.verified",
		Job:         job.Name,
		Action:      action,
		Status:      recoveryloop.StatusRecovered,
		Attempt:     1,
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
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	opts *options,
	prev recoveryloop.JobState,
	stderr io.Writer,
) {
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
		PendingSince:        now,
		PendingDeadline:     now.Add(opts.verifyGrace),
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
		Job:         job.Name,
		Status:      recoveryloop.StatusFailed,
		Action:      recoveryloop.ActionNone,
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
	body := fmt.Sprintf(
		"recovery-loop could not restore %s. Pulse %q absent for %s. %d consecutive recoveries failed to clear it (last action %s). %s",
		job.Name, pulse, absentFor.Round(time.Minute), attempts, action, reason)

	if err := absencealarm.AppendJournal(opts.absenceJournal, absencealarm.JournalRecord{
		Time:   now,
		Kind:   "recovery.human_needed",
		Pulse:  pulse,
		Status: absencealarm.StatusAbsent,
		Reason: body,
		Misses: attempts,
	}); err != nil {
		fmt.Fprintf(stderr, "recovery-loop: append absence escalation: %v\n", err)
	}

	st := state.Jobs[job.Name]
	st.LastEscalated = now
	st.HumanNeeded = true
	state.Jobs[job.Name] = st

	if notifyFn == nil {
		return
	}
	notifyCtx, cancel := context.WithTimeout(context.Background(), defaultNotifyTimeout)
	defer cancel()
	title := fmt.Sprintf("HUMAN NEEDED: %s not recovered", job.Name)
	if err := notifyFn(notifyCtx, title, body); err != nil {
		fmt.Fprintf(stderr, "recovery-loop: notify: %v\n", err)
	}
}

func emitReport(stdout, stderr io.Writer, rep recoveryloop.Heartbeat, jsonOut bool) {
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "recovery-loop: encode report: %v\n", err)
		}
		return
	}
	fmt.Fprintf(stdout, "recovery-loop report (%s)\n", rep.TickTime.Format(time.RFC3339))
	for _, r := range rep.Results {
		line := fmt.Sprintf("  %-14s %s", r.Status, r.Job)
		if r.Action != recoveryloop.ActionNone {
			line += fmt.Sprintf(" (action %s)", r.Action)
		}
		if r.HumanNeeded {
			line += " [HUMAN NEEDED]"
		}
		if r.Reason != "" {
			line += " - " + r.Reason
		}
		if r.Error != "" {
			line += fmt.Sprintf(" (error: %s)", r.Error)
		}
		fmt.Fprintln(stdout, line)
	}
	switch {
	case rep.Failed > 0 || rep.HumanNeeded > 0:
		fmt.Fprintf(stdout, "Status: ALARM (%d recovery failure(s), %d human needed)\n", rep.Failed, rep.HumanNeeded)
	case rep.Pending > 0:
		fmt.Fprintf(stdout, "Status: PENDING (%d remediation(s) awaiting verification)\n", rep.Pending)
	default:
		fmt.Fprintf(stdout, "Status: OK (no unresolved recovery failures)\n")
	}
}
