package compaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
)

var storageKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// ErrAntiLoopRejected identifies a policy rejection from BeginAttempt. Storage,
// migration, and persistence failures do not wrap this sentinel.
var ErrAntiLoopRejected = errors.New("compaction anti-loop rejected")

// loadStateForSession reads compaction accounting by stable session ID. The
// caller supplies the current display name so a rename cannot split the
// anti-loop ledger. Callers that will mutate the returned state must already
// hold the stable-session lock through the corresponding save or Begin/Mark
// operation.
//
// A legacy display-name-keyed file is migrated only when both its embedded
// display name and stable ID match exactly. An ID-less file is ambiguous after
// name reuse and fails closed. Migration first moves a proven file to a
// stable-ID-keyed recovery path, preventing a crash from leaving the old name
// available to a replacement session.
func loadStateForSession(baseDir, sessionID, displayName string) (*CompactionState, error) {
	if err := validateStorageKey("session ID", sessionID); err != nil {
		return nil, err
	}
	if displayName == "" {
		return nil, fmt.Errorf("session display name is empty")
	}

	path := stableStateFile(baseDir, sessionID)
	state, err := loadStableState(path, sessionID, displayName)
	if err != nil || state != nil {
		return state, err
	}

	migrationPath, err := claimLegacyState(baseDir, sessionID, displayName, path)
	if err != nil {
		return nil, err
	}
	if migrationPath == "" {
		return newCompactionState(sessionID, displayName), nil
	}
	return finishLegacyMigration(baseDir, sessionID, displayName, migrationPath)
}

