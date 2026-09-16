package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
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

// RL-56: a give-up report must distinguish the action that actually ran from
// the newly proposed remedy that the give-up policy suppressed.
func TestCLI_GiveUpDistinguishesExecutedAndSuppressedActions(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"worker","launchd_label":"com.example.worker",`+
		`"binary_path":"/missing/worker","install_cmd":["install-worker"],"pulse":"worker-tick"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"worker-tick","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"worker":{"consecutive_failures":5,"last_attempt_time":%q,`+
			`"last_action":"kickstart","last_status":"failed","human_needed":true}}}`,
		now.Add(-time.Hour).Format(time.RFC3339)))

	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.example.worker": {Loaded: true, Status: 0},
	})
	host.FileExists = func(string) bool { return false }
	var notification string
	notify := func(_ context.Context, _ string, body string) error {
		notification = body
		return nil
	}
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--give-up-after", "5"), &stdout, &stderr, host, notify); code != 1 {
		t.Fatalf("give-up exit = %d, want 1", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("give-up executed the suppressed remediation: %v", *calls)
	}
	if !strings.Contains(stdout.String(), "(action kickstart)") ||
		!strings.Contains(stdout.String(), "planned reinstall remediation suppressed") {
		t.Fatalf("report conflated executed and suppressed actions:\n%s", stdout.String())
	}
	records := f.absenceRecords(t)
	if len(records) != 1 {
		t.Fatalf("escalation records = %d, want 1", len(records))
	}
	journalBody := fmt.Sprint(records[0]["reason"])
	for sink, body := range map[string]string{"journal": journalBody, "notification": notification} {
		if !strings.Contains(body, "last action kickstart") ||
			!strings.Contains(body, "planned reinstall remediation suppressed") {
			t.Errorf("%s conflated executed and suppressed actions: %q", sink, body)
		}
	}
	if got := f.jobState(t, "worker").LastAction; got != recoveryloop.ActionKickstart {
		t.Fatalf("last_action = %q, want executed kickstart", got)
	}
}

// RL-57: escalation machine data and prose preserve an undetermined probe;
// they must not fabricate an observed absence.
func TestCLI_UndeterminedPulseEscalationPreservesStatus(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"worker","launchd_label":"com.example.worker","pulse":"worker-tick"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"worker-tick","status":"undetermined","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.example.worker": {Loaded: true, Status: 0},
	})
	host.LaunchctlKickstart = func(context.Context, string) error { return errors.New("kickstart failed") }
	var notification string
	notify := func(_ context.Context, _ string, body string) error {
		notification = body
		return nil
	}
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--escalate-after", "1", "--give-up-after", "0"), &stdout, &stderr, host, notify); code != 1 {
		t.Fatalf("failed recovery exit = %d, want 1", code)
	}
	records := f.absenceRecords(t)
	if len(records) != 1 || fmt.Sprint(records[0]["status"]) != string(absencealarm.StatusUndetermined) {
		t.Fatalf("escalation status = %#v, want undetermined", records)
	}
	for sink, body := range map[string]string{
		"journal": fmt.Sprint(records[0]["reason"]), "notification": notification,
	} {
		if !strings.Contains(body, "observation is undetermined") || strings.Contains(body, `pulse "worker-tick" absent`) {
			t.Errorf("%s fabricated absence from an undetermined probe: %q", sink, body)
		}
	}
}

func TestEscalationPulseContextDoesNotFabricateAbsence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		job        recoveryloop.Job
		truth      recoveryloop.PulseTruth
		wantStatus absencealarm.Status
		wantText   string
	}{
		{
			name: "unobserved named pulse", job: recoveryloop.Job{Name: "worker", Pulse: "worker-tick"},
			wantStatus: absencealarm.StatusUndetermined, wantText: "no current observation",
		},
		{
			name: "pulse-less structural job", job: recoveryloop.Job{Name: "worker"},
			wantStatus: absencealarm.StatusUndetermined, wantText: "no pulse configured",
		},
		{
			name: "present structural condition", job: recoveryloop.Job{Name: "worker", Pulse: "worker-tick"},
			truth: recoveryloop.PulseTruth{"worker-tick": {
				Known: true, Status: absencealarm.StatusPresent,
			}},
			wantStatus: absencealarm.StatusPresent, wantText: "is present",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, status, condition := escalationPulseContext(tc.job, tc.truth, time.Hour)
			if status != tc.wantStatus || !strings.Contains(condition, tc.wantText) || strings.Contains(condition, " absent") {
				t.Fatalf("status=%q condition=%q, want %q containing %q", status, condition, tc.wantStatus, tc.wantText)
			}
		})
	}
}

