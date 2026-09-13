package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/recoveryloop"
)

// fixture builds the file layout one tick needs.
type fixture struct {
	dir       string
	cfg       string
	state     string
	journal   string
	absJrnl   string
	absHB     string
	absState  string
	heartbeat string
	snooze    string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := t.TempDir()
	return &fixture{
		dir:       d,
		cfg:       filepath.Join(d, "jobs.json"),
		state:     filepath.Join(d, "state.json"),
		journal:   filepath.Join(d, "recovery.jsonl"),
		absJrnl:   filepath.Join(d, "absence-alarm.jsonl"),
		absHB:     filepath.Join(d, "absence.heartbeat.json"),
		absState:  filepath.Join(d, "absence-state.json"),
		heartbeat: filepath.Join(d, "recovery.heartbeat.json"),
		snooze:    filepath.Join(d, "snooze.json"),
	}
}

func (f *fixture) write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func (f *fixture) args(extra ...string) []string {
	return append([]string{
		"--config", f.cfg,
		"--state", f.state,
		"--journal", f.journal,
		"--absence-journal", f.absJrnl,
		"--absence-heartbeat", f.absHB,
		"--absence-state", f.absState,
		"--heartbeat", f.heartbeat,
		"--snooze", f.snooze,
	}, extra...)
}

func (f *fixture) jobState(t *testing.T, name string) recoveryloop.JobState {
	t.Helper()
	st, err := recoveryloop.LoadState(f.state)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	return st.Jobs[name]
}

// absenceRecords returns every record appended to the absence-alarm journal.
func (f *fixture) absenceRecords(t *testing.T) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(f.absJrnl)
	if err != nil {
		return nil
	}
	var out []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("parse absence record %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func hostAt(now time.Time, launchd map[string]recoveryloop.LaunchdJobInfo) (recoveryloop.HostOps, *[]string) {
	calls := &[]string{}
	return recoveryloop.HostOps{
		Now:        func() time.Time { return now },
		FileExists: func(string) bool { return true },
		LaunchdList: func(context.Context) (map[string]recoveryloop.LaunchdJobInfo, error) {
			return launchd, nil
		},
		LaunchctlBootout:   func(_ context.Context, l string) error { *calls = append(*calls, "bootout:"+l); return nil },
		LaunchctlBootstrap: func(_ context.Context, p string) error { *calls = append(*calls, "bootstrap:"+p); return nil },
		// The defect under test: kickstart always succeeds. launchd accepts
		// the request; that says nothing about the job producing its pulse.
		LaunchctlKickstart: func(_ context.Context, l string) error { *calls = append(*calls, "kickstart:"+l); return nil },
		RunCommand: func(_ context.Context, a []string) (int, string, error) {
			*calls = append(*calls, "run:"+strings.Join(a, " "))
			return 0, "ok", nil
		},
	}, calls
}

const absenceAlarmJob = `{"jobs":[{"name":"absence-alarm","launchd_label":"com.dear-agent.absence-alarm","pulse":"absence-alarm-heartbeat"}]}`

// RL-24 (live incident replay): absence-alarm exits 1 by design while any pulse
// is absent. Its own heartbeat pulse is present, so it is alive and must not be
// restarted. This is the loop that ran 555 consecutive "recoveries" on a
// healthy monitor.
func TestCLI_PresentPulse_NoKickstartOnAlarmExitCode(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present"},{"name":"mergeloop-tick","status":"absent"}]}`,
		now.Format(time.RFC3339)))

	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args(), &stdout, &stderr, host, nil)

	if len(*calls) != 0 {
		t.Errorf("took remediation action %v on a job whose pulse is present", *calls)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0. stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "healthy") {
		t.Errorf("stdout does not report healthy: %s", stdout.String())
	}
}

// RL-26: a kickstart whose command succeeded while the pulse stays absent must
// NOT be reported as recovered, and must not reset the failure counter.
func TestCLI_CommandSucceededButPulseStillAbsent_NotRecovered(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)

	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	var stdout, stderr bytes.Buffer
	run(f.args(), &stdout, &stderr, host, nil)

	if len(*calls) == 0 {
		t.Fatal("expected a kickstart for an absent pulse")
	}
	if strings.Contains(stdout.String(), "recovered") {
		t.Errorf("reported RECOVERED while the pulse is still absent:\n%s", stdout.String())
	}
	js := f.jobState(t, "absence-alarm")
	if js.LastStatus == recoveryloop.StatusRecovered {
		t.Errorf("persisted last_status=recovered while the pulse is still absent")
	}
}

