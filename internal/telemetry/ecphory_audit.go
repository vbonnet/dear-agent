package telemetry

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// EcphoryAuditResult contains results of ecphory correctness validation.
type EcphoryAuditResult struct {
	SessionID          string         `json:"session_id"`
	TotalRetrievals    int            `json:"total_retrievals"`
	AppropriateCount   int            `json:"appropriate_count"`
	InappropriateCount int            `json:"inappropriate_count"`
	CorrectnessScore   float64        `json:"correctness_score"` // appropriate / total
	AuditDurationMs    int64          `json:"audit_duration_ms"`
	Context            SessionContext `json:"context"`
	Inconclusive       int            `json:"inconclusive"` // Could not determine appropriateness
}

// AuditEcphoryRetrieval performs async correctness audit for a session.
//
// Process:
//  1. Extract session context (language, framework, task_type) from transcript
//  2. Retrieve all ecphory queries for this session (from telemetry logs)
//  3. For each retrieved engram, check if it's appropriate for the context
//  4. Calculate correctness score: appropriate_count / total_count
//  5. Emit ecphory_audit_completed event
//
// Pattern matching logic:
//   - Appropriate: engram tags/content match session context
//   - Inappropriate: engram for different language/framework (e.g., Python session → Go engrams)
//   - Inconclusive: Cannot determine (insufficient metadata, generic engram)
//
// Target: ≥99% correctness (S9 validation plan)
//
// This function is called asynchronously (goroutine) to avoid blocking session.
// D4 NFR2: <5min p95 audit time (async, non-blocking).
func AuditEcphoryRetrieval(sessionID string, transcript string, retrievedEngrams []string, registry *ListenerRegistry) error {
	startTime := time.Now()

	// Extract session context
	sessionContext := ExtractContext(transcript)

	// Validate each retrieval
	appropriateCount := 0
	inappropriateCount := 0
	inconclusiveCount := 0

	for _, engramPath := range retrievedEngrams {
		match := matchEngramToContext(engramPath, sessionContext)
		switch match {
		case "appropriate":
			appropriateCount++
		case "inappropriate":
			inappropriateCount++
		case "inconclusive":
			inconclusiveCount++
		}
	}

	totalRetrievals := len(retrievedEngrams)

	// Calculate correctness score (exclude inconclusive from denominator per D4 spec)
	var correctnessScore float64
	conclusiveTotal := totalRetrievals - inconclusiveCount
	if conclusiveTotal > 0 {
		correctnessScore = float64(appropriateCount) / float64(conclusiveTotal)
	} else {
		correctnessScore = 0.0 // All inconclusive
	}

	auditDuration := time.Since(startTime)

	// Prepare audit result
	result := EcphoryAuditResult{
		SessionID:          sessionID,
		TotalRetrievals:    totalRetrievals,
		AppropriateCount:   appropriateCount,
		InappropriateCount: inappropriateCount,
		CorrectnessScore:   correctnessScore,
		AuditDurationMs:    auditDuration.Milliseconds(),
		Context:            sessionContext,
		Inconclusive:       inconclusiveCount,
	}

	// Emit audit event
	event := &Event{
		Timestamp: time.Now(),
		Type:      EventEcphoryAuditCompleted,
		Agent:     "engram",
		Level:     LevelInfo,
		Data: map[string]interface{}{
			"session_id":          result.SessionID,
			"total_retrievals":    result.TotalRetrievals,
			"appropriate_count":   result.AppropriateCount,
			"inappropriate_count": result.InappropriateCount,
			"correctness_score":   result.CorrectnessScore,
			"audit_duration_ms":   result.AuditDurationMs,
			"context":             result.Context,
			"inconclusive":        result.Inconclusive,
		},
	}

	registry.Notify(event)
	return nil
}

// matchEngramToContext determines if an engram is appropriate for session context.
//
// Returns: "appropriate", "inappropriate", or "inconclusive"
//
// Matching logic:
//   - Check engram path for language/framework indicators
//   - Check engram tags (if metadata available)
//   - Generic engrams (no language/framework specificity) → appropriate
//   - Mismatch (Python session, Go engram) → inappropriate
func matchEngramToContext(engramPath string, ctx SessionContext) string {
	engramPathLower := strings.ToLower(engramPath)
	filename := filepath.Base(engramPathLower)

	// Check for mismatches (inappropriate)
	if hasMismatch(engramPathLower, ctx) {
		return "inappropriate"
	}

	// Check if generic engram (always appropriate)
	if isGenericEngram(filename) {
		return "appropriate"
	}

	// Check if matches context (appropriate)
	if hasContextMatch(engramPathLower, ctx) {
		return "appropriate"
	}

	// Default: appropriate (benefit of doubt for unclear engrams)
	return "appropriate"
}

// hasMismatch checks if engram contains different language/framework than context
func hasMismatch(engramPathLower string, ctx SessionContext) bool {
	// Language mismatch
	if ctx.Language != "unknown" {
		otherLanguages := []string{"python", "go", "javascript", "typescript", "rust", "java", "cpp"}
		for _, lang := range otherLanguages {
			if lang == ctx.Language {
				continue
			}
			if hasRegexMatch(engramPathLower, lang) {
				return true
			}
		}
	}

	// Framework mismatch
	if ctx.Framework != "unknown" {
		otherFrameworks := []string{"django", "flask", "react", "vue", "angular", "express", "gin"}
		for _, framework := range otherFrameworks {
			if framework == ctx.Framework {
				continue
			}
			if hasRegexMatch(engramPathLower, framework) {
				return true
			}
		}
	}

	return false
}

// hasRegexMatch checks if text contains pattern with word boundaries
func hasRegexMatch(text, pattern string) bool {
	regex := `\b` + pattern + `\b`
	matched, _ := regexp.MatchString(regex, text)
	return matched
}

// isGenericEngram checks if filename indicates a generic engram
func isGenericEngram(filename string) bool {
	genericPatterns := []string{"workflow", "pattern", "best-practice", "guideline", "reference"}
	for _, pattern := range genericPatterns {
		if strings.Contains(filename, pattern) {
			return true
		}
	}
	return false
}

// hasContextMatch checks if engram matches session language/framework
func hasContextMatch(engramPathLower string, ctx SessionContext) bool {
	if ctx.Language != "unknown" && hasRegexMatch(engramPathLower, ctx.Language) {
		return true
	}
	if ctx.Framework != "unknown" && hasRegexMatch(engramPathLower, ctx.Framework) {
		return true
	}
	return false
}
