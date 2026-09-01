package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// maxSnoozeHorizon bounds how far into the future a snooze may reach (AA-14).
// The mergeloop lesson: "disabled temporarily" must not silently become
// "disabled forever".
const maxSnoozeHorizon = 14 * 24 * time.Hour

// reNotifyPoints are the alarm ages at which a standing alarm re-notifies
// (AA-11). After the last point it repeats every reNotifyRepeat.
var reNotifyPoints = []time.Duration{time.Hour, 6 * time.Hour, 24 * time.Hour}

const reNotifyRepeat = 24 * time.Hour

// Snooze is one operator-declared, expiring silence for a pulse.
type Snooze struct {
	Pulse  string    `json:"pulse"`
	Until  time.Time `json:"until"`
	Reason string    `json:"reason,omitempty"`
}

// loadSnoozes reads and validates the snooze file. A missing file means no
// snoozes; an invalid snooze refuses the run (AA-14).
func loadSnoozes(path string, now time.Time) (map[string]Snooze, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Snooze{}, nil
		}
		return nil, fmt.Errorf("read snooze file: %w", err)
	}
	var entries []Snooze
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse snooze file %s: %w", path, err)
	}
	out := make(map[string]Snooze, len(entries))
	for _, s := range entries {
		if s.Pulse == "" {
			return nil, fmt.Errorf("snooze file %s: entry with empty pulse name", path)
		}
		if s.Until.IsZero() {
			return nil, fmt.Errorf("snooze file %s: pulse %q has no expiry; a snooze without an expiry is a permanent silence and is refused", path, s.Pulse)
		}
		if s.Until.After(now.Add(maxSnoozeHorizon)) {
			return nil, fmt.Errorf("snooze file %s: pulse %q expiry %s exceeds the %s horizon", path, s.Pulse, s.Until.Format(time.RFC3339), maxSnoozeHorizon)
		}
		out[s.Pulse] = s
	}
	return out, nil
}

// pulseAlarm is the persisted per-pulse alarm state used for notification
// dedup (AA-10..AA-12).
type pulseAlarm struct {
	Since        time.Time `json:"since"`
	LastNotified time.Time `json:"last_notified"`
	Misses       int       `json:"misses"`
}

type alarmState struct {
	Pulses map[string]pulseAlarm `json:"pulses"`
}

// loadAlarmState reads the persisted alarm state. Any read or parse failure
// returns an empty state and the error: the caller reports it and proceeds,
// so lost dedup state degrades toward louder, never toward silent (AA-18).
func loadAlarmState(path string) (alarmState, error) {
	st := alarmState{Pulses: map[string]pulseAlarm{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, fmt.Errorf("read alarm state: %w", err)
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return alarmState{Pulses: map[string]pulseAlarm{}}, fmt.Errorf("parse alarm state %s: %w", path, err)
	}
	if st.Pulses == nil {
		st.Pulses = map[string]pulseAlarm{}
	}
	return st, nil
}

func saveAlarmState(path string, st alarmState) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// notifyDecision says what, if anything, to dispatch for a pulse this tick.
type notifyDecision int

const (
	notifyNone notifyDecision = iota
	notifyAlarm
	notifyRecovery
)

// updateAlarm advances one pulse's alarm state and decides whether to notify
// (AA-10, AA-11, AA-12). It mutates st in place.
func updateAlarm(st *alarmState, name string, alarming bool, now time.Time) notifyDecision {
	prev, wasAlarming := st.Pulses[name]
	if !alarming {
		if wasAlarming {
			delete(st.Pulses, name)
			return notifyRecovery
		}
		return notifyNone
	}
	if !wasAlarming {
		st.Pulses[name] = pulseAlarm{Since: now, LastNotified: now, Misses: 1}
		return notifyAlarm
	}
	prev.Misses++
	if due := reNotifyDue(prev.Since, prev.LastNotified, now); due {
		prev.LastNotified = now
		st.Pulses[name] = prev
		return notifyAlarm
	}
	st.Pulses[name] = prev
	return notifyNone
}

// reNotifyDue reports whether a re-notification point has been crossed since
// the last notification (AA-11): points at 1h, 6h, 24h of alarm age, then
// every 24h.
func reNotifyDue(since, lastNotified, now time.Time) bool {
	for _, p := range reNotifyPoints {
		at := since.Add(p)
		if at.After(lastNotified) && !at.After(now) {
			return true
		}
	}
	last := since.Add(reNotifyPoints[len(reNotifyPoints)-1])
	for at := last.Add(reNotifyRepeat); !at.After(now); at = at.Add(reNotifyRepeat) {
		if at.After(lastNotified) {
			return true
		}
	}
	return false
}

// journalRecord is one appended escalation-journal line (AA-09).
type journalRecord struct {
	Time     time.Time `json:"time"`
	Kind     string    `json:"kind"`
	Pulse    string    `json:"pulse"`
	Status   Status    `json:"status"`
	Reason   string    `json:"reason,omitempty"`
	Expect   string    `json:"expect,omitempty"`
	Window   string    `json:"window,omitempty"`
	Evidence time.Time `json:"evidence,omitzero"`
	Misses   int       `json:"misses,omitempty"`
}

func appendJournal(path string, rec journalRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Close()
}

// heartbeat is the self-liveness record written every completed tick (AA-16).
type heartbeat struct {
	TickTime time.Time `json:"tick_time"`
	Results  []Result  `json:"results"`
}

func writeHeartbeat(path string, hb heartbeat) error {
	raw, err := json.MarshalIndent(hb, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, raw)
}

// writeFileAtomic writes via a temp file + rename so a reader never observes
// a torn file and an mtime is only ever advanced by a complete write.
func writeFileAtomic(path string, raw []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
