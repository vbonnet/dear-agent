// Package promptcache provides Claude API cache control headers for system prompts
// and cache break detection to identify when prompt caching stops working.
//
// Two-tier caching:
//   - Default: ephemeral (5-minute TTL, API default)
//   - Optional: 1-hour TTL for stable system prompts
//
// Cache break detection records prompt snapshots before API calls and checks
// if cache read ratios drop below 5% of expected, indicating a cache break.
package promptcache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CacheControl represents the cache_control header for Claude API messages.
type CacheControl struct {
	Type string `json:"type"`          // always "ephemeral"
	TTL  int    `json:"ttl,omitempty"` // optional TTL in seconds (0 = API default ~5min)
}

// Tier specifies the caching tier for a prompt segment.
type Tier int

const (
	// TierDefault uses the API's default ephemeral caching (~5 minutes).
	TierDefault Tier = iota
	// TierPersistent uses a 1-hour TTL for stable prompt content.
	TierPersistent
)

// TTL1Hour is the TTL value for persistent cache tier (3600 seconds).
const TTL1Hour = 3600

// GetCacheControl returns the appropriate cache_control header for a prompt segment.
// TierDefault returns ephemeral with no explicit TTL (API default ~5min).
// TierPersistent returns ephemeral with 1-hour TTL.
func GetCacheControl(tier Tier) CacheControl {
	switch tier {
	case TierPersistent:
		return CacheControl{Type: "ephemeral", TTL: TTL1Hour}
	case TierDefault:
		return CacheControl{Type: "ephemeral"}
	}

	return CacheControl{Type: "ephemeral"}
}

// PromptSnapshot captures the state of a prompt segment before an API call.
type PromptSnapshot struct {
	Hash      string    // SHA-256 of prompt content
	TokenEst  int       // estimated token count (len/4)
	Timestamp time.Time // when snapshot was taken
	Source    string    // identifies where this prompt came from
}

// CacheBreakEvent records a detected cache break.
type CacheBreakEvent struct {
	Timestamp    time.Time
	Source       string  // which prompt source broke
	ExpectedRead int     // expected cache read tokens
	ActualRead   int     // actual cache read tokens
	ReadRatio    float64 // actual/expected
	DiffPath     string  // path to diff file, if written
}

// Detector tracks prompt snapshots and detects cache breaks.
type Detector struct {
	mu            sync.Mutex
	snapshots     map[string]PromptSnapshot // source -> last snapshot
	breaks        []CacheBreakEvent
	diffDir       string    // directory for cache-break diff files
	maxSources    int       // max tracked sources (default 10)
	suppressUntil time.Time // suppress false positives after compaction
}

// DetectorConfig configures a cache break detector.
type DetectorConfig struct {
	DiffDir    string // directory for cache-break-*.diff files (default: ~/.engram/tmp)
	MaxSources int    // max tracked prompt sources (default: 10)
}

// NewDetector creates a cache break detector.
func NewDetector(cfg DetectorConfig) *Detector {
	if cfg.DiffDir == "" {
		home, _ := os.UserHomeDir()
		cfg.DiffDir = filepath.Join(home, ".engram", "tmp")
	}
	if cfg.MaxSources <= 0 {
		cfg.MaxSources = 10
	}
	return &Detector{
		snapshots:  make(map[string]PromptSnapshot),
		diffDir:    cfg.DiffDir,
		maxSources: cfg.MaxSources,
	}
}

// RecordSnapshot captures the current state of a prompt segment before an API call.
// Returns the snapshot for later comparison.
func (d *Detector) RecordSnapshot(source, content string) PromptSnapshot {
	d.mu.Lock()
	defer d.mu.Unlock()

	snap := PromptSnapshot{
		Hash:      hashContent(content),
		TokenEst:  EstimateTokens(content),
		Timestamp: time.Now(),
		Source:    source,
	}

	// Enforce max sources by evicting oldest if needed
	if _, exists := d.snapshots[source]; !exists && len(d.snapshots) >= d.maxSources {
		d.evictOldest()
	}

	d.snapshots[source] = snap
	return snap
}

