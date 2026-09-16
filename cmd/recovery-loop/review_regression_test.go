package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

// RL-52: an older structural pulse is a duplicate observation and must not
// override the current direct launchd snapshot, including while clearing a
// standing failure.
func TestCLI_StructuralPulseAlarmDefersToDirectLaunchdState(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"mergeloop","launchd_label":"com.dear-agent.mergeloop","pulse":"mergeloop-loaded","pulse_is_structural":true}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"mergeloop-loaded","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"mergeloop":{"consecutive_failures":1,"last_attempt_time":%q,"last_action":"kickstart","last_status":"failed"}}}`,
		now.Add(-time.Hour).Format(time.RFC3339)))

	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.mergeloop": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args(), &stdout, &stderr, host, nil)

	if code != 0 {
		t.Fatalf("exit = %d, want 0 from current healthy launchd state; stdout:\n%s\nstderr:\n%s",
			code, stdout.String(), stderr.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("stale structural pulse caused remediation: %v", *calls)
	}
	if js := f.jobState(t, "mergeloop"); js.LastStatus != recoveryloop.StatusRecovered ||
		js.ConsecutiveFailures != 0 {
		t.Fatalf("direct structural recovery did not clear prior failure: %+v", js)
	}
}

// RL-51: a failed action's evidence boundary is when the action actually ran,
// not the earlier tick-start observation time.
func TestCLI_FailedActionUsesActualExecutionBoundary(t *testing.T) {
	f := newFixture(t)
	tickAt := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	evidenceAt := tickAt.Add(time.Minute)
	actionAt := tickAt.Add(2 * time.Minute)
	f.write(t, f.cfg, `{"jobs":[{"name":"worker","launchd_label":"com.example.worker","plist_path":"/tmp/worker.plist","pulse":"worker-tick"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"worker-tick","status":"present","evidence":%q}]}`,
		tickAt.Format(time.RFC3339), evidenceAt.Format(time.RFC3339)))

	host, _ := hostAt(tickAt, map[string]recoveryloop.LaunchdJobInfo{})
	nowCalls := 0
	host.Now = func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return tickAt
		}
		return actionAt
	}
	host.LaunchctlBootstrap = func(context.Context, string) error {
		return errors.New("bootstrap failed")
	}
	var firstOut, firstErr bytes.Buffer
	if code := run(f.args(), &firstOut, &firstErr, host, nil); code != 1 {
		t.Fatalf("failed action exit = %d, want 1", code)
	}
	if got := f.jobState(t, "worker").LastAttemptTime; !got.Equal(actionAt) {
		t.Fatalf("last_attempt_time = %s, want actual action time %s", got, actionAt)
	}

	// The same evidence is newer than tickAt but older than actionAt. It must
	// not clear the failed action once the structural condition is healthy.
	next := tickAt.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"worker-tick","status":"present","evidence":%q}]}`,
		next.Format(time.RFC3339), evidenceAt.Format(time.RFC3339)))
	host2, calls2 := hostAt(next, map[string]recoveryloop.LaunchdJobInfo{
		"com.example.worker": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args(), &stdout, &stderr, host2, nil); code != 1 {
		t.Fatalf("pre-action evidence exit = %d, want 1; stdout:\n%s", code, stdout.String())
	}
	if len(*calls2) != 0 {
		t.Fatalf("healthy structural state unexpectedly ran an action: %v", *calls2)
	}
	if js := f.jobState(t, "worker"); js.ConsecutiveFailures != 1 ||
		js.LastStatus != recoveryloop.StatusUnavailable {
		t.Fatalf("pre-action evidence cleared failed remediation: %+v", js)
	}
}

// RL-51: settling a pending attempt must retain its original action boundary,
// not replace it with the later settlement time.
func TestCLI_SettledFailureKeepsPendingActionBoundary(t *testing.T) {
	f := newFixture(t)
	actionAt := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	now := actionAt.Add(time.Hour)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"last_status":"pending-verification","pending_action":"kickstart","pending_since":%q,"pending_deadline":%q}}}`,
		actionAt.Format(time.RFC3339), actionAt.Add(30*time.Minute).Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil); code != 1 {
		t.Fatalf("settled failure exit = %d, want 1", code)
	}
	if got := f.jobState(t, "absence-alarm").LastAttemptTime; !got.Equal(actionAt) {
		t.Fatalf("last_attempt_time = %s, want pending action time %s", got, actionAt)
	}
}