func loadStableState(path, sessionID, displayName string) (*CompactionState, error) {
	state, err := readCompactionState(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if state.SessionID == "" {
		// Path equality cannot prove ownership: a historical display name can
		// equal a later caller-controlled stable ID. Without an embedded stable
		// identity, name reuse remains ambiguous and must never authorize or
		// consume policy for the replacement.
		return nil, fmt.Errorf("compaction state %s has no stable session ID", path)
	}
	if err := validateLoadedState(state, sessionID); err != nil {
		return nil, fmt.Errorf("validate compaction state %s: %w", path, err)
	}
	state.SessionName = displayName
	return state, nil
}

func claimLegacyState(baseDir, sessionID, displayName, stablePath string) (string, error) {
	migrationPath := legacyMigrationFile(baseDir, sessionID)
	if _, migrationErr := os.Stat(migrationPath); migrationErr == nil {
		return migrationPath, nil
	} else if !errors.Is(migrationErr, os.ErrNotExist) {
		return "", fmt.Errorf("inspect compaction state migration %s: %w", migrationPath, migrationErr)
	}

	legacyPath, ok := legacyStatePath(baseDir, displayName, stablePath)
	if !ok {
		return "", nil
	}
	legacyState, err := readCompactionState(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := validateLegacyState(legacyState, sessionID, displayName); err != nil {
		if errors.Is(err, errLegacyStateIDLess) {
			// Every ledger written before the SessionID field existed is
			// ID-less by construction, and a display name is reusable, so this
			// file cannot be proven to belong to the session now asking for
			// it. Adopting it could hand a replacement session another
			// session's compaction history. Refusing it forever is not the
			// alternative: validation runs before the forceable anti-loop
			// check, so a hard error here permanently breaks compaction for
			// every upgraded session, `--force` included.
			//
			// Quarantine resolves both: the ambiguous history is never
			// consumed, the bytes are kept for inspection, and the caller
			// starts a fresh ledger. The cost is that such a session's
			// anti-loop budget restarts once, which is bounded and visible,
			// unlike a permanently unusable command.
			if qErr := quarantineAmbiguousLegacyState(legacyPath); qErr != nil {
				return "", qErr
			}
			return "", nil
		}
		return "", fmt.Errorf("refuse legacy compaction state migration %s: %w", legacyPath, err)
	}

	if err := os.Rename(legacyPath, migrationPath); err != nil {
		return "", fmt.Errorf("claim legacy compaction state %s: %w", legacyPath, err)
	}
	if err := fileutil.SyncDir(filepath.Dir(migrationPath)); err != nil {
		return "", fmt.Errorf("persist legacy compaction state claim %s: %w", migrationPath, err)
	}
	return migrationPath, nil
}

func legacyStatePath(baseDir, displayName, stablePath string) (string, bool) {
	// Display names are not storage identifiers. If the old name cannot be a
	// single safe path component, skip migration instead of probing a path
	// outside the ledger directory.
	if validateStorageKey("legacy session name", displayName) != nil {
		return "", false
	}
	legacyPath := stateFile(baseDir, displayName)
	return legacyPath, legacyPath != stablePath
}

func newCompactionState(sessionID, displayName string) *CompactionState {
	return &CompactionState{SessionID: sessionID, SessionName: displayName}
}

// saveStateForSession atomically replaces a stable-ID-keyed compaction ledger.
// The caller must hold the corresponding stable-session lock; atomic replacement
// prevents partial readers but intentionally does not merge concurrent writers.
func saveStateForSession(baseDir, sessionID string, state *CompactionState) error {
	if err := validateStorageKey("session ID", sessionID); err != nil {
		return err
	}
	if state == nil {
		return fmt.Errorf("compaction state is nil")
	}
	if state.SessionID != sessionID {
		return fmt.Errorf("compaction state identity drift: loaded %q, requested %q", state.SessionID, sessionID)
	}
	if state.SessionName == "" {
		return fmt.Errorf("session display name is empty")
	}
	if err := validateAttemptRecords(state.History); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal compaction state: %w", err)
	}
	data = append(data, '\n')
	if err := fileutil.AtomicWrite(stableStateFile(baseDir, sessionID), data, 0o600); err != nil {
		return fmt.Errorf("write compaction state: %w", err)
	}
	return nil
}

// Attempt is a stable-session-bound handle for one durably begun compaction.
// It deliberately does not expose storage identity or mutation ordering to
// callers; the caller keeps its stable-session lock until Mark returns.
type Attempt struct {
	baseDir     string
	sessionID   string
	displayName string
	record      CompactionRecord
}

// ID returns the persisted audit identifier for this attempt.
func (a *Attempt) ID() string {
	if a == nil {
		return ""
	}
	return a.record.AttemptID
}

// BeginAttempt checks the anti-loop policy and durably records a pending
// attempt before delivery. The caller must already hold the stable-session lock
// and keep it through delivery and Attempt.Mark. A crash after this returns
// leaves a pending attempt, which counts conservatively against the policy.
func BeginAttempt(baseDir, sessionID, displayName, promptFile string, forced bool) (*Attempt, error) {
	if promptFile == "" {
		return nil, fmt.Errorf("compaction prompt file is empty")
	}
	state, err := loadStateForSession(baseDir, sessionID, displayName)
	if err != nil {
		return nil, err
	}
	if err := CheckAntiLoop(state, forced); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAntiLoopRejected, err)
	}
	now := time.Now()
	attempt := CompactionRecord{
		AttemptID:        uuid.NewString(),
		Timestamp:        now,
		OutcomeUpdatedAt: now,
		Outcome:          AttemptOutcomePending,
		PromptFile:       promptFile,
		Forced:           forced,
	}
	state.History = append(state.History, attempt)
	if err := saveStateForSession(baseDir, sessionID, state); err != nil {
		return nil, fmt.Errorf("persist pending compaction attempt: %w", err)
	}
	return &Attempt{
		baseDir:     baseDir,
		sessionID:   sessionID,
		displayName: displayName,
		record:      attempt,
	}, nil
}

