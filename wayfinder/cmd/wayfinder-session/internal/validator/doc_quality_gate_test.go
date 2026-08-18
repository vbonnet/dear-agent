package validator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestValidateDocQuality_MissingFile tests validation fails when doc file doesn't exist
func TestValidateDocQuality_MissingFile(t *testing.T) {
	tempDir := t.TempDir()

	err := validateDocQuality("SPEC", tempDir)
	if err == nil {
		t.Fatal("expected error for missing SPEC-solution-requirements.md, got nil")
	}

	verr := &ValidationError{}
	ok := errors.As(err, &verr)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if verr.Phase != "complete SPEC" {
		t.Errorf("expected Phase='complete SPEC', got '%s'", verr.Phase)
	}
	if !containsString(verr.Reason, "SPEC-solution-requirements.md does not exist") {
		t.Errorf("expected Reason to mention missing file, got '%s'", verr.Reason)
	}
}

// TestValidateDocQuality_NonValidatedPhase tests non-DESIGN/SPEC/PLAN phases skip validation
func TestValidateDocQuality_NonValidatedPhase(t *testing.T) {
	tempDir := t.TempDir()

	// Test phases that should NOT trigger validation (DESIGN, SPEC, PLAN are validated)
	phases := []string{"CHARTER", "PROBLEM", "RESEARCH", "SETUP", "BUILD", "RETRO"}

	for _, phase := range phases {
		err := validateDocQuality(phase, tempDir)
		if err != nil {
			t.Errorf("phase %s should skip validation, got error: %v", phase, err)
		}
	}
}

func TestRunReviewSkillUsesReachableDeterministicReview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ARCHITECTURE.md")
	content := "# Architecture\n\n## Context\nThis section records the current system boundaries and constraints.\n\n## Design\nThis section records the chosen seams, invariants, and trade-offs in enough detail for implementation.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	score, issues, err := runReviewSkill("review-architecture", path)
	if err != nil {
		t.Fatal(err)
	}
	if score < minDocQualityScore || len(issues) != 0 {
		t.Fatalf("deterministic review = score %.1f, issues %v; want a reachable pass", score, issues)
	}
}

