package recoveryloop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/absencealarm"
)

// RL-23: a pulse that is present in the latest absence-alarm heartbeat must be
// readable as NOT alarming. The journal-only reader cannot express this, which
// is why a pulse that alarmed once stayed "alarming" forever.
func TestLoadPulseTruth_PresentPulseClears(t *testing.T) {
	dir := t.TempDir()
	hb := filepath.Join(dir, "absence-alarm.heartbeat.json")
	writeFile(t, hb, `{
	  "tick_time": "2026-09-09T08:00:00Z",
	  "results": [
	    {"name": "disk-watchdog-tick", "status": "present"},
	    {"name": "mergeloop-tick", "status": "absent"}
	  ]
	}`)
	st := filepath.Join(dir, "absence-alarm-state.json")
	writeFile(t, st, `{"pulses": {"mergeloop-tick": {"since": "2026-09-03T08:00:00Z"}}}`)

	now := time.Date(2026, 9, 9, 8, 5, 0, 0, time.UTC)
	truth, err := LoadPulseTruth(hb, st, now, time.Hour)
	if err != nil {
		t.Fatalf("LoadPulseTruth: %v", err)
	}
	if truth.Alarming("disk-watchdog-tick") {
		t.Error("disk-watchdog-tick is present in the heartbeat but read as alarming")
	}
	if !truth.Alarming("mergeloop-tick") {
		t.Error("mergeloop-tick is absent in the heartbeat but read as healthy")
	}
	if got := truth.AbsentFor("mergeloop-tick", now); got != 6*24*time.Hour+5*time.Minute {
		t.Errorf("AbsentFor = %s, want 144h5m", got)
	}
}

// RL-24: a present pulse is authoritative evidence of life. A periodic job that
// exits non-zero to signal an alarm (absence-alarm exits 1 while any pulse is
// absent) must not be treated as wedged and restarted.
func TestPlanJob_PresentPulseOverridesNonZeroExit(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "absence-alarm",
		LaunchdLabel: "com.dear-agent.absence-alarm",
		Pulse:        "absence-alarm-heartbeat",
	}
	launchd := map[string]LaunchdJobInfo{
		// Loaded, not currently running, last run exited 1: absence-alarm's
		// documented "alarms are present" exit code.
		"com.dear-agent.absence-alarm": {Label: "com.dear-agent.absence-alarm", PID: 0, Status: 1, Loaded: true},
	}
	truth := PulseTruth{"absence-alarm-heartbeat": {Known: true, Status: absencealarm.StatusPresent}}

	action, status, reason := PlanJob(job, nil, truth, launchd, host, host.Now())
	if action != ActionNone || status != StatusHealthy {
		t.Errorf("got action=%q status=%q reason=%q; want none/healthy: a present pulse proves the job is alive", action, status, reason)
	}
}

// RL-25: planning must never classify a job as RECOVERED. Recovery is an
// observed post-condition, not an intention.
func TestPlanJob_NeverClaimsRecovered(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "mergeloop",
		LaunchdLabel: "com.dear-agent.mergeloop",
		PlistPath:    "/tmp/mergeloop.plist",
		Pulse:        "mergeloop-tick",
	}
	truth := PulseTruth{"mergeloop-tick": {Known: true, Status: absencealarm.StatusAbsent}}

	action, status, _ := PlanJob(job, nil, truth, map[string]LaunchdJobInfo{}, host, host.Now())
	if action != ActionBootstrap {
		t.Fatalf("action = %q, want bootstrap", action)
	}
	if status == StatusRecovered {
		t.Fatal("PlanJob returned RECOVERED before any action ran: this is the false-green defect")
	}
	if status != StatusUnhealthy {
		t.Errorf("status = %q, want unhealthy", status)
	}
}

