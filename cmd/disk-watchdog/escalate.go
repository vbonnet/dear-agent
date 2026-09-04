package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
	"github.com/vbonnet/dear-agent/pkg/notify"
)

// A fail-closed remediation that cannot reclaim has to escalate.
//
// Historically this watchdog did the opposite. Across 2549 consecutive failing
// ticks it produced no escalation at all: it latched the admission brake,
// appended to a decision trail with no reader, and exited 1 into launchd,
// which discards exit codes. Every one of those ticks was a silent loop, and
// the disk filled to 100% more than once with a human as the only remediation.
//
// Two failure modes have to be closed at once, and they pull against each
// other. Silence is the one that happened. Its obvious fix, notifying on
// every stalled tick, produces a notification every five minutes for as long
// as the condition lasts, which trains an operator to mute the channel and reproduces
// the silence with extra steps.
//
// So: every stalled tick is journalled, and notifications follow the escalating
// cadence in pkg/absencealarm (first stall, then 1h, 6h, 24h, then daily).
// That package is the host's alarm state machine and its backoff has already
// been reasoned about; a second one here would be one more thing to keep in
// sync.

// escalationSource identifies this watchdog in escalation records.
const escalationSource = "disk-watchdog"

// stallAlarmName is the single alarm this watchdog keeps state for.
const stallAlarmName = "disk-watchdog.remediation"

// escalateTimeout bounds one notification dispatch, independently of the tick.
const escalateTimeout = 15 * time.Second

// notifier is the dispatch seam; nil means the real desktop notifier.
type notifier func(ctx context.Context, title, body string) error

// escalationRecord is one appended escalation-journal line.
type escalationRecord struct {
	Time   time.Time `json:"time"`
	Kind   string    `json:"kind"`
	Source string    `json:"source"`
	Reason string    `json:"reason,omitempty"`
	Misses int       `json:"misses,omitempty"`
}

// remediationStalled reports whether this tick left the host breached with
// nothing reclaimed.
//
// The classification deliberately keys on reclaim rather than on the exit
// status of one remediation leg. On 2026-09-04 the worktree sweep was killed
// by the very exhaustion it existed to relieve; had a cache trim freed 44 GiB
// on the same tick, that tick remediated, and paging about the killed sweep
// would be noise. Conversely a sweep that returns cleanly having deleted
// nothing has not remediated, however healthy its exit status looks. That is
// the shape that hid this problem for a month.
// The sweep result is accepted but deliberately not consulted: the parameter
// documents what this function is choosing NOT to key on.
func remediationStalled(breached bool, _ *sweepResult, reclaimed int64) bool {
	return breached && reclaimed <= 0
}

// escalateStall advances the stall alarm and dispatches what this tick owes.
//
// Journalling and notification are independent on purpose: a notifier that
// cannot dispatch must not also cost the durable record. That combination is
// exactly how 2549 failures left no evidence anywhere.
func escalateStall(cfg config, stalled bool, reason string, now time.Time) {
	// A --dry-run tick inspects; it does not page and does not mutate state.
	if cfg.dryRun || cfg.escalationState == "" {
		return
	}

	state, err := absencealarm.LoadAlarmState(cfg.escalationState)
	if err != nil {
		fmt.Fprintf(os.Stderr, "disk-watchdog: warning: read escalation state: %v\n", err)
	}
	decision := absencealarm.UpdateAlarm(&state, stallAlarmName, stalled, now)
	if serr := absencealarm.SaveAlarmState(cfg.escalationState, state); serr != nil {
		// Losing the state file costs the backoff, not the alarm: the next
		// stalled tick will notify again rather than go quiet.
		fmt.Fprintf(os.Stderr, "disk-watchdog: warning: save escalation state: %v\n", serr)
	}

	if stalled {
		appendEscalation(cfg, escalationRecord{
			Time:   now.UTC(),
			Kind:   "disk.remediation.stalled",
			Source: escalationSource,
			Reason: reason,
			Misses: state.Pulses[stallAlarmName].Misses,
		})
	}

	title, body := escalationMessage(decision, reason, state.Pulses[stallAlarmName].Since, now)
	if title == "" {
		return
	}
	if decision == absencealarm.NotifyRecovery {
		appendEscalation(cfg, escalationRecord{
			Time:   now.UTC(),
			Kind:   "disk.remediation.recovered",
			Source: escalationSource,
		})
	}
	dispatch(cfg, title, body)
}

// escalationMessage renders the notification for a decision, or ("","") when
// this tick owes none.
func escalationMessage(d absencealarm.NotifyDecision, reason string, since, now time.Time) (title, body string) {
	switch d {
	case absencealarm.NotifyAlarm:
		title = "Disk watchdog: remediation cannot reclaim"
		body = reason
		if !since.IsZero() && now.Sub(since) > time.Minute {
			body = fmt.Sprintf("%s\nStanding for %s.", reason, now.Sub(since).Round(time.Minute))
		}
		return title, body + "\nRun `disk-watchdog --json` for the current tick."
	case absencealarm.NotifyRecovery:
		return "Disk watchdog: remediation reclaiming again", "A tick reclaimed space; the stall alarm is cleared."
	case absencealarm.NotifyNone:
		return "", ""
	default:
		return "", ""
	}
}

func dispatch(cfg config, title, body string) {
	send := cfg.notify
	if send == nil {
		send = desktopNotify
	}
	ctx, cancel := context.WithTimeout(context.Background(), escalateTimeout)
	defer cancel()
	if err := send(ctx, title, body); err != nil {
		// The journal already holds the record, so a failed dispatch degrades
		// the escalation rather than losing it.
		fmt.Fprintf(os.Stderr, "disk-watchdog: warning: escalation notify: %v\n", err)
	}
}

func desktopNotify(ctx context.Context, title, body string) error {
	return notify.NewDesktopDispatcher().Dispatch(ctx, &notify.Notification{
		ID:        fmt.Sprintf("%s-%d", escalationSource, time.Now().UnixNano()),
		Title:     title,
		Body:      body,
		Level:     slog.LevelError,
		Source:    escalationSource,
		Timestamp: time.Now(),
	})
}

// appendEscalation writes one JSONL record to the escalation journal.
func appendEscalation(cfg config, rec escalationRecord) {
	if cfg.escalationJournal == "" {
		return
	}
	if err := appendJSONL(cfg.escalationJournal, rec); err != nil {
		fmt.Fprintf(os.Stderr, "disk-watchdog: warning: escalation journal append: %v\n", err)
	}
}

func appendJSONL(path string, rec escalationRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, werr := f.Write(append(raw, '\n')); werr != nil {
		// Report both: a close that also fails can mean the record never
		// reached the journal, and reporting only the write hides that.
		return errors.Join(werr, f.Close())
	}
	return f.Close()
}

// defaultEscalationJournal and defaultEscalationState sit beside the
// absence-alarm's journal, so every host-level escalation lands in one place.
func defaultEscalationJournal() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agm/escalation/disk-watchdog.jsonl"
	}
	return filepath.Join(home, ".agm", "escalation", "disk-watchdog.jsonl")
}

func defaultEscalationState() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agm/escalation/disk-watchdog-state.json"
	}
	return filepath.Join(home, ".agm", "escalation", "disk-watchdog-state.json")
}