func TestValidateSingleDocumentHasReachablePass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PLAN-design.md")
	content := `---
phase: PLAN
phase_name: Delivery plan
wayfinder_session_id: test-session
created_at: 2026-07-22T09:00:00-07:00
---
# Implementation Plan

## Context
This section records the current system boundaries and constraints that shape the work.

## Design
This section records the chosen sequence, validation evidence, dependencies, and important delivery trade-offs.
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateSingleDocument(dir, "PLAN", "PLAN-design.md", "review-architecture"); err != nil {
		t.Fatalf("validateSingleDocument() blocked a substantive local review: %v", err)
	}
}

func TestBuiltinReviewIssuesHandlesLeadingFrontmatter(t *testing.T) {
	substantive := "# Implementation Plan\n\n## Context\nThis section records the current system boundaries and constraints that shape the work.\n\n## Design\nThis section records the chosen sequence, validation evidence, dependencies, and important delivery trade-offs.\n"
	frontmatter := "---\nphase: PLAN\nphase_name: Delivery plan\nwayfinder_session_id: test-session\ncreated_at: 2026-07-22T09:00:00-07:00\n---\n"

	tests := []struct {
		name         string
		content      string
		wantErr      string
		wantIssue    string
		wantNoIssues bool
	}{
		{
			name:         "canonical frontmatter",
			content:      frontmatter + substantive,
			wantNoIssues: true,
		},
		{
			name:      "missing heading after frontmatter",
			content:   frontmatter + "Implementation Plan\n\n## Context\nEnough context words make this document substantive for structural review.\n\n## Design\nEnough design words describe sequencing, validation, dependencies, and trade-offs.\n",
			wantIssue: "document must begin with a level-one heading",
		},
		{
			name:    "unclosed frontmatter",
			content: "---\nphase: PLAN\n" + substantive,
			wantErr: "unclosed YAML frontmatter",
		},
		{
			name:    "invalid frontmatter",
			content: "---\nphase: [PLAN\n---\n" + substantive,
			wantErr: "invalid YAML frontmatter",
		},
		{
			name:         "later delimiter is prose",
			content:      substantive + "\n---\nA later thematic break remains part of the Markdown body.\n",
			wantNoIssues: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues, err := builtinReviewIssues("review-architecture", tt.content)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantIssue != "" && !containsString(strings.Join(issues, "; "), tt.wantIssue) {
				t.Fatalf("issues = %v, want %q", issues, tt.wantIssue)
			}
			if tt.wantNoIssues && len(issues) != 0 {
				t.Fatalf("issues = %v, want none", issues)
			}
		})
	}
}

func TestRunReviewSkillDoesNotPassHeadingOnlyFiller(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ARCHITECTURE.md")
	content := "# Architecture\n\n## Context\n" + strings.Repeat("filler ", 20) + "\n\n## Design\n" + strings.Repeat("filler ", 20) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	score, issues, err := runReviewSkill("review-architecture", path)
	if err != nil {
		t.Fatal(err)
	}
	if score >= minDocQualityScore || len(issues) == 0 {
		t.Fatalf("heading-only filler received score %.1f and issues %v; want substantive-review failure", score, issues)
	}
}

func TestBuiltinReviewIssuesAcceptsCanonicalADRStatusFormatting(t *testing.T) {
	formats := []string{
		"Status: Accepted",
		"**Status**: Accepted",
		"**Status:** Accepted",
	}
	for _, statusLine := range formats {
		t.Run(statusLine, func(t *testing.T) {
			content := "# ADR-001 Example\n\n" + statusLine + "\n\n## Context\nThis context records enough substantive detail to make the decision reviewable.\n\n## Decision\nThis decision records the selected approach and its important trade-offs.\n"
			issues, err := builtinReviewIssues("review-adr", content)
			if err != nil {
				t.Fatal(err)
			}
			if len(issues) != 0 {
				t.Fatalf("status line %q produced issues %v", statusLine, issues)
			}
		})
	}
}

func TestBuiltinReviewIssuesRejectsMissingADRStatusValue(t *testing.T) {
	content := "# ADR-001 Example\n\n**Status:**\n\n## Context\nThis context records enough substantive detail to make the decision reviewable.\n\n## Decision\nThis decision records the selected approach and its important trade-offs.\n"
	issues, err := builtinReviewIssues("review-adr", content)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(strings.Join(issues, "; "), "ADR must contain a Status line") {
		t.Fatalf("issues = %v, want missing Status line finding", issues)
	}
}

// TestValidateDocQuality_FileTooLarge tests validation fails for oversized files
func TestValidateDocQuality_FileTooLarge(t *testing.T) {
	tempDir := t.TempDir()
	docPath := filepath.Join(tempDir, "SPEC-solution-requirements.md")

	// Create file larger than 10MB limit
	largeContent := make([]byte, maxDocFileSizeBytes+1)
	if err := os.WriteFile(docPath, largeContent, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateDocQuality("SPEC", tempDir)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}

	verr := &ValidationError{}
	ok := errors.As(err, &verr)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !containsString(verr.Reason, "too large") {
		t.Errorf("expected Reason to mention file size, got '%s'", verr.Reason)
	}
}

// TestCalculateFileHash tests hash calculation for file content
func TestCalculateFileHash(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	// Create test file with known content
	content := []byte("Hello, World!")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	hash1, err := calculateFileHash(testFile)
	if err != nil {
		t.Fatalf("calculateFileHash failed: %v", err)
	}

	if hash1 == "" {
		t.Error("expected non-empty hash")
	}

	// Calculate hash again - should be identical
	hash2, err := calculateFileHash(testFile)
	if err != nil {
		t.Fatalf("calculateFileHash failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("hash should be deterministic: %s != %s", hash1, hash2)
	}

	// Modify file - hash should change
	newContent := []byte("Modified content")
	if err := os.WriteFile(testFile, newContent, 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	hash3, err := calculateFileHash(testFile)
	if err != nil {
		t.Fatalf("calculateFileHash failed: %v", err)
	}

	if hash1 == hash3 {
		t.Error("hash should change when content changes")
	}
}

// TestCheckCache_CacheMiss tests cache lookup when cache doesn't exist
func TestCheckCache_CacheMiss(t *testing.T) {
	tempDir := t.TempDir()

	score, hit := checkCache(tempDir, "SPEC.md", "dummy_hash")
	if hit {
		t.Error("expected cache miss for non-existent cache")
	}
	if score != 0 {
		t.Errorf("expected score=0 for cache miss, got %.1f", score)
	}
}

// TestCheckCache_HashMismatch tests cache miss when hash differs
func TestCheckCache_HashMismatch(t *testing.T) {
	tempDir := t.TempDir()

	// Create cache with one hash
	if err := updateCache(tempDir, "SPEC.md", "hash1", 9.0); err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Lookup with different hash - should be cache miss
	score, hit := checkCache(tempDir, "SPEC.md", "hash2")
	if hit {
		t.Error("expected cache miss for hash mismatch")
	}
	if score != 0 {
		t.Errorf("expected score=0 for cache miss, got %.1f", score)
	}
}

// TestCheckCache_CacheHit tests cache lookup when hash matches
func TestCheckCache_CacheHit(t *testing.T) {
	tempDir := t.TempDir()

	// Create cache entry
	expectedScore := 9.2
	if err := updateCache(tempDir, "SPEC.md", "hash123", expectedScore); err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Lookup with matching hash - should be cache hit
	score, hit := checkCache(tempDir, "SPEC.md", "hash123")
	if !hit {
		t.Error("expected cache hit for matching hash")
	}
	if score != expectedScore {
		t.Errorf("expected score=%.1f, got %.1f", expectedScore, score)
	}
}

// TestUpdateCache_CreateNew tests creating cache from scratch
func TestUpdateCache_CreateNew(t *testing.T) {
	tempDir := t.TempDir()

	err := updateCache(tempDir, "SPEC.md", "hash123", 8.5)
	if err != nil {
		t.Fatalf("updateCache failed: %v", err)
	}

	// Verify cache file was created
	cachePath := filepath.Join(tempDir, ".wayfinder-cache", "doc-quality-scores.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Verify cache content
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("failed to read cache: %v", err)
	}

	var cache DocQualityCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("failed to parse cache: %v", err)
	}

	entry, exists := cache["SPEC.md"]
	if !exists {
		t.Fatal("expected SPEC.md entry in cache")
	}

	if entry.FileHash != "hash123" {
		t.Errorf("expected hash='hash123', got '%s'", entry.FileHash)
	}
	if entry.Score != 8.5 {
		t.Errorf("expected score=8.5, got %.1f", entry.Score)
	}
}

// TestUpdateCache_UpdateExisting tests updating existing cache entry
func TestUpdateCache_UpdateExisting(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial cache entry
	if err := updateCache(tempDir, "SPEC.md", "hash1", 7.0); err != nil {
		t.Fatalf("failed to create initial cache: %v", err)
	}

	// Update with new hash and score
	if err := updateCache(tempDir, "SPEC.md", "hash2", 9.0); err != nil {
		t.Fatalf("failed to update cache: %v", err)
	}

	// Verify updated entry
	score, hit := checkCache(tempDir, "SPEC.md", "hash2")
	if !hit {
		t.Error("expected cache hit for updated hash")
	}
	if score != 9.0 {
		t.Errorf("expected updated score=9.0, got %.1f", score)
	}

	// Old hash should not be found
	_, hit = checkCache(tempDir, "SPEC.md", "hash1")
	if hit {
		t.Error("old hash should not match after update")
	}
}

// TestUpdateCache_MultipleFiles tests cache with multiple doc files
func TestUpdateCache_MultipleFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Add multiple entries
	if err := updateCache(tempDir, "SPEC.md", "hash_spec", 8.5); err != nil {
		t.Fatalf("failed to add SPEC.md: %v", err)
	}
	if err := updateCache(tempDir, "ARCHITECTURE.md", "hash_arch", 9.0); err != nil {
		t.Fatalf("failed to add ARCHITECTURE.md: %v", err)
	}

	// Verify both entries exist independently
	specScore, specHit := checkCache(tempDir, "SPEC.md", "hash_spec")
	if !specHit || specScore != 8.5 {
		t.Errorf("SPEC.md cache incorrect: hit=%v, score=%.1f", specHit, specScore)
	}

	archScore, archHit := checkCache(tempDir, "ARCHITECTURE.md", "hash_arch")
	if !archHit || archScore != 9.0 {
		t.Errorf("ARCHITECTURE.md cache incorrect: hit=%v, score=%.1f", archHit, archScore)
	}
}

// TestCheckCache_CorruptedCache tests graceful handling of corrupted cache
func TestCheckCache_CorruptedCache(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, ".wayfinder-cache")
	cachePath := filepath.Join(cacheDir, "doc-quality-scores.json")

	// Create cache directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	// Write corrupted JSON
	if err := os.WriteFile(cachePath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write corrupted cache: %v", err)
	}

	// Should treat as cache miss, not crash
	score, hit := checkCache(tempDir, "SPEC.md", "hash123")
	if hit {
		t.Error("expected cache miss for corrupted cache")
	}
	if score != 0 {
		t.Errorf("expected score=0 for corrupted cache, got %.1f", score)
	}
}

// TestValidateDocFileSize_ValidSize tests file within size limits
func TestValidateDocFileSize_ValidSize(t *testing.T) {
	tempDir := t.TempDir()
	docPath := filepath.Join(tempDir, "SPEC.md")

	// Create file well under 10MB limit
	content := make([]byte, 1024) // 1KB
	if err := os.WriteFile(docPath, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	err := validateDocFileSize(docPath, "SPEC", "SPEC.md")
	if err != nil {
		t.Errorf("expected no error for valid size, got: %v", err)
	}
}

// TestCacheTimestamp tests cache entries include timestamp
func TestCacheTimestamp(t *testing.T) {
	tempDir := t.TempDir()

	before := time.Now()
	if err := updateCache(tempDir, "SPEC.md", "hash123", 8.5); err != nil {
		t.Fatalf("updateCache failed: %v", err)
	}
	after := time.Now()

	// Read cache and verify timestamp
	cachePath := filepath.Join(tempDir, ".wayfinder-cache", "doc-quality-scores.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("failed to read cache: %v", err)
	}

	var cache DocQualityCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("failed to parse cache: %v", err)
	}

	entry := cache["SPEC.md"]
	if entry.Timestamp.Before(before) || entry.Timestamp.After(after) {
		t.Errorf("timestamp %v not within expected range [%v, %v]", entry.Timestamp, before, after)
	}
}
