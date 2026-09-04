package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A fail-closed remediation that cannot reclaim must escalate.
//
// Historically it did the opposite: 2549 consecutive failing ticks produced no
// escalation at all. The watchdog latched the admission brake, appended to a
// decision trail nothing reads, and exited 1 into launchd, which discards it.
// Every one of those ticks was a silent loop.
//
// The condition that matters is not "remediation returned an error". It is
// "the host is still breached and this tick reclaimed nothing". A tick whose
// worktree sweep was killed but whose cache trim freed 44 GiB has remediated.

type recordedNotification struct {
	title string
	body  string
}

type fakeNotifier struct {
	sent []recordedNotification
	err  error
}

func (f *fakeNotifier) notify(_ context.Context, title, body string) error {
	f.sent = append(f.sent, recordedNotification{title: title, body: body})
	return f.err
}

func readJournal(t *testing.T, path string) []escalationRecord {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var recs []escalationRecord
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var r escalationRecord
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("decode journal: %v", err)
		}
		recs = append(recs, r)
	}
	return recs
}

// TestRemediationOutcome_ClassifiesReclaim is the policy this whole fix turns
// on: reclaimed bytes, not the exit status of one remediation leg, decide
// whether the host was helped.
func TestRemediationOutcome_ClassifiesReclaim(t *testing.T) {
	killedSweep := &sweepResult{Error: "agm worktree sweep --execute: signal: killed"}

	for _, tc := range []struct {
		name      string
		breached  bool
		rem       *sweepResult
		reclaimed int64
		want      bool
	}{
		{"healthy tick does not escalate", false, nil, 0, false},
		{"breached, sweep killed, nothing reclaimed", true, killedSweep, 0, true},
		{"breached, sweep killed, cache trim reclaimed", true, killedSweep, 44 << 30, false},
		{"breached, sweep clean but reclaimed nothing", true, &sweepResult{}, 0, true},
		{"breached, remediation skipped entirely", true, nil, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := remediationStalled(tc.breached, tc.rem, tc.reclaimed); got != tc.want {
				t.Fatalf("remediationStalled(%v, %+v, %d) = %v, want %v",
					tc.breached, tc.rem, tc.reclaimed, got, tc.want)
			}
		})
	}
}

// TestEscalate_NotifiesAndJournalsOnFirstStall: the first stalled tick must
// reach a human channel and leave an auditable record.
func TestEscalate_NotifiesAndJournalsOnFirstStall(t *testing.T) {
	dir := t.TempDir()
	n := &fakeNotifier{}
	cfg := config{
		escalationJournal: filepath.Join(dir, "escalation.jsonl"),
		escalationState:   filepath.Join(dir, "state.json"),
		notify:            n.notify,
	}

	escalateStall(cfg, true, "disk 97% used, 12.0 GiB free; remediation reclaimed 0 bytes", time.Now())

	if len(n.sent) != 1 {
		t.Fatalf("len(notifications) = %d, want 1", len(n.sent))
	}
	if n.sent[0].title == "" || n.sent[0].body == "" {
		t.Fatalf("notification must carry a title and body, got %+v", n.sent[0])
	}
	recs := readJournal(t, cfg.escalationJournal)
	if len(recs) != 1 {
		t.Fatalf("len(journal) = %d, want 1", len(recs))
	}
	if recs[0].Kind != "disk.remediation.stalled" {
		t.Fatalf("Kind = %q, want disk.remediation.stalled", recs[0].Kind)
	}
	if recs[0].Reason == "" {
		t.Fatalf("journal record must carry the reason, got %+v", recs[0])
	}
}

// TestEscalate_DoesNotNotifyEveryTick is the other half of the 2549-failure
// lesson. Silence was one failure mode; a notification every five minutes for
// nine days is the failure mode that trains an operator to ignore the channel.
// Every stalled tick is journalled; only escalation points re-notify.
func TestEscalate_DoesNotNotifyEveryTick(t *testing.T) {
	dir := t.TempDir()
	n := &fakeNotifier{}
	cfg := config{
		escalationJournal: filepath.Join(dir, "escalation.jsonl"),
		escalationState:   filepath.Join(dir, "state.json"),
		notify:            n.notify,
	}

	start := time.Now()
	// Twelve consecutive stalled ticks, five minutes apart: one hour of alarm.
	for i := range 12 {
		escalateStall(cfg, true, "still stalled", start.Add(time.Duration(i)*5*time.Minute))
	}
	if len(n.sent) != 1 {
		t.Fatalf("len(notifications) = %d after an hour of five-minute ticks, want 1", len(n.sent))
	}
	if recs := readJournal(t, cfg.escalationJournal); len(recs) != 12 {
		t.Fatalf("len(journal) = %d, want every stalled tick recorded (12)", len(recs))
	}

	// Crossing the one-hour escalation point re-notifies.
	escalateStall(cfg, true, "still stalled", start.Add(61*time.Minute))
	if len(n.sent) != 2 {
		t.Fatalf("len(notifications) = %d after crossing the 1h point, want 2", len(n.sent))
	}
}

// TestEscalate_RecoveryClearsTheAlarm: a healthy tick must clear the state so
// the next stall notifies immediately instead of inheriting a spent backoff.
func TestEscalate_RecoveryClearsTheAlarm(t *testing.T) {
	dir := t.TempDir()
	n := &fakeNotifier{}
	cfg := config{
		escalationJournal: filepath.Join(dir, "escalation.jsonl"),
		escalationState:   filepath.Join(dir, "state.json"),
		notify:            n.notify,
	}

	start := time.Now()
	escalateStall(cfg, true, "stalled", start)
	escalateStall(cfg, false, "", start.Add(5*time.Minute)) // recovered
	escalateStall(cfg, true, "stalled again", start.Add(10*time.Minute))

	// alarm, recovery, alarm.
	if len(n.sent) != 3 {
		t.Fatalf("len(notifications) = %d, want 3 (alarm, recovery, alarm)", len(n.sent))
	}
}

// TestEscalate_DryRunIsSilent: a --dry-run tick inspects, it does not page.
func TestEscalate_DryRunIsSilent(t *testing.T) {
	dir := t.TempDir()
	n := &fakeNotifier{}
	cfg := config{
		dryRun:            true,
		escalationJournal: filepath.Join(dir, "escalation.jsonl"),
		escalationState:   filepath.Join(dir, "state.json"),
		notify:            n.notify,
	}

	escalateStall(cfg, true, "stalled", time.Now())

	if len(n.sent) != 0 {
		t.Fatalf("dry run sent %d notification(s), want 0", len(n.sent))
	}
	if recs := readJournal(t, cfg.escalationJournal); len(recs) != 0 {
		t.Fatalf("dry run wrote %d journal record(s), want 0", len(recs))
	}
}

// TestEscalate_NotifierFailureStillJournals: the journal is the durable record.
// A notifier that cannot dispatch must not also cost us the audit trail:
// that combination is precisely how 2549 failures left no evidence.
func TestEscalate_NotifierFailureStillJournals(t *testing.T) {
	dir := t.TempDir()
	n := &fakeNotifier{err: os.ErrPermission}
	cfg := config{
		escalationJournal: filepath.Join(dir, "escalation.jsonl"),
		escalationState:   filepath.Join(dir, "state.json"),
		notify:            n.notify,
	}

	escalateStall(cfg, true, "stalled", time.Now())

	if recs := readJournal(t, cfg.escalationJournal); len(recs) != 1 {
		t.Fatalf("len(journal) = %d after a failed notification, want 1", len(recs))
	}
}
