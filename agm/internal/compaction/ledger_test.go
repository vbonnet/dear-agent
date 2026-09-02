package compaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadStateForSessionMigratesLegacyNameKeyToStableID(t *testing.T) {
	baseDir := t.TempDir()
	legacy := &CompactionState{
		SessionID:   "stable-123",
		SessionName: "old-display",
		History: []CompactionRecord{{
			Timestamp:  time.Now().Add(-3 * time.Hour),
			PromptFile: "legacy-prompt.md",
		}},
	}
	if err := SaveState(baseDir, legacy); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	state, err := loadStateForSession(baseDir, "stable-123", "old-display")
	if err != nil {
		t.Fatalf("LoadStateForSession() error = %v", err)
	}
	if state.SessionID != "stable-123" || state.SessionName != "old-display" {
		t.Fatalf("identity = (%q, %q), want (%q, %q)", state.SessionID, state.SessionName, "stable-123", "old-display")
	}
	if len(state.History) != 1 || state.History[0].PromptFile != "legacy-prompt.md" {
		t.Fatalf("migrated history = %+v", state.History)
	}
	if _, err := os.Stat(stateFile(baseDir, "old-display")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy file still available after migration: %v", err)
	}
	if _, err := os.Stat(stableStateFile(baseDir, "stable-123")); err != nil {
		t.Fatalf("stable state file missing: %v", err)
	}
	if _, err := os.Stat(legacyMigrationFile(baseDir, "stable-123")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("migration claim still present: %v", err)
	}
}

func TestLoadStateForSessionRecoversClaimedLegacyMigration(t *testing.T) {
	baseDir := t.TempDir()
	legacy := &CompactionState{SessionID: "stable-id", SessionName: "display"}
	if err := SaveState(baseDir, legacy); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := os.Rename(stateFile(baseDir, "display"), legacyMigrationFile(baseDir, "stable-id")); err != nil {
		t.Fatalf("simulate interrupted migration: %v", err)
	}

	state, err := loadStateForSession(baseDir, "stable-id", "renamed-after-crash")
	if err != nil {
		t.Fatalf("LoadStateForSession() recovery error = %v", err)
	}
	if state.SessionID != "stable-id" {
		t.Fatalf("SessionID = %q, want stable-id", state.SessionID)
	}
	if state.SessionName != "renamed-after-crash" {
		t.Fatalf("SessionName = %q, want renamed-after-crash", state.SessionName)
	}
	if _, err := os.Stat(stableStateFile(baseDir, "stable-id")); err != nil {
		t.Fatalf("recovered stable state missing: %v", err)
	}
}

func TestLoadStateForSessionRejectsAmbiguousIDLessLegacyState(t *testing.T) {
	baseDir := t.TempDir()
	legacy := &CompactionState{
		SessionName:     "reused-name",
		CompactionCount: 2,
	}
	if err := SaveState(baseDir, legacy); err != nil {
		t.Fatal(err)
	}
	legacyPath := stateFile(baseDir, "reused-name")

	_, err := loadStateForSession(baseDir, "replacement-id", "reused-name")
	if err == nil || !strings.Contains(err.Error(), "ownership is ambiguous") {
		t.Fatalf("loadStateForSession() error = %v, want ambiguous ownership", err)
	}
	if _, statErr := os.Stat(legacyPath); statErr != nil {
		t.Fatalf("ambiguous legacy file was mutated: %v", statErr)
	}
	if _, statErr := os.Stat(stableStateFile(baseDir, "replacement-id")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("replacement stable ledger was created from ambiguous state: %v", statErr)
	}
}