// RL-26: after a remediation command succeeds, a pulse that is still absent
// means the recovery did NOT work. "The command ran" is not recovery.
func TestVerifyRecovery_StillAbsentIsNotRecovered(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "absence-alarm",
		LaunchdLabel: "com.dear-agent.absence-alarm",
		Pulse:        "absence-alarm-heartbeat",
	}
	launchd := map[string]LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Label: "com.dear-agent.absence-alarm", PID: 0, Status: 1, Loaded: true},
	}
	// The structural condition cleared (job is loaded) but the pulse has not.
	truth := PulseTruth{"absence-alarm-heartbeat": {Known: true, Status: absencealarm.StatusAbsent}}

	outcome := VerifyRecovery(job, ActionKickstart, truth, launchd, host, host.Now(), time.Time{})
	if outcome.Verified {
		t.Fatal("VerifyRecovery reported success while the pulse is still absent")
	}
	if outcome.Status != StatusPending {
		t.Errorf("status = %q, want pending-verification", outcome.Status)
	}
}

// RL-27: once the pulse comes back, the recovery is verified.
func TestVerifyRecovery_PulseReturnedIsRecovered(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "absence-alarm",
		LaunchdLabel: "com.dear-agent.absence-alarm",
		Pulse:        "absence-alarm-heartbeat",
	}
	launchd := map[string]LaunchdJobInfo{
		"com.dear-agent.absence-alarm": {Label: "com.dear-agent.absence-alarm", PID: 0, Status: 0, Loaded: true},
	}
	truth := PulseTruth{"absence-alarm-heartbeat": {Known: true, Status: absencealarm.StatusPresent}}

	outcome := VerifyRecovery(job, ActionKickstart, truth, launchd, host, host.Now(), time.Time{})
	if !outcome.Verified || outcome.Status != StatusRecovered {
		t.Errorf("got verified=%v status=%q; want verified recovered", outcome.Verified, outcome.Status)
	}
}

// RL-28: a structural condition that survives the action is an immediate
// failure; there is nothing to wait for.
func TestVerifyRecovery_StillUnloadedIsImmediateFailure(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "mergeloop",
		LaunchdLabel: "com.dear-agent.mergeloop",
		PlistPath:    "/tmp/mergeloop.plist",
		Pulse:        "mergeloop-tick",
	}
	truth := PulseTruth{"mergeloop-tick": {Known: true, Status: absencealarm.StatusAbsent}}

	outcome := VerifyRecovery(job, ActionBootstrap, truth, map[string]LaunchdJobInfo{}, host, host.Now(), time.Time{})
	if outcome.Verified {
		t.Fatal("VerifyRecovery reported success while the launchd job is still unloaded")
	}
	if outcome.Status != StatusFailed {
		t.Errorf("status = %q, want failed", outcome.Status)
	}
}