// RL-51: give-up is a no-action state. It must not move the proof boundary
// past a delayed heartbeat that legitimately post-dates the last real action.
func TestCLI_GiveUpDoesNotMoveActionBoundary(t *testing.T) {
	f := newFixture(t)
	lastActionAt := time.Date(2026, 9, 16, 7, 0, 0, 0, time.UTC)
	giveUpTick := lastActionAt.Add(time.Hour)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":5,"last_attempt_time":%q,"last_action":"kickstart","last_status":"failed","human_needed":true,"last_escalated":%q}}}`,
		lastActionAt.Format(time.RFC3339), giveUpTick.Format(time.RFC3339)))
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		giveUpTick.Format(time.RFC3339)))
	host1, _ := hostAt(giveUpTick, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var out1, err1 bytes.Buffer
	if code := run(f.args(), &out1, &err1, host1, nil); code != 1 {
		t.Fatalf("give-up exit = %d, want 1", code)
	}
	if got := f.jobState(t, "absence-alarm").LastAttemptTime; !got.Equal(lastActionAt) {
		t.Fatalf("give-up moved last_attempt_time to %s; want %s", got, lastActionAt)
	}

	// This evidence arrived after the last action but before the give-up tick;
	// it appeared only in the following heartbeat publication.
	recoveredEvidence := lastActionAt.Add(30 * time.Minute)
	next := giveUpTick.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		next.Format(time.RFC3339), recoveredEvidence.Format(time.RFC3339)))
	host2, calls2 := hostAt(next, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var out2, err2 bytes.Buffer
	if code := run(f.args(), &out2, &err2, host2, nil); code != 0 {
		t.Fatalf("delayed recovery evidence exit = %d, want 0; stdout:\n%s", code, out2.String())
	}
	if len(*calls2) != 0 {
		t.Fatalf("recovered job ran another action: %v", *calls2)
	}
	if js := f.jobState(t, "absence-alarm"); js.LastStatus != recoveryloop.StatusRecovered ||
		js.ConsecutiveFailures != 0 || js.HumanNeeded {
		t.Fatalf("delayed valid evidence did not recover job: %+v", js)
	}
}

// RL-53: a future LastEscalated value is clock-correction damage, not a
// multi-day suppression window for a standing human-needed outage.
func TestCLI_FutureLastEscalatedDoesNotSuppressEscalation(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":5,"last_attempt_time":%q,"last_action":"kickstart","last_status":"failed","human_needed":true,"last_escalated":%q}}}`,
		now.Add(-time.Hour).Format(time.RFC3339), now.Add(48*time.Hour).Format(time.RFC3339)))
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	notifications := 0
	var stdout, stderr bytes.Buffer
	if code := run(f.args(), &stdout, &stderr, host, countingNotifier(&notifications)); code != 1 {
		t.Fatalf("standing failure exit = %d, want 1", code)
	}
	if notifications != 1 {
		t.Fatalf("notifications = %d, want 1 after corrected clock", notifications)
	}
	if got := f.jobState(t, "absence-alarm").LastEscalated; !got.Equal(now) {
		t.Fatalf("last_escalated = %s, want successful delivery time %s", got, now)
	}
}

// RL-53/RL-40: correcting a future delivery timestamp is contingent on an
// accepted delivery. When both sinks fail, retaining the damaged timestamp is
// safer than falsely recording success; escalationDue will retry next tick.
func TestCLI_FutureLastEscalatedRemainsUntilDeliverySucceeds(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	future := now.Add(48 * time.Hour)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":5,"last_attempt_time":%q,"last_action":"kickstart","last_status":"failed","human_needed":true,"last_escalated":%q}}}`,
		now.Add(-time.Hour).Format(time.RFC3339), future.Format(time.RFC3339)))

	blocker := f.dir + "/blocked"
	f.write(t, blocker, "not a directory")
	args := append(f.args(), "--absence-journal", blocker+"/absence.jsonl")
	failing := func(context.Context, string, string) error {
		return errors.New("no notification daemon")
	}
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, host, failing); code != 1 {
		t.Fatalf("standing failure exit = %d, want 1", code)
	}
	if got := f.jobState(t, "absence-alarm").LastEscalated; !got.Equal(future) {
		t.Fatalf("last_escalated = %s after both sinks failed, want original future value %s", got, future)
	}
}

