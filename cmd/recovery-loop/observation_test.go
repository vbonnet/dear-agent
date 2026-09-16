package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

// RL-41: a post-action launchd listing that cannot be obtained is missing
// evidence, not negative evidence.
//
// Falling back to the pre-action snapshot hands VerifyRecovery the very defect
// that triggered the action, so a transient listing failure would score the
// remediation a failure and count toward escalation without the current host
// ever being observed.
func TestCLI_FailedPostActionListingIsPendingAndUnavailable(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, `{"jobs":[{"name":"absence-alarm","launchd_label":"com.dear-agent.absence-alarm",`+
		`"plist_path":"/tmp/com.dear-agent.absence-alarm.plist","pulse":"absence-alarm-heartbeat"}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))

	// The job is unloaded, so the pre-action snapshot says "not loaded". The
	// bootstrap succeeds; the verification listing then fails.
	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{})
	var listCalls int
	host.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		listCalls++
		if listCalls == 1 {
			return map[string]recoveryloop.LaunchdJobInfo{}, nil
		}
		return nil, errors.New("launchctl: connection interrupted")
	}

	var stdout, stderr bytes.Buffer
	code := run(f.args(), &stdout, &stderr, host, nil)

	if listCalls < 2 {
		t.Fatalf("expected a post-action re-list, got %d listing calls", listCalls)
	}
	js := f.jobState(t, "absence-alarm")
	if js.LastStatus == recoveryloop.StatusFailed {
		t.Errorf("scored a failure from the pre-action snapshot after the re-list failed:\n%s", stdout.String())
	}
	if js.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d; an unobservable host must not count against the job",
			js.ConsecutiveFailures)
	}
	if js.LastStatus != recoveryloop.StatusPending {
		t.Errorf("last_status = %q, want pending", js.LastStatus)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 while required post-action observation is unavailable", code)
	}
	var hb recoveryloop.Heartbeat
	raw, err := os.ReadFile(f.heartbeat)
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	if err := json.Unmarshal(raw, &hb); err != nil {
		t.Fatalf("parse heartbeat: %v", err)
	}
	if hb.Pending != 1 || hb.Unavailable != 1 || len(hb.Results) != 1 ||
		hb.Results[0].Status != recoveryloop.StatusPending {
		t.Errorf("heartbeat did not preserve pending while marking the observation unavailable: %+v", hb)
	}
}

// RL-12/RL-41: a launchd failure on the tick after an action must preserve the
// open verification, but the tick itself is not green because a required
// structural observation could not be made.
func TestCLI_OpenVerificationLaunchdFailureStaysPendingAndExitsUnavailable(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	now := t0.Add(10 * time.Minute)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_attempt_time":%q,`+
			`"last_action":"kickstart","last_status":"pending-verification","pending_action":"kickstart",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		t0.Format(time.RFC3339), t0.Format(time.RFC3339), t0.Add(30*time.Minute).Format(time.RFC3339)))

	host, calls := hostAt(now, nil)
	host.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return nil, errors.New("launchctl: connection interrupted")
	}

	var stdout, stderr bytes.Buffer
	code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)

	if code != 1 {
		t.Errorf("exit = %d, want 1 while verification observation is unavailable", code)
	}
	if len(*calls) != 0 {
		t.Errorf("open verification fired a second remediation: %v", *calls)
	}
	js := f.jobState(t, "absence-alarm")
	if js.LastStatus != recoveryloop.StatusPending || js.ConsecutiveFailures != 1 ||
		!js.PendingSince.Equal(t0) || !js.PendingDeadline.Equal(t0.Add(30*time.Minute)) {
		t.Errorf("unavailable observation changed the pending lifecycle: %+v", js)
	}
	if len(f.recoveryRecords(t)) != 0 {
		t.Error("holding an existing verification appended a new recovery attempt")
	}
	var hb recoveryloop.Heartbeat
	raw, err := os.ReadFile(f.heartbeat)
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	if err := json.Unmarshal(raw, &hb); err != nil {
		t.Fatalf("parse heartbeat: %v", err)
	}
	if hb.Pending != 1 || hb.Unavailable != 1 || len(hb.Results) != 1 ||
		hb.Results[0].Status != recoveryloop.StatusPending {
		t.Errorf("heartbeat did not expose pending plus unavailable: %+v", hb)
	}
}

// RL-12/RL-34/RL-54: an open verification cannot turn invalid pulse truth into a
// green pending tick or, after the deadline, into a fabricated job failure.
// Missing/stale source data is a negative owner observation under RL-54, but
// malformed data remains unavailable. The attempt stays pending and loud.
func TestCLI_OpenVerificationInvalidPulseStaysPendingAndUnavailable(t *testing.T) {
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		now  time.Time
	}{
		{name: "before deadline", now: t0.Add(10 * time.Minute)},
		{name: "after deadline", now: t0.Add(40 * time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.write(t, f.cfg, absenceAlarmJob)
			f.write(t, f.absHB, `{not-json`)
			f.write(t, f.state, fmt.Sprintf(
				`{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_action":"kickstart",`+
					`"last_status":"pending-verification","pending_action":"kickstart",`+
					`"pending_since":%q,"pending_deadline":%q}}}`,
				t0.Format(time.RFC3339), t0.Add(30*time.Minute).Format(time.RFC3339)))

			host, calls := hostAt(tc.now, map[string]recoveryloop.LaunchdJobInfo{
				"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
			})
			var stdout, stderr bytes.Buffer
			code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)

			if code != 1 {
				t.Errorf("exit = %d, want 1 while pulse observation is unavailable", code)
			}
			if len(*calls) != 0 {
				t.Errorf("invalid pulse truth fired a second remediation: %v", *calls)
			}
			js := f.jobState(t, "absence-alarm")
			if js.LastStatus != recoveryloop.StatusPending || js.ConsecutiveFailures != 1 ||
				!js.PendingSince.Equal(t0) || !js.PendingDeadline.Equal(t0.Add(30*time.Minute)) ||
				!js.MissedVerificationDeadline.IsZero() {
				t.Errorf("invalid pulse truth changed the pending lifecycle: %+v", js)
			}
			if len(f.recoveryRecords(t)) != 0 {
				t.Error("invalid pulse truth fabricated a recovery outcome")
			}
			var hb recoveryloop.Heartbeat
			raw, err := os.ReadFile(f.heartbeat)
			if err != nil {
				t.Fatalf("read heartbeat: %v", err)
			}
			if err := json.Unmarshal(raw, &hb); err != nil {
				t.Fatalf("parse heartbeat: %v", err)
			}
			if hb.Pending != 1 || hb.Unavailable != 1 || hb.Failed != 0 ||
				len(hb.Results) != 1 || !strings.Contains(hb.Results[0].Reason, "pulse") {
				t.Errorf("heartbeat did not expose pending plus unavailable pulse truth: %+v", hb)
			}
		})
	}
}