// RL-29: an unknown pulse cannot be used to claim recovery. "I could not check"
// is not health.
func TestVerifyRecovery_UnknownPulseIsNotRecovered(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{
		Name:         "token-refresher",
		LaunchdLabel: "com.dear-agent.token-refresher",
		Pulse:        "token-refresher-tick",
	}
	launchd := map[string]LaunchdJobInfo{
		"com.dear-agent.token-refresher": {Label: "com.dear-agent.token-refresher", PID: 0, Status: 0, Loaded: true},
	}
	outcome := VerifyRecovery(job, ActionKickstart, PulseTruth{}, launchd, host, host.Now(), time.Time{})
	if outcome.Verified {
		t.Fatal("VerifyRecovery claimed success for a pulse it never observed")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// RL-24/RL-29: only an explicitly present pulse is proof of life.
//
// Status.Alarming() is false for "absent" and "undetermined" only, so deriving
// presence as "not alarming" silently promoted "snoozed" and any status this
// binary does not recognise into positive evidence of health. That is the same
// false-green defect this branch exists to remove, one layer down: a snooze
// suppresses an alarm, it does not observe a pulse.
func TestLoadPulseTruth_NonPresentStatusIsNotProofOfLife(t *testing.T) {
	dir := t.TempDir()
	hb := filepath.Join(dir, "absence-alarm.heartbeat.json")
	writeFile(t, hb, `{
	  "tick_time": "2026-09-09T08:00:00Z",
	  "results": [
	    {"name": "snoozed-pulse", "status": "snoozed"},
	    {"name": "future-pulse", "status": "degraded"},
	    {"name": "live-pulse", "status": "present"}
	  ]
	}`)
	st := filepath.Join(dir, "absence-alarm-state.json")
	writeFile(t, st, `{"pulses": {}}`)

	now := time.Date(2026, 9, 9, 8, 5, 0, 0, time.UTC)
	truth, err := LoadPulseTruth(hb, st, now, time.Hour)
	if err != nil {
		t.Fatalf("LoadPulseTruth: %v", err)
	}
	if truth.Present("snoozed-pulse") {
		t.Error("a snoozed pulse was read as present: a snooze suppresses an alarm, it does not observe a pulse")
	}
	if truth.Present("future-pulse") {
		t.Error("an unrecognised status was read as present: an undecodable status is not evidence of health")
	}
	if !truth.Present("live-pulse") {
		t.Error("an explicitly present pulse was not read as present")
	}
}

// RL-29: a job whose pulse is only snoozed must not verify as recovered.
func TestVerifyRecovery_SnoozedPulseDoesNotVerify(t *testing.T) {
	host, _ := mockHostOps()
	job := Job{Name: "sandbox-gc", LaunchdLabel: "com.dear-agent.sandbox-gc", Pulse: "sandbox-gc-tick"}
	launchd := map[string]LaunchdJobInfo{
		"com.dear-agent.sandbox-gc": {Label: "com.dear-agent.sandbox-gc", Loaded: true, Status: 0},
	}
	truth := PulseTruth{"sandbox-gc-tick": {Known: true, Status: absencealarm.StatusSnoozed}}

	out := VerifyRecovery(job, ActionKickstart, truth, launchd, host, host.Now(), time.Time{})
	if out.Verified || out.Status == StatusRecovered {
		t.Errorf("got verified=%v status=%q; a snoozed pulse is not an observation of recovery", out.Verified, out.Status)
	}
}

// RL-32: a heartbeat carrying no usable tick_time is stale evidence of unknown
// age, not fresh evidence. Bypassing the age check for a zero timestamp let a
// corrupt or truncated heartbeat confirm recoveries indefinitely.
func TestLoadPulseTruth_MissingTickTimeIsRefused(t *testing.T) {
	dir := t.TempDir()
	hb := filepath.Join(dir, "absence-alarm.heartbeat.json")
	writeFile(t, hb, `{"results": [{"name": "disk-watchdog-tick", "status": "present"}]}`)
	st := filepath.Join(dir, "absence-alarm-state.json")
	writeFile(t, st, `{"pulses": {}}`)

	now := time.Date(2026, 9, 9, 8, 5, 0, 0, time.UTC)
	truth, err := LoadPulseTruth(hb, st, now, time.Hour)
	if err == nil {
		t.Fatal("a heartbeat with no tick_time was accepted; undated evidence must not confirm a recovery")
	}
	if truth.Present("disk-watchdog-tick") {
		t.Error("facts were returned from an undated heartbeat")
	}
}

// RL-32: a corrupt alarm state must not be able to produce facts that a caller
// treating the error as non-fatal would then act on.
func TestLoadPulseTruth_CorruptAlarmStateYieldsError(t *testing.T) {
	dir := t.TempDir()
	hb := filepath.Join(dir, "absence-alarm.heartbeat.json")
	writeFile(t, hb, `{"tick_time": "2026-09-09T08:00:00Z",
	  "results": [{"name": "disk-watchdog-tick", "status": "present"}]}`)
	st := filepath.Join(dir, "absence-alarm-state.json")
	writeFile(t, st, `{ this is not json`)

	now := time.Date(2026, 9, 9, 8, 5, 0, 0, time.UTC)
	if _, err := LoadPulseTruth(hb, st, now, time.Hour); err == nil {
		t.Fatal("a corrupt alarm state was accepted silently")
	}
}

// RL-39: a pulse observed BEFORE the action ran cannot prove the action worked.
//
// Pulse truth is read once at the start of a tick. Passing that pre-action fact
// to VerifyRecovery lets a structural fix be confirmed by evidence that predates
// it: a job unloaded minutes ago still has a fresh file-mtime pulse, so a
// bootstrap that succeeds structurally but never actually runs the job would
// reset the failure count as a verified recovery. Post-action proof has to
// post-date the action.
func TestVerifyRecovery_PreActionPulseIsNotPostActionProof(t *testing.T) {
	host, _ := mockHostOps()
	actionAt := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	job := Job{Name: "disk-watchdog", LaunchdLabel: "com.dear-agent.disk-watchdog", Pulse: "disk-watchdog-tick"}
	launchd := map[string]LaunchdJobInfo{
		"com.dear-agent.disk-watchdog": {Loaded: true, Status: 0},
	}
	// Present, but observed five minutes before the remediation ran.
	truth := PulseTruth{"disk-watchdog-tick": {
		Known:    true,
		Status:   absencealarm.StatusPresent,
		Evidence: actionAt.Add(-5 * time.Minute),
	}}

	out := VerifyRecovery(job, ActionBootstrap, truth, launchd, host, actionAt, actionAt)
	if out.Verified || out.Status == StatusRecovered {
		t.Errorf("got verified=%v status=%q reason=%q; a pulse from before the action does not prove the action worked",
			out.Verified, out.Status, out.Reason)
	}
	if out.Status != StatusPending {
		t.Errorf("status = %q, want pending: the answer is not in yet", out.Status)
	}
}

// RL-39: once the pulse is observed after the action, that is real proof.
func TestVerifyRecovery_PostActionPulseVerifies(t *testing.T) {
	host, _ := mockHostOps()
	actionAt := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	later := actionAt.Add(20 * time.Minute)
	job := Job{Name: "disk-watchdog", LaunchdLabel: "com.dear-agent.disk-watchdog", Pulse: "disk-watchdog-tick"}
	launchd := map[string]LaunchdJobInfo{
		"com.dear-agent.disk-watchdog": {Loaded: true, Status: 0},
	}
	truth := PulseTruth{"disk-watchdog-tick": {
		Known:    true,
		Status:   absencealarm.StatusPresent,
		Evidence: actionAt.Add(10 * time.Minute),
	}}

	out := VerifyRecovery(job, ActionBootstrap, truth, launchd, host, later, actionAt)
	if !out.Verified || out.Status != StatusRecovered {
		t.Errorf("got verified=%v status=%q reason=%q; a pulse observed after the action is proof it worked",
			out.Verified, out.Status, out.Reason)
	}
}

// RL-32: a heartbeat dated in the future is evidence of a broken clock, not
// fresh evidence. now.Sub(tick) is negative for a future tick, so an age-only
// check accepts it however far ahead it is, and its last "present" results then
// suppress remediation until wall time catches up.
func TestLoadPulseTruth_FutureTickTimeIsRefused(t *testing.T) {
	dir := t.TempDir()
	hb := filepath.Join(dir, "absence-alarm.heartbeat.json")
	writeFile(t, hb, `{"tick_time":"2026-09-20T08:00:00Z",
	  "results":[{"name":"disk-watchdog-tick","status":"present"}]}`)
	st := filepath.Join(dir, "absence-alarm-state.json")
	writeFile(t, st, `{"pulses": {}}`)

	now := time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC)
	truth, err := LoadPulseTruth(hb, st, now, time.Hour)
	if err == nil {
		t.Fatal("a heartbeat dated a week in the future was accepted")
	}
	if truth.Present("disk-watchdog-tick") {
		t.Error("facts were returned from a future-dated heartbeat")
	}
}
