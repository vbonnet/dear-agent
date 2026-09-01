// Command absence-alarm alarms on the ABSENCE of expected positive events.
//
// It is the fleet-wide generalization of the disk-watchdog's reaper-liveness
// check (DW-17). Every silent multi-week outage in the 2026-07/08 window had
// the same shape: a mechanism stopped producing its positive event (a merge,
// a span, a heartbeat, a completed sweep) and nothing was watching for that
// event, so a dead process looked exactly like a healthy idle one. Monitors
// that alarm on the presence of errors cannot see this failure class,
// because a dead process emits no errors.
//
// The absence alarm evaluates a declarative set of "pulses" - expected
// positive events, each with a maximum silence window - on every launchd
// tick, and alarms loudly (desktop notification + escalation journal + exit
// code) when any pulse has not been observed inside its window. It runs on
// the host, outside the mesh, Dispatch, and every agent session, because a
// watcher that lives inside the thing being watched dies with it.
//
// Usage:
//
//	absence-alarm                       # evaluate pulses; notify + exit 1 on absence
//	absence-alarm --json                # machine-readable report on stdout
//	absence-alarm --dry-run             # evaluate + report only; no side effects
//	absence-alarm --config PATH         # pulse config (JSON)
//	absence-alarm --snooze PATH         # operator snoozes (JSON, expiring)
//	absence-alarm --state PATH          # notification-dedup state
//	absence-alarm --journal PATH        # escalation journal (JSONL, append)
//	absence-alarm --heartbeat PATH      # self-liveness heartbeat file
//
// Exit codes: 0 = every pulse present or validly snoozed; 1 = at least one
// pulse absent or undetermined; 2 = usage/config error. Notification and
// state I/O failures are reported on stderr and never change the exit code.
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

	"github.com/vbonnet/dear-agent/pkg/notify"
)

// notifier delivers one alarm or recovery message to the operator.
type notifier func(ctx context.Context, title, body string) error

func desktopNotifier() notifier {
	d := notify.NewDesktopDispatcher()
	return func(ctx context.Context, title, body string) error {
		return d.Dispatch(ctx, &notify.Notification{
			ID:        fmt.Sprintf("absence-alarm-%d", time.Now().UnixNano()),
			Title:     title,
			Body:      body,
			Level:     slog.LevelError,
			Source:    "absence-alarm",
			Timestamp: time.Now(),
		})
	}
}

