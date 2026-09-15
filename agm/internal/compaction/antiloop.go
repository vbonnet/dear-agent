// Package compaction provides compaction-related functionality.
package compaction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
)

const (
	// CooldownDuration is the minimum time between compactions.
	CooldownDuration = 2 * time.Hour
	// MaxCompactionsPerWindow is the maximum number of compactions within a rolling window.
	MaxCompactionsPerWindow = 3
	// CompactionWindow is the rolling window for counting compactions.
	CompactionWindow = 24 * time.Hour
)

// CompactionState tracks compaction history for a session.
type CompactionState struct {
	SessionID       string             `json:"session_id,omitempty"`
	SessionName     string             `json:"session_name"`
	LastCompaction  time.Time          `json:"last_compaction"`
	CompactionCount int                `json:"compaction_count"`
	History         []CompactionRecord `json:"history"`
}

// AttemptOutcome is the persisted delivery-accounting result for a compaction
// attempt. Empty outcomes are legacy confirmed records.
type AttemptOutcome string

// Persisted attempt outcomes distinguish confirmed delivery from ambiguity and
// positive proof that no command was sent.
const (
	AttemptOutcomePending         AttemptOutcome = "pending"
	AttemptOutcomeConfirmed       AttemptOutcome = "confirmed"
	AttemptOutcomeUncertain       AttemptOutcome = "uncertain"
	AttemptOutcomeDefiniteNotSent AttemptOutcome = "definite_not_sent"
)

// CompactionRecord is a single compaction event.
type CompactionRecord struct {
	AttemptID        string         `json:"attempt_id,omitempty"`
	Timestamp        time.Time      `json:"timestamp"`
	OutcomeUpdatedAt time.Time      `json:"outcome_updated_at,omitzero"`
	Outcome          AttemptOutcome `json:"outcome,omitempty"`
	PromptFile       string         `json:"prompt_file"`
	Forced           bool           `json:"forced"`
}

// stateDir returns the compaction-state directory under baseDir.
func stateDir(baseDir string) string {
	return filepath.Join(baseDir, "compaction-state")
}

// stateFile returns the path to a session's compaction state file.
func stateFile(baseDir, sessionName string) string {
	return filepath.Join(stateDir(baseDir), sessionName+".json")
}

// LoadState reads legacy display-name-keyed compaction state. It remains for
// compatibility; delivery callers use BeginAttempt so accounting is keyed by
// stable session ID. It returns zero-value state if the file does not exist.
func LoadState(baseDir, sessionName string) (*CompactionState, error) {
	if err := validateStorageKey("session name", sessionName); err != nil {
		return nil, err
	}
	path := stateFile(baseDir, sessionName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CompactionState{SessionName: sessionName}, nil
		}
		return nil, fmt.Errorf("read compaction state: %w", err)
	}
	var s CompactionState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse compaction state: %w", err)
	}
	return &s, nil
}

// SaveState atomically writes legacy display-name-keyed compaction state. It
// remains for compatibility; delivery callers use Attempt.Mark.
func SaveState(baseDir string, state *CompactionState) error {
	if state == nil {
		return fmt.Errorf("compaction state is nil")
	}
	if err := validateStorageKey("session name", state.SessionName); err != nil {
		return err
	}
	if err := validateAttemptRecords(state.History); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal compaction state: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.AtomicWrite(stateFile(baseDir, state.SessionName), data, 0o600); err != nil {
		return fmt.Errorf("write compaction state: %w", err)
	}
	return nil
}

// recentCompactions counts compactions within the rolling window.
func recentCompactions(history []CompactionRecord, now time.Time) int {
	cutoff := now.Add(-CompactionWindow)
	count := 0
	for _, r := range history {
		if attemptCountsForAntiLoop(r.Outcome) && r.Timestamp.After(cutoff) {
			count++
		}
	}
	return count
}

func attemptCountsForAntiLoop(outcome AttemptOutcome) bool {
	// Unknown values count conservatively. Only a persisted, explicit proof that
	// delivery did not occur releases the attempt from the anti-loop budget.
	return outcome != AttemptOutcomeDefiniteNotSent
}

func latestCountedAttempt(history []CompactionRecord) time.Time {
	var latest time.Time
	for _, record := range history {
		if attemptCountsForAntiLoop(record.Outcome) && record.Timestamp.After(latest) {
			latest = record.Timestamp
		}
	}
	return latest
}

// CheckAntiLoop returns an error if compaction should be blocked, unless force is true.
func CheckAntiLoop(state *CompactionState, force bool) error {
	if force {
		return nil
	}
	now := time.Now()
	recent := recentCompactions(state.History, now)
	if recent >= MaxCompactionsPerWindow {
		return fmt.Errorf("session '%s' has reached maximum compactions in the last %s (%d/%d). Use --force to override",
			state.SessionName, CompactionWindow, recent, MaxCompactionsPerWindow)
	}
	lastCompaction := state.LastCompaction
	if latest := latestCountedAttempt(state.History); latest.After(lastCompaction) {
		lastCompaction = latest
	}
	if !lastCompaction.IsZero() {
		elapsed := now.Sub(lastCompaction)
		if elapsed < CooldownDuration {
			remaining := CooldownDuration - elapsed
			return fmt.Errorf("session '%s' was compacted %s ago (cooldown: %s, remaining: %s). Use --force to override",
				state.SessionName, elapsed.Round(time.Second), CooldownDuration, remaining.Round(time.Second))
		}
	}
	return nil
}

// RecordCompaction updates state after a successful compaction.
func RecordCompaction(state *CompactionState, promptFile string, forced bool) *CompactionState {
	now := time.Now()
	state.LastCompaction = now
	state.CompactionCount++
	state.History = append(state.History, CompactionRecord{
		AttemptID:        uuid.NewString(),
		Timestamp:        now,
		OutcomeUpdatedAt: now,
		Outcome:          AttemptOutcomeConfirmed,
		PromptFile:       promptFile,
		Forced:           forced,
	})
	return state
}