func TestLoadStateForSessionRejectsIDLessLedgerWhenNameEqualsStableID(t *testing.T) {
	baseDir := t.TempDir()
	legacy := &CompactionState{
		SessionName:     "same-name-and-id",
		CompactionCount: 2,
	}
	if err := SaveState(baseDir, legacy); err != nil {
		t.Fatal(err)
	}
	path := stateFile(baseDir, legacy.SessionName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, err = loadStateForSession(baseDir, legacy.SessionName, legacy.SessionName)
	if err == nil || !strings.Contains(err.Error(), "has no stable session ID") {
		t.Fatalf("loadStateForSession() error = %v, want missing stable identity", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ambiguous same-path ledger was removed: %v", readErr)
	}
	if string(after) != string(before) {
		t.Fatalf("ambiguous same-path ledger was mutated:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestLoadStateForSessionCarriesHistoryAcrossDisplayRename(t *testing.T) {
	baseDir := t.TempDir()
	state := &CompactionState{
		SessionID:   "stable-id",
		SessionName: "before-rename",
		History: []CompactionRecord{{
			Timestamp:  time.Now().Add(-3 * time.Hour),
			PromptFile: "prompt.md",
		}},
	}
	if err := saveStateForSession(baseDir, "stable-id", state); err != nil {
		t.Fatalf("SaveStateForSession() error = %v", err)
	}

	loaded, err := loadStateForSession(baseDir, "stable-id", "after-rename")
	if err != nil {
		t.Fatalf("LoadStateForSession() error = %v", err)
	}
	if loaded.SessionName != "after-rename" || len(loaded.History) != 1 {
		t.Fatalf("renamed state = %+v", loaded)
	}
	if err := saveStateForSession(baseDir, "stable-id", loaded); err != nil {
		t.Fatalf("persist renamed display: %v", err)
	}
	reloaded, err := loadStateForSession(baseDir, "stable-id", "after-rename")
	if err != nil {
		t.Fatalf("reload renamed state: %v", err)
	}
	if reloaded.SessionName != "after-rename" || len(reloaded.History) != 1 {
		t.Fatalf("reloaded renamed state = %+v", reloaded)
	}
}

func TestLoadStateForSessionRejectsEmbeddedStableIdentityDrift(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(stateDir(baseDir), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(&CompactionState{SessionID: "different-id", SessionName: "display"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stableStateFile(baseDir, "requested-id"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = loadStateForSession(baseDir, "requested-id", "display")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("LoadStateForSession() error = %v, want identity mismatch", err)
	}
}

func TestLoadStateForSessionRejectsLegacyClaimedByAnotherID(t *testing.T) {
	baseDir := t.TempDir()
	legacy := &CompactionState{SessionID: "prior-owner", SessionName: "reused-name"}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir(baseDir), 0o700); err != nil {
		t.Fatal(err)
	}
	legacyPath := stateFile(baseDir, "reused-name")
	if err := os.WriteFile(legacyPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = loadStateForSession(baseDir, "replacement-id", "reused-name")
	if err == nil || !strings.Contains(err.Error(), "prior-owner") {
		t.Fatalf("LoadStateForSession() error = %v, want prior-owner rejection", err)
	}
	if _, statErr := os.Stat(legacyPath); statErr != nil {
		t.Fatalf("unsafe legacy claim was mutated: %v", statErr)
	}
}

func TestLoadStateForSessionDoesNotProbeUnsafeLegacyPath(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "base")
	outsidePath := filepath.Join(root, "outside.json")
	outside := []byte(`{"session_name":"../../outside","compaction_count":99}`)
	if err := os.WriteFile(outsidePath, outside, 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := loadStateForSession(baseDir, "stable-id", "../../outside")
	if err != nil {
		t.Fatalf("LoadStateForSession() error = %v", err)
	}
	if state.CompactionCount != 0 || state.SessionID != "stable-id" {
		t.Fatalf("unsafe legacy path influenced state: %+v", state)
	}
	got, err := os.ReadFile(outsidePath)
	if err != nil || string(got) != string(outside) {
		t.Fatalf("outside file changed: data=%q err=%v", got, err)
	}
}

func TestSaveStateForSessionRejectsIdentityDrift(t *testing.T) {
	err := saveStateForSession(t.TempDir(), "requested-id", &CompactionState{
		SessionID:   "different-id",
		SessionName: "display",
	})
	if err == nil || !strings.Contains(err.Error(), "identity drift") {
		t.Fatalf("SaveStateForSession() error = %v, want identity drift", err)
	}
}

func TestSaveStateForSessionAtomicReplacementSurvivesConcurrentReads(t *testing.T) {
	baseDir := t.TempDir()
	initial := &CompactionState{SessionID: "stable-id", SessionName: "display"}
	if err := saveStateForSession(baseDir, "stable-id", initial); err != nil {
		t.Fatal(err)
	}

	const writes = 100
	stop := make(chan struct{})
	errs := make(chan error, writes)
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				state, err := loadStateForSession(baseDir, "stable-id", "display")
				if err != nil {
					errs <- err
					return
				}
				if state.SessionID != "stable-id" {
					errs <- fmt.Errorf("read torn identity %q", state.SessionID)
					return
				}
			}
		})
	}
	for i := range writes {
		state := &CompactionState{
			SessionID:       "stable-id",
			SessionName:     "display",
			CompactionCount: i,
			History: []CompactionRecord{{
				Timestamp:  time.Now().Add(-3 * time.Hour),
				PromptFile: strings.Repeat("x", i+1),
			}},
		}
		if err := saveStateForSession(baseDir, "stable-id", state); err != nil {
			close(stop)
			readers.Wait()
			t.Fatalf("SaveStateForSession() write %d error = %v", i, err)
		}
	}
	close(stop)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent read failed: %v", err)
	}
}

func TestBeginAttemptPersistsPendingBeforeDelivery(t *testing.T) {
	baseDir := t.TempDir()
	attempt, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false)
	if err != nil {
		t.Fatalf("BeginAttempt() error = %v", err)
	}
	if attempt.ID() == "" || attempt.record.Outcome != AttemptOutcomePending {
		t.Fatalf("attempt = %+v", attempt.record)
	}
	state, err := loadStateForSession(baseDir, "stable-id", "display")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.History) != 1 || state.History[0].Outcome != AttemptOutcomePending {
		t.Fatalf("persisted history = %+v", state.History)
	}
	if err := CheckAntiLoop(state, false); err == nil {
		t.Fatal("pending attempt did not block the cooldown")
	}
}

