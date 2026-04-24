package errormemory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	s := NewStore("~/test/path.jsonl")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, "test/path.jsonl")
	if s.Path() != expected {
		t.Errorf("expected path %q, got %q", expected, s.Path())
	}
}

func TestLoadEmpty(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "nonexistent.jsonl"))
	records, err := s.Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected empty slice, got %d records", len(records))
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "test.jsonl"))

	now := time.Now().Truncate(time.Second)
	records := []ErrorRecord{
		{
			ID:            "abc123",
			Pattern:       "cd command",
			ErrorCategory: "bash-blocker",
			CommandSample: "cd /tmp && go test",
			Remediation:   "Use absolute paths",
			Count:         5,
			FirstSeen:     now.Add(-24 * time.Hour),
			LastSeen:      now,
			TTLExpiry:     now.Add(DefaultTTL),
			SessionIDs:    []string{"s1", "s2"},
			Source:        SourceBashBlocker,
		},
		{
			ID:            "def456",
			Pattern:       "ls command",
			ErrorCategory: "bash-blocker",
			CommandSample: "ls -la",
			Remediation:   "Use Glob tool",
			Count:         3,
			FirstSeen:     now.Add(-12 * time.Hour),
			LastSeen:      now,
			TTLExpiry:     now.Add(DefaultTTL),
			SessionIDs:    []string{"s3"},
			Source:        SourceBashBlocker,
		},
	}

	if err := s.Save(records); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded) != len(records) {
		t.Fatalf("expected %d records, got %d", len(records), len(loaded))
	}

	for i, rec := range loaded {
		if rec.ID != records[i].ID {
			t.Errorf("record %d: expected ID %q, got %q", i, records[i].ID, rec.ID)
		}
		if rec.Pattern != records[i].Pattern {
			t.Errorf("record %d: expected Pattern %q, got %q", i, records[i].Pattern, rec.Pattern)
		}
		if rec.Count != records[i].Count {
			t.Errorf("record %d: expected Count %d, got %d", i, records[i].Count, rec.Count)
		}
	}
}

func TestUpsert(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "test.jsonl"))

	now := time.Now().Truncate(time.Second)

	// Insert new record
	rec := ErrorRecord{
		Pattern:       "cd command",
		ErrorCategory: "bash-blocker",
		CommandSample: "cd /tmp",
		Remediation:   "Use absolute paths",
		Count:         1,
		FirstSeen:     now,
		LastSeen:      now,
		SessionIDs:    []string{"s1"},
		Source:        SourceBashBlocker,
	}

	result, err := s.Upsert(rec)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if result.Count != 1 {
		t.Errorf("expected count 1, got %d", result.Count)
	}
	if result.ID == "" {
		t.Error("expected non-empty ID")
	}

	// Upsert same pattern — should increment count
	later := now.Add(1 * time.Hour)
	rec2 := ErrorRecord{
		Pattern:       "cd command",
		ErrorCategory: "bash-blocker",
		CommandSample: "cd /var",
		Remediation:   "Use absolute paths",
		Count:         1,
		FirstSeen:     later,
		LastSeen:      later,
		SessionIDs:    []string{"s2"},
		Source:        SourceBashBlocker,
	}

	result2, err := s.Upsert(rec2)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if result2.Count != 2 {
		t.Errorf("expected count 2, got %d", result2.Count)
	}
	if !result2.LastSeen.Equal(later) {
		t.Errorf("expected LastSeen %v, got %v", later, result2.LastSeen)
	}

	// Verify only one record in store
	records, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func TestUpsertSessionIDCap(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "test.jsonl"))

	now := time.Now().Truncate(time.Second)

	// Insert with 4 session IDs
	rec := ErrorRecord{
		Pattern:       "grep command",
		ErrorCategory: "bash-blocker",
		Count:         1,
		FirstSeen:     now,
		LastSeen:      now,
		SessionIDs:    []string{"s1", "s2", "s3", "s4"},
		Source:        SourceBashBlocker,
	}

	_, err := s.Upsert(rec)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Upsert with 3 more session IDs — total would be 7, should be capped at 5
	rec2 := ErrorRecord{
		Pattern:       "grep command",
		ErrorCategory: "bash-blocker",
		Count:         1,
		LastSeen:      now.Add(time.Hour),
		SessionIDs:    []string{"s5", "s6", "s7"},
		Source:        SourceBashBlocker,
	}

	result, err := s.Upsert(rec2)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if len(result.SessionIDs) != 5 {
		t.Errorf("expected 5 session IDs, got %d: %v", len(result.SessionIDs), result.SessionIDs)
	}

	// Should keep the last 5: s3, s4, s5, s6, s7
	expected := []string{"s3", "s4", "s5", "s6", "s7"}
	for i, sid := range result.SessionIDs {
		if sid != expected[i] {
			t.Errorf("session ID %d: expected %q, got %q", i, expected[i], sid)
		}
	}
}

