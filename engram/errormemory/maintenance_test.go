package errormemory

import (
	"strings"
	"testing"
	"time"
)

func TestPruneExpired(t *testing.T) {
	now := time.Now()
	records := []ErrorRecord{
		{Pattern: "valid1", TTLExpiry: now.Add(24 * time.Hour)},
		{Pattern: "expired1", TTLExpiry: now.Add(-1 * time.Hour)},
		{Pattern: "valid2", TTLExpiry: now.Add(48 * time.Hour)},
		{Pattern: "expired2", TTLExpiry: now.Add(-24 * time.Hour)},
	}

	result := PruneExpired(records)
	if len(result) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(result))
	}

	for _, rec := range result {
		if strings.HasPrefix(rec.Pattern, "expired") {
			t.Errorf("expired record %q should have been pruned", rec.Pattern)
		}
	}
}

func TestTopN(t *testing.T) {
	now := time.Now()

	records := []ErrorRecord{
		{Pattern: "low-count-recent", Count: 2, LastSeen: now},
		{Pattern: "high-count-old", Count: 100, LastSeen: now.Add(-30 * 24 * time.Hour)},
		{Pattern: "medium-count-recent", Count: 50, LastSeen: now.Add(-1 * time.Hour)},
	}

	top := TopN(records, 2)
	if len(top) != 2 {
		t.Fatalf("expected 2 records, got %d", len(top))
	}

	// medium-count-recent should rank highest: 50 * ~1.0 = ~50
	// high-count-old: 100 * (1/31) = ~3.2
	// low-count-recent: 2 * ~1.0 = ~2
	if top[0].Pattern != "medium-count-recent" {
		t.Errorf("expected top record to be 'medium-count-recent', got %q", top[0].Pattern)
	}

	// Test with n > len(records)
	all := TopN(records, 10)
	if len(all) != 3 {
		t.Errorf("expected 3 records when n > len, got %d", len(all))
	}

	// Test with empty records
	empty := TopN(nil, 5)
	if empty != nil {
		t.Errorf("expected nil for empty input, got %v", empty)
	}
}

func TestStats(t *testing.T) {
	now := time.Now()
	past := now.Add(-48 * time.Hour)

	records := []ErrorRecord{
		{
			Pattern:   "cd command",
			Count:     10,
			FirstSeen: past,
			LastSeen:  now.Add(-1 * time.Hour),
			TTLExpiry: now.Add(24 * time.Hour), // valid
		},
		{
			Pattern:   "ls command",
			Count:     5,
			FirstSeen: past.Add(12 * time.Hour),
			LastSeen:  now,
			TTLExpiry: now.Add(-1 * time.Hour), // expired
		},
		{
			Pattern:   "cd command", // duplicate pattern name
			Count:     3,
			FirstSeen: past.Add(24 * time.Hour),
			LastSeen:  now.Add(-2 * time.Hour),
			TTLExpiry: now.Add(48 * time.Hour), // valid
		},
	}

	stats := Stats(records)

	if stats.TotalRecords != 3 {
		t.Errorf("TotalRecords: expected 3, got %d", stats.TotalRecords)
	}
	if stats.ExpiredRecords != 1 {
		t.Errorf("ExpiredRecords: expected 1, got %d", stats.ExpiredRecords)
	}
	if stats.UniquePatterns != 2 {
		t.Errorf("UniquePatterns: expected 2, got %d", stats.UniquePatterns)
	}
	if stats.TotalCount != 18 {
		t.Errorf("TotalCount: expected 18, got %d", stats.TotalCount)
	}
	if stats.TopPattern != "cd command" {
		t.Errorf("TopPattern: expected 'cd command', got %q", stats.TopPattern)
	}
	if stats.TopCount != 10 {
		t.Errorf("TopCount: expected 10, got %d", stats.TopCount)
	}
	if !stats.OldestRecord.Equal(past) {
		t.Errorf("OldestRecord: expected %v, got %v", past, stats.OldestRecord)
	}
	if !stats.NewestRecord.Equal(now) {
		t.Errorf("NewestRecord: expected %v, got %v", now, stats.NewestRecord)
	}

	// Test empty
	emptyStats := Stats(nil)
	if emptyStats.TotalRecords != 0 {
		t.Errorf("empty stats TotalRecords: expected 0, got %d", emptyStats.TotalRecords)
	}
}

func TestFormatSummary(t *testing.T) {
	now := time.Now()

	records := []ErrorRecord{
		{
			Pattern:     "ls/cat/grep/head/tail",
			Remediation: "use Glob/Read/Grep tools",
			Count:       1247,
			LastSeen:    now.Add(-2 * time.Hour),
		},
		{
			Pattern:     "cd",
			Remediation: "use absolute paths or -C flag",
			Count:       892,
			LastSeen:    now.Add(-24 * time.Hour),
		},
	}

	text, tokenCount := FormatSummary(records, 10)

	if !strings.Contains(text, "[error-memory] Common mistakes to avoid:") {
		t.Error("missing header in summary")
	}
	if !strings.Contains(text, "Do NOT use") {
		t.Error("missing 'Do NOT use' prefix in summary")
	}
	if !strings.Contains(text, "1,247x") {
		t.Errorf("missing formatted count '1,247x' in summary:\n%s", text)
	}
	if !strings.Contains(text, "2h ago") {
		t.Errorf("missing age '2h ago' in summary:\n%s", text)
	}
	if tokenCount <= 0 {
		t.Errorf("expected positive token count, got %d", tokenCount)
	}

	// Test empty
	emptyText, emptyTokens := FormatSummary(nil, 10)
	if emptyText != "" {
		t.Errorf("expected empty text for nil records, got %q", emptyText)
	}
	if emptyTokens != 0 {
		t.Errorf("expected 0 tokens for nil records, got %d", emptyTokens)
	}
}