func TestBeginAttemptDistinguishesPolicyRejectionFromLedgerFailure(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginAttempt(baseDir, "stable-id", "display", "prompt-2.md", false); !errors.Is(err, ErrAntiLoopRejected) {
		t.Fatalf("second BeginAttempt() error = %v, want ErrAntiLoopRejected", err)
	}

	corruptBase := t.TempDir()
	if err := os.MkdirAll(stateDir(corruptBase), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stableStateFile(corruptBase, "stable-id"), []byte(`{"session_id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginAttempt(corruptBase, "stable-id", "display", "prompt.md", false); err == nil || errors.Is(err, ErrAntiLoopRejected) {
		t.Fatalf("corrupt-ledger BeginAttempt() error = %v, want non-policy failure", err)
	}
}

func TestValidateAttemptRecordsRejectsMalformedAccounting(t *testing.T) {
	now := time.Now()
	valid := CompactionRecord{
		AttemptID:        "attempt-id",
		Timestamp:        now,
		OutcomeUpdatedAt: now,
		Outcome:          AttemptOutcomePending,
		PromptFile:       "prompt.md",
	}
	tests := []struct {
		name   string
		mutate func(*CompactionRecord)
	}{
		{name: "missing attempt timestamp", mutate: func(record *CompactionRecord) { record.Timestamp = time.Time{} }},
		{name: "missing outcome timestamp", mutate: func(record *CompactionRecord) { record.OutcomeUpdatedAt = time.Time{} }},
		{name: "missing prompt audit", mutate: func(record *CompactionRecord) { record.PromptFile = "" }},
		{name: "unknown outcome", mutate: func(record *CompactionRecord) { record.Outcome = AttemptOutcome("future") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := valid
			test.mutate(&record)
			if err := validateAttemptRecords([]CompactionRecord{record}); err == nil {
				t.Fatalf("validateAttemptRecords(%+v) error = nil, want fail-closed rejection", record)
			}
		})
	}
}

func TestAttemptOutcomesCountConservativelyForAntiLoop(t *testing.T) {
	now := time.Now()
	state := &CompactionState{
		SessionID:   "stable-id",
		SessionName: "display",
		History: []CompactionRecord{
			{AttemptID: "pending", Timestamp: now.Add(-4 * time.Hour), Outcome: AttemptOutcomePending},
			{AttemptID: "confirmed", Timestamp: now.Add(-3 * time.Hour), Outcome: AttemptOutcomeConfirmed},
			{AttemptID: "uncertain", Timestamp: now.Add(-2 * time.Hour), Outcome: AttemptOutcomeUncertain},
			{AttemptID: "not-sent", Timestamp: now.Add(-1 * time.Hour), Outcome: AttemptOutcomeDefiniteNotSent},
		},
	}
	if got := recentCompactions(state.History, now); got != 3 {
		t.Fatalf("recentCompactions() = %d, want 3", got)
	}
	if err := CheckAntiLoop(state, false); err == nil {
		t.Fatal("pending, confirmed, and uncertain attempts did not exhaust the window")
	}
}

func TestMarkAttemptDefiniteNotSentReleasesAntiLoop(t *testing.T) {
	baseDir := t.TempDir()
	attempt, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Mark(AttemptOutcomeDefiniteNotSent); err != nil {
		t.Fatalf("Attempt.Mark() error = %v", err)
	}
	if attempt.record.Outcome != AttemptOutcomeDefiniteNotSent {
		t.Fatalf("outcome = %q", attempt.record.Outcome)
	}
	state, err := loadStateForSession(baseDir, "stable-id", "display")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckAntiLoop(state, false); err != nil {
		t.Fatalf("definite-not-sent attempt still counted: %v", err)
	}
}

func TestMarkAttemptConfirmedIsIdempotent(t *testing.T) {
	baseDir := t.TempDir()
	attempt, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := attempt.Mark(AttemptOutcomeConfirmed); err != nil {
			t.Fatalf("Attempt.Mark() error = %v", err)
		}
	}
	state, err := loadStateForSession(baseDir, "stable-id", "display")
	if err != nil {
		t.Fatal(err)
	}
	if state.CompactionCount != 1 || state.SessionName != "display" {
		t.Fatalf("state after idempotent mark = %+v", state)
	}
}

func TestAttemptMarkUncertainUpdatesCountedAttemptSummary(t *testing.T) {
	baseDir := t.TempDir()
	attempt, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Mark(AttemptOutcomeUncertain); err != nil {
		t.Fatalf("Attempt.Mark(uncertain) error = %v", err)
	}
	state, err := loadStateForSession(baseDir, "stable-id", "display")
	if err != nil {
		t.Fatal(err)
	}
	if state.CompactionCount != 1 {
		t.Fatalf("CompactionCount = %d, want 1 for uncertain attempt", state.CompactionCount)
	}
	if state.LastCompaction.IsZero() {
		t.Fatal("LastCompaction is zero for uncertain attempt")
	}
	if err := CheckAntiLoop(state, false); err == nil {
		t.Fatal("uncertain attempt did not remain in anti-loop cooldown")
	}
}

func TestMarkAttemptRejectsContradictoryTerminalOutcome(t *testing.T) {
	baseDir := t.TempDir()
	attempt, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := attempt.Mark(AttemptOutcomeUncertain); err != nil {
		t.Fatal(err)
	}
	err = attempt.Mark(AttemptOutcomeConfirmed)
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("contradictory MarkAttempt() error = %v", err)
	}
}

func TestBeginAttemptFailsClosedOnCorruptLedger(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(stateDir(baseDir), 0o700); err != nil {
		t.Fatal(err)
	}
	path := stableStateFile(baseDir, "stable-id")
	corrupt := []byte(`{"session_id":`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false); err == nil {
		t.Fatal("BeginAttempt() replaced a corrupt ledger")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(corrupt) {
		t.Fatalf("corrupt ledger was changed: data=%q err=%v", got, err)
	}
}

func TestMarkAttemptWriteFailureLeavesPendingFailClosed(t *testing.T) {
	baseDir := t.TempDir()
	attempt, err := BeginAttempt(baseDir, "stable-id", "display", "prompt.md", false)
	if err != nil {
		t.Fatal(err)
	}
	dir := stateDir(baseDir)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := attempt.Mark(AttemptOutcomeConfirmed); err == nil {
		t.Fatal("MarkAttempt() unexpectedly succeeded in read-only ledger directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	state, err := loadStateForSession(baseDir, "stable-id", "display")
	if err != nil {
		t.Fatal(err)
	}
	if state.History[0].Outcome != AttemptOutcomePending {
		t.Fatalf("failed mark changed persisted outcome to %q", state.History[0].Outcome)
	}
	if err := CheckAntiLoop(state, false); err == nil {
		t.Fatal("pending attempt after failed accounting did not remain fail-closed")
	}
}

// A crash between claiming a legacy file and stamping its identity leaves an
// ID-less claim file at the ID-keyed migration path. Every state file written
// before the SessionID field existed is ID-less, so this is the ordinary
// recovery case, not an exotic one. The claim path proves ownership by itself,
// so recovery must complete rather than refusing the process's own
// half-finished migration forever.
func TestLoadStateForSessionRecoversIDLessClaimedLegacyMigration(t *testing.T) {
	baseDir := t.TempDir()
	legacy := &CompactionState{
		SessionName:     "display",
		CompactionCount: 3,
		History: []CompactionRecord{{
			Timestamp:  time.Now().Add(-2 * time.Hour),
			PromptFile: "legacy-prompt.md",
		}},
	}
	if err := SaveState(baseDir, legacy); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := os.Rename(stateFile(baseDir, "display"), legacyMigrationFile(baseDir, "stable-id")); err != nil {
		t.Fatalf("simulate crash after claim, before stamp: %v", err)
	}

	state, err := loadStateForSession(baseDir, "stable-id", "display")
	if err != nil {
		t.Fatalf("loadStateForSession() error = %v, want recovery of the ID-less claim", err)
	}
	if state.SessionID != "stable-id" {
		t.Errorf("SessionID = %q, want stable-id: recovery must stamp the identity", state.SessionID)
	}
	if state.CompactionCount != 3 || len(state.History) != 1 {
		t.Errorf("recovered state = %+v, want the legacy count and history preserved", state)
	}
	if _, err := os.Stat(legacyMigrationFile(baseDir, "stable-id")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("claim file still present after recovery: %v", err)
	}
	if _, err := os.Stat(stableStateFile(baseDir, "stable-id")); err != nil {
		t.Errorf("recovered stable state missing: %v", err)
	}
}

// A claim file whose embedded ID disagrees with the ID-keyed path it sits at is
// still refused: the path proves ownership only when the bytes do not claim
// otherwise.
func TestLoadStateForSessionRejectsMismatchedClaimedLegacyMigration(t *testing.T) {
	baseDir := t.TempDir()
	legacy := &CompactionState{SessionID: "other-session", SessionName: "display"}
	if err := SaveState(baseDir, legacy); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if err := os.Rename(stateFile(baseDir, "display"), legacyMigrationFile(baseDir, "stable-id")); err != nil {
		t.Fatalf("stage mismatched claim: %v", err)
	}

	if _, err := loadStateForSession(baseDir, "stable-id", "display"); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("loadStateForSession() error = %v, want an identity mismatch refusal", err)
	}
}
