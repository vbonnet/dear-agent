// Package ecphory implements the 3-tier memory retrieval system for Engram.
//
// Ecphory (from Greek: ἐκφορά "retrieval") is the process of reconstructing a memory
// from a cue. This package implements a three-tier retrieval pipeline:
//
// Tier 1 (Filter): Fast frontmatter-based filtering by tags and agent
// Tier 2 (Rank): Semantic relevance scoring using Anthropic API
// Tier 3 (Budget): Token-aware selection within budget constraints
//
// Example usage:
//
//	ecphory, err := NewEcphory("/path/to/engrams", 10000)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	results, err := ecphory.Query(ctx, "error handling", []string{"go"}, "claude-code")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// The retrieval pipeline ensures relevant engrams are retrieved efficiently while
// respecting token budget constraints for AI agents.
package ecphory

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/engram"
	"gopkg.in/yaml.v3"
)

// Ecphory implements the 3-tier memory retrieval system.
// It combines fast frontmatter filtering, semantic ranking, and token budgeting
// to retrieve the most relevant engrams for a given query.
type Ecphory struct {
	index           *Index           // Tier 1: Frontmatter index for fast filtering
	ranker          *Ranker          // Tier 2: Semantic ranker using Anthropic API
	parser          *engram.Parser   // Parser for loading engram content
	tokenBudget     int              // Tier 3: Maximum tokens to return
	eventBus        EventBus         // Optional: EventBus for telemetry (nil = disabled)
	basePath        string           // Base path for computing relative paths (privacy)
	contextDetector *ContextDetector // Task 1.3.2: Failure context detection for boosting
	triggerPaths    map[string]bool  // Optional: paths with active triggers for boosting
}

// NewEcphory creates a new ecphory retrieval system.
//
// Parameters:
//   - engramPath: Directory containing .ai.md engram files
//   - tokenBudget: Maximum tokens to return (typically 10000-50000)
//
// Returns an error if the frontmatter index cannot be built or if the
// Anthropic API ranker cannot be initialized (requires ANTHROPIC_API_KEY).
func NewEcphory(engramPath string, tokenBudget int) (*Ecphory, error) {
	// Build frontmatter index
	idx := NewIndex()
	if err := idx.Build(engramPath); err != nil {
		return nil, fmt.Errorf("failed to build index: %w", err)
	}

	// P0-2 FIX: Create ranker and clean up index if ranker initialization fails
	ranker, err := NewRanker()
	if err != nil {
		// Clean up index resources before returning error
		idx.Clear()
		return nil, fmt.Errorf("failed to create ranker: %w", err)
	}

	return &Ecphory{
		index:           idx,
		ranker:          ranker,
		parser:          engram.NewParser(),
		tokenBudget:     tokenBudget,
		eventBus:        nil, // Default: no telemetry
		basePath:        engramPath,
		contextDetector: NewContextDetector(), // Task 1.3.2: Initialize context detector
	}, nil
}

// EventBus interface for publishing telemetry events
// This matches core/internal/consolidation/eventbus.go pattern
type EventBus interface {
	Publish(ctx context.Context, event *Event) error
}

// Event represents an EventBus event
type Event struct {
	Topic     string
	Publisher string
	Timestamp time.Time
	Data      map[string]interface{}
}

// WithEventBus returns an option that enables EventBus telemetry
func WithEventBus(bus EventBus) func(*Ecphory) {
	return func(e *Ecphory) {
		e.eventBus = bus
	}
}

// WithTriggerPaths returns an option that sets the active trigger paths for boosting.
// Engrams whose paths appear in the map will receive a +20.0 relevance boost during ranking.
func WithTriggerPaths(paths map[string]bool) func(*Ecphory) {
	return func(e *Ecphory) {
		e.triggerPaths = paths
	}
}

// ApplyOptions applies functional options to Ecphory
func (e *Ecphory) ApplyOptions(opts ...func(*Ecphory)) {
	for _, opt := range opts {
		opt(e)
	}
}

