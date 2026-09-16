package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

// RL-58/RL-59: failure to pay historical delivery debt must not consume the
// only durable slot or banner opportunity for a new incident in the same tick.
func TestCLI_ResolvedDurableRetryFailureQueuesAndNotifiesNewOutage(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	pending := absencealarm.JournalRecord{
		Time: now.Add(-2 * time.Hour), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "resolved incident durable debt", Misses: 2,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			LastStatus: recoveryloop.StatusRecovered, LastAction: recoveryloop.ActionKickstart,
			LastEscalated: now.Add(-time.Hour), PendingEscalations: []absencealarm.JournalRecord{pending},
		},
	}}); err != nil {
		t.Fatalf("save resolved pending escalation: %v", err)
	}
	blocker := f.dir + "/new-outage-blocker"
	f.write(t, blocker, "not a directory")
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	host.LaunchctlKickstart = func(context.Context, string) error { return errors.New("new outage") }
	var notified []string
	notify := func(_ context.Context, _ string, body string) error {
		notified = append(notified, body)
		return nil
	}
	args := append(f.args("--escalate-after", "1", "--give-up-after", "0"),
		"--absence-journal", blocker+"/absence.jsonl")
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, host, notify); code != 1 {
		t.Fatalf("new outage exit = %d, want 1", code)
	}
	if len(notified) != 1 || strings.Contains(notified[0], pending.Reason) {
		t.Fatalf("fresh outage notification lost or stale: %#v", notified)
	}
	queue := f.jobState(t, "absence-alarm").PendingEscalations
	if len(queue) != 2 || queue[0].Reason != pending.Reason ||
		queue[1].Reason == pending.Reason {
		t.Fatalf("durable queue did not preserve old and new events in order: %+v", queue)
	}

	// Once the re-escalation interval expires, historical queue head A must
	// neither suppress incident B nor supply B's desktop narrative.
	later := now.Add(reEscalateInterval + time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		later.Format(time.RFC3339)))
	host2, _ := hostAt(later, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	host2.LaunchctlKickstart = func(context.Context, string) error { return errors.New("new outage continues") }
	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr, host2, notify); code != 1 {
		t.Fatalf("re-escalation exit = %d, want 1", code)
	}
	if len(notified) != 2 || strings.Contains(notified[1], pending.Reason) {
		t.Fatalf("historical debt supplied or suppressed current narrative: %#v", notified)
	}
}

// RL-38: the global retry prepass is still a write path, so dry-run must leave
// its durable queue byte-identical and must not invoke either escalation sink.
func TestCLI_DryRunDoesNotRetryPendingDurableEscalations(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	pending := absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "must remain pending in dry-run", Misses: 5,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			ConsecutiveFailures: 5, LastStatus: recoveryloop.StatusFailed,
			LastAction: recoveryloop.ActionKickstart, HumanNeeded: true,
			PendingEscalations:  []absencealarm.JournalRecord{pending},
			PendingNotification: &pending,
		},
	}}); err != nil {
		t.Fatalf("save pending escalation: %v", err)
	}
	before, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatalf("read state before dry-run: %v", err)
	}
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	notifications := 0
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--dry-run", "--give-up-after", "5"),
		&stdout, &stderr, host, countingNotifier(&notifications)); code != 1 {
		t.Fatalf("dry-run standing outage exit = %d, want 1", code)
	}
	after, err := os.ReadFile(f.state)
	if err != nil {
		t.Fatalf("read state after dry-run: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("dry-run mutated durable escalation state\nbefore: %s\nafter: %s", before, after)
	}
	if notifications != 0 || len(f.absenceRecords(t)) != 0 {
		t.Fatalf("dry-run invoked escalation sinks: notifications=%d records=%d",
			notifications, len(f.absenceRecords(t)))
	}
}