// RL-48: a failed initial launchd observation is unknown state, not proof that
// every configured service is unloaded. The loop must not bootstrap from an
// empty map returned alongside an error, and the tick itself must not read
// green when a critical observation could not be made.
func TestCLI_InitialLaunchdListingFailureDoesNotMutate(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":1,"human_needed":false,"last_status":"failed"}}}`)

	host, calls := hostAt(now, nil)
	host.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return nil, errors.New("launchctl: connection interrupted")
	}

	var stdout, stderr bytes.Buffer
	code := run(f.args(), &stdout, &stderr, host, nil)

	if len(*calls) != 0 {
		t.Fatalf("mutated launchd from an unavailable listing: %v", *calls)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1 while launchd state was unavailable; stdout:\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "observation unavailable") {
		t.Errorf("report did not name the observation failure:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "healthy") || strings.Contains(stdout.String(), "recovered") {
		t.Errorf("unobserved launchd state was reported green:\n%s", stdout.String())
	}
	if _, err := os.Stat(f.journal); !os.IsNotExist(err) {
		t.Errorf("initial observation failure wrote a recovery journal entry: %v", err)
	}
	js := f.jobState(t, "absence-alarm")
	if js.ConsecutiveFailures != 1 || js.HumanNeeded {
		t.Errorf("observation failure mutated standing failure state: %+v", js)
	}
	if !js.PendingDeadline.IsZero() || js.PendingAction != "" {
		t.Errorf("observation failure invented a pending action: %+v", js)
	}
	var hb recoveryloop.Heartbeat
	raw, err := os.ReadFile(f.heartbeat)
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	if err := json.Unmarshal(raw, &hb); err != nil {
		t.Fatalf("parse heartbeat: %v", err)
	}
	if hb.Unavailable != 1 || len(hb.Results) != 1 ||
		hb.Results[0].Status != recoveryloop.StatusUnavailable ||
		hb.Results[0].Action != recoveryloop.ActionNone {
		t.Errorf("heartbeat does not expose the unavailable no-action result: %+v", hb)
	}
}

