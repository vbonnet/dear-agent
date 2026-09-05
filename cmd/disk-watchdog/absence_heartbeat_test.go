package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

// writeAbsenceHeartbeat writes an absence-alarm heartbeat fixture and returns its path.
func writeAbsenceHeartbeat(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "absence-alarm.heartbeat.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write absence heartbeat: %v", err)
	}
	return p
}

func absenceHeartbeatJSON(tickTime time.Time) string {
	return `{"tick_time":"` + tickTime.UTC().Format(time.RFC3339Nano) + `"}`
}

func TestCheckAbsenceAlarmHealth_RecentHeartbeatIsHealthy(t *testing.T) {
	now := time.Now()
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now.Add(-10*time.Minute)))
	cfg := config{absenceHeartbeatPath: hbPath, absenceMaxAge: 30 * time.Minute}

	got := checkAbsenceAlarmHealth(cfg, now)
	if got == nil {
		t.Fatal("expected non-nil absence health")
	}
	if got.Stale {
		t.Fatalf("recent heartbeat should be healthy, got: %+v", got)
	}
	if got.Age < 9*time.Minute || got.Age > 11*time.Minute {
		t.Errorf("Age = %s, want ~10m", got.Age)
	}
	if got.Reason != "" {
		t.Errorf("expected empty reason for healthy heartbeat, got %q", got.Reason)
	}
}

func TestCheckAbsenceAlarmHealth_StaleHeartbeatAlarms(t *testing.T) {
	now := time.Now()
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now.Add(-45*time.Minute)))
	cfg := config{absenceHeartbeatPath: hbPath, absenceMaxAge: 30 * time.Minute}

	got := checkAbsenceAlarmHealth(cfg, now)
	if got == nil {
		t.Fatal("expected non-nil absence health")
	}
	if !got.Stale {
		t.Fatalf("heartbeat older than window must be stale, got: %+v", got)
	}
	if !strings.Contains(got.Reason, "absence-alarm is stale") {
		t.Errorf("reason should report staleness, got %q", got.Reason)
	}
	if !strings.Contains(got.Reason, "45m") {
		t.Errorf("reason should report age ~45m, got %q", got.Reason)
	}
}

func TestCheckAbsenceAlarmHealth_MissingHeartbeatIsStale(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "nonexistent.json")
	cfg := config{absenceHeartbeatPath: missingPath, absenceMaxAge: 30 * time.Minute}

	got := checkAbsenceAlarmHealth(cfg, time.Now())
	if got == nil || !got.Stale {
		t.Fatalf("missing heartbeat file must be stale, got: %+v", got)
	}
	if !strings.Contains(got.Reason, "unreadable") {
		t.Errorf("reason should report unreadable, got %q", got.Reason)
	}
}

func TestCheckAbsenceAlarmHealth_InvalidJSONIsStale(t *testing.T) {
	hbPath := writeAbsenceHeartbeat(t, "not-json-content")
	cfg := config{absenceHeartbeatPath: hbPath, absenceMaxAge: 30 * time.Minute}

	got := checkAbsenceAlarmHealth(cfg, time.Now())
	if got == nil || !got.Stale {
		t.Fatalf("invalid JSON heartbeat must be stale, got: %+v", got)
	}
	if !strings.Contains(got.Reason, "invalid JSON") {
		t.Errorf("reason should report invalid JSON, got %q", got.Reason)
	}
}

func TestCheckAbsenceAlarmHealth_MissingTickTimeIsStale(t *testing.T) {
	hbPath := writeAbsenceHeartbeat(t, `{"pid":1234}`)
	cfg := config{absenceHeartbeatPath: hbPath, absenceMaxAge: 30 * time.Minute}

	got := checkAbsenceAlarmHealth(cfg, time.Now())
	if got == nil || !got.Stale {
		t.Fatalf("heartbeat missing tick_time must be stale, got: %+v", got)
	}
	if !strings.Contains(got.Reason, "no tick_time") {
		t.Errorf("reason should report missing tick_time, got %q", got.Reason)
	}
}

