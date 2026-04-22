package ops

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/engram/errormemory"
)

func TestRecordErrorMemory(t *testing.T) {
	// Use a temp dir for the store to avoid polluting the real DB
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-error-memory.jsonl")

	// Override the store path by calling Upsert directly (same logic as recordErrorMemory)
	store := errormemory.NewStore(dbPath)
	now := time.Now()
	rec := errormemory.ErrorRecord{
		Pattern:       "test stall pattern",
		ErrorCategory: ErrMemCatStall,
		CommandSample: "",
		Remediation:   "test remediation",
		Count:         1,
		FirstSeen:     now,
		LastSeen:      now,
		SessionIDs:    []string{"test-session-1"},
		Source:        SourceAGMStall,
	}

	result, err := store.Upsert(rec)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if result.Count != 1 {
		t.Errorf("expected count 1, got %d", result.Count)
	}
	if result.Pattern != "test stall pattern" {
		t.Errorf("expected pattern 'test stall pattern', got %q", result.Pattern)
	}
	if result.ErrorCategory != ErrMemCatStall {
		t.Errorf("expected category %q, got %q", ErrMemCatStall, result.ErrorCategory)
	}
	if result.Source != SourceAGMStall {
		t.Errorf("expected source %q, got %q", SourceAGMStall, result.Source)
	}

	// Upsert same pattern — count should increment
	rec2 := errormemory.ErrorRecord{
		Pattern:       "test stall pattern",
		ErrorCategory: ErrMemCatStall,
		CommandSample: "updated command",
		Remediation:   "test remediation",
		Count:         1,
		FirstSeen:     now,
		LastSeen:      now.Add(time.Hour),
		SessionIDs:    []string{"test-session-2"},
		Source:        SourceAGMStall,
	}

	result2, err := store.Upsert(rec2)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if result2.Count != 2 {
		t.Errorf("expected count 2 after second upsert, got %d", result2.Count)
	}
	if result2.CommandSample != "updated command" {
		t.Errorf("expected updated command sample, got %q", result2.CommandSample)
	}

	// Verify both session IDs are tracked
	if len(result2.SessionIDs) != 2 {
		t.Errorf("expected 2 session IDs, got %d: %v", len(result2.SessionIDs), result2.SessionIDs)
	}

	// Verify the DB file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected DB file to exist")
	}
}

func TestRecordErrorMemoryAllCategories(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-error-memory.jsonl")
	store := errormemory.NewStore(dbPath)
	now := time.Now()

	categories := []struct {
		category string
		source   string
	}{
		{ErrMemCatPermissionPrompt, SourceAGMStall},
		{ErrMemCatStall, SourceAGMStall},
		{ErrMemCatErrorLoop, SourceAGMStall},
		{ErrMemCatQualityGate, SourceAGMQualityGate},
		{ErrMemCatFalseCompletion, SourceAGMTrust},
		{ErrMemCatSessionDown, SourceAGMCrossCheck},
		{ErrMemCatEnterBug, SourceAGMCrossCheck},
		{ErrMemCatBuildFailure, SourceAGMQualityGate},
	}

	for _, tc := range categories {
		rec := errormemory.ErrorRecord{
			Pattern:       "test pattern for " + tc.category,
			ErrorCategory: tc.category,
			Remediation:   "test fix for " + tc.category,
			Count:         1,
			FirstSeen:     now,
			LastSeen:      now,
			SessionIDs:    []string{"test-session"},
			Source:        tc.source,
		}

		_, err := store.Upsert(rec)
		if err != nil {
			t.Errorf("Upsert failed for category %q: %v", tc.category, err)
		}
	}

	// Verify all records were stored
	records, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(records) != len(categories) {
		t.Errorf("expected %d records, got %d", len(categories), len(records))
	}
}

func TestRecordErrorMemoryConstants(t *testing.T) {
	// Verify constants are non-empty and distinct
	sources := []string{
		SourceAGMStall,
		SourceAGMQualityGate,
		SourceAGMTrust,
		SourceAGMArchive,
		SourceAGMCrossCheck,
	}
	seen := make(map[string]bool)
	for _, s := range sources {
		if s == "" {
			t.Error("source constant is empty")
		}
		if seen[s] {
			t.Errorf("duplicate source constant: %q", s)
		}
		seen[s] = true
	}

	categories := []string{
		ErrMemCatPermissionPrompt,
		ErrMemCatStall,
		ErrMemCatErrorLoop,
		ErrMemCatQualityGate,
		ErrMemCatFalseCompletion,
		ErrMemCatSessionDown,
		ErrMemCatEnterBug,
		ErrMemCatBuildFailure,
	}
	seen = make(map[string]bool)
	for _, c := range categories {
		if c == "" {
			t.Error("category constant is empty")
		}
		if seen[c] {
			t.Errorf("duplicate category constant: %q", c)
		}
		seen[c] = true
	}
}