// Query performs 3-tier ecphory retrieval to find relevant engrams.
//
// The query pipeline:
//  1. Filter: Fast frontmatter filtering by tags and agent
//  2. Rank: Semantic ranking using Anthropic API for relevance scores
//  3. Budget: Load engrams within token budget, highest relevance first
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - query: Natural language search query (e.g., "error handling patterns")
//   - sessionID: Session identifier for telemetry tracking (required, non-empty)
//   - transcript: Conversation transcript or context (required, non-empty)
//   - tags: Optional tags to filter by (e.g., ["go", "errors"])
//   - agent: Optional agent to filter by (e.g., "claude-code")
//
// Returns a slice of engrams sorted by relevance (highest first), limited by
// token budget. Returns nil if no matching engrams found. Falls back to
// unranked results if the Anthropic API is unavailable.
func (e *Ecphory) Query(ctx context.Context, query string, sessionID string, transcript string, tags []string, agent string) ([]*engram.Engram, error) {
	startTime := time.Now()

	// P1-4: Check context cancellation before starting
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Tier 1: Fast filter using frontmatter index
	candidates := e.fastFilter(tags, agent)
	if len(candidates) == 0 {
		// Publish empty result event
		e.publishEcphoryEvent(ctx, query, sessionID, transcript, tags, agent, nil, 0, time.Since(startTime))
		return nil, nil
	}

	// P1-4: Check context cancellation before expensive API call
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Tier 2: API ranking for relevance scoring
	ranked, err := e.ranker.Rank(ctx, query, candidates)
	if err != nil {
		// Fall back to unranked candidates on API error (resilient design)
		log.Printf("ecphory: ranking failed, falling back to unranked candidates: %v", err)
		results := e.loadEngrams(candidates)
		tokensUsed := e.estimateTokens(results)
		e.publishEcphoryEvent(ctx, query, sessionID, transcript, tags, agent, results, tokensUsed, time.Since(startTime))
		return results, nil //nolint:nilerr // Intentional: fallback succeeded
	}

	// Task 1.3.2: Apply failure boosting for debugging queries
	e.applyFailureBoosting(query, ranked)

	// Apply trigger boosting for engrams with active triggers
	if len(e.triggerPaths) > 0 {
		applyTriggerBoosting(ranked, e.triggerPaths)
	}

	// Sort by relevance (descending)
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Relevance > ranked[j].Relevance
	})

	// P1-4: Check context cancellation before loading
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	// Tier 3: Load engrams within token budget
	results := e.loadWithinBudget(ranked)
	tokensUsed := e.estimateTokens(results)

	// Publish ecphory event (async, non-blocking)
	e.publishEcphoryEvent(ctx, query, sessionID, transcript, tags, agent, results, tokensUsed, time.Since(startTime))

	// Update frontmatter metadata (async, non-blocking)
	e.updateFrontmatterMetadata(results)

	return results, nil
}

// fastFilter performs tier 1 filtering using frontmatter index
func (e *Ecphory) fastFilter(tags []string, agent string) []string {
	var candidates []string

	// Filter by tags if provided
	if len(tags) > 0 {
		candidates = e.index.FilterByTags(tags)
	} else {
		candidates = e.index.All()
	}

	// Filter by agent
	if agent != "" {
		agentCandidates := e.index.FilterByAgent(agent)
		candidates = intersect(candidates, agentCandidates)
	}

	return candidates
}

// applyFailureBoosting boosts relevance scores for failure reflections when in debugging context
// (Task 1.3.2: Add failure ranking boost)
//
// When a query is detected as debugging-related (e.g., "why did this fail?"), this method:
// 1. Identifies the specific error category (syntax, permission, timeout, tool_misuse, other)
// 2. Boosts relevance by +25.0 for reflections with matching error_category
// 3. Leaves non-reflection engrams unchanged
//
// This ensures that past failures of the same category are prioritized during debugging,
// implementing the "learn from mistakes" pattern for the Mistake Notebook system.
func (e *Ecphory) applyFailureBoosting(query string, ranked []RankingResult) {
	// Detect if this is a debugging context
	isDebugging, errorCategory := e.contextDetector.DetectContext(query)
	if !isDebugging {
		// Not a debugging query, no boosting needed
		return
	}

	// Boost reflections with matching error category
	for i := range ranked {
		// Read file to extract frontmatter
		data, err := os.ReadFile(ranked[i].Path)
		if err != nil {
			// Skip files that can't be read
			continue
		}

		// Extract error_category from frontmatter YAML
		category := extractErrorCategory(data)
		if category == "" {
			// Not a reflection or no error_category field
			continue
		}

		// Boost if category matches
		if category == string(errorCategory) {
			// Apply +25.0 boost (significant but not overwhelming)
			ranked[i].Relevance += 25.0

			// Cap at 100.0 to maintain score range
			if ranked[i].Relevance > 100.0 {
				ranked[i].Relevance = 100.0
			}
		}
	}
}

// extractErrorCategory extracts the error_category field from reflection frontmatter
// Returns empty string if not found or if this is not a reflection
func extractErrorCategory(data []byte) string {
	// Find frontmatter boundaries
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return ""
	}

	rest := data[4:] // Skip opening ---\n
	idx := bytes.Index(rest, []byte("\n---\n"))
	if idx == -1 {
		return ""
	}

	frontmatter := rest[:idx]

	// Parse YAML into map to access arbitrary fields
	var fm map[string]interface{}
	if err := yaml.Unmarshal(frontmatter, &fm); err != nil {
		return ""
	}

	// Extract error_category field
	if category, ok := fm["error_category"].(string); ok {
		return category
	}

	return ""
}

