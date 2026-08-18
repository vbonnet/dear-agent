package mergeloop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PRRecord is the persisted per-PR state the loop carries across ticks.
type PRRecord struct {
	Number           int       `json:"number"`
	AgentAttempts    int       `json:"agent_attempts"`
	LastFailureSig   string    `json:"last_failure_sig,omitempty"`
	AgentSession     string    `json:"agent_session,omitempty"`
	LastActionAt     time.Time `json:"last_action_at"`
	LastState        State     `json:"last_state,omitempty"`
	FirstSeenAt      time.Time `json:"first_seen_at"`
	EscalatedAt      time.Time `json:"escalated_at,omitzero"`
	EscalationReason string    `json:"escalation_reason,omitempty"`
}

// Tracker persists per-PR attempt and timing state to a JSON file so the loop
// is stateful across the idempotent ticks that launchd invokes. It is safe for
// concurrent use within a process.
type Tracker struct {
	mu      sync.Mutex
	path    string
	repo    string
	records map[int]*PRRecord
}

// LoadTracker reads (or initializes) the tracker state for a repo. path is the
// JSON state file; an empty path uses the default under the dear-agent state
// dir. A missing file is not an error.
func LoadTracker(repo, path string) (*Tracker, error) {
	if path == "" {
		path = filepath.Join(StateDir(), "mergeloop-state.json")
	}
	t := &Tracker{path: path, repo: repo, records: map[int]*PRRecord{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return nil, fmt.Errorf("reading tracker state %s: %w", path, err)
	}
	var stored map[string]map[int]*PRRecord
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("parsing tracker state %s: %w", path, err)
	}
	if recs, ok := stored[repo]; ok && recs != nil {
		t.records = recs
	}
	return t, nil
}

// Get returns the record for a PR, creating a zero record (with FirstSeenAt set
// to now) on first sight.
func (t *Tracker) Get(num int, now time.Time) *PRRecord {
	t.mu.Lock()
	defer t.mu.Unlock()
	r, ok := t.records[num]
	if !ok {
		r = &PRRecord{Number: num, FirstSeenAt: now}
		t.records[num] = r
	}
	return r
}

// Attempts returns the recorded agent-fix attempt count for a PR.
func (t *Tracker) Attempts(num int) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r, ok := t.records[num]; ok {
		return r.AgentAttempts
	}
	return 0
}

// RecordAgentSpawn increments the attempt count for a PR, records the failure
// signature that prompted it, and the spawned session name.
func (t *Tracker) RecordAgentSpawn(num int, failureSig, session string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.records[num]
	if r == nil {
		r = &PRRecord{Number: num, FirstSeenAt: now}
		t.records[num] = r
	}
	// Reset the counter when the failure signature changes — a different
	// failure is a fresh problem, not a stuck retry of the same one.
	if r.LastFailureSig != "" && r.LastFailureSig != failureSig {
		r.AgentAttempts = 0
	}
	r.AgentAttempts++
	r.LastFailureSig = failureSig
	r.AgentSession = session
	r.LastActionAt = now
}

// ResetAttemptsIfSigChanged zeroes the agent-attempt counter for a PR when the
// supplied failure signature differs from the one recorded at the last spawn.
// It is the pre-Classify guard against the StateAbandoned deadlock: a genuinely
// new failure (or a human's fresh push, which changes the failing-check set) is
// a new problem, not a continuation of an exhausted retry budget. Ownership of
// LastFailureSig stays with RecordAgentSpawn; this only resets the count.
func (t *Tracker) ResetAttemptsIfSigChanged(num int, sig string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.records[num]
	if r == nil {
		return
	}
	if r.LastFailureSig != "" && r.LastFailureSig != sig {
		r.AgentAttempts = 0
	}
}

// ResetAttemptsIfPassing zeroes the agent-attempt counter and clears the failure signature
// for a PR when CI passes without projection error, establishing that the prior failure
// episode has concluded.
func (t *Tracker) ResetAttemptsIfPassing(num int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.records[num]
	if r == nil {
		return
	}
	r.AgentAttempts = 0
	r.LastFailureSig = ""
}

// RecordAction stamps a PR's last-action time and state (used for stall
// detection). Call after any mechanical action (rebase, merge attempt).
func (t *Tracker) RecordAction(num int, state State, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.records[num]
	if r == nil {
		r = &PRRecord{Number: num, FirstSeenAt: now}
		t.records[num] = r
	}
	r.LastActionAt = now
	r.LastState = state
}

// RecordEscalation marks a PR as escalated to a human with a reason.
func (t *Tracker) RecordEscalation(num int, reason string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.records[num]
	if r == nil {
		r = &PRRecord{Number: num, FirstSeenAt: now}
		t.records[num] = r
	}
	r.EscalatedAt = now
	r.EscalationReason = reason
}

// Forget drops a PR's record (e.g. after it merges).
func (t *Tracker) Forget(num int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.records, num)
}

// Save persists the tracker state atomically (write-temp-then-rename).
func (t *Tracker) Save() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return fmt.Errorf("creating tracker state dir: %w", err)
	}
	// Read-modify-write the multi-repo map so concurrent repos don't clobber.
	// A parse error must NOT be swallowed: silently continuing would write back
	// only this repo's records and clobber every other repo's state. Surface it
	// so the caller can preserve the existing (corrupt-but-recoverable) file.
	stored := map[string]map[int]*PRRecord{}
	if data, err := os.ReadFile(t.path); err == nil && len(data) > 0 {
		if err := json.Unmarshal(data, &stored); err != nil {
			return fmt.Errorf("parsing existing tracker state %s: %w", t.path, err)
		}
	}
	stored[t.repo] = t.records
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tracker state: %w", err)
	}
	tmp := t.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing tracker state: %w", err)
	}
	if err := os.Rename(tmp, t.path); err != nil {
		return fmt.Errorf("renaming tracker state: %w", err)
	}
	return nil
}

// StateDir returns the dear-agent state directory, honoring the same override
// (DEAR_AGENT_STATE_DIR) and default as the other safe-* tools.
func StateDir() string {
	if d := os.Getenv("DEAR_AGENT_STATE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "dear-agent")
	}
	return filepath.Join(home, ".local", "state", "dear-agent")
}
