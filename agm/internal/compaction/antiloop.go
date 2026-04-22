package compaction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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
	SessionName     string             `json:"session_name"`
	LastCompaction  time.Time          `json:"last_compaction"`
	CompactionCount int                `json:"compaction_count"`
	History         []CompactionRecord `json:"history"`
}

// CompactionRecord is a single compaction event.
type CompactionRecord struct {
	Timestamp  time.Time `json:"timestamp"`
	PromptFile string    `json:"prompt_file"`
	Forced     bool      `json:"forced"`
}

// stateDir returns the compaction-state directory under baseDir.
func stateDir(baseDir string) string {
	return filepath.Join(baseDir, "compaction-state")
}

// stateFile returns the path to a session's compaction state file.
func stateFile(baseDir, sessionName string) string {
	return filepath.Join(stateDir(baseDir), sessionName+".json")
}

// LoadState reads compaction state for a session. Returns zero-value state if file does not exist.
func LoadState(baseDir, sessionName string) (*CompactionState, error) {
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

// SaveState writes compaction state to disk.
func SaveState(baseDir string, state *CompactionState) error {
	dir := stateDir(baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create compaction-state dir: %w", err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal compaction state: %w", err)
	}
	return os.WriteFile(stateFile(baseDir, state.SessionName), data, 0o644)
}

// recentCompactions counts compactions within the rolling window.
func recentCompactions(history []CompactionRecord, now time.Time) int {
	cutoff := now.Add(-CompactionWindow)
	count := 0
	for _, r := range history {
		if r.Timestamp.After(cutoff) {
			count++
		}
	}
	return count
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
	if !state.LastCompaction.IsZero() {
		elapsed := now.Sub(state.LastCompaction)
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
		Timestamp:  now,
		PromptFile: promptFile,
		Forced:     forced,
	})
	return state
}