func TestCheckAbsenceAlarmHealth_InvalidTickTimeFormatIsStale(t *testing.T) {
	hbPath := writeAbsenceHeartbeat(t, `{"tick_time":"invalid-timestamp"}`)
	cfg := config{absenceHeartbeatPath: hbPath, absenceMaxAge: 30 * time.Minute}

	got := checkAbsenceAlarmHealth(cfg, time.Now())
	if got == nil || !got.Stale {
		t.Fatalf("heartbeat with unparseable tick_time must be stale, got: %+v", got)
	}
	if !strings.Contains(got.Reason, "invalid tick_time") {
		t.Errorf("reason should report invalid tick_time, got %q", got.Reason)
	}
}

func TestCheckAbsenceAlarmHealth_ClockSkewTolerance(t *testing.T) {
	now := time.Now()

	// Timestamp within 5 minutes in future is tolerated
	toleratedFuture := now.Add(2 * time.Minute)
	hbPath1 := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(toleratedFuture))
	cfg1 := config{absenceHeartbeatPath: hbPath1, absenceMaxAge: 30 * time.Minute}
	got1 := checkAbsenceAlarmHealth(cfg1, now)
	if got1 == nil || got1.Stale {
		t.Fatalf("timestamp within 5m future should be tolerated, got: %+v", got1)
	}
	if got1.Age != 0 {
		t.Errorf("future timestamp within tolerance should have age 0, got %s", got1.Age)
	}

	// Timestamp beyond 5 minutes in future is an alarm
	excessiveFuture := now.Add(10 * time.Minute)
	hbPath2 := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(excessiveFuture))
	cfg2 := config{absenceHeartbeatPath: hbPath2, absenceMaxAge: 30 * time.Minute}
	got2 := checkAbsenceAlarmHealth(cfg2, now)
	if got2 == nil || !got2.Stale {
		t.Fatalf("timestamp beyond 5m future must be stale, got: %+v", got2)
	}
	if !strings.Contains(got2.Reason, "clock skew") {
		t.Errorf("reason should mention clock skew, got %q", got2.Reason)
	}
}

func TestCheckAbsenceAlarmHealth_DisabledWhenWindowIsZeroOrPathEmpty(t *testing.T) {
	now := time.Now()
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now.Add(-10*time.Minute)))

	// Window is zero -> disabled (DW-48)
	cfgZero := config{absenceHeartbeatPath: hbPath, absenceMaxAge: 0}
	if got := checkAbsenceAlarmHealth(cfgZero, now); got != nil {
		t.Fatalf("zero max-age must disable check, got: %+v", got)
	}

	// Path is empty -> disabled (DW-48)
	cfgEmpty := config{absenceHeartbeatPath: "", absenceMaxAge: 30 * time.Minute}
	if got := checkAbsenceAlarmHealth(cfgEmpty, now); got != nil {
		t.Fatalf("empty path must disable check, got: %+v", got)
	}
}

func TestRun_NegativeAbsenceMaxAgeIsUsageError(t *testing.T) {
	var out bytes.Buffer
	code, err := run([]string{"--absence-max-age", "-30m"}, &out)
	if code != 2 || err == nil {
		t.Fatalf("negative absence-max-age must exit 2 with error, got code %d, err %v", code, err)
	}
	if !strings.Contains(err.Error(), "absence-max-age") {
		t.Errorf("error should mention absence-max-age, got %v", err)
	}
}

func hermeticRunArgsWithAbsence(t *testing.T, hbPath, maxAge string) []string {
	t.Helper()
	return []string{
		"--path", t.TempDir(),
		"--trail", filepath.Join(t.TempDir(), "trail.jsonl"),
		"--brake", filepath.Join(t.TempDir(), "brake.json"),
		"--agm", filepath.Join(t.TempDir(), "no-such-agm"),
		"--gc-max-age", "0",
		"--absence-heartbeat", hbPath,
		"--absence-max-age", maxAge,
		"--e2e-cache-dir", t.TempDir(),
		"--free-warn-gb", "0.0001",
		"--free-critical-gb", "0.0001",
		"--inode-warn", "0.999999",
		"--inode-critical", "0.999999",
	}
}

func TestRun_HealthyAbsenceHeartbeatKeepsTickGreen(t *testing.T) {
	var out bytes.Buffer
	now := time.Now()
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now.Add(-5*time.Minute)))

	code, err := run(hermeticRunArgsWithAbsence(t, hbPath, "30m"), &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (output: %s)", code, out.String())
	}
	if !strings.Contains(out.String(), "Status: OK") {
		t.Errorf("output should report OK:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "absence alarm: ok") {
		t.Errorf("output should report absence alarm ok:\n%s", out.String())
	}
}