// RL-26/RL-08: once the verification grace period lapses with the pulse still
// absent, the recovery is a failure and the counter must climb. Under the
// defect this counter was pinned at 0 forever.
func TestCLI_GraceLapsedStillAbsent_IncrementsFailures(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)
	launchd := map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	}

	// Three ticks an hour apart, pulse absent throughout.
	for i, at := range []time.Time{t0, t0.Add(time.Hour), t0.Add(2 * time.Hour)} {
		f.write(t, f.absHB, fmt.Sprintf(
			`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
			at.Format(time.RFC3339)))
		host, _ := hostAt(at, launchd)
		var stdout, stderr bytes.Buffer
		run(f.args("--verify-grace", "30m"), &stdout, &stderr, host, nil)
		t.Logf("tick %d:\n%s", i, stdout.String())
	}

	js := f.jobState(t, "absence-alarm")
	if js.ConsecutiveFailures == 0 {
		t.Fatal("consecutive_failures is still 0 after three ticks with the pulse continuously absent")
	}
}

// RL-30: after N consecutive unverified recoveries, escalate LOUDLY onto the
// absence-alarm path, naming the pulse and how long it has been absent.
func TestCLI_EscalatesToAbsencePathAfterN(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)
	launchd := map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	}

	var lastCode int
	for _, at := range []time.Time{t0, t0.Add(time.Hour), t0.Add(2 * time.Hour), t0.Add(3 * time.Hour)} {
		f.write(t, f.absHB, fmt.Sprintf(
			`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
			at.Format(time.RFC3339)))
		host, _ := hostAt(at, launchd)
		var stdout, stderr bytes.Buffer
		lastCode = run(f.args("--verify-grace", "30m", "--escalate-after", "2"), &stdout, &stderr, host, nil)
	}

	recs := f.absenceRecords(t)
	if len(recs) == 0 {
		t.Fatal("nothing was written to the absence-alarm journal: the escalation never reached a human-facing sink")
	}
	var found map[string]any
	for _, r := range recs {
		if r["kind"] == "recovery.human_needed" {
			found = r
		}
	}
	if found == nil {
		t.Fatalf("no recovery.human_needed record on the absence path; got %v", recs)
	}
	if found["pulse"] != "absence-alarm-heartbeat" {
		t.Errorf("escalation does not name the pulse: %v", found)
	}
	reason, _ := found["reason"].(string)
	if !strings.Contains(reason, "absent for") {
		t.Errorf("escalation does not say how long the pulse has been absent: %q", reason)
	}
	if lastCode != 1 {
		t.Errorf("exit = %d, want 1 while a job needs a human", lastCode)
	}
	if js := f.jobState(t, "absence-alarm"); !js.HumanNeeded {
		t.Error("human_needed was not persisted")
	}
}

// RL-27: when the pulse comes back, the recovery is verified and the failure
// counter resets.
func TestCLI_PulseReturns_VerifiedRecovered(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	launchd := map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	}

	// Tick 1: absent -> kickstart, pending.
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		t0.Format(time.RFC3339)))
	host1, _ := hostAt(t0, launchd)
	var o1, e1 bytes.Buffer
	run(f.args("--verify-grace", "30m"), &o1, &e1, host1, nil)

	// Tick 2: the pulse is back.
	t1 := t0.Add(10 * time.Minute)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present"}]}`,
		t1.Format(time.RFC3339)))
	host2, calls2 := hostAt(t1, launchd)
	var o2, e2 bytes.Buffer
	code := run(f.args("--verify-grace", "30m"), &o2, &e2, host2, nil)

	if code != 0 {
		t.Errorf("exit = %d, want 0 once the pulse returned. stdout:\n%s", code, o2.String())
	}
	if len(*calls2) != 0 {
		t.Errorf("kept acting after the pulse returned: %v", *calls2)
	}
	js := f.jobState(t, "absence-alarm")
	if js.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0 after a verified recovery", js.ConsecutiveFailures)
	}
}

// countingNotifier records every escalation banner dispatched.
func countingNotifier(n *int) notifier {
	return func(context.Context, string, string) error { *n++; return nil }
}

// absenceKinds returns the Kind of each absence-alarm record, in order.
func (f *fixture) absenceKinds(t *testing.T) []string {
	t.Helper()
	var kinds []string
	for _, r := range f.absenceRecords(t) {
		kinds = append(kinds, fmt.Sprint(r["kind"]))
	}
	return kinds
}

