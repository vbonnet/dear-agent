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
	heartbeatPath  string
	dryRun         bool
	jsonOut        bool
	timeout        time.Duration
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

	alarmingPulses, _ := recoveryloop.LoadAbsenceAlarms(opts.absenceJournal)

	ctx := context.Background()
	launchdJobs, listErr := host.LaunchdList(ctx)
	if listErr != nil {
		fmt.Fprintf(stderr, "recovery-loop: launchd list: %v\n", listErr)
	}

	rep := recoveryloop.Heartbeat{
		TickTime: now,
	}

	for _, job := range jobs {
		processJob(ctx, job, opts, &state, &rep, snoozes, alarmingPulses, launchdJobs, host, notifyFn, now, stderr)
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

func processJob(
	ctx context.Context,
	job recoveryloop.Job,
	opts *options,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	snoozes map[string]absencealarm.Snooze,
	alarmingPulses map[string]bool,
	launchdJobs map[string]recoveryloop.LaunchdJobInfo,
	host recoveryloop.HostOps,
	notifyFn notifier,
	now time.Time,
	stderr io.Writer,
) {
	action, plannedStatus, reason := recoveryloop.PlanJob(job, snoozes, alarmingPulses, launchdJobs, host, now)

	if action == recoveryloop.ActionNone {
		rep.Results = append(rep.Results, recoveryloop.Result{
			Job:    job.Name,
			Status: plannedStatus,
			Action: action,
			Reason: reason,
		})
		switch plannedStatus {
		case recoveryloop.StatusHealthy:
			rep.Healthy++
		case recoveryloop.StatusSnoozed:
			rep.Snoozed++
		case recoveryloop.StatusRecovered:
			rep.Recovered++
		case recoveryloop.StatusFailed:
			rep.Failed++
		}
		return
	}

	if opts.dryRun {
		rep.Results = append(rep.Results, recoveryloop.Result{
			Job:    job.Name,
			Status: recoveryloop.StatusRecovered,
			Action: action,
			Reason: reason + " (planned, dry-run)",
		})
		rep.Recovered++
		return
	}

	actionCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	execErr := recoveryloop.ExecuteRecovery(actionCtx, job, action, host)
	cancel()

	if execErr == nil {
		recordSuccess(job, action, reason, now, state, rep, opts.journalPath)
	} else {
		recordFailure(job, action, reason, execErr, now, state, rep, opts.journalPath, notifyFn, stderr)
	}
}

func recordSuccess(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	reason string,
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	journalPath string,
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
	_ = recoveryloop.AppendJournal(journalPath, recoveryloop.JournalRecord{
		Time:        now,
		Kind:        "recovery.attempt",
		Job:         job.Name,
		Action:      action,
		Status:      recoveryloop.StatusRecovered,
		Attempt:     1,
		HumanNeeded: false,
		Reason:      reason,
	})
}

func recordFailure(
	job recoveryloop.Job,
	action recoveryloop.ActionType,
	reason string,
	execErr error,
	now time.Time,
	state *recoveryloop.State,
	rep *recoveryloop.Heartbeat,
	journalPath string,
	notifyFn notifier,
	stderr io.Writer,
) {
	attempts := state.Jobs[job.Name].ConsecutiveFailures + 1
	humanNeeded := attempts >= 2

	state.Jobs[job.Name] = recoveryloop.JobState{
		ConsecutiveFailures: attempts,
		LastAttemptTime:     now,
		LastAction:          action,
		LastStatus:          recoveryloop.StatusFailed,
		HumanNeeded:         humanNeeded,
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
		dispatchEscalation(job.Name, action, attempts, execErr, notifyFn, stderr)
	}

	_ = recoveryloop.AppendJournal(journalPath, recoveryloop.JournalRecord{
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

func dispatchEscalation(
	jobName string,
	action recoveryloop.ActionType,
	attempts int,
	execErr error,
	notifyFn notifier,
	stderr io.Writer,
) {
	if notifyFn == nil {
		return
	}
	notifyCtx, cancel := context.WithTimeout(context.Background(), defaultNotifyTimeout)
	defer cancel()
	title := fmt.Sprintf("HUMAN NEEDED: %s recovery failed", jobName)
	body := fmt.Sprintf("Recovery action %s failed (%d consecutive failures): %v", action, attempts, execErr)
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
	if rep.Failed > 0 || rep.HumanNeeded > 0 {
		fmt.Fprintf(stdout, "Status: ALARM (%d recovery failure(s), %d human needed)\n", rep.Failed, rep.HumanNeeded)
	} else {
		fmt.Fprintf(stdout, "Status: OK (all jobs healthy, snoozed, or recovered)\n")
	}
}