// RL-58: changing a runtime threshold is not recovery evidence. A standing
// human-needed gate persists through another failure and clears only after a
// later verified recovery.
func TestCLI_HumanNeededSurvivesEscalationThresholdIncrease(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":2,"last_action":"kickstart","last_status":"failed","human_needed":true}}}`)
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	host.LaunchctlKickstart = func(context.Context, string) error { return errors.New("kickstart failed") }
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--escalate-after", "10", "--give-up-after", "0"), &stdout, &stderr, host, nil); code != 1 {
		t.Fatalf("failed action exit = %d, want 1", code)
	}
	if js := f.jobState(t, "absence-alarm"); !js.HumanNeeded || js.ConsecutiveFailures != 3 {
		t.Fatalf("threshold increase cleared standing human-needed state: %+v", js)
	}
	if !strings.Contains(stdout.String(), "HUMAN NEEDED") {
		t.Fatalf("standing human-needed state disappeared from report:\n%s", stdout.String())
	}

	next := now.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		next.Format(time.RFC3339), next.Format(time.RFC3339)))
	host2, _ := hostAt(next, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	if code := run(f.args("--escalate-after", "10", "--give-up-after", "0"), &stdout, &stderr, host2, nil); code != 0 {
		t.Fatalf("verified recovery exit = %d, want 0; stdout:\n%s", code, stdout.String())
	}
	if js := f.jobState(t, "absence-alarm"); js.HumanNeeded || js.ConsecutiveFailures != 0 ||
		js.LastStatus != recoveryloop.StatusRecovered {
		t.Fatalf("verified recovery did not clear human-needed state: %+v", js)
	}
}

// RL-59: a desktop banner cannot mask failure of the required durable
// escalation journal. The next tick retries only the missing durable sink.
func TestCLI_PartialEscalationRetriesDurableJournalOnly(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":5,"last_action":"kickstart","last_status":"failed","human_needed":true}}}`)
	blocker := f.dir + "/blocked"
	f.write(t, blocker, "not a directory")
	blockedJournal := blocker + "/absence.jsonl"
	notifications := 0
	notify := func(context.Context, string, string) error {
		notifications++
		return nil
	}
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	firstArgs := append(f.args("--give-up-after", "5"), "--absence-journal", blockedJournal)
	if code := run(firstArgs, &stdout, &stderr, host, notify); code != 1 {
		t.Fatalf("partial escalation exit = %d, want 1", code)
	}
	first := f.jobState(t, "absence-alarm")
	if len(first.PendingEscalations) != 1 || !first.LastEscalated.Equal(now) || notifications != 1 {
		t.Fatalf("partial delivery state=%+v notifications=%d", first, notifications)
	}

	next := now.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		next.Format(time.RFC3339)))
	host2, _ := hostAt(next, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	if code := run(f.args("--give-up-after", "5"), &stdout, &stderr, host2, notify); code != 1 {
		t.Fatalf("durable retry exit = %d, want 1", code)
	}
	if notifications != 1 {
		t.Fatalf("rate-limited banner repeated during journal-only retry: %d notifications", notifications)
	}
	if records := f.absenceRecords(t); len(records) != 1 {
		t.Fatalf("durable retry records = %d, want 1", len(records))
	}
	if js := f.jobState(t, "absence-alarm"); len(js.PendingEscalations) != 0 {
		t.Fatalf("successful durable retry left journal pending: %+v", js)
	}
}