// RL-32: the freshness limit must be a real limit. A nonpositive value skipped
// every heartbeat check, including the missing and future tick_time cases, so
// evidence from any point in history stayed authoritative.
func TestCLI_NonPositiveMaxHeartbeatAgeRejected(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	for _, v := range []string{"0", "-1h"} {
		var stdout, stderr bytes.Buffer
		code := run(f.args("--max-heartbeat-age", v), &stdout, &stderr, mustHost(now), nil)
		if code != 2 {
			t.Errorf("--max-heartbeat-age=%s exited %d, want 2 (usage error)", v, code)
		}
		if !strings.Contains(stderr.String(), "max-heartbeat-age") {
			t.Errorf("--max-heartbeat-age=%s gave no usage message: %s", v, stderr.String())
		}
	}
}

// RL-44: a dry run that plans remediation must not summarise as OK. The same
// report saying "action bootstrap" and "Status: OK" is a report contradicting
// itself, which is exactly what dry-run exists to prevent.
func TestCLI_DryRunWithPlannedWorkIsNotOK(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{})
	var stdout, stderr bytes.Buffer
	run(f.args("--dry-run"), &stdout, &stderr, host, nil)

	out := stdout.String()
	if !strings.Contains(out, "planned, dry-run") {
		t.Fatalf("expected a planned remediation in the report:\n%s", out)
	}
	if strings.Contains(out, "Status: OK") {
		t.Errorf("dry-run reported a planned remediation and Status: OK in the same report:\n%s", out)
	}
}

// RL-39 on the clearing path: a failed attempt must not become a recovery just
// because a long-window pulse from before the action is still inside its
// freshness window.
//
// The pending path rejects pre-action evidence and eventually counts a deadline
// failure. On the next tick PlanJob sees that same unchanged pulse, returns
// HEALTHY, and this path would record RECOVERED and clear the counter, so the
// failure it just recorded evaporates without anything new being observed.
func TestCLI_StalePulseDoesNotClearAFailedJob(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	// A failed attempt at 08:00, already counted.
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":2,"human_needed":true,`+
			`"last_attempt_time":%q,"last_action":"kickstart","last_status":"failed"}}}`,
		t0.Format(time.RFC3339)))

	// A later tick. The pulse is "present", but it was observed BEFORE the
	// remediation and has simply not aged out of its window yet.
	t1 := t0.Add(20 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		t1.Format(time.RFC3339), t0.Add(-30*time.Minute).Format(time.RFC3339)))

	host, _ := hostAt(t1, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	run(f.args(), &stdout, &stderr, host, nil)

	if strings.Contains(stdout.String(), "recovered") {
		t.Errorf("cleared a failed job using a pulse observed before its last action:\n%s", stdout.String())
	}
	if js := f.jobState(t, "absence-alarm"); js.ConsecutiveFailures == 0 {
		t.Error("reset the failure count without observing anything new")
	}
}

