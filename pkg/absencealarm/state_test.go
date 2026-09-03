package absencealarm

import (
	"encoding/json"
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
	sn, err := LoadSnoozes(path, t0)
	if err != nil {
		t.Fatalf("LoadSnoozes: %v", err)
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
	_, err := LoadSnoozes(path, t0)
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
	_, err := LoadSnoozes(path, t0)
	if err == nil || !strings.Contains(err.Error(), "horizon") {
		t.Fatalf("err = %v, want horizon refusal", err)
	}
}

func TestLoadSnoozes_MissingFileIsEmpty(t *testing.T) {
	sn, err := LoadSnoozes(filepath.Join(t.TempDir(), "absent.json"), t0)
	if err != nil || len(sn) != 0 {
		t.Fatalf("got %v %v, want empty and no error", sn, err)
	}
}

// AA-10: a fresh alarm notifies once.
func TestUpdateAlarm_TransitionNotifies(t *testing.T) {
	st := AlarmState{Pulses: map[string]PulseAlarm{}}
	if d := UpdateAlarm(&st, "spans", true, t0); d != NotifyAlarm {
		t.Fatalf("decision = %v, want NotifyAlarm", d)
	}
	if st.Pulses["spans"].Misses != 1 {
		t.Errorf("misses = %d, want 1", st.Pulses["spans"].Misses)
	}
}

// AA-11: a standing alarm stays quiet between escalation points and
// re-notifies when one is crossed.
func TestUpdateAlarm_Backoff(t *testing.T) {
	st := AlarmState{Pulses: map[string]PulseAlarm{}}
	UpdateAlarm(&st, "spans", true, t0)

	// 10 minutes later: no re-notification.
	if d := UpdateAlarm(&st, "spans", true, t0.Add(10*time.Minute)); d != NotifyNone {
		t.Fatalf("at 10m: decision = %v, want NotifyNone", d)
	}
	// Crossing the 1h point: re-notify.
	if d := UpdateAlarm(&st, "spans", true, t0.Add(65*time.Minute)); d != NotifyAlarm {
		t.Fatalf("at 65m: decision = %v, want NotifyAlarm", d)
	}
	// 2h: quiet again (next point is 6h).
	if d := UpdateAlarm(&st, "spans", true, t0.Add(2*time.Hour)); d != NotifyNone {
		t.Fatalf("at 2h: decision = %v, want NotifyNone", d)
	}
	// 7h: crossed the 6h point.
	if d := UpdateAlarm(&st, "spans", true, t0.Add(7*time.Hour)); d != NotifyAlarm {
		t.Fatalf("at 7h: decision = %v, want NotifyAlarm", d)
	}
	// 25h: crossed the 24h point.
	if d := UpdateAlarm(&st, "spans", true, t0.Add(25*time.Hour)); d != NotifyAlarm {
		t.Fatalf("at 25h: decision = %v, want NotifyAlarm", d)
	}
	// 30h: quiet (next repeat is 48h).
	if d := UpdateAlarm(&st, "spans", true, t0.Add(30*time.Hour)); d != NotifyNone {
		t.Fatalf("at 30h: decision = %v, want NotifyNone", d)
	}
	// 49h: 24h repeat cadence holds — a standing alarm never falls silent.
	if d := UpdateAlarm(&st, "spans", true, t0.Add(49*time.Hour)); d != NotifyAlarm {
		t.Fatalf("at 49h: decision = %v, want NotifyAlarm", d)
	}
	if st.Pulses["spans"].Misses != 8 {
		t.Errorf("misses = %d, want 8", st.Pulses["spans"].Misses)
	}
}

// AA-12: recovery notifies once and clears state.
func TestUpdateAlarm_Recovery(t *testing.T) {
	st := AlarmState{Pulses: map[string]PulseAlarm{}}
	UpdateAlarm(&st, "spans", true, t0)
	if d := UpdateAlarm(&st, "spans", false, t0.Add(20*time.Minute)); d != NotifyRecovery {
		t.Fatalf("decision = %v, want NotifyRecovery", d)
	}
	if _, ok := st.Pulses["spans"]; ok {
		t.Error("alarm state not cleared on recovery")
	}
	// A pulse that was never Alarming stays silent.
	if d := UpdateAlarm(&st, "spans", false, t0.Add(30*time.Minute)); d != NotifyNone {
		t.Fatalf("healthy pulse: decision = %v, want NotifyNone", d)
	}
}

// AA-18: a corrupt state file yields an empty state plus an error, so every
// standing alarm re-notifies rather than staying silent.
func TestLoadAlarmState_CorruptDegradesLouder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := LoadAlarmState(path)
	if err == nil {
		t.Fatal("want an error for corrupt state")
	}
	if len(st.Pulses) != 0 {
		t.Fatalf("state not emptied: %+v", st)
	}
	if d := UpdateAlarm(&st, "spans", true, t0); d != NotifyAlarm {
		t.Fatalf("decision = %v, want NotifyAlarm after state loss", d)
	}
}

func TestAlarmStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := AlarmState{Pulses: map[string]PulseAlarm{"spans": {Since: t0, LastNotified: t0, Misses: 3}}}
	if err := SaveAlarmState(path, st); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadAlarmState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Pulses["spans"].Misses != 3 {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

// AA-09: one alarm record per append, carrying the pulse name, status, reason,
// window, evidence timestamp, and consecutive-miss count, appended rather than
// overwritten so an earlier record is never lost.
func TestAppendJournalAppendsOneRecordPerCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "journal.jsonl")
	evidence := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	first := JournalRecord{
		Time:     time.Date(2026, 9, 1, 12, 5, 0, 0, time.UTC),
		Kind:     "absence.alarm",
		Pulse:    "mergeloop-tick",
		Status:   StatusAbsent,
		Reason:   "no tick within 24h",
		Expect:   "a mergeloop tick within 24h",
		Window:   "24h",
		Evidence: evidence,
		Misses:   3,
	}
	if err := AppendJournal(path, first); err != nil {
		t.Fatalf("AppendJournal() error = %v", err)
	}
	second := first
	second.Pulse = "otel-receiving"
	second.Misses = 1
	if err := AppendJournal(path, second); err != nil {
		t.Fatalf("AppendJournal() second error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("journal has %d line(s), want 2:\n%s", len(lines), raw)
	}

	var got JournalRecord
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("first record is not one JSON object: %v", err)
	}
	if got.Pulse != first.Pulse || got.Status != first.Status || got.Reason != first.Reason ||
		got.Window != first.Window || got.Misses != first.Misses || !got.Evidence.Equal(evidence) {
		t.Errorf("first record round-trip = %+v, want %+v", got, first)
	}

	var secondGot JournalRecord
	if err := json.Unmarshal([]byte(lines[1]), &secondGot); err != nil {
		t.Fatalf("second record is not one JSON object: %v", err)
	}
	if secondGot.Pulse != "otel-receiving" {
		t.Errorf("second record pulse = %q, want otel-receiving", secondGot.Pulse)
	}
}

// A journal path whose parent cannot be created is reported, not swallowed.
func TestAppendJournalReportsUnusablePath(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := AppendJournal(filepath.Join(blocker, "journal.jsonl"), JournalRecord{Pulse: "p"}); err == nil {
		t.Fatal("AppendJournal() error = nil for a path under a regular file")
	}
}

// A clock that jumped forward during an alarming tick and was then corrected
// leaves the persisted timestamps after now. Every re-notification point is
// then unreachable, so without a reset a standing alarm stays silent until
// wall time catches up.
func TestUpdateAlarmRecoversFromAClockRollback(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	future := now.Add(72 * time.Hour)

	st := AlarmState{Pulses: map[string]PulseAlarm{
		"spans": {Since: future, LastNotified: future, Misses: 4},
	}}

	if got := UpdateAlarm(&st, "spans", true, now); got != NotifyAlarm {
		t.Fatalf("UpdateAlarm() = %v with future timestamps, want NotifyAlarm", got)
	}

	got := st.Pulses["spans"]
	if got.Since.After(now) || got.LastNotified.After(now) {
		t.Errorf("timestamps still in the future: Since=%v LastNotified=%v now=%v", got.Since, got.LastNotified, now)
	}

	// The cadence resumes rather than re-notifying on every subsequent tick.
	if next := UpdateAlarm(&st, "spans", true, now.Add(time.Minute)); next != NotifyNone {
		t.Errorf("UpdateAlarm() = %v one minute later, want NotifyNone", next)
	}
}