func TestRun_StaleAbsenceHeartbeatAlarmsAtWarn(t *testing.T) {
	var out bytes.Buffer
	now := time.Now()
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now.Add(-2*time.Hour)))

	code, err := run(hermeticRunArgsWithAbsence(t, hbPath, "30m"), &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1 for stale absence heartbeat (output: %s)", code, out.String())
	}
	if !strings.Contains(out.String(), "Status: ALARM (warn)") {
		t.Errorf("output should report ALARM (warn):\n%s", out.String())
	}
	if !strings.Contains(out.String(), "absence alarm: STALE") {
		t.Errorf("output should report STALE absence alarm:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "absence-alarm is stale") {
		t.Errorf("output should include staleness reason in reasons:\n%s", out.String())
	}
}

func TestRun_StaleAbsenceHeartbeatDoesNotEngageBrake(t *testing.T) {
	var out bytes.Buffer
	brake := filepath.Join(t.TempDir(), "brake.json")
	now := time.Now()
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now.Add(-2*time.Hour)))

	args := hermeticRunArgsWithAbsence(t, hbPath, "30m")
	for i, a := range args {
		if a == "--brake" {
			args[i+1] = brake
		}
	}
	code, err := run(args, &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if _, err := os.Stat(brake); err == nil {
		t.Errorf("stale absence heartbeat must not engage admission brake, but %s exists", brake)
	}
}

func TestRun_AbsenceHeartbeatJSONOutput(t *testing.T) {
	var out bytes.Buffer
	now := time.Now()
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now.Add(-40*time.Minute)))

	args := append(hermeticRunArgsWithAbsence(t, hbPath, "30m"), "--json")
	code, err := run(args, &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}

	var rep struct {
		Level        string `json:"level"`
		OK           bool   `json:"ok"`
		AbsenceAlarm struct {
			Stale      bool   `json:"stale"`
			TickTime   string `json:"tick_time"`
			AgeSeconds int64  `json:"age_seconds"`
			Reason     string `json:"reason"`
		} `json:"absence_alarm"`
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal json: %v, raw output:\n%s", err, out.String())
	}
	if rep.Level != supervisor.PressureWarn.String() {
		t.Errorf("level = %q, want %q", rep.Level, supervisor.PressureWarn)
	}
	if rep.OK {
		t.Errorf("ok = true, want false")
	}
	if !rep.AbsenceAlarm.Stale {
		t.Errorf("expected AbsenceAlarm.Stale to be true")
	}
	if rep.AbsenceAlarm.TickTime == "" {
		t.Errorf("expected AbsenceAlarm.TickTime to be populated")
	}
	if rep.AbsenceAlarm.AgeSeconds < 2300 {
		t.Errorf("expected AgeSeconds ~2400 (40m), got %d", rep.AbsenceAlarm.AgeSeconds)
	}
}

func TestDefaultAbsenceHeartbeatPath_RespectsXDGStateHome(t *testing.T) {
	customState := filepath.Join(t.TempDir(), "custom-state")
	t.Setenv("XDG_STATE_HOME", customState)

	got := defaultAbsenceHeartbeatPath()
	want := filepath.Join(customState, "dear-agent", "absence-alarm.heartbeat.json")
	if got != want {
		t.Errorf("defaultAbsenceHeartbeatPath() = %q, want %q", got, want)
	}
}

func TestRun_AbsenceHeartbeatJSONOutput_ZeroAgeRetained(t *testing.T) {
	var out bytes.Buffer
	now := time.Now()
	// Fresh heartbeat timestamped now -> age is 0s
	hbPath := writeAbsenceHeartbeat(t, absenceHeartbeatJSON(now))

	args := append(hermeticRunArgsWithAbsence(t, hbPath, "30m"), "--json")
	code, err := run(args, &out)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	aa, ok := raw["absence_alarm"].(map[string]any)
	if !ok || aa == nil {
		t.Fatalf("expected absence_alarm in JSON output: %v", raw)
	}
	ageVal, hasAge := aa["age_seconds"]
	if !hasAge {
		t.Errorf("expected age_seconds to be present in absence_alarm even when 0: %v", aa)
	}
	if ageVal.(float64) != 0 {
		t.Errorf("expected age_seconds to be 0, got %v", ageVal)
	}
}