// CheckCacheBreak compares actual cache read tokens against the last snapshot
// for a source. If the read ratio drops to <= 5% of expected, it's a cache break.
// Returns nil if no break detected, or the break event.
func (d *Detector) CheckCacheBreak(source string, actualReadTokens int, currentContent string) *CacheBreakEvent {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Suppress false positives after compaction
	if time.Now().Before(d.suppressUntil) {
		return nil
	}

	snap, ok := d.snapshots[source]
	if !ok {
		return nil
	}

	if snap.TokenEst <= 0 {
		return nil
	}

	ratio := float64(actualReadTokens) / float64(snap.TokenEst)
	if ratio > 0.05 {
		return nil // cache is working (>5% hit rate)
	}

	// Cache break detected
	event := CacheBreakEvent{
		Timestamp:    time.Now(),
		Source:       source,
		ExpectedRead: snap.TokenEst,
		ActualRead:   actualReadTokens,
		ReadRatio:    ratio,
	}

	// Write diff if content changed
	currentHash := hashContent(currentContent)
	if currentHash != snap.Hash {
		diffPath := d.writeDiff(source, snap, currentContent)
		event.DiffPath = diffPath
	}

	d.breaks = append(d.breaks, event)
	return &event
}

// SuppressAfterCompaction suppresses false positive cache break detection
// for the specified duration after an intentional compaction or cache clear.
func (d *Detector) SuppressAfterCompaction(duration time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.suppressUntil = time.Now().Add(duration)
}

// Breaks returns all detected cache break events.
func (d *Detector) Breaks() []CacheBreakEvent {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]CacheBreakEvent, len(d.breaks))
	copy(result, d.breaks)
	return result
}

// EstimateTokens provides a rough token count estimate using the len/4 heuristic
// with 4/3 padding, matching Claude Code's approach.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	base := len(text) / 4
	return base + base/3 // ~33% padding
}

// hashContent returns a truncated SHA-256 hex digest of content.
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8])
}

// writeDiff writes a cache-break diff file showing what changed.
func (d *Detector) writeDiff(source string, oldSnap PromptSnapshot, newContent string) string {
	if err := os.MkdirAll(d.diffDir, 0o700); err != nil {
		return ""
	}

	ts := time.Now().Format("20060102-150405")
	safeName := strings.ReplaceAll(source, "/", "_")
	filename := fmt.Sprintf("cache-break-%s-%s.diff", safeName, ts)
	path := filepath.Join(d.diffDir, filename)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("--- Cache Break Detected ---\n"))
	b.WriteString(fmt.Sprintf("Source: %s\n", source))
	b.WriteString(fmt.Sprintf("Time: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Previous hash: %s (est %d tokens)\n", oldSnap.Hash, oldSnap.TokenEst))
	b.WriteString(fmt.Sprintf("Current hash: %s (est %d tokens)\n", hashContent(newContent), EstimateTokens(newContent)))
	b.WriteString(fmt.Sprintf("---\n"))
	// Write first 500 chars of new content as context
	preview := newContent
	if len(preview) > 500 {
		preview = preview[:500] + "\n... (truncated)"
	}
	b.WriteString(fmt.Sprintf("Current content preview:\n%s\n", preview))

	os.WriteFile(path, []byte(b.String()), 0o600)
	return path
}

// evictOldest removes the oldest snapshot to stay under maxSources.
func (d *Detector) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for k, v := range d.snapshots {
		if oldestKey == "" || v.Timestamp.Before(oldestTime) {
			oldestKey = k
			oldestTime = v.Timestamp
		}
	}

	if oldestKey != "" {
		delete(d.snapshots, oldestKey)
	}
}