// RL-47: evidence materially ahead of the current tick is a clock anomaly,
// not post-attempt proof. absence-alarm deliberately tolerates up to five
// minutes of source skew, so the recovery loop must apply its stricter proof
// boundary on every path that can clear a failure, not only VerifyRecovery.
func TestCLI_FuturePulseDoesNotClearAFailedJob(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":2,"human_needed":true,`+
			`"last_attempt_time":%q,"last_action":"kickstart","last_status":"failed"}}}`,
		t0.Format(time.RFC3339)))

	now := t0.Add(time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Add(3*time.Minute).Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	run(f.args(), &stdout, &stderr, host, nil)

	if strings.Contains(stdout.String(), "recovered") {
		t.Errorf("future-dated pulse cleared a failed job:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "future") {
		t.Errorf("future evidence was rejected without an operator-visible reason:\n%s", stdout.String())
	}
	js := f.jobState(t, "absence-alarm")
	if js.ConsecutiveFailures != 2 || !js.HumanNeeded {
		t.Errorf("future evidence reset standing failure state: %+v", js)
	}
}

// RL-47 also applies to older persisted state that has a failure count but no
// attempt timestamp. A missing boundary weakens the ordering proof; it must
// not disable the independent future-clock check.
func TestCLI_FuturePulseDoesNotClearFailureWithoutAttemptTimestamp(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_status":"failed"}}}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Add(3*time.Minute).Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	run(f.args(), &stdout, &stderr, host, nil)

	if strings.Contains(stdout.String(), "recovered") || !strings.Contains(stdout.String(), "future") {
		t.Errorf("future evidence without an attempt timestamp was not rejected:\n%s", stdout.String())
	}
	if js := f.jobState(t, "absence-alarm"); js.ConsecutiveFailures != 1 {
		t.Errorf("future evidence reset legacy failure state: %+v", js)
	}
}

// RL-39: a present status with no evidence timestamp cannot establish that the
// observation followed a failed attempt, even when legacy state also lacks an
// attempt timestamp.
func TestCLI_UndatedPulseDoesNotClearFailureWithoutAttemptTimestamp(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_status":"failed"}}}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present"}]}`,
		now.Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args(), &stdout, &stderr, host, nil)

	if code != 1 || strings.Contains(stdout.String(), "recovered") ||
		!strings.Contains(stdout.String(), "no evidence timestamp") {
		t.Errorf("undated evidence cleared or hid a standing failure; exit=%d stdout:\n%s", code, stdout.String())
	}
	if js := f.jobState(t, "absence-alarm"); js.ConsecutiveFailures != 1 ||
		js.LastStatus != recoveryloop.StatusUnavailable {
		t.Errorf("undated evidence mutated legacy failure state: %+v", js)
	}
}

// RL-30/RL-41: a launchctl hiccup must not postpone a failure the heartbeat already settles.
// For a pulse-backed job whose grace window expired with the pulse still
// absent, the verdict does not depend on the listing at all, and holding it
// every time launchctl stumbles would defer a real escalation indefinitely.
func TestCLI_ExpiredAbsentPulseSettlesDespiteLaunchdError(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	now := t0.Add(2 * time.Hour)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":1,"pending_action":"kickstart",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		t0.Format(time.RFC3339), t0.Add(30*time.Minute).Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{})
	host.LaunchdList = func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
		return nil, errors.New("launchctl: connection interrupted")
	}

	var stdout, stderr bytes.Buffer
	run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)

	js := f.jobState(t, "absence-alarm")
	if js.LastStatus == recoveryloop.StatusPending {
		t.Errorf("held a verdict the heartbeat already settled:\n%s", stdout.String())
	}
	if js.ConsecutiveFailures < 2 {
		t.Errorf("consecutive_failures = %d; the expired grace window with an absent pulse is a failure",
			js.ConsecutiveFailures)
	}
	if !strings.Contains(stdout.String(), `pulse "absence-alarm-heartbeat" did not return`) {
		t.Errorf("expired pulse verdict was not reported:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "not loaded") {
		t.Errorf("launchd observation failure was fabricated into an unloaded diagnosis:\n%s", stdout.String())
	}
	recs := f.recoveryRecords(t)
	if len(recs) != 1 || !strings.Contains(recs[0].Reason, "did not return") ||
		strings.Contains(recs[0].Reason, "not loaded") {
		t.Errorf("durable failure reason did not preserve the conclusive pulse verdict: %+v", recs)
	}
}

// RL-49: a pulse that returns after the grace deadline still proves the
// condition is currently clear, but the missed recovery target must remain
// visible in the result instead of disappearing from the journal narrative.
func TestCLI_LatePulseIsRecoveredAndReportedLate(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	now := t0.Add(40 * time.Minute)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), t0.Add(35*time.Minute).Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":1,"pending_action":"kickstart",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		t0.Format(time.RFC3339), t0.Add(30*time.Minute).Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)

	if code != 0 {
		t.Errorf("exit = %d, want 0 after observing the condition clear; stdout:\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "recovered") || !strings.Contains(stdout.String(), "after the verification deadline") {
		t.Errorf("late recovery did not report both recovery and lateness:\n%s", stdout.String())
	}
	if js := f.jobState(t, "absence-alarm"); js.LastStatus != recoveryloop.StatusRecovered {
		t.Errorf("last_status = %q, want recovered", js.LastStatus)
	}
	recs := f.recoveryRecords(t)
	if len(recs) != 1 || recs[0].Kind != "recovery.verified" ||
		!strings.Contains(recs[0].Reason, "after the verification deadline") {
		t.Errorf("durable recovery record lost the late-arrival evidence: %+v", recs)
	}
}

// RL-49 also applies when one tick first records the missed deadline and the
// following tick observes the pulse return. Clearing PendingDeadline must not
// erase the provenance needed to report that recovery was late.
func TestCLI_PulseReturningAfterCountedDeadlineFailureRemainsReportedLate(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	deadline := t0.Add(30 * time.Minute)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"pending_action":"kickstart",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		t0.Format(time.RFC3339), deadline.Format(time.RFC3339)))

	failedAt := deadline.Add(time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		failedAt.Format(time.RFC3339)))
	hostFailed, _ := hostAt(failedAt, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, hostFailed, nil); code != 1 {
		t.Fatalf("deadline-failure tick exited %d, want 1; stdout:\n%s", code, stdout.String())
	}
	failed := f.jobState(t, "absence-alarm")
	if !failed.MissedVerificationDeadline.Equal(deadline) || failed.LastStatus != recoveryloop.StatusFailed {
		t.Fatalf("deadline provenance was not retained after failure: %+v", failed)
	}

	recoveredAt := t0.Add(40 * time.Minute)
	evidenceAt := t0.Add(35 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		recoveredAt.Format(time.RFC3339), evidenceAt.Format(time.RFC3339)))
	hostRecovered, _ := hostAt(recoveredAt, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, hostRecovered, nil)
	if code != 0 || !strings.Contains(stdout.String(), "after the verification deadline") {
		t.Errorf("post-failure recovery lost its late attribution; exit=%d stdout:\n%s", code, stdout.String())
	}
	recs := f.recoveryRecords(t)
	if len(recs) != 2 || recs[1].Kind != "recovery.verified" ||
		!strings.Contains(recs[1].Reason, "after the verification deadline") {
		t.Errorf("durable post-failure recovery lost deadline provenance: %+v", recs)
	}
}

// RL-49: a structural verdict can also settle an already-expired pending
// attempt. The latest non-superseded deadline remains the lateness boundary if
// a later pulse observation proves the condition recovered.
func TestCLI_StructuralFailureAfterDeadlineRetainsLateRecoveryBoundary(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	deadline := t0.Add(30 * time.Minute)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"pending_action":"kickstart",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		t0.Format(time.RFC3339), deadline.Format(time.RFC3339)))

	failedAt := t0.Add(40 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		failedAt.Format(time.RFC3339), failedAt.Format(time.RFC3339)))
	hostFailed, _ := hostAt(failedAt, map[string]recoveryloop.LaunchdJobInfo{})
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, hostFailed, nil); code != 1 {
		t.Fatalf("structural-failure tick exited %d, want 1; stdout:\n%s", code, stdout.String())
	}
	failed := f.jobState(t, "absence-alarm")
	if !failed.MissedVerificationDeadline.Equal(deadline) || failed.LastStatus != recoveryloop.StatusFailed {
		t.Fatalf("expired structural failure lost deadline provenance: %+v", failed)
	}

	recoveredAt := t0.Add(50 * time.Minute)
	evidenceAt := t0.Add(45 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		recoveredAt.Format(time.RFC3339), evidenceAt.Format(time.RFC3339)))
	hostRecovered, _ := hostAt(recoveredAt, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, hostRecovered, nil)
	if code != 0 || !strings.Contains(stdout.String(), "after the verification deadline") {
		t.Errorf("recovery after expired structural failure lost late attribution; exit=%d stdout:\n%s",
			code, stdout.String())
	}
}

// A fresh remediation supersedes deadline provenance from the prior attempt.
// If the new attempt fails before opening its own verification, a later pulse
// must not be annotated against the older attempt's deadline.
func TestCLI_NewFailedAttemptClearsSupersededDeadlineProvenance(t *testing.T) {
	f := newFixture(t)
	oldDeadline := time.Date(2026, 9, 16, 7, 30, 0, 0, time.UTC)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"consecutive_failures":1,"last_action":"kickstart",`+
			`"last_status":"failed","missed_verification_deadline":%q}}}`,
		oldDeadline.Format(time.RFC3339)))
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	hostFailed, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	hostFailed.LaunchctlKickstart = func(context.Context, string) error {
		return errors.New("kickstart failed")
	}
	var stdout, stderr bytes.Buffer
	if code := run(f.args("--escalate-after", "5"), &stdout, &stderr, hostFailed, nil); code != 1 {
		t.Fatalf("superseding failed attempt exited %d, want 1", code)
	}
	if failed := f.jobState(t, "absence-alarm"); !failed.MissedVerificationDeadline.IsZero() {
		t.Fatalf("new attempt retained superseded deadline provenance: %+v", failed)
	}

	recoveredAt := now.Add(10 * time.Minute)
	evidenceAt := now.Add(5 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		recoveredAt.Format(time.RFC3339), evidenceAt.Format(time.RFC3339)))
	hostRecovered, _ := hostAt(recoveredAt, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	code := run(f.args("--escalate-after", "5"), &stdout, &stderr, hostRecovered, nil)
	if code != 0 || !strings.Contains(stdout.String(), "recovered") {
		t.Errorf("fresh evidence did not clear the superseding failure; exit=%d stdout:\n%s", code, stdout.String())
	}
	if strings.Contains(stdout.String(), "after the verification deadline") {
		t.Errorf("recovery was annotated against a superseded deadline:\n%s", stdout.String())
	}
}

