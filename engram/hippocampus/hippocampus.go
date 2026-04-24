package hippocampus

import (
	"fmt"
	"os"
	"time"
)

// Hippocampus consolidates memories from short-term (session history)
// to long-term storage (structured artifacts).
//
// Brain analogy: During sleep, hippocampus transfers memories to cortex.
// In Engram: Consolidate session history to structured summaries.
type Hippocampus struct {
	archiveDir string // ~/engram/consolidation/sleep-cycles/
}

// Consolidation represents the result of memory consolidation.
type Consolidation struct {
	SessionID          string
	Timestamp          time.Time
	TokensBefore       int
	TokensAfter        int
	Decisions          []Decision
	Outcomes           []Outcome
	TechnicalLearnings []Learning
	ProcessLearnings   []Learning
	ActivePlan         *Plan
	Engrams            []string
	ArchivePath        string
}

// Decision represents a key decision made during the session.
type Decision struct {
	Title     string
	Rationale string
	Impact    string
}

// Outcome represents a concrete result achieved.
type Outcome struct {
	Description string
	Evidence    string
}

// Learning represents knowledge gained.
type Learning struct {
	Learning    string
	Context     string
	Application string
}

// Plan represents active Wayfinder Plan state.
type Plan struct {
	Status       string
	CurrentPhase string
	NextSteps    []string
}

// New creates a new Hippocampus instance.
func New(archiveDir string) (*Hippocampus, error) {
	// Ensure archive directory exists
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create archive dir: %w", err)
	}

	return &Hippocampus{
		archiveDir: archiveDir,
	}, nil
}

// ConsolidateMemory extracts key information from session history.
//
// Brain analogy: Transfer memories from hippocampus to cortex during sleep.
//
// Phase 5 V1: Stub implementation (creates consolidation structure).
// Phase 5 V2: Full extraction with pattern matching.
// Phase 6+: LLM-enhanced semantic extraction.
func (h *Hippocampus) ConsolidateMemory(sessionID string, history string) (*Consolidation, error) {
	consolidation := &Consolidation{
		SessionID: sessionID,
		Timestamp: time.Now(),
	}

	// Extract key information
	var err error

	consolidation.Decisions, err = h.extractDecisions(history)
	if err != nil {
		return nil, fmt.Errorf("failed to extract decisions: %w", err)
	}

	consolidation.Outcomes, err = h.extractOutcomes(history)
	if err != nil {
		return nil, fmt.Errorf("failed to extract outcomes: %w", err)
	}

	consolidation.TechnicalLearnings, err = h.extractTechnicalLearnings(history)
	if err != nil {
		return nil, fmt.Errorf("failed to extract technical learnings: %w", err)
	}

	consolidation.ProcessLearnings, err = h.extractProcessLearnings(history)
	if err != nil {
		return nil, fmt.Errorf("failed to extract process learnings: %w", err)
	}

	consolidation.ActivePlan, err = h.extractActivePlan(history)
	if err != nil {
		return nil, fmt.Errorf("failed to extract plan: %w", err)
	}

	consolidation.Engrams, err = h.extractEngrams(history)
	if err != nil {
		return nil, fmt.Errorf("failed to extract engrams: %w", err)
	}

	// Archive full history
	archivePath, err := h.archiveSession(sessionID, history)
	if err != nil {
		return nil, fmt.Errorf("failed to archive session: %w", err)
	}
	consolidation.ArchivePath = archivePath

	// Generate consolidation artifact
	if err := h.generateConsolidationArtifact(consolidation); err != nil {
		return nil, fmt.Errorf("failed to generate artifact: %w", err)
	}

	return consolidation, nil
}

// ArchiveDir returns the archive directory path.
func (h *Hippocampus) ArchiveDir() string {
	return h.archiveDir
}