func TestEnforceMaxRecords(t *testing.T) {
	// Test that enforceMaxRecords trims oldest records
	now := time.Now().Truncate(time.Second)
	records := make([]ErrorRecord, 10)
	for i := 0; i < 10; i++ {
		records[i] = ErrorRecord{
			ID:       recordID("pattern-"+string(rune('A'+i)), "cat"),
			Pattern:  "pattern-" + string(rune('A'+i)),
			Count:    1,
			LastSeen: now.Add(time.Duration(i) * time.Hour),
		}
	}

	result := enforceMaxRecords(records, 5)
	if len(result) != 5 {
		t.Fatalf("expected 5 records, got %d", len(result))
	}
	// Should keep the 5 most recent (indices 5-9 by LastSeen)
	for i := 0; i < 5; i++ {
		if result[i].LastSeen.Before(now.Add(5 * time.Hour)) {
			t.Errorf("record %d has LastSeen %v, expected >= %v", i, result[i].LastSeen, now.Add(5*time.Hour))
		}
	}
}

func TestEnforceMaxRecords_UnderLimit(t *testing.T) {
	records := []ErrorRecord{
		{ID: "a", Pattern: "p1"},
		{ID: "b", Pattern: "p2"},
	}
	result := enforceMaxRecords(records, 5)
	if len(result) != 2 {
		t.Fatalf("expected 2 records (unchanged), got %d", len(result))
	}
}

func TestUpsertEnforcesMaxRecords(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "test.jsonl"))

	now := time.Now().Truncate(time.Second)

	// Seed the store with MaxRecords records
	records := make([]ErrorRecord, MaxRecords)
	for i := 0; i < MaxRecords; i++ {
		records[i] = ErrorRecord{
			ID:            recordID("pattern-existing", "cat-"+string(rune(i%256))),
			Pattern:       "pattern-existing",
			ErrorCategory: "cat-" + string(rune(i%256)),
			Count:         1,
			FirstSeen:     now.Add(-time.Duration(MaxRecords-i) * time.Hour),
			LastSeen:      now.Add(-time.Duration(MaxRecords-i) * time.Hour),
			TTLExpiry:     now.Add(DefaultTTL),
			Source:        SourceBashBlocker,
		}
	}
	if err := s.Save(records); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Upsert one more (new pattern) — should not exceed MaxRecords
	newRec := ErrorRecord{
		Pattern:       "brand-new-pattern",
		ErrorCategory: "brand-new-cat",
		Count:         1,
		FirstSeen:     now,
		LastSeen:      now,
		SessionIDs:    []string{"s-new"},
		Source:        SourceBashBlocker,
	}
	_, err := s.Upsert(newRec)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(loaded) > MaxRecords {
		t.Errorf("expected at most %d records, got %d", MaxRecords, len(loaded))
	}
}

func TestRecordID(t *testing.T) {
	id1 := recordID("cd command", "bash-blocker")
	id2 := recordID("cd command", "bash-blocker")
	id3 := recordID("ls command", "bash-blocker")

	if id1 != id2 {
		t.Errorf("same inputs should produce same ID: %q vs %q", id1, id2)
	}
	if id1 == id3 {
		t.Errorf("different inputs should produce different IDs: %q vs %q", id1, id3)
	}
	if len(id1) != 16 {
		t.Errorf("expected ID length 16, got %d", len(id1))
	}
}
