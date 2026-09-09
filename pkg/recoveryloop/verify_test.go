package recoveryloop

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	truth := PulseTruth{"absence-alarm-heartbeat": {Known: true, Alarming: false}}

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
	truth := PulseTruth{"mergeloop-tick": {Known: true, Alarming: true}}

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
	truth := PulseTruth{"absence-alarm-heartbeat": {Known: true, Alarming: true}}

	outcome := VerifyRecovery(job, ActionKickstart, truth, launchd, host, host.Now())
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
	truth := PulseTruth{"absence-alarm-heartbeat": {Known: true, Alarming: false}}

	outcome := VerifyRecovery(job, ActionKickstart, truth, launchd, host, host.Now())
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
	truth := PulseTruth{"mergeloop-tick": {Known: true, Alarming: true}}

	outcome := VerifyRecovery(job, ActionBootstrap, truth, map[string]LaunchdJobInfo{}, host, host.Now())
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
	outcome := VerifyRecovery(job, ActionKickstart, PulseTruth{}, launchd, host, host.Now())
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