// A structural pulse is not recovery evidence (RL-45), so its timestamp must
// not be described as a late recovery signal when structural checks alone
// establish the outcome.
func TestCLI_StructuralRecoveryDoesNotAttributeLatePulse(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	now := t0.Add(40 * time.Minute)
	f.write(t, f.cfg, `{"jobs":[{"name":"mergeloop","launchd_label":"com.dear-agent.mergeloop",`+
		`"pulse":"mergeloop-loaded","pulse_is_structural":true}]}`)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"mergeloop-loaded","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), t0.Add(35*time.Minute).Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"mergeloop":{"pending_action":"kickstart",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		t0.Format(time.RFC3339), t0.Add(30*time.Minute).Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.mergeloop": {Loaded: true, PID: 1, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)

	if code != 0 || !strings.Contains(stdout.String(), "recovered") {
		t.Errorf("structural recovery was not recorded; exit=%d stdout:\n%s", code, stdout.String())
	}
	if strings.Contains(stdout.String(), "after the verification deadline") {
		t.Errorf("structural recovery was falsely attributed to pulse timing:\n%s", stdout.String())
	}
}

// RL-50: a persisted deadline from a clock that jumped forward must not hold
// verification open for hours or days after the clock is corrected. Capping
// the remaining wait does not move PendingSince, so present evidence from
// before the original action still cannot manufacture a recovery.
func TestCLI_FuturePendingDeadlineIsBoundedAfterClockCorrection(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	futureAction := now.Add(24 * time.Hour)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		now.Format(time.RFC3339), now.Format(time.RFC3339)))
	f.write(t, f.state, fmt.Sprintf(
		`{"jobs":{"absence-alarm":{"pending_action":"kickstart",`+
			`"pending_since":%q,"pending_deadline":%q}}}`,
		futureAction.Format(time.RFC3339), futureAction.Add(30*time.Minute).Format(time.RFC3339)))

	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)

	js := f.jobState(t, "absence-alarm")
	wantDeadline := now.Add(30 * time.Minute)
	if !js.PendingDeadline.Equal(wantDeadline) {
		t.Errorf("pending_deadline = %s, want bounded deadline %s", js.PendingDeadline, wantDeadline)
	}
	if js.LastStatus == recoveryloop.StatusRecovered {
		t.Errorf("clock correction accepted evidence from before the original action:\n%s", stdout.String())
	}

	// The bounded deadline must be a liveness bound, not merely a rewritten
	// timestamp. One tick after it expires, the same pre-action evidence
	// settles as a failure without firing a second remediation.
	later := wantDeadline.Add(time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		later.Format(time.RFC3339), now.Format(time.RFC3339)))
	hostLater, laterCalls := hostAt(later, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, hostLater, nil)
	settled := f.jobState(t, "absence-alarm")
	if code != 1 || settled.LastStatus != recoveryloop.StatusFailed ||
		settled.ConsecutiveFailures != 1 || !settled.PendingDeadline.IsZero() {
		t.Errorf("bounded verification did not settle after its deadline; exit=%d state=%+v stdout:\n%s",
			code, settled, stdout.String())
	}
	if !settled.LastAttemptTime.Equal(later) || settled.LastAttemptTime.After(later) {
		t.Errorf("clock-corrected settlement retained future proof boundary %s, want %s",
			settled.LastAttemptTime, later)
	}
	if len(*calls) != 0 || len(*laterCalls) != 0 {
		t.Errorf("clock-correction settlement fired another remediation: first=%v later=%v", *calls, *laterCalls)
	}

	// Once the quarantined attempt has settled, evidence newer than that
	// settlement must be usable even though it still predates the original
	// jumped-forward timestamp. Otherwise the corrected clock blocks recovery
	// until wall time catches up a day later.
	recoveryTick := later.Add(10 * time.Minute)
	recoveryEvidence := later.Add(5 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present","evidence":%q}]}`,
		recoveryTick.Format(time.RFC3339), recoveryEvidence.Format(time.RFC3339)))
	hostRecovered, recoveryCalls := hostAt(recoveryTick, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, Status: 0},
	})
	stdout.Reset()
	stderr.Reset()
	if code := run(f.args("--verify-grace", "30m"), &stdout, &stderr, hostRecovered, nil); code != 0 {
		t.Fatalf("post-correction recovery exit = %d, want 0; stdout:\n%s", code, stdout.String())
	}
	if !recoveryEvidence.Before(futureAction) {
		t.Fatal("test evidence no longer predates the original future boundary")
	}
	if len(*recoveryCalls) != 0 {
		t.Fatalf("post-correction recovery fired another remediation: %v", *recoveryCalls)
	}
	if js := f.jobState(t, "absence-alarm"); js.LastStatus != recoveryloop.StatusRecovered ||
		js.ConsecutiveFailures != 0 {
		t.Fatalf("post-correction evidence did not recover the job: %+v", js)
	}
}