// RL-59 applies before give-up too. A failed action can partially deliver its
// escalation, and the following tick must retry the exact durable record even
// when the new action transitions into pending verification.
func TestCLI_PartialEscalationRetriesBeforePendingTransition(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_action":"kickstart","last_status":"failed"}}}`)
	blocker := f.dir + "/blocked-before-give-up"
	f.write(t, blocker, "not a directory")
	notifications := 0
	notify := func(context.Context, string, string) error {
		notifications++
		return nil
	}
	host1, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	host1.LaunchctlKickstart = func(context.Context, string) error { return errors.New("kickstart failed") }
	firstArgs := append(f.args("--escalate-after", "2", "--give-up-after", "5"),
		"--absence-journal", blocker+"/absence.jsonl")
	var stdout, stderr bytes.Buffer
	if code := run(firstArgs, &stdout, &stderr, host1, notify); code != 1 {
		t.Fatalf("partial escalation exit = %d, want 1", code)
	}
	first := f.jobState(t, "absence-alarm")
	if len(first.PendingEscalations) != 1 || first.ConsecutiveFailures != 2 || notifications != 1 {
		t.Fatalf("first failure state=%+v notifications=%d", first, notifications)
	}
	original := first.PendingEscalations[0]

	next := now.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		next.Format(time.RFC3339)))
	host2, _ := hostAt(next, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	if code := run(f.args("--escalate-after", "2", "--give-up-after", "5"), &stdout, &stderr, host2, notify); code != 1 {
		t.Fatalf("pending transition exit = %d, want 1", code)
	}
	if notifications != 1 {
		t.Fatalf("partial-delivery banner repeated: %d notifications", notifications)
	}
	records := f.absenceRecords(t)
	if len(records) != 1 || fmt.Sprint(records[0]["reason"]) != original.Reason {
		t.Fatalf("durable retry did not preserve exact record: records=%#v original=%+v", records, original)
	}
	js := f.jobState(t, "absence-alarm")
	if len(js.PendingEscalations) != 0 || js.LastStatus != recoveryloop.StatusPending {
		t.Fatalf("durable retry did not survive transition into pending: %+v", js)
	}
}

// RL-35/RL-59: a snooze defers a rejected durable escalation without either
// delivering it over the operator's acknowledgement or losing the payload.
func TestCLI_SnoozeDefersPendingDurableEscalation(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.snooze, fmt.Sprintf(
		`[{"pulse":"absence-alarm-heartbeat","until":%q,"reason":"operator investigating"}]`,
		now.Add(time.Hour).Format(time.RFC3339)))
	pending := &absencealarm.JournalRecord{
		Time: now.Add(-10 * time.Minute), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "original durable escalation", Misses: 5,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			ConsecutiveFailures: 5, LastAction: recoveryloop.ActionKickstart,
			LastStatus: recoveryloop.StatusFailed, HumanNeeded: true,
			LastEscalated: now.Add(-5 * time.Minute), PendingEscalations: []absencealarm.JournalRecord{*pending},
		},
	}}); err != nil {
		t.Fatalf("save pending escalation state: %v", err)
	}
	notifications := 0
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--give-up-after", "5"), &stdout, &stderr, host, countingNotifier(&notifications)); code != 0 {
		t.Fatalf("snoozed retry exit = %d, want 0", code)
	}
	if len(f.absenceRecords(t)) != 0 || notifications != 0 {
		t.Fatalf("snoozed escalation was delivered: records=%d notifications=%d",
			len(f.absenceRecords(t)), notifications)
	}
	if js := f.jobState(t, "absence-alarm"); len(js.PendingEscalations) != 1 || js.LastStatus != recoveryloop.StatusSnoozed {
		t.Fatalf("snooze discarded pending escalation: %+v", js)
	}

	next := now.Add(10 * time.Minute)
	f.write(t, f.snooze, `[]`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		next.Format(time.RFC3339)))
	host2, _ := hostAt(next, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	if code := run(f.args("--give-up-after", "5"), &stdout, &stderr, host2, countingNotifier(&notifications)); code != 1 {
		t.Fatalf("unsnoozed retry exit = %d, want 1", code)
	}
	records := f.absenceRecords(t)
	if len(records) != 1 || fmt.Sprint(records[0]["reason"]) != pending.Reason {
		t.Fatalf("unsnoozed retry did not deliver exact pending record: %#v", records)
	}
	if notifications != 0 {
		t.Fatalf("unsnoozed durable retry repeated recent notification: %d", notifications)
	}
	if js := f.jobState(t, "absence-alarm"); len(js.PendingEscalations) != 0 {
		t.Fatalf("unsnoozed durable retry remained pending: %+v", js)
	}
}

// RL-58: verified recovery ends one standing outage, including its escalation
// rate-limit window. A later independent outage must be allowed to notify.
func TestCLI_VerifiedRecoveryResetsEscalationRateLimit(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_attempt_time":%q,`+
			`"last_action":"kickstart","last_status":"failed","human_needed":true,"last_escalated":%q}}}`,
		now.Add(-2*time.Hour).Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339)))
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--escalate-after", "1", "--give-up-after", "0"), &stdout, &stderr, host, nil); code != 0 {
		t.Fatalf("verified recovery exit = %d, want 0", code)
	}
	if got := f.jobState(t, "absence-alarm").LastEscalated; !got.IsZero() {
		t.Fatalf("verified recovery retained prior outage rate limit: %s", got)
	}

	next := now.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		next.Format(time.RFC3339)))
	host2, _ := hostAt(next, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	host2.LaunchctlKickstart = func(context.Context, string) error { return errors.New("new outage") }
	notifications := 0
	stdout.Reset()
	stderr.Reset()
	if code := run(f.args("--escalate-after", "1", "--give-up-after", "0"), &stdout, &stderr, host2, countingNotifier(&notifications)); code != 1 {
		t.Fatalf("new outage exit = %d, want 1", code)
	}
	if notifications != 1 {
		t.Fatalf("new outage notifications = %d, want 1", notifications)
	}
}