// Mark atomically persists the final accounting result for a pending
// attempt. Confirmed, uncertain, and still-pending attempts count against the
// anti-loop policy; only definite-not-sent releases the attempt. Repeating the
// same terminal mark is idempotent, while contradictory terminal transitions
// are rejected.
func (a *Attempt) Mark(outcome AttemptOutcome) error {
	if a == nil || a.record.AttemptID == "" {
		return fmt.Errorf("compaction attempt is empty")
	}
	if !isTerminalAttemptOutcome(outcome) {
		return fmt.Errorf("invalid terminal compaction attempt outcome %q", outcome)
	}
	state, err := loadStateForSession(a.baseDir, a.sessionID, a.displayName)
	if err != nil {
		return err
	}
	for i := range state.History {
		record := &state.History[i]
		if record.AttemptID != a.record.AttemptID {
			continue
		}
		if record.Outcome != AttemptOutcomePending {
			if record.Outcome != outcome {
				return fmt.Errorf("compaction attempt %q is already %q", a.record.AttemptID, record.Outcome)
			}
			// Persist a current display-name change even for an idempotent retry.
			if err := saveStateForSession(a.baseDir, a.sessionID, state); err != nil {
				return fmt.Errorf("persist compaction attempt: %w", err)
			}
			a.record = *record
			return nil
		}

		record.Outcome = outcome
		record.OutcomeUpdatedAt = time.Now()
		if outcome == AttemptOutcomeConfirmed || outcome == AttemptOutcomeUncertain {
			state.CompactionCount++
			if record.Timestamp.After(state.LastCompaction) {
				state.LastCompaction = record.Timestamp
			}
		}
		if err := saveStateForSession(a.baseDir, a.sessionID, state); err != nil {
			return fmt.Errorf("persist compaction attempt outcome: %w", err)
		}
		a.record = *record
		return nil
	}
	return fmt.Errorf("compaction attempt %q not found for stable session %q", a.record.AttemptID, a.sessionID)
}

func stableStateFile(baseDir, sessionID string) string {
	return filepath.Join(stateDir(baseDir), sessionID+".json")
}

// errLegacyStateIDLess marks a display-name-keyed legacy ledger with no
// embedded stable session ID. It is recoverable by quarantine rather than a
// hard failure; every other legacy validation failure stays fatal.
var errLegacyStateIDLess = errors.New("embedded stable session ID is empty and ownership is ambiguous")

// quarantineAmbiguousLegacyState moves an unattributable legacy ledger aside so
// it is never consumed and never re-examined, while keeping its bytes. The
// suffix search is bounded: a session that has quarantined this many ledgers
// has a problem no rename will fix.
func quarantineAmbiguousLegacyState(legacyPath string) error {
	for i := range 100 {
		target := legacyPath + ".ambiguous"
		if i > 0 {
			target = fmt.Sprintf("%s.ambiguous.%d", legacyPath, i)
		}
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect quarantine target %s: %w", target, err)
		}
		if err := os.Rename(legacyPath, target); err != nil {
			return fmt.Errorf("quarantine ambiguous compaction state %s: %w", legacyPath, err)
		}
		if err := fileutil.SyncDir(filepath.Dir(target)); err != nil {
			return fmt.Errorf("persist quarantine of %s: %w", legacyPath, err)
		}
		return nil
	}
	return fmt.Errorf("quarantine ambiguous compaction state %s: too many quarantined ledgers", legacyPath)
}

func legacyMigrationFile(baseDir, sessionID string) string {
	return stableStateFile(baseDir, sessionID) + ".legacy-migration"
}

func readCompactionState(path string) (*CompactionState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("read compaction state %s: %w", path, err)
	}
	var state CompactionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse compaction state %s: %w", path, err)
	}
	return &state, nil
}

func finishLegacyMigration(baseDir, sessionID, displayName, migrationPath string) (*CompactionState, error) {
	state, err := readCompactionState(migrationPath)
	if err != nil {
		return nil, err
	}
	if err := validateClaimedLegacyState(state, sessionID); err != nil {
		return nil, fmt.Errorf("refuse claimed legacy compaction state migration %s: %w", migrationPath, err)
	}
	state.SessionID = sessionID
	state.SessionName = displayName
	if err := saveStateForSession(baseDir, sessionID, state); err != nil {
		return nil, fmt.Errorf("publish migrated compaction state: %w", err)
	}
	if err := os.Remove(migrationPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove migrated compaction state claim %s: %w", migrationPath, err)
	}
	if err := fileutil.SyncDir(filepath.Dir(migrationPath)); err != nil {
		return nil, fmt.Errorf("persist migrated compaction state claim removal %s: %w", migrationPath, err)
	}
	return state, nil
}