// report is the full tick output (AA-21, AA-22).
type report struct {
	TickTime time.Time `json:"tick_time"`
	Results  []Result  `json:"results"`
	Alarming int       `json:"alarming"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, defaultProbes(), desktopNotifier()))
}

// tick carries one evaluation pass's shared context so the per-pulse work
// stays in one place.
type tick struct {
	pr             probes
	notifyFn       notifier
	stderr         io.Writer
	dryRun         bool
	journalPath    string
	now            time.Time
	st             *alarmState
	snoozes        map[string]Snooze
	launchdListing string
	launchdErr     error
}

func run(args []string, stdout, stderr io.Writer, pr probes, notifyFn notifier) int {
	fs := flag.NewFlagSet("absence-alarm", flag.ContinueOnError)
	fs.SetOutput(stderr)
	home, _ := os.UserHomeDir()
	var (
		configPath    = fs.String("config", filepath.Join(home, ".config", "dear-agent", "absence-alarm-pulses.json"), "pulse config file (JSON)")
		snoozePath    = fs.String("snooze", filepath.Join(home, ".config", "dear-agent", "absence-alarm-snooze.json"), "snooze file (JSON)")
		statePath     = fs.String("state", filepath.Join(home, ".local", "state", "dear-agent", "absence-alarm-state.json"), "notification-dedup state file")
		journalPath   = fs.String("journal", filepath.Join(home, ".agm", "escalation", "absence-alarm.jsonl"), "escalation journal (JSONL, append)")
		heartbeatPath = fs.String("heartbeat", filepath.Join(home, ".local", "state", "dear-agent", "absence-alarm.heartbeat.json"), "self-liveness heartbeat file")
		jsonOut       = fs.Bool("json", false, "emit the report as JSON on stdout")
		dryRun        = fs.Bool("dry-run", false, "evaluate and report only; no notifications, state, journal, or heartbeat writes")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	now := pr.now()

	pulses, err := loadPulseConfig(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "absence-alarm: %v\n", err)
		return 2
	}
	snoozes, err := loadSnoozes(*snoozePath, now)
	if err != nil {
		fmt.Fprintf(stderr, "absence-alarm: %v\n", err)
		return 2
	}
	st, stErr := loadAlarmState(*statePath)
	if stErr != nil {
		// AA-18: losing dedup state degrades toward louder, never silent.
		fmt.Fprintf(stderr, "absence-alarm: %v (treating all alarms as new)\n", stErr)
	}

	ctx := context.Background()
	tk := &tick{
		pr: pr, notifyFn: notifyFn, stderr: stderr, dryRun: *dryRun,
		journalPath: *journalPath, now: now, st: &st, snoozes: snoozes,
	}
	// One launchd listing shared by every launchd pulse.
	for _, p := range pulses {
		if p.Type == PulseLaunchdLoaded {
			tk.launchdListing, tk.launchdErr = pr.launchdList(ctx)
			break
		}
	}

	rep := report{TickTime: now}
	for _, p := range pulses {
		res := tk.process(ctx, p)
		if res.Status.alarming() {
			rep.Alarming++
		}
		rep.Results = append(rep.Results, res)
	}

	if !*dryRun {
		if err := saveAlarmState(*statePath, st); err != nil {
			fmt.Fprintf(stderr, "absence-alarm: save state: %v\n", err)
		}
		if err := writeHeartbeat(*heartbeatPath, heartbeat{TickTime: now, Results: rep.Results}); err != nil {
			fmt.Fprintf(stderr, "absence-alarm: write heartbeat: %v\n", err)
		}
	}

	emitReport(stdout, stderr, rep, *jsonOut)
	if rep.Alarming > 0 {
		return 1
	}
	return 0
}

// process evaluates one pulse, advances its alarm state, journals it when
// alarming (AA-09), and dispatches transition/backoff notifications
// (AA-10..AA-12) unless dry-run is set (AA-20).
func (t *tick) process(ctx context.Context, p Pulse) Result {
	var res Result
	sn, snoozed := t.snoozes[p.Name]
	if snoozed && sn.Until.After(t.now) {
		// AA-13: validly snoozed pulses do not probe and do not notify.
		res = Result{Name: p.Name, Status: StatusSnoozed, Expect: p.Expect, Window: p.Window,
			Reason: fmt.Sprintf("snoozed until %s: %s", sn.Until.Format(time.RFC3339), sn.Reason)}
	} else {
		res = evaluatePulse(ctx, p, t.pr, t.launchdListing, t.launchdErr)
		if snoozed && res.Status.alarming() {
			// AA-15: an expired snooze is part of the story.
			res.Reason = fmt.Sprintf("%s (snooze expired %s)", res.Reason, sn.Until.Format(time.RFC3339))
		}
	}

	decision := updateAlarm(t.st, p.Name, res.Status.alarming(), t.now)
	res.Misses = t.st.Pulses[p.Name].Misses
	if t.dryRun {
		return res
	}
	if res.Status.alarming() {
		rec := journalRecord{Time: t.now, Kind: "absence.alarm", Pulse: p.Name, Status: res.Status,
			Reason: res.Reason, Expect: p.Expect, Window: p.Window, Evidence: res.Evidence, Misses: res.Misses}
		if err := appendJournal(t.journalPath, rec); err != nil {
			fmt.Fprintf(t.stderr, "absence-alarm: journal append: %v\n", err)
		}
	}
	switch decision {
	case notifyAlarm:
		title := fmt.Sprintf("ABSENT: %s", p.Name)
		if res.Status == StatusUndetermined {
			title = fmt.Sprintf("UNDETERMINED: %s", p.Name)
		}
		t.dispatch(ctx, title, fmt.Sprintf("Expected %s. %s", expectOrName(p), res.Reason))
	case notifyRecovery:
		t.dispatch(ctx, fmt.Sprintf("RECOVERED: %s", p.Name), fmt.Sprintf("%s is present again.", expectOrName(p)))
	case notifyNone:
	}
	return res
}

// dispatch sends one notification; a failure is reported and never changes
// the exit code (AA-17).
func (t *tick) dispatch(ctx context.Context, title, body string) {
	if err := t.notifyFn(ctx, title, body); err != nil {
		fmt.Fprintf(t.stderr, "absence-alarm: notify: %v\n", err)
	}
}

func expectOrName(p Pulse) string {
	if p.Expect != "" {
		return p.Expect
	}
	return p.Name
}

func emitReport(stdout, stderr io.Writer, rep report, jsonOut bool) {
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "absence-alarm: encode report: %v\n", err)
		}
		return
	}
	fmt.Fprintf(stdout, "absence-alarm report (%s)\n", rep.TickTime.Format(time.RFC3339))
	for _, r := range rep.Results {
		line := fmt.Sprintf("  %-14s %s", r.Status, r.Name)
		if r.Reason != "" {
			line += " - " + r.Reason
		}
		if r.Misses > 1 {
			line += fmt.Sprintf(" (miss #%d)", r.Misses)
		}
		fmt.Fprintln(stdout, line)
	}
	if rep.Alarming > 0 {
		fmt.Fprintf(stdout, "Status: ALARM (%d pulse(s) absent or undetermined)\n", rep.Alarming)
	} else {
		fmt.Fprintln(stdout, "Status: OK (every pulse present or snoozed)")
	}
}