// RL-37/RL-40/RL-59: one undelivered standing-incident record remains one
// record across ticks; retrying failed sinks must not grow a duplicate queue.
func TestCLI_NoSinkDoesNotDuplicateStandingIncidentQueue(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	pending := absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "one standing incident", Misses: 5,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			ConsecutiveFailures: 5, LastStatus: recoveryloop.StatusFailed,
			LastAction: recoveryloop.ActionKickstart, HumanNeeded: true,
			PendingEscalations:  []absencealarm.JournalRecord{pending},
			PendingNotification: &pending,
		},
	}}); err != nil {
		t.Fatalf("save standing escalation: %v", err)
	}
	blocker := f.dir + "/all-sinks-blocker"
	f.write(t, blocker, "not a directory")
	notifications := 0
	notify := func(context.Context, string, string) error {
		notifications++
		return errors.New("desktop unavailable")
	}
	for tick := range 2 {
		at := now.Add(time.Duration(tick) * time.Minute)
		f.write(t, f.absHB, fmt.Sprintf(
			`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
			at.Format(time.RFC3339)))
		host, _ := hostAt(at, map[string]recoveryloop.LaunchdJobInfo{
			"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
		})
		args := append(f.args("--give-up-after", "5"),
			"--absence-journal", blocker+"/absence.jsonl")
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr, host, notify); code != 1 {
			t.Fatalf("tick %d exit = %d, want 1", tick+1, code)
		}
		queue := f.jobState(t, "absence-alarm").PendingEscalations
		if len(queue) != 1 || queue[0].Reason != pending.Reason {
			t.Fatalf("tick %d duplicated standing queue: %+v", tick+1, queue)
		}
	}
	if notifications != 2 {
		t.Fatalf("failed notification attempts = %d, want one per tick", notifications)
	}
}

// RL-35/RL-59: queue entries retain their original pulse identity. Draining
// an unsnoozed head must stop before a later record whose own pulse is snoozed.
func TestCLI_PendingQueueChecksSnoozePerRecord(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"worker","launchd_label":"com.example.worker","pulse":"current-tick"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"current-tick","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	f.write(t, f.snooze, fmt.Sprintf(
		`[{"pulse":"new-tick","until":%q,"reason":"new pulse maintenance"}]`,
		now.Add(time.Hour).Format(time.RFC3339)))
	oldRecord := absencealarm.JournalRecord{
		Time: now.Add(-2 * time.Hour), Kind: "recovery.human_needed",
		Pulse: "old-tick", Status: absencealarm.StatusAbsent, Reason: "old pulse debt", Misses: 2,
	}
	newRecord := absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "new-tick", Status: absencealarm.StatusAbsent, Reason: "new pulse debt", Misses: 3,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"worker": {
			ConsecutiveFailures: 3, LastStatus: recoveryloop.StatusFailed,
			HumanNeeded: true, PendingEscalations: []absencealarm.JournalRecord{oldRecord, newRecord},
		},
	}}); err != nil {
		t.Fatalf("save mixed-pulse queue: %v", err)
	}
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.example.worker": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args(), &stdout, &stderr, host, nil); code != 0 {
		t.Fatalf("snoozed mixed queue exit = %d, want 0", code)
	}
	records := f.absenceRecords(t)
	if len(records) != 1 || fmt.Sprint(records[0]["reason"]) != oldRecord.Reason {
		t.Fatalf("prepass did not stop at snoozed record: %#v", records)
	}
	queue := f.jobState(t, "worker").PendingEscalations
	if len(queue) != 1 || queue[0].Reason != newRecord.Reason {
		t.Fatalf("snoozed queue suffix was not retained exactly: %+v", queue)
	}
}

// RL-58/RL-59: banner retries happen only after current observation. A pulse
// that proves recovery this tick must silence the old incident's narrative.
func TestCLI_ReturnedPulseSuppressesStalePendingNotification(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	pending := absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "old incident is not recovered", Misses: 2,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			ConsecutiveFailures: 2, LastAttemptTime: now.Add(-2 * time.Hour),
			LastAction: recoveryloop.ActionKickstart, LastStatus: recoveryloop.StatusFailed,
			HumanNeeded: true, PendingEscalations: []absencealarm.JournalRecord{pending},
			LastEscalated: now.Add(-time.Hour), PendingNotification: &pending,
		},
	}}); err != nil {
		t.Fatalf("save active pending escalation: %v", err)
	}
	blocker := f.dir + "/recovered-blocker"
	f.write(t, blocker, "not a directory")
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	notifications := 0
	args := append(f.args(), "--absence-journal", blocker+"/absence.jsonl")
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, host, countingNotifier(&notifications)); code != 0 {
		t.Fatalf("verified recovery exit = %d, want 0", code)
	}
	if notifications != 0 {
		t.Fatalf("verified recovery emitted %d stale notifications", notifications)
	}
	st := f.jobState(t, "absence-alarm")
	if st.HumanNeeded || st.LastStatus != recoveryloop.StatusRecovered ||
		len(st.PendingEscalations) != 1 || st.PendingNotification != nil || !st.LastEscalated.IsZero() {
		t.Fatalf("recovery/delivery-debt state = %+v", st)
	}
}

// RL-59: durable and desktop delivery have independent debts. Repairing the
// journal on tick N+1 must not erase a banner that failed on tick N.
func TestCLI_DurableRecoveryStillRetriesFailedNotification(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":5,"last_action":"kickstart","last_status":"failed","human_needed":true}}}`)
	blocker := f.dir + "/first-tick-blocker"
	f.write(t, blocker, "not a directory")
	notifications := 0
	notify := func(context.Context, string, string) error {
		notifications++
		if notifications == 1 {
			return errors.New("desktop unavailable")
		}
		return nil
	}
	runTick := func(at time.Time, args []string) {
		t.Helper()
		f.write(t, f.absHB, fmt.Sprintf(
			`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
			at.Format(time.RFC3339)))
		host, _ := hostAt(at, map[string]recoveryloop.LaunchdJobInfo{
			"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
		})
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr, host, notify); code != 1 {
			t.Fatalf("tick at %s exit = %d, want 1", at, code)
		}
	}
	firstArgs := append(f.args("--give-up-after", "5"),
		"--absence-journal", blocker+"/absence.jsonl")
	runTick(now, firstArgs)
	first := f.jobState(t, "absence-alarm")
	if len(first.PendingEscalations) != 1 || first.PendingNotification == nil {
		t.Fatalf("first tick did not persist both delivery debts: %+v", first)
	}

	next := now.Add(time.Minute)
	runTick(next, f.args("--give-up-after", "5"))
	second := f.jobState(t, "absence-alarm")
	if len(second.PendingEscalations) != 0 || second.PendingNotification != nil ||
		!second.LastEscalated.Equal(next) {
		t.Fatalf("independent debt retry state = %+v", second)
	}
	if notifications != 2 || len(f.absenceRecords(t)) != 1 {
		t.Fatalf("retry deliveries notifications=%d records=%d, want 2/1",
			notifications, len(f.absenceRecords(t)))
	}
}

// RL-59: an unconfigured job has no current observation. Its durable record
// still retries, but a blocked journal must not trigger an unverified banner.
func TestCLI_RemovedJobDoesNotRetryPendingNotification(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"active-worker","launchd_label":"com.example.active-worker"}]}`)
	pending := absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "retired-tick", Status: absencealarm.StatusAbsent,
		Reason: "retired worker historical alert", Misses: 3,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"retired-worker": {
			ConsecutiveFailures: 3, LastStatus: recoveryloop.StatusFailed, HumanNeeded: true,
			PendingEscalations: []absencealarm.JournalRecord{pending}, PendingNotification: &pending,
		},
	}}); err != nil {
		t.Fatalf("save removed job debt: %v", err)
	}
	blocker := f.dir + "/removed-job-blocker"
	f.write(t, blocker, "not a directory")
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.example.active-worker": {Loaded: true, Status: 0},
	})
	notifications := 0
	args := append(f.args(), "--absence-journal", blocker+"/absence.jsonl")
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, host, countingNotifier(&notifications)); code != 0 {
		t.Fatalf("removed job retry exit = %d, want 0", code)
	}
	st := f.jobState(t, "retired-worker")
	if notifications != 0 || len(st.PendingEscalations) != 1 || st.PendingNotification == nil {
		t.Fatalf("removed job emitted or lost debt: notifications=%d state=%+v", notifications, st)
	}
}