// RL-29: structural health is not pulse health.
//
// When pulse truth is unavailable, PlanJob falls through to "job is healthy" on
// the structural checks alone. Converting that into a verified RECOVERY for a
// job that has a pulse reintroduces the exact defect this branch removes: it
// reports that a condition cleared without ever observing the condition.
func TestCLI_StructuralHealthIsNotVerifiedRecovery(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	// No heartbeat file at all: no pulse truth exists.
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":3,"human_needed":true}}}`)

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args(), &stdout, &stderr, host, nil)

	if strings.Contains(stdout.String(), "recovered") {
		t.Errorf("claimed RECOVERED for a pulsed job with no pulse evidence:\n%s", stdout.String())
	}
	js := f.jobState(t, "absence-alarm")
	if js.ConsecutiveFailures == 0 {
		t.Error("reset the failure counter on structural evidence alone")
	}
	if code == 0 && js.HumanNeeded {
		t.Errorf("exit 0 while the job still needs a human. stderr: %s", stderr.String())
	}
}

// RL-32: pulse truth older than the maximum heartbeat age must not be used.
// A stale heartbeat means the monitor stopped, so its last reading proves
// nothing about now.
func TestCLI_StaleHeartbeatYieldsNoPulseTruth(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	// Heartbeat from three days ago that still claims the pulse was present.
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"present"}]}`,
		now.Add(-72*time.Hour).Format(time.RFC3339)))
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":2,"human_needed":true}}}`)

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 0},
	})
	var stdout, stderr bytes.Buffer
	run(f.args(), &stdout, &stderr, host, nil)

	if strings.Contains(stdout.String(), "recovered") {
		t.Errorf("a 72h-old heartbeat confirmed a recovery:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String()+stdout.String(), "heartbeat") {
		t.Errorf("stale heartbeat was not reported anywhere.\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
	}
}

// RL-31: past the give-up threshold, remediation is suppressed and the job
// stays loudly escalated. Restarting a job that five recoveries did not fix is
// thrash, and the live incident ran 555 such no-op kickstarts on one job.
func TestCLI_GiveUpSuppressesRemediation(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":5,"human_needed":true}}}`)

	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args("--give-up-after", "5"), &stdout, &stderr, host, nil)

	if len(*calls) != 0 {
		t.Errorf("kept remediating past the give-up threshold: %v", *calls)
	}
	if code != 1 {
		t.Errorf("exit = %d, want 1: the job still needs a human. stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "HUMAN NEEDED") {
		t.Errorf("gave up without staying loud:\n%s", stdout.String())
	}
}

// RL-31: a standing give-up must stay loud without re-alarming on every tick.
// Appending an identical escalation record every 10 minutes forever is how a
// human-facing sink becomes noise that nobody reads.
func TestCLI_GiveUpDoesNotReEscalateEveryTick(t *testing.T) {
	f := newFixture(t)
	t0 := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":5,"human_needed":true}}}`)
	launchd := map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	}

	banners := 0
	for _, at := range []time.Time{t0, t0.Add(10 * time.Minute), t0.Add(20 * time.Minute), t0.Add(30 * time.Minute)} {
		f.write(t, f.absHB, fmt.Sprintf(
			`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
			at.Format(time.RFC3339)))
		host, _ := hostAt(at, launchd)
		var stdout, stderr bytes.Buffer
		run(f.args("--give-up-after", "5"), &stdout, &stderr, host, countingNotifier(&banners))
	}

	kinds := f.absenceKinds(t)
	if len(kinds) > 1 {
		t.Errorf("appended %d escalation records over 4 ticks of one standing outage: %v", len(kinds), kinds)
	}
	if banners > 1 {
		t.Errorf("dispatched %d banners for one standing outage", banners)
	}
}

// RL-11: --dry-run reports what it would do and changes nothing. With a pending
// verification persisted, the settle path runs before the dry-run guard and can
// write state, journal records, an escalation and a desktop banner.
func TestCLI_DryRunSettlingPendingMutatesNothing(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)
	// A remediation from two hours ago whose grace window has long lapsed.
	before := `{"jobs":{"absence-alarm":{"consecutive_failures":1,"human_needed":false,` +
		`"pending_action":"kickstart","pending_since":"2026-09-09T08:00:00Z",` +
		`"pending_deadline":"2026-09-09T08:30:00Z"}}}`
	f.write(t, f.state, before)

	host, calls := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	banners := 0
	var stdout, stderr bytes.Buffer
	run(f.args("--dry-run"), &stdout, &stderr, host, countingNotifier(&banners))

	if got, err := os.ReadFile(f.state); err != nil || string(got) != before {
		t.Errorf("--dry-run rewrote the state file:\n got: %s\nwant: %s", got, before)
	}
	if _, err := os.Stat(f.journal); err == nil {
		t.Error("--dry-run appended to the recovery journal")
	}
	if _, err := os.Stat(f.absJrnl); err == nil {
		t.Error("--dry-run appended to the absence-alarm escalation journal")
	}
	if banners != 0 {
		t.Errorf("--dry-run dispatched %d desktop banners", banners)
	}
	if len(*calls) != 0 {
		t.Errorf("--dry-run executed host actions: %v", *calls)
	}
}

// RL-05/RL-22: a snooze added while a verification is pending must be honoured.
// Otherwise an operator who silences a known outage still gets escalated at.
func TestCLI_SnoozeHonouredWhilePendingVerification(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.snooze, `[{"pulse":"absence-alarm","until":"2026-09-10T00:00:00Z","reason":"known outage, fix in flight"}]`)
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":1,`+
		`"pending_action":"kickstart","pending_since":"2026-09-09T08:00:00Z",`+
		`"pending_deadline":"2026-09-09T08:30:00Z"}}}`)

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	banners := 0
	var stdout, stderr bytes.Buffer
	code := run(f.args(), &stdout, &stderr, host, countingNotifier(&banners))

	if !strings.Contains(stdout.String(), "snoozed") {
		t.Errorf("an active snooze was ignored while settling a pending verification:\n%s", stdout.String())
	}
	if banners != 0 {
		t.Errorf("escalated %d times over a snoozed job", banners)
	}
	if code != 0 {
		t.Errorf("exit = %d, want 0 for a snoozed job. stderr: %s", code, stderr.String())
	}
}

// RL-30: a job that already crossed the escalation threshold must stay visible
// as needing a human while its next remediation is pending. Dropping the flag
// on the pending result makes the process exit 0 and the fleet read green in
// the middle of an unresolved outage.
func TestCLI_HumanNeededSurvivesPendingVerification(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))
	f.write(t, f.absState, `{"pulses":{"absence-alarm-heartbeat":{"since":"2026-09-03T08:00:00Z"}}}`)
	f.write(t, f.state, `{"jobs":{"absence-alarm":{"consecutive_failures":2,"human_needed":true}}}`)

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	var stdout, stderr bytes.Buffer
	code := run(f.args("--give-up-after", "0"), &stdout, &stderr, host, nil)

	if code != 1 {
		t.Errorf("exit = %d, want 1: the job needed a human before this tick and nothing cleared it.\n%s\n%s",
			code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "HUMAN NEEDED") {
		t.Errorf("a standing human-needed job went quiet while pending:\n%s", stdout.String())
	}
	if js := f.jobState(t, "absence-alarm"); !js.HumanNeeded {
		t.Error("persisted human_needed=false while the outage stands")
	}
}

// RL-10: the attempt number in the journal is the ordinal of this recovery
// attempt. Journalling the pre-action counter makes the first attempt read as
// attempt 0 and every later one off by one.
func TestCLI_PendingAttemptNumberedFromOne(t *testing.T) {
	f := newFixture(t)
	now := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	f.write(t, f.cfg, absenceAlarmJob)
	f.write(t, f.absHB, fmt.Sprintf(
		`{"tick_time":%q,"results":[{"name":"absence-alarm-heartbeat","status":"absent"}]}`,
		now.Format(time.RFC3339)))

	host, _ := hostAt(now, map[string]recoveryloop.LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Loaded: true, PID: 0, Status: 1},
	})
	var stdout, stderr bytes.Buffer
	run(f.args(), &stdout, &stderr, host, nil)

	raw, err := os.ReadFile(f.journal)
	if err != nil {
		t.Fatalf("read recovery journal: %v", err)
	}
	var rec map[string]any
	first := strings.SplitN(strings.TrimSpace(string(raw)), "\n", 2)[0]
	if err := json.Unmarshal([]byte(first), &rec); err != nil {
		t.Fatalf("parse journal record %q: %v", first, err)
	}
	if got := fmt.Sprint(rec["attempt"]); got != "1" {
		t.Errorf("first pending attempt journalled as %q, want \"1\": %s", got, first)
	}
}