// RL-58/RL-59: when historical delivery debt outlives the incident, paying
// that debt must not carry the old incident's notification window into a new
// outage discovered later in the same tick.
func TestCLI_ResolvedDurableRetryDoesNotSuppressNewOutage(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	pending := &absencealarm.JournalRecord{
		Time: now.Add(-2 * time.Hour), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "resolved incident durable debt", Misses: 2,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			LastStatus: recoveryloop.StatusRecovered, LastAction: recoveryloop.ActionKickstart,
			LastEscalated: now.Add(-time.Hour), PendingEscalations: []absencealarm.JournalRecord{*pending},
		},
	}}); err != nil {
		t.Fatalf("save resolved pending escalation: %v", err)
	}
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	host.LaunchctlKickstart = func(context.Context, string) error { return errors.New("new outage") }
	notifications := 0
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--escalate-after", "1", "--give-up-after", "0"),
		&stdout, &stderr, host, countingNotifier(&notifications)); code != 1 {
		t.Fatalf("new outage exit = %d, want 1", code)
	}
	if notifications != 1 {
		t.Fatalf("historical retry suppressed new-outage notification: %d", notifications)
	}
	records := f.absenceRecords(t)
	if len(records) != 2 || fmt.Sprint(records[0]["reason"]) != pending.Reason {
		t.Fatalf("durable history/current records = %#v, want exact retry then new outage", records)
	}
}

// RL-59: if historical durable delivery still fails after the incident has
// recovered, retain the audit debt without emitting a contradictory stale
// "not recovered" desktop banner.
func TestCLI_ResolvedDurableRetryFailureDoesNotRenotify(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	pending := &absencealarm.JournalRecord{
		Time: now.Add(-2 * time.Hour), Kind: "recovery.human_needed",
		Pulse: "absence-alarm-heartbeat", Status: absencealarm.StatusAbsent,
		Reason: "resolved incident durable debt", Misses: 2,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"absence-alarm": {
			LastStatus: recoveryloop.StatusRecovered, LastAction: recoveryloop.ActionKickstart,
			PendingEscalations: []absencealarm.JournalRecord{*pending},
		},
	}}); err != nil {
		t.Fatalf("save resolved pending escalation: %v", err)
	}
	blocker := f.dir + "/resolved-blocker"
	f.write(t, blocker, "not a directory")
	args := append(f.args(), "--absence-journal", blocker+"/absence.jsonl")
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	notifications := 0
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr, host, countingNotifier(&notifications)); code != 0 {
		t.Fatalf("resolved job exit = %d, want 0", code)
	}
	if notifications != 0 {
		t.Fatalf("resolved incident emitted %d stale notifications", notifications)
	}
	if js := f.jobState(t, "absence-alarm"); len(js.PendingEscalations) != 1 {
		t.Fatal("failed historical journal retry discarded delivery debt")
	}
}

