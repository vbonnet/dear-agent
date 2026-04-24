package promptcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetCacheControl_Default(t *testing.T) {
	cc := GetCacheControl(TierDefault)
	if cc.Type != "ephemeral" {
		t.Errorf("type = %q, want ephemeral", cc.Type)
	}
	if cc.TTL != 0 {
		t.Errorf("TTL = %d, want 0 for default tier", cc.TTL)
	}
}

func TestGetCacheControl_Persistent(t *testing.T) {
	cc := GetCacheControl(TierPersistent)
	if cc.Type != "ephemeral" {
		t.Errorf("type = %q, want ephemeral", cc.Type)
	}
	if cc.TTL != TTL1Hour {
		t.Errorf("TTL = %d, want %d", cc.TTL, TTL1Hour)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		min   int
		max   int
	}{
		{"", 0, 0},
		{"hello world", 2, 5},          // 11 chars -> ~3-4 tokens
		{"a longer string here", 5, 8}, // 20 chars -> ~6-7 tokens
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.input)
		if got < tt.min || got > tt.max {
			t.Errorf("EstimateTokens(%q) = %d, want [%d, %d]", tt.input, got, tt.min, tt.max)
		}
	}
}

func TestDetector_RecordAndCheck_NoBreak(t *testing.T) {
	d := NewDetector(DetectorConfig{DiffDir: t.TempDir()})

	content := "You are a helpful assistant. Follow these rules..."
	d.RecordSnapshot("system_prompt", content)

	// Simulate good cache hit (50% of estimated tokens read from cache)
	est := EstimateTokens(content)
	event := d.CheckCacheBreak("system_prompt", est/2, content)
	if event != nil {
		t.Error("expected no cache break with 50% hit rate")
	}
}

func TestDetector_RecordAndCheck_Break(t *testing.T) {
	d := NewDetector(DetectorConfig{DiffDir: t.TempDir()})

	content := "You are a helpful assistant. Follow these rules for doing work."
	d.RecordSnapshot("system_prompt", content)

	// Simulate cache break (0 tokens read from cache)
	newContent := "Completely different system prompt with new instructions."
	event := d.CheckCacheBreak("system_prompt", 0, newContent)
	if event == nil {
		t.Fatal("expected cache break with 0% hit rate")
	}

	if event.Source != "system_prompt" {
		t.Errorf("source = %q, want system_prompt", event.Source)
	}
	if event.ReadRatio != 0 {
		t.Errorf("readRatio = %f, want 0", event.ReadRatio)
	}
	if event.DiffPath == "" {
		t.Error("expected diff file to be written")
	}

	// Verify diff file exists
	if _, err := os.Stat(event.DiffPath); os.IsNotExist(err) {
		t.Errorf("diff file not found: %s", event.DiffPath)
	}
}

func TestDetector_CheckCacheBreak_UnknownSource(t *testing.T) {
	d := NewDetector(DetectorConfig{DiffDir: t.TempDir()})

	event := d.CheckCacheBreak("unknown", 0, "content")
	if event != nil {
		t.Error("expected nil for unknown source")
	}
}

func TestDetector_SuppressAfterCompaction(t *testing.T) {
	d := NewDetector(DetectorConfig{DiffDir: t.TempDir()})

	content := "system prompt content here for caching test"
	d.RecordSnapshot("system_prompt", content)

	// Suppress for 1 hour
	d.SuppressAfterCompaction(1 * time.Hour)

	// Should not detect break during suppression
	event := d.CheckCacheBreak("system_prompt", 0, "different content entirely")
	if event != nil {
		t.Error("expected suppression to prevent break detection")
	}
}

func TestDetector_Breaks(t *testing.T) {
	d := NewDetector(DetectorConfig{DiffDir: t.TempDir()})

	content := "original system prompt for break tracking test"
	d.RecordSnapshot("src1", content)
	d.CheckCacheBreak("src1", 0, "changed content for testing")

	breaks := d.Breaks()
	if len(breaks) != 1 {
		t.Fatalf("expected 1 break, got %d", len(breaks))
	}
	if breaks[0].Source != "src1" {
		t.Errorf("source = %q, want src1", breaks[0].Source)
	}
}

func TestDetector_MaxSources(t *testing.T) {
	d := NewDetector(DetectorConfig{
		DiffDir:    t.TempDir(),
		MaxSources: 3,
	})

	d.RecordSnapshot("a", "content a")
	time.Sleep(time.Millisecond)
	d.RecordSnapshot("b", "content b")
	time.Sleep(time.Millisecond)
	d.RecordSnapshot("c", "content c")
	time.Sleep(time.Millisecond)
	d.RecordSnapshot("d", "content d") // should evict "a"

	// "a" should be evicted
	event := d.CheckCacheBreak("a", 0, "content a")
	if event != nil {
		t.Error("expected 'a' to be evicted")
	}

	// "d" should exist
	d.RecordSnapshot("d", "content d for recheck")
	event = d.CheckCacheBreak("d", 0, "different d content")
	if event == nil {
		t.Error("expected 'd' to still be tracked")
	}
}

func TestDetector_DiffFileContent(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{DiffDir: dir})

	d.RecordSnapshot("test_source", "original prompt content for diff testing")
	event := d.CheckCacheBreak("test_source", 0, "new prompt content after change")
	if event == nil {
		t.Fatal("expected cache break")
	}

	data, err := os.ReadFile(event.DiffPath)
	if err != nil {
		t.Fatalf("read diff: %v", err)
	}

	content := string(data)
	if !contains(content, "Cache Break Detected") {
		t.Error("diff missing header")
	}
	if !contains(content, "test_source") {
		t.Error("diff missing source name")
	}
}

func TestDetector_NoDiffWhenContentUnchanged(t *testing.T) {
	dir := t.TempDir()
	d := NewDetector(DetectorConfig{DiffDir: dir})

	content := "same prompt content that stays identical"
	d.RecordSnapshot("same", content)

	// Cache break with same content (e.g., cache eviction without content change)
	event := d.CheckCacheBreak("same", 0, content)
	if event == nil {
		t.Fatal("expected cache break event")
	}
	if event.DiffPath != "" {
		t.Error("expected no diff file when content unchanged")
	}
}

func TestDetector_Threshold5Percent(t *testing.T) {
	d := NewDetector(DetectorConfig{DiffDir: t.TempDir()})

	// Use large content for meaningful token estimates (reduce rounding noise)
	content := string(make([]byte, 4000)) // ~1333 tokens estimated
	d.RecordSnapshot("threshold", content)

	est := EstimateTokens(content)

	// 10% should NOT trigger
	tenPercent := est / 10
	event := d.CheckCacheBreak("threshold", tenPercent, content)
	if event != nil {
		t.Errorf("10%% should not trigger break (read=%d, est=%d)", tenPercent, est)
	}

	// 1% SHOULD trigger
	onePercent := est / 100
	event = d.CheckCacheBreak("threshold", onePercent, content)
	if event == nil {
		t.Errorf("1%% should trigger break (read=%d, est=%d)", onePercent, est)
	}
}

func TestDiffDir_DefaultPath(t *testing.T) {
	d := NewDetector(DetectorConfig{})
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".engram", "tmp")
	if d.diffDir != expected {
		t.Errorf("diffDir = %q, want %q", d.diffDir, expected)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