// loadWithinBudget loads engrams within token budget
func (e *Ecphory) loadWithinBudget(ranked []RankingResult) []*engram.Engram {
	var result []*engram.Engram
	tokensUsed := 0

	for _, r := range ranked {
		eg, err := e.parser.Parse(r.Path)
		if err != nil {
			// P0-4: Log parse errors instead of silently ignoring
			log.Printf("ecphory: failed to load engram at %s: %v", r.Path, err)
			continue
		}

		// P1-1: Token estimation limitation - using char/4 heuristic
		// For production use, consider using a proper tokenizer like tiktoken
		tokens := len(eg.Content) / 4

		if tokensUsed+tokens > e.tokenBudget {
			// Budget exhausted
			break
		}

		result = append(result, eg)
		tokensUsed += tokens
	}

	return result
}

// loadEngrams loads engrams from paths
func (e *Ecphory) loadEngrams(paths []string) []*engram.Engram {
	var result []*engram.Engram

	for _, path := range paths {
		eg, err := e.parser.Parse(path)
		if err != nil {
			// P0-4: Log parse errors instead of silently ignoring
			log.Printf("ecphory: failed to load engram at %s: %v", path, err)
			continue
		}
		result = append(result, eg)
	}

	return result
}

// intersect returns intersection of two string slices
func intersect(a, b []string) []string {
	set := make(map[string]bool)
	for _, s := range a {
		set[s] = true
	}

	var result []string
	for _, s := range b {
		if set[s] {
			result = append(result, s)
		}
	}

	return result
}

// Close cleans up ecphory resources
func (e *Ecphory) Close() error {
	// Close ranker (which may have HTTP client or API connections)
	if e.ranker != nil {
		return e.ranker.Close()
	}
	return nil
}

// publishEcphoryEvent publishes a telemetry event for an ecphory query
func (e *Ecphory) publishEcphoryEvent(ctx context.Context, query string, sessionID string, transcript string, tags []string, agent string, results []*engram.Engram, tokensUsed int, duration time.Duration) {
	if e.eventBus == nil {
		return // Telemetry disabled
	}

	// Build relative paths for privacy
	paths := make([]string, len(results))
	for i, eg := range results {
		paths[i] = e.relativePath(eg.Path)
	}

	event := &Event{
		Topic:     "ecphory.query",
		Publisher: "ecphory",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"query":        query,
			"session_id":   sessionID,
			"transcript":   transcript,
			"tags":         tags,
			"agent":        agent,
			"result_count": len(results),
			"result_paths": paths,
			"tokens_used":  tokensUsed,
			"duration_ms":  duration.Milliseconds(),
			"token_budget": e.tokenBudget,
		},
	}

	// Publish asynchronously (non-blocking)
	go func() {
		if err := e.eventBus.Publish(ctx, event); err != nil {
			// Log publish errors (eventbus may also log internally)
			log.Printf("failed to publish ecphory event: %v", err)
		}
	}()
}

// updateFrontmatterMetadata updates RetrievalCount and LastAccessed for loaded engrams
func (e *Ecphory) updateFrontmatterMetadata(engrams []*engram.Engram) {
	if len(engrams) == 0 {
		return
	}

	// Run updates asynchronously (non-blocking)
	go func() {
		for _, eg := range engrams {
			if err := e.incrementRetrievalCount(eg.Path); err != nil {
				log.Printf("ecphory: failed to update metadata for %s: %v", eg.Path, err)
			}
		}
	}()
}

// incrementRetrievalCount atomically updates retrieval_count and last_accessed
func (e *Ecphory) incrementRetrievalCount(path string) error {
	// Re-parse to get current frontmatter
	eg, err := e.parser.Parse(path)
	if err != nil {
		return err
	}

	// Increment count and update timestamp
	eg.Frontmatter.RetrievalCount++
	eg.Frontmatter.LastAccessed = time.Now()

	// Write updated frontmatter
	return e.writeFrontmatter(path, &eg.Frontmatter, eg.Content)
}

// writeFrontmatter writes updated frontmatter to an engram file using atomic rename.
func (e *Ecphory) writeFrontmatter(path string, fm *engram.Frontmatter, content string) error {
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fmBytes)
	buf.WriteString("---\n")
	buf.WriteString(content)

	// Preserve original file permissions
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", e.relativePath(path), err)
	}

	// Atomic write: temp file in same directory, then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, buf.Bytes(), info.Mode().Perm()); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// relativePath converts absolute path to relative (privacy-preserving)
func (e *Ecphory) relativePath(absPath string) string {
	if e.basePath == "" {
		return absPath
	}

	relPath, err := filepath.Rel(e.basePath, absPath)
	if err != nil {
		// Fallback to filename only
		return filepath.Base(absPath)
	}

	// Normalize path separators and remove home directory
	relPath = filepath.ToSlash(relPath)
	relPath = strings.TrimPrefix(relPath, "/home/")
	relPath = strings.TrimPrefix(relPath, "user/")

	return relPath
}

// estimateTokens estimates total tokens from engram content
func (e *Ecphory) estimateTokens(engrams []*engram.Engram) int {
	total := 0
	for _, eg := range engrams {
		// Use same heuristic as loadWithinBudget
		total += len(eg.Content) / 4
	}
	return total
}
