package session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sebdah/goldie/v2"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestManifestGeneration_NewSession tests golden snapshot for new session manifest
func TestManifestGeneration_NewSession(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	// Create a new session manifest with fixed timestamps for reproducibility
	fixedTime := time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-session-001",
		Name:          "test-new-session",
		CreatedAt:     fixedTime,
		UpdatedAt:     fixedTime,
		Lifecycle:     "", // active/stopped
		Workspace:     "oss",
		Context: manifest.Context{
			Project: "~/src/test-project",
			Purpose: "Testing manifest golden snapshots",
			Tags:    []string{"test", "golden", "new"},
			Notes:   "This is a new session for testing",
		},
		Claude: manifest.Claude{
			UUID: "claude-uuid-12345",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-new-session-tmux",
		},
		Harness: "claude-code",
	}

	// Marshal to JSON for golden comparison
	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	g.Assert(t, "manifest-new-session", jsonData)
}

// TestManifestGeneration_ResumedSession tests golden snapshot for resumed session
func TestManifestGeneration_ResumedSession(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	// Create a resumed session manifest
	createdTime := time.Date(2026, 2, 19, 14, 30, 0, 0, time.UTC)
	updatedTime := time.Date(2026, 2, 20, 10, 15, 0, 0, time.UTC)
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-session-002",
		Name:          "test-resumed-session",
		CreatedAt:     createdTime,
		UpdatedAt:     updatedTime,
		Lifecycle:     "", // active
		Workspace:     "acme",
		Context: manifest.Context{
			Project: "~/src/resumed-project",
			Purpose: "Testing resumed session manifest",
			Tags:    []string{"test", "golden", "resumed"},
			Notes:   "Session resumed after stopping",
		},
		Claude: manifest.Claude{
			UUID: "claude-uuid-67890",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-resumed-session-tmux",
		},
		Harness: "claude-code",
	}

	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	g.Assert(t, "manifest-resumed-session", jsonData)
}

// TestManifestGeneration_ArchivedSession tests golden snapshot for archived session
func TestManifestGeneration_ArchivedSession(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	// Create an archived session manifest
	createdTime := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	updatedTime := time.Date(2026, 2, 1, 16, 45, 0, 0, time.UTC)
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-session-003",
		Name:          "test-archived-session",
		CreatedAt:     createdTime,
		UpdatedAt:     updatedTime,
		Lifecycle:     manifest.LifecycleArchived,
		Workspace:     "oss",
		Context: manifest.Context{
			Project: "~/src/archived-project",
			Purpose: "Completed testing project",
			Tags:    []string{"test", "golden", "archived", "completed"},
			Notes:   "Project completed and archived",
		},
		Claude: manifest.Claude{
			UUID: "claude-uuid-archived-111",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-archived-session-tmux",
		},
		Harness: "claude-code",
	}

	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	g.Assert(t, "manifest-archived-session", jsonData)
}

// TestManifestGeneration_WithEngramMetadata tests manifest with Engram integration
func TestManifestGeneration_WithEngramMetadata(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	fixedTime := time.Date(2026, 2, 20, 11, 30, 0, 0, time.UTC)
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-session-004",
		Name:          "test-engram-session",
		CreatedAt:     fixedTime,
		UpdatedAt:     fixedTime,
		Lifecycle:     "",
		Workspace:     "oss",
		Context: manifest.Context{
			Project: "~/src/engram-project",
			Purpose: "Testing Engram integration",
			Tags:    []string{"test", "engram", "golden"},
		},
		Claude: manifest.Claude{
			UUID: "claude-uuid-engram-999",
		},
		Tmux: manifest.Tmux{
			SessionName: "test-engram-session-tmux",
		},
		Harness: "claude-code",
		EngramMetadata: &manifest.EngramMetadata{
			Enabled:   true,
			Query:     "test query for engrams",
			EngramIDs: []string{"engram-001", "engram-002", "engram-003"},
			LoadedAt:  fixedTime,
			Count:     3,
		},
	}

	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	g.Assert(t, "manifest-engram-session", jsonData)
}

// TestManifestGeneration_GeminiAgent tests manifest with Gemini agent
func TestManifestGeneration_GeminiAgent(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	fixedTime := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-session-005",
		Name:          "test-gemini-session",
		CreatedAt:     fixedTime,
		UpdatedAt:     fixedTime,
		Lifecycle:     "",
		Workspace:     "oss",
		Context: manifest.Context{
			Project: "~/src/gemini-project",
			Purpose: "Testing Gemini agent configuration",
			Tags:    []string{"test", "gemini", "golden"},
		},
		Claude: manifest.Claude{
			UUID: "", // Empty for Gemini sessions
		},
		Tmux: manifest.Tmux{
			SessionName: "test-gemini-session-tmux",
		},
		Harness: "gemini-cli",
	}

	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	g.Assert(t, "manifest-gemini-agent", jsonData)
}

// TestManifestGeneration_MinimalFields tests manifest with minimal required fields
func TestManifestGeneration_MinimalFields(t *testing.T) {
	g := goldie.New(t,
		goldie.WithFixtureDir("../../test/golden"),
		goldie.WithNameSuffix(".json"),
	)

	fixedTime := time.Date(2026, 2, 20, 13, 0, 0, 0, time.UTC)
	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     "test-session-006",
		Name:          "test-minimal-session",
		CreatedAt:     fixedTime,
		UpdatedAt:     fixedTime,
		Lifecycle:     "",
		Context: manifest.Context{
			Project: "~/src/minimal-project",
		},
		Claude: manifest.Claude{},
		Tmux: manifest.Tmux{
			SessionName: "test-minimal-session-tmux",
		},
	}

	jsonData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	g.Assert(t, "manifest-minimal-fields", jsonData)
}
