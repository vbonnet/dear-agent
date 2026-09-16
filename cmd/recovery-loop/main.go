// Command recovery-loop self-heals dead or wedged fleet background jobs (ce-a1uqr).
//
// It reads pulse truth from the absence-alarm HEARTBEAT, which reports presence
// as well as absence, evaluates critical launchd services and binaries against a
// declarative registry, enforces expiring snooze policies, executes bounded
// remediation actions (reinstall, bootstrap, kickstart), VERIFIES that the
// condition actually cleared before calling anything recovered, tracks
// consecutive failures to clear, and journals every attempt.
//
// The absence-alarm escalation journal is an OUTPUT only: human-needed records
// are appended to it. It is append-only and records absences alone, so a pulse
// that recovered leaves no trace in it; reading it as current state can make a
// cleared pulse appear alarming forever.
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
// 1 = at least one recovery failed, awaits verification, requires human
// intervention, or could not be observed safely;
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
	if opts.maxHBAge <= 0 {
		// A nonpositive value skipped every heartbeat freshness check,
		// including the missing and future tick_time cases, so a heartbeat
		// from any point in history stayed authoritative. This flag has no
		// documented disable value; it must be a real limit.
		fmt.Fprintf(stderr, "recovery-loop: --max-heartbeat-age must be positive\n")
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
	backfillPulseKind(jobs)
	return jobs, nil
}

// backfillPulseKind marks a job's pulse structural when the built-in registry
// says that pulse is structural.
//
// deploy/manifest.yaml declares recovery-loop-jobs absent-only, so a host that
// already has a job config keeps it forever. Without this, PulseIsStructural
// would live in the repository and never reach a running loop, and mergeloop
// would keep reading HEALTHY on the strength of a loaded-only pulse: the flag
// would be a fix nobody received. The match is on the pulse name, so a config
// that has since moved a job to its activity pulse is untouched.
func backfillPulseKind(jobs []recoveryloop.Job) {
	structural := make(map[string]bool)
	for _, b := range recoveryloop.DefaultJobs() {
		if b.Pulse != "" && b.PulseIsStructural {
			structural[b.Pulse] = true
		}
	}
	for i := range jobs {
		if jobs[i].Pulse != "" && structural[jobs[i].Pulse] {
			jobs[i].PulseIsStructural = true
		}
	}
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
	truth, pulseSource, err := recoveryloop.LoadPulseTruth(opts.absenceHB, opts.absenceState, now, opts.maxHBAge)
	if err != nil {
		fmt.Fprintf(stderr, "recovery-loop: load pulse truth: %v\n", err)
	}

	ctx := context.Background()
	launchdJobs, launchdErr := host.LaunchdList(ctx)
	if launchdErr != nil {
		fmt.Fprintf(stderr, "recovery-loop: launchd list: %v\n", launchdErr)
	}

	rep := recoveryloop.Heartbeat{
		TickTime: now,
	}

	for _, job := range jobs {
		processJob(ctx, job, opts, &state, &rep, snoozes, truth, pulseSource, launchdJobs, launchdErr, host, notifyFn, now, stderr)
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

	if rep.Failed > 0 || rep.Pending > 0 || rep.HumanNeeded > 0 || rep.Unavailable > 0 {
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
	case rep.Failed > 0 || rep.HumanNeeded > 0 || rep.Unavailable > 0:
		fmt.Fprintf(stdout, "Status: ALARM (%d recovery failure(s), %d observation unavailable, %d human needed)\n",
			rep.Failed, rep.Unavailable, rep.HumanNeeded)
	case rep.Planned > 0:
		// A dry run that just listed work to do is not a clean bill of health.
		// Reporting OK underneath a planned remediation is the same report
		// contradicting itself, which is what operators use dry-run to avoid.
		fmt.Fprintf(stdout, "Status: ACTION NEEDED (%d remediation(s) planned, none executed)\n", rep.Planned)
	case rep.Pending > 0:
		fmt.Fprintf(stdout, "Status: PENDING (%d remediation(s) awaiting verification)\n", rep.Pending)
	default:
		fmt.Fprintf(stdout, "Status: OK (no unresolved recovery failures)\n")
	}
}

// evidenceStampCLI renders a probe observation time for an operator-facing
// reason string.
func evidenceStampCLI(t time.Time) string {
	if t.IsZero() {
		return "at an unrecorded time"
	}
	return "at " + t.Format(time.RFC3339)
}

// pulseVerdictIsConclusive reports whether the pulse alone already settles an
// open verification, so a missing launchd listing changes nothing.
//
// Only the negative direction qualifies. A returned pulse still needs the
// structural re-check before it can verify a recovery (RL-39), but a pulse that
// has not come back by its deadline is a failure whatever launchctl says.
func pulseVerdictIsConclusive(
	job recoveryloop.Job,
	truth recoveryloop.PulseTruth,
	pulseSource recoveryloop.PulseSourceStatus,
	prev recoveryloop.JobState,
	now time.Time,
) bool {
	if job.Pulse == "" || job.PulseIsStructural {
		return false
	}
	if job.Pulse == recoveryloop.AbsenceAlarmHeartbeatPulse && pulseSource.RequiresRecovery() {
		return now.After(prev.PendingDeadline)
	}
	return truth.VerificationObservationAvailable(job.Pulse, prev.PendingSince, now) &&
		truth.Alarming(job.Pulse) && now.After(prev.PendingDeadline)
}