// RL-55: a carried verification inside its grace window is unresolved work,
// not a clean tick, even when all required observations are available.
func TestCLI_CarriedPendingRecoveryExitsNonzero(t *testing.T) {
	f := newFixture(t)
	actionAt := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	now := actionAt.Add(10 * time.Minute)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"last_status":"pending-verification","pending_action":"kickstart","pending_since":%q,"pending_deadline":%q}}}`,
		actionAt.Format(time.RFC3339), actionAt.Add(30*time.Minute).Format(time.RFC3339)))
	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)
	if code != 1 {
		t.Fatalf("pending verification exit = %d, want 1; stdout:\n%s", code, stdout.String())
	}
	if len(*calls) != 0 {
		t.Fatalf("open verification fired another action: %v", *calls)
	}
	if !strings.Contains(stdout.String(), "pending-verification") {
		t.Fatalf("report did not expose pending verification:\n%s", stdout.String())
	}
}

// RL-54: stale source data restarts only the source owner. Other jobs remain
// observation-unavailable until the source publishes fresh truth.
func TestCLI_StalePulseTruthRestartsSourceOwnerOnly(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[
		{"name":"absence-alarm","launchd_label":"com.dear-agent.absence-alarm","pulse":"absence-alarm-heartbeat"},
		{"name":"disk-watchdog","launchd_label":"com.dear-agent.disk-watchdog","pulse":"disk-watchdog-tick"}
	]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[
			{"name":"absence-alarm-heartbeat","status":"present"},
			{"name":"disk-watchdog-tick","status":"present"}
		]}`,
		t0.Add(-2*time.Hour).Format(time.RFC3339)))
	launchd := map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
		"com.dear-agent.disk-watchdog": {Loaded: true, Status: 0},
	}
	host1, calls1 := hostAt(t0, launchd)
	var out1, err1 bytes.Buffer
	if code := run(f.args(), &out1, &err1, host1, nil); code != 1 {
		t.Fatalf("stale source exit = %d, want 1; stdout:\n%s", code, out1.String())
	}
	wantCall := "kickstart:com.dear-agent.absence-alarm"
	if len(*calls1) != 1 || (*calls1)[0] != wantCall {
		t.Fatalf("stale source calls = %v, want [%s]", *calls1, wantCall)
	}
	if js := f.jobState(t, "absence-alarm"); js.LastStatus != recoveryloop.StatusPending {
		t.Fatalf("source restart was not held pending fresh evidence: %+v", js)
	}

	// A fresh post-action self-heartbeat verifies the source recovery; the
	// downstream activity pulse can be evaluated normally again.
	t1 := t0.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[
			{"name":"absence-alarm-heartbeat","status":"present","evidence":%q},
			{"name":"disk-watchdog-tick","status":"present","evidence":%q}
		]}`,
		t1.Format(time.RFC3339), t0.Add(time.Minute).Format(time.RFC3339), t1.Format(time.RFC3339)))
	host2, calls2 := hostAt(t1, launchd)
	var out2, err2 bytes.Buffer
	if code := run(f.args(), &out2, &err2, host2, nil); code != 0 {
		t.Fatalf("fresh source recovery exit = %d, want 0; stdout:\n%s\nstderr:\n%s",
			code, out2.String(), err2.String())
	}
	if len(*calls2) != 0 {
		t.Fatalf("fresh source caused another action: %v", *calls2)
	}
}

// RL-30/RL-54: stale source data is a negative liveness observation for its
// owner. If restarting that owner does not produce a fresh heartbeat by the
// deadline, the open attempt must become a counted failure rather than remain
// pending forever as though the observation were unavailable. The source
// verdict remains conclusive even if launchd cannot be listed on that tick.
func TestCLI_StalePulseTruthPastDeadlineCountsOwnerFailure(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present"}]}`,
		t0.Add(-2*time.Hour).Format(time.RFC3339)))
	launchd := map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	}

	host1, calls1 := hostAt(t0, launchd)
	var out1, err1 bytes.Buffer
	if code := run(f.args("--verify-grace", "30m"), &out1, &err1, host1, nil); code != 1 {
		t.Fatalf("initial stale source exit = %d, want 1; stdout:\n%s", code, out1.String())
	}
	if len(*calls1) != 1 || (*calls1)[0] != "kickstart:com.dear-agent.absence-alarm" {
		t.Fatalf("initial stale source calls = %v, want one owner kickstart", *calls1)
	}

	afterDeadline := t0.Add(time.Hour)
	host2, calls2 := hostAt(afterDeadline, launchd)
	host2.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return nil, errors.New("launchctl: connection interrupted")
	}
	var out2, err2 bytes.Buffer
	if code := run(f.args("--verify-grace", "30m"), &out2, &err2, host2, nil); code != 1 {
		t.Fatalf("expired stale source exit = %d, want 1; stdout:\n%s", code, out2.String())
	}
	if len(*calls2) != 0 {
		t.Fatalf("open verification fired a second remediation: %v", *calls2)
	}
	js := f.jobState(t, "absence-alarm")
	if js.LastStatus != recoveryloop.StatusFailed || js.ConsecutiveFailures != 1 {
		t.Fatalf("persistent stale source did not become a counted failure: %+v", js)
	}
	if !js.PendingDeadline.IsZero() || js.PendingAction != "" {
		t.Fatalf("settled stale-source failure retained pending state: %+v", js)
	}
}
