package tracking

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTracker_RecordAccess(t *testing.T) {
	updater := NewMetadataUpdater()
	tracker := NewTracker(updater)

	path := "/test/engram.ai.md"
	now := time.Now()

	// Record single access
	tracker.RecordAccess(path, now)

	if tracker.Len() != 1 {
		t.Errorf("expected 1 pending update, got %d", tracker.Len())
	}

	// Record second access to same engram
	tracker.RecordAccess(path, now.Add(1*time.Second))

	if tracker.Len() != 1 {
		t.Errorf("expected 1 pending update (same path), got %d", tracker.Len())
	}

	// Check access record
	tracker.mu.RLock()
	record := tracker.accessLog[path]
	tracker.mu.RUnlock()

	if record.Count != 2 {
		t.Errorf("expected count=2, got %d", record.Count)
	}
}

func TestTracker_MultipleEngrams(t *testing.T) {
	updater := NewMetadataUpdater()
	tracker := NewTracker(updater)

	path1 := "/test/engram1.ai.md"
	path2 := "/test/engram2.ai.md"
	now := time.Now()

	tracker.RecordAccess(path1, now)
	tracker.RecordAccess(path2, now)

	if tracker.Len() != 2 {
		t.Errorf("expected 2 pending updates, got %d", tracker.Len())
	}
}

func TestTracker_Flush_EmptyLog(t *testing.T) {
	updater := NewMetadataUpdater()
	tracker := NewTracker(updater)

	// Flush with no pending updates should not error
	if err := tracker.Flush(); err != nil {
		t.Errorf("flush failed: %v", err)
	}

	if tracker.Len() != 0 {
		t.Errorf("expected 0 pending updates after flush, got %d", tracker.Len())
	}
}

func TestMetadataUpdater_Serialize(t *testing.T) {
	// This is a basic test - full integration test is separate
	updater := NewMetadataUpdater()

	// Create a test engram
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")

	content := []byte(`---
type: pattern
title: Test Pattern
description: A test pattern
tags: [test]
---

# Test Content
This is test content.
`)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Update metadata
	record := &AccessRecord{
		Count:      1,
		LastAccess: time.Now(),
	}

	if err := updater.UpdateMetadata(testFile, record); err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read updated file: %v", err)
	}

	// Check that metadata fields are present
	dataStr := string(data)
	if !contains(dataStr, "retrieval_count: 1") {
		t.Error("expected retrieval_count: 1 in updated file")
	}

	if !contains(dataStr, "last_accessed:") {
		t.Error("expected last_accessed in updated file")
	}

	if !contains(dataStr, "encoding_strength: 1") {
		t.Error("expected encoding_strength: 1 in updated file")
	}

	if !contains(dataStr, "created_at:") {
		t.Error("expected created_at in updated file")
	}

	// Verify content is preserved
	if !contains(dataStr, "# Test Content") {
		t.Error("expected content to be preserved")
	}
}

func TestMetadataUpdater_IncrementalUpdates(t *testing.T) {
	updater := NewMetadataUpdater()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")

	content := []byte(`---
type: pattern
title: Test Pattern
description: A test pattern
tags: [test]
---

# Test Content
`)

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// First update
	record1 := &AccessRecord{
		Count:      3,
		LastAccess: time.Now(),
	}

	if err := updater.UpdateMetadata(testFile, record1); err != nil {
		t.Fatalf("first update failed: %v", err)
	}

	// Second update (should increment from previous)
	record2 := &AccessRecord{
		Count:      2,
		LastAccess: time.Now().Add(1 * time.Hour),
	}

	if err := updater.UpdateMetadata(testFile, record2); err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	// Verify count is cumulative
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Should be 3 + 2 = 5
	if !contains(string(data), "retrieval_count: 5") {
		t.Errorf("expected retrieval_count: 5 after incremental updates, got: %s", string(data))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
