package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// AA-13: a snooze with a near expiry is accepted.
func TestLoadSnoozes_ValidExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snooze.json")
	doc := `[{"pulse":"mergeloop","until":"2026-09-03T00:00:00Z","reason":"migrating merge policy"}]`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	sn, err := loadSnoozes(path, t0)
	if err != nil {
		t.Fatalf("loadSnoozes: %v", err)
	}
	if sn["mergeloop"].Reason != "migrating merge policy" {
		t.Fatalf("snooze not loaded: %+v", sn)
	}
}

// AA-14: a snooze without an expiry is refused — a permanent silence.
func TestLoadSnoozes_NoExpiryRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snooze.json")
	doc := `[{"pulse":"mergeloop","reason":"off for now"}]`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadSnoozes(path, t0)
	if err == nil || !strings.Contains(err.Error(), "permanent silence") {
		t.Fatalf("err = %v, want no-expiry refusal", err)
	}
}

// AA-14: an expiry beyond the horizon is refused.
func TestLoadSnoozes_BeyondHorizonRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snooze.json")
	doc := `[{"pulse":"mergeloop","until":"2027-01-01T00:00:00Z"}]`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadSnoozes(path, t0)
	if err == nil || !strings.Contains(err.Error(), "horizon") {
		t.Fatalf("err = %v, want horizon refusal", err)
	}
}

func TestLoadSnoozes_MissingFileIsEmpty(t *testing.T) {
	sn, err := loadSnoozes(filepath.Join(t.TempDir(), "absent.json"), t0)
	if err != nil || len(sn) != 0 {
		t.Fatalf("got %v %v, want empty and no error", sn, err)
	}
}

// AA-10: a fresh alarm notifies once.
func TestUpdateAlarm_TransitionNotifies(t *testing.T) {
	st := alarmState{Pulses: map[string]pulseAlarm{}}
	if d := updateAlarm(&st, "spans", true, t0); d != notifyAlarm {
		t.Fatalf("decision = %v, want notifyAlarm", d)
	}
	if st.Pulses["spans"].Misses != 1 {
		t.Errorf("misses = %d, want 1", st.Pulses["spans"].Misses)
	}
}

// AA-11: a standing alarm stays quiet between escalation points and
// re-notifies when one is crossed.
func TestUpdateAlarm_Backoff(t *testing.T) {
	st := alarmState{Pulses: map[string]pulseAlarm{}}
	updateAlarm(&st, "spans", true, t0)

	// 10 minutes later: no re-notification.
	if d := updateAlarm(&st, "spans", true, t0.Add(10*time.Minute)); d != notifyNone {
		t.Fatalf("at 10m: decision = %v, want notifyNone", d)
	}
	// Crossing the 1h point: re-notify.
	if d := updateAlarm(&st, "spans", true, t0.Add(65*time.Minute)); d != notifyAlarm {
		t.Fatalf("at 65m: decision = %v, want notifyAlarm", d)
	}
	// 2h: quiet again (next point is 6h).
	if d := updateAlarm(&st, "spans", true, t0.Add(2*time.Hour)); d != notifyNone {
		t.Fatalf("at 2h: decision = %v, want notifyNone", d)
	}
	// 7h: crossed the 6h point.
	if d := updateAlarm(&st, "spans", true, t0.Add(7*time.Hour)); d != notifyAlarm {
		t.Fatalf("at 7h: decision = %v, want notifyAlarm", d)
	}
	// 25h: crossed the 24h point.
	if d := updateAlarm(&st, "spans", true, t0.Add(25*time.Hour)); d != notifyAlarm {
		t.Fatalf("at 25h: decision = %v, want notifyAlarm", d)
	}
	// 30h: quiet (next repeat is 48h).
	if d := updateAlarm(&st, "spans", true, t0.Add(30*time.Hour)); d != notifyNone {
		t.Fatalf("at 30h: decision = %v, want notifyNone", d)
	}
	// 49h: 24h repeat cadence holds — a standing alarm never falls silent.
	if d := updateAlarm(&st, "spans", true, t0.Add(49*time.Hour)); d != notifyAlarm {
		t.Fatalf("at 49h: decision = %v, want notifyAlarm", d)
	}
	if st.Pulses["spans"].Misses != 8 {
		t.Errorf("misses = %d, want 8", st.Pulses["spans"].Misses)
	}
}

// AA-12: recovery notifies once and clears state.
func TestUpdateAlarm_Recovery(t *testing.T) {
	st := alarmState{Pulses: map[string]pulseAlarm{}}
	updateAlarm(&st, "spans", true, t0)
	if d := updateAlarm(&st, "spans", false, t0.Add(20*time.Minute)); d != notifyRecovery {
		t.Fatalf("decision = %v, want notifyRecovery", d)
	}
	if _, ok := st.Pulses["spans"]; ok {
		t.Error("alarm state not cleared on recovery")
	}
	// A pulse that was never alarming stays silent.
	if d := updateAlarm(&st, "spans", false, t0.Add(30*time.Minute)); d != notifyNone {
		t.Fatalf("healthy pulse: decision = %v, want notifyNone", d)
	}
}

// AA-18: a corrupt state file yields an empty state plus an error, so every
// standing alarm re-notifies rather than staying silent.
func TestLoadAlarmState_CorruptDegradesLouder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := loadAlarmState(path)
	if err == nil {
		t.Fatal("want an error for corrupt state")
	}
	if len(st.Pulses) != 0 {
		t.Fatalf("state not emptied: %+v", st)
	}
	if d := updateAlarm(&st, "spans", true, t0); d != notifyAlarm {
		t.Fatalf("decision = %v, want notifyAlarm after state loss", d)
	}
}

func TestAlarmStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := alarmState{Pulses: map[string]pulseAlarm{"spans": {Since: t0, LastNotified: t0, Misses: 3}}}
	if err := saveAlarmState(path, st); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadAlarmState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Pulses["spans"].Misses != 3 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}