// validateClaimedLegacyState validates a claim file, which lives at a path
// already keyed by the stable session ID (legacyMigrationFile). Ownership is
// therefore established by the path, not by the bytes, so an empty embedded ID
// is accepted here: a claim file is a legacy file that has been renamed but not
// yet stamped, and every state file written before the SessionID field existed
// is ID-less by construction. Requiring a non-empty ID would make crash
// recovery impossible for exactly the files the claim step exists to recover -
// the process would refuse its own half-finished migration forever. A
// non-empty ID that disagrees with the path is still a hard error.
func validateClaimedLegacyState(state *CompactionState, sessionID string) error {
	if state.SessionID != "" && state.SessionID != sessionID {
		return fmt.Errorf("embedded stable session ID %q does not match %q", state.SessionID, sessionID)
	}
	if state.SessionName == "" {
		return fmt.Errorf("embedded display name is empty")
	}
	return validateAttemptRecords(state.History)
}

// validateLegacyState validates a legacy file found at a display-name-keyed
// path, before it is claimed. Here an empty embedded ID IS ambiguous: display
// names are reusable, so an ID-less file at that path cannot be proven to
// belong to the session now asking for it, and adopting it would let a
// replacement session inherit another session's compaction history.
func validateLegacyState(state *CompactionState, sessionID, displayName string) error {
	if state.SessionID == "" {
		return errLegacyStateIDLess
	}
	if err := validateClaimedLegacyState(state, sessionID); err != nil {
		return err
	}
	if state.SessionName != displayName {
		return fmt.Errorf("embedded display name %q does not match %q", state.SessionName, displayName)
	}
	return nil
}

func validateLoadedState(state *CompactionState, sessionID string) error {
	if state.SessionID != sessionID {
		return fmt.Errorf("embedded stable session ID %q does not match %q", state.SessionID, sessionID)
	}
	if state.SessionName == "" {
		return fmt.Errorf("embedded display name is empty")
	}
	return validateAttemptRecords(state.History)
}

func validateAttemptRecords(history []CompactionRecord) error {
	seen := make(map[string]struct{}, len(history))
	for i, record := range history {
		if record.Timestamp.IsZero() {
			return fmt.Errorf("compaction history[%d] has no attempt timestamp", i)
		}
		switch record.Outcome {
		case "":
			if record.AttemptID != "" {
				return fmt.Errorf("compaction history[%d] has attempt ID %q without an outcome", i, record.AttemptID)
			}
		case AttemptOutcomePending, AttemptOutcomeConfirmed, AttemptOutcomeUncertain, AttemptOutcomeDefiniteNotSent:
			if record.AttemptID == "" {
				return fmt.Errorf("compaction history[%d] outcome %q has no attempt ID", i, record.Outcome)
			}
			if record.OutcomeUpdatedAt.IsZero() {
				return fmt.Errorf("compaction history[%d] outcome %q has no outcome timestamp", i, record.Outcome)
			}
			if record.PromptFile == "" {
				return fmt.Errorf("compaction history[%d] outcome %q has no prompt audit path", i, record.Outcome)
			}
		default:
			return fmt.Errorf("compaction history[%d] has unknown outcome %q", i, record.Outcome)
		}
		if record.AttemptID == "" {
			continue
		}
		if _, exists := seen[record.AttemptID]; exists {
			return fmt.Errorf("compaction history has duplicate attempt ID %q", record.AttemptID)
		}
		seen[record.AttemptID] = struct{}{}
	}
	return nil
}

func isTerminalAttemptOutcome(outcome AttemptOutcome) bool {
	return outcome == AttemptOutcomeConfirmed ||
		outcome == AttemptOutcomeUncertain ||
		outcome == AttemptOutcomeDefiniteNotSent
}

func validateStorageKey(label, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", label)
	}
	if len(value) > 100 {
		return fmt.Errorf("%s is too long (max 100 characters)", label)
	}
	if value == "." || value == ".." || !storageKeyPattern.MatchString(value) {
		return fmt.Errorf("invalid %s %q", label, value)
	}
	return nil
}
