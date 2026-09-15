package recoveryloop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// JobState tracks the recovery lifecycle and consecutive failure count for one job.
type JobState struct {
	ConsecutiveFailures int            `json:"consecutive_failures"`
	LastAttemptTime     time.Time      `json:"last_attempt_time"`
	LastAction          ActionType     `json:"last_action,omitempty"`
	LastStatus          RecoveryStatus `json:"last_status,omitempty"`
	HumanNeeded         bool           `json:"human_needed"`

	// PendingAction is the remediation awaiting confirmation that the pulse
	// came back. A pending verification is the record that an action ran and
	// has not yet been shown to have worked; without it the loop had no way
	// to notice that the pulse never returned.
	PendingAction ActionType `json:"pending_action,omitempty"`
	// PendingSince is when that action ran.
	PendingSince time.Time `json:"pending_since,omitzero"`
	// PendingDeadline is when the pulse must have returned by. Past it, an
	// un-cleared pulse converts the attempt into a counted failure.
	PendingDeadline time.Time `json:"pending_deadline,omitzero"`
	// UnhealthySince is when the job was first seen unhealthy without having
	// recovered since. It bounds how long a condition has really persisted.
	UnhealthySince time.Time `json:"unhealthy_since,omitzero"`
	// LastEscalated is when this job last reached a human-facing sink, used
	// to keep a standing escalation loud without duplicating it every tick.
	LastEscalated time.Time `json:"last_escalated,omitzero"`
}

// State is the persisted recovery state keyed by job name.
type State struct {
	Jobs map[string]JobState `json:"jobs"`
}

// LoadState reads the persisted recovery state. Any read or parse failure
// returns an empty state and the error so the caller can warn and proceed (RL-18).
func LoadState(path string) (State, error) {
	st := State{Jobs: map[string]JobState{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, fmt.Errorf("read recovery state %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{Jobs: map[string]JobState{}}, fmt.Errorf("parse recovery state %s: %w", path, err)
	}
	if st.Jobs == nil {
		st.Jobs = map[string]JobState{}
	}
	return st, nil
}

// SaveState atomically writes the recovery state to disk.
func SaveState(path string, st State) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode recovery state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir recovery state dir: %w", err)
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write recovery state temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename recovery state: %w", err)
	}
	return nil
}

// JournalRecord is one entry appended to the recovery journal (RL-10).
type JournalRecord struct {
	Time        time.Time      `json:"time"`
	Kind        string         `json:"kind"`
	Job         string         `json:"job"`
	Action      ActionType     `json:"action"`
	Status      RecoveryStatus `json:"status"`
	Attempt     int            `json:"attempt"`
	HumanNeeded bool           `json:"human_needed"`
	Reason      string         `json:"reason,omitempty"`
	Error       string         `json:"error,omitempty"`
	Output      string         `json:"output,omitempty"`
}

// AppendJournal appends one recovery attempt record to the journal (RL-10).
func AppendJournal(path string, rec JournalRecord) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir journal dir: %w", err)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encode journal record: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open journal %s: %w", path, err)
	}
	if _, werr := f.Write(append(raw, '\n')); werr != nil {
		return errors.Join(werr, f.Close())
	}
	return f.Close()
}

// Heartbeat records tick liveness and recovery results for independent watchdogs (RL-16).
type Heartbeat struct {
	TickTime    time.Time `json:"tick_time"`
	Recovered   int       `json:"recovered"`
	Pending     int       `json:"pending"`
	Planned     int       `json:"planned"`
	Failed      int       `json:"failed"`
	HumanNeeded int       `json:"human_needed"`
	Healthy     int       `json:"healthy"`
	Snoozed     int       `json:"snoozed"`
	Results     []Result  `json:"results"`
}

// WriteHeartbeat atomically writes the recovery loop heartbeat file.
func WriteHeartbeat(path string, hb Heartbeat) error {
	raw, err := json.MarshalIndent(hb, "", "  ")
	if err != nil {
		return fmt.Errorf("encode heartbeat: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir heartbeat dir: %w", err)
	}
	tmp := fmt.Sprintf("%s.tmp.%d", path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write heartbeat temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename heartbeat: %w", err)
	}
	return nil
}