// RL-35/RL-59: a snooze on the currently configured job defers every queued
// record, even when an older record names a pre-rename pulse.
func TestCLI_CurrentJobSnoozeDefersHistoricalPulseDebt(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"worker","launchd_label":"com.example.worker","pulse":"new-tick"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"new-tick","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.snooze, fmt.Sprintf(
		`[{"pulse":"new-tick","until":%q,"reason":"current maintenance"}]`,
		now.Add(time.Hour).Format(time.RFC3339)))
	pending := absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "old-tick", Status: absencealarm.StatusAbsent,
		Reason: "pre-rename debt", Misses: 2,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"worker": {
			ConsecutiveFailures: 2, LastStatus: recoveryloop.StatusFailed, HumanNeeded: true,
			PendingEscalations: []absencealarm.JournalRecord{pending},
		},
	}}); err != nil {
		t.Fatalf("save historical debt: %v", err)
	}
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.example.worker": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args(), &stdout, &stderr, host, nil); code != 0 {
		t.Fatalf("snoozed job exit = %d, want 0", code)
	}
	if len(f.absenceRecords(t)) != 0 || len(f.jobState(t, "worker").PendingEscalations) != 1 {
		t.Fatal("current job snooze did not defer historical pulse debt")
	}
}

// RL-59: an unavailable observation is not proof that an incident remains
// unresolved, so it cannot authorize replaying an old desktop narrative.
func TestCLI_ObservationUnavailableDefersPendingNotification(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	pending := absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "unconfirmed standing incident", Misses: 2,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			ConsecutiveFailures: 2, LastStatus: recoveryloop.StatusFailed, HumanNeeded: true,
			PendingEscalations: []absencealarm.JournalRecord{pending}, PendingNotification: &pending,
		},
	}}); err != nil {
		t.Fatalf("save pending notification: %v", err)
	}
	blocker := f.dir + "/unavailable-blocker"
	f.write(t, blocker, "not a directory")
	host, _ := hostAt(now, nil)
	host.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return nil, errors.New("launchd unavailable")
	}
	notifications := 0
	args := append(f.args(), "--absence-journal", blocker+"/absence.jsonl")
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, host, countingNotifier(&notifications)); code != 1 {
		t.Fatalf("unavailable observation exit = %d, want 1", code)
	}
	st := f.jobState(t, "absence-alarm")
	if notifications != 0 || len(st.PendingEscalations) != 1 || st.PendingNotification == nil {
		t.Fatalf("unavailable observation emitted or lost debt: notifications=%d state=%+v", notifications, st)
	}
}
