package hippocampus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	// Create temporary archive directory
	tmpDir := filepath.Join(os.TempDir(), "hippocampus-test")
	defer os.RemoveAll(tmpDir)

	h, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create Hippocampus: %v", err)
	}

	if h.ArchiveDir() != tmpDir {
		t.Errorf("Expected archive dir %s, got %s", tmpDir, h.ArchiveDir())
	}

	// Verify directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Errorf("Archive directory was not created")
	}
}

func TestConsolidateMemory(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "hippocampus-test-consolidate")
	defer os.RemoveAll(tmpDir)

	h, _ := New(tmpDir)

	// Test history with markers
	history := `
# Engram Brain Refactor - Phase 4

## Decision: Use CLI wrapper for Thalamus
CSM uses internal/ packages which are not exportable.

- Completed: Phase 4 Thalamus Wrapper
- Implemented: Sleep Cycle stub
- Created: core/thalamus/ module

Learned: CSM requires CLI wrapper approach
Discovered: Go internal/ packages not exportable

engrams used: bash-command-simplification.ai.md, claude-code-tool-usage.ai.md

Current Phase: Phase 5 Sleep Cycles Implementation
`

	consolidation, err := h.ConsolidateMemory("test-session-123", history)
	if err != nil {
		t.Fatalf("Consolidation failed: %v", err)
	}

	// Verify consolidation structure
	if consolidation.SessionID != "test-session-123" {
		t.Errorf("Unexpected session ID: %s", consolidation.SessionID)
	}

	// Verify decisions extracted
	if len(consolidation.Decisions) == 0 {
		t.Errorf("Expected decisions to be extracted")
	}

	// Verify outcomes extracted
	if len(consolidation.Outcomes) == 0 {
		t.Errorf("Expected outcomes to be extracted")
	}

	// Verify learnings extracted
	if len(consolidation.TechnicalLearnings) == 0 {
		t.Errorf("Expected technical learnings to be extracted")
	}

	// Verify engrams extracted
	if len(consolidation.Engrams) == 0 {
		t.Errorf("Expected engrams to be extracted")
	}

	// Verify active plan extracted
	if consolidation.ActivePlan == nil {
		t.Errorf("Expected active plan to be extracted")
	}

	// Verify archive path set
	if consolidation.ArchivePath == "" {
		t.Errorf("Expected archive path to be set")
	}

	// Verify consolidation artifact was created
	artifactPath := filepath.Join(tmpDir, consolidation.Timestamp.Format("2006-01-02-15-04-05")+".md")
	if _, err := os.Stat(artifactPath); os.IsNotExist(err) {
		t.Errorf("Consolidation artifact was not created at %s", artifactPath)
	}

	// Verify archive file was created
	if _, err := os.Stat(consolidation.ArchivePath); os.IsNotExist(err) {
		t.Errorf("Archive file was not created at %s", consolidation.ArchivePath)
	}
}

func TestExtractDecisions(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "hippocampus-test-decisions")
	defer os.RemoveAll(tmpDir)

	h, _ := New(tmpDir)

	history := `
## Decision: Use CLI wrapper for Thalamus
CSM uses internal/ packages.

**Decision**: Consolidate MCP plugins into Synapse
Multiple plugins became single directory.
`

	decisions := h.extractDecisions(history)

	if len(decisions) != 2 {
		t.Errorf("Expected 2 decisions, got %d", len(decisions))
	}

	if decisions[0].Title != "Use CLI wrapper for Thalamus" {
		t.Errorf("Unexpected decision title: %s", decisions[0].Title)
	}
}

func TestExtractOutcomes(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "hippocampus-test-outcomes")
	defer os.RemoveAll(tmpDir)

	h, _ := New(tmpDir)

	history := `
- Completed: Phase 4 Thalamus Wrapper
- Implemented: Sleep Cycle stub
- Created: core/thalamus/ module
`

	outcomes := h.extractOutcomes(history)

	if len(outcomes) != 3 {
		t.Errorf("Expected 3 outcomes, got %d", len(outcomes))
	}
}

func TestExtractEngrams(t *testing.T) {
	tmpDir := filepath.Join(os.TempDir(), "hippocampus-test-engrams")
	defer os.RemoveAll(tmpDir)

	h, _ := New(tmpDir)

	history := `
Using bash-command-simplification.ai.md and claude-code-tool-usage.ai.md.
Also referenced ask-user-question-usage.ai.md.
`

	engrams := h.extractEngrams(history)

	if len(engrams) != 3 {
		t.Errorf("Expected 3 engrams, got %d", len(engrams))
	}

	// Verify deduplication (if same engram mentioned twice)
	seen := make(map[string]bool)
	for _, e := range engrams {
		if seen[e] {
			t.Errorf("Duplicate engram found: %s", e)
		}
		seen[e] = true
	}
}