// RL-59: the durable payload belongs to persisted state, not to the current
// registry entry. Removing a job from config must not orphan its retry.
func TestCLI_RemovedJobStillRetriesPendingDurableEscalation(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"active-worker","launchd_label":"com.example.active-worker"}]}`)
	pending := &absencealarm.JournalRecord{
		Time: now.Add(-time.Hour), Kind: "recovery.human_needed",
		Pulse: "retired-worker-tick", Status: absencealarm.StatusAbsent,
		Reason: "orphan-resistant durable record", Misses: 3,
	}
	if err := recoveryloop.SaveState(f.state, recoveryloop.State{Jobs: map[string]recoveryloop.JobState{
		"retired-worker": {
			ConsecutiveFailures: 3, LastStatus: recoveryloop.StatusFailed,
			HumanNeeded: true, PendingEscalations: []absencealarm.JournalRecord{*pending},
		},
	}}); err != nil {
		t.Fatalf("save orphan pending escalation: %v", err)
	}
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.example.active-worker": {Loaded: true, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args(), &stdout, &stderr, host, nil); code != 0 {
		t.Fatalf("orphan retry exit = %d, want 0 after delivery", code)
	}
	records := f.absenceRecords(t)
	if len(records) != 1 || fmt.Sprint(records[0]["reason"]) != pending.Reason {
		t.Fatalf("removed job's durable record was orphaned: %#v", records)
	}
	if js := f.jobState(t, "retired-worker"); len(js.PendingEscalations) != 0 {
		t.Fatalf("removed job's delivered record remained pending: %+v", js)
	}
}

// A successful journal append with a rate-limited desktop notifier is fully
// delivered and must not emit a contradictory "still pending" diagnostic.
func TestCLI_RateLimitedNotificationDoesNotMisreportDurableDelivery(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_action":"kickstart",`+
			`"last_status":"failed","human_needed":true,"last_escalated":%q}}}`,
		now.Add(-time.Hour).Format(time.RFC3339)))
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	host.LaunchctlKickstart = func(context.Context, string) error { return errors.New("still unhealthy") }
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--escalate-after", "1", "--give-up-after", "0"), &stdout, &stderr, host, nil); code != 1 {
		t.Fatalf("standing failure exit = %d, want 1", code)
	}
	if strings.Contains(stderr.String(), "durable escalation") && strings.Contains(stderr.String(), "still pending") {
		t.Fatalf("successful durable delivery was logged as pending: %s", stderr.String())
	}
	if js := f.jobState(t, "absence-alarm"); len(js.PendingEscalations) != 0 {
		t.Fatalf("successful durable delivery remained pending: %+v", js)
	}
}

// RL-60: a missing binary is conclusive even when launchd cannot be
// re-observed after an apparently successful reinstall.
func TestCLI_PostActionLaunchdFailureCannotHideMissingBinary(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"worker","launchd_label":"com.example.worker",`+
		`"binary_path":"/missing/worker","install_cmd":["install-worker"],"pulse":"worker-tick"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"worker-tick","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	host, calls := hostAt(now, nil)
	host.FileExists = func(string) bool { return false }
	listCalls := 0
	host.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		listCalls++
		if listCalls == 1 {
			return map[string]recoveryloop.LaunchdJobInfo{
				"com.example.worker": {Loaded: true, Status: 0},
			}, nil
		}
		return nil, errors.New("launchctl: connection interrupted")
	}
	var stdout, stderr bytes.Buffer
	if code := run(f.args(), &stdout, &stderr, host, nil); code != 1 {
		t.Fatalf("failed reinstall exit = %d, want 1", code)
	}
	if listCalls != 1 || len(*calls) != 1 || (*calls)[0] != "run:install-worker" {
		t.Fatalf("observations=%d actions=%v, want one initial listing and one reinstall", listCalls, *calls)
	}
	js := f.jobState(t, "worker")
	if js.LastStatus != recoveryloop.StatusFailed || js.ConsecutiveFailures != 1 || !js.PendingDeadline.IsZero() {
		t.Fatalf("missing binary was hidden behind pending launchd observation: %+v", js)
	}
}

// RL-60 also applies to an attempt carried from an earlier tick: the loop may
// settle it from the independent file predicate without firing a second action.
func TestCLI_OpenVerificationLaunchdFailureCannotHideMissingBinary(t *testing.T) {
	f := newFixture(t)
	actionAt := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	now := actionAt.Add(10 * time.Minute)
	f.write(t, f.cfg, `{"jobs":[{"name":"worker","launchd_label":"com.example.worker",`+
		`"binary_path":"/missing/worker","install_cmd":["install-worker"],"pulse":"worker-tick"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"worker-tick","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"worker":{"last_status":"pending-verification","pending_action":"reinstall",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		actionAt.Format(time.RFC3339), actionAt.Add(30*time.Minute).Format(time.RFC3339)))
	host, calls := hostAt(now, nil)
	host.FileExists = func(string) bool { return false }
	host.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return nil, errors.New("launchctl: connection interrupted")
	}
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil); code != 1 {
		t.Fatalf("settled carried reinstall exit = %d, want 1", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("carried verification fired another remediation: %v", *calls)
	}
	js := f.jobState(t, "worker")
	if js.LastStatus != recoveryloop.StatusFailed || js.ConsecutiveFailures != 1 ||
		!js.PendingDeadline.IsZero() || js.PendingAction != "" {
		t.Fatalf("independent missing-binary failure stayed pending: %+v", js)
	}
}
