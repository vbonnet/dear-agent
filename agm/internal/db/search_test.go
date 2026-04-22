package db

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// setupSearchTestDB creates an in-memory SQLite database with schema for search tests
func setupSearchTestDB(t *testing.T) *DB {
	db, err := Open(":memory:")
	require.NoError(t, err, "Failed to open in-memory database")
	return db
}

// insertTestSession inserts a test session into the database
func insertTestSession(t *testing.T, db *DB, m *manifest.Manifest) {
	// Marshal tags and engram IDs to JSON
	tagsJSON := "null"
	if len(m.Context.Tags) > 0 {
		tagsBytes, _ := json.Marshal(m.Context.Tags)
		tagsJSON = string(tagsBytes)
	}

	engramIDsJSON := "null"
	engramEnabled := 0
	var engramLoadedAt interface{} = nil
	if m.EngramMetadata != nil && m.EngramMetadata.Enabled {
		engramEnabled = 1
		if len(m.EngramMetadata.EngramIDs) > 0 {
			idsBytes, _ := json.Marshal(m.EngramMetadata.EngramIDs)
			engramIDsJSON = string(idsBytes)
		}
		if !m.EngramMetadata.LoadedAt.IsZero() {
			engramLoadedAt = m.EngramMetadata.LoadedAt
		}
	}

	query := `
		INSERT INTO sessions (
			session_id, name, schema_version, created_at, updated_at,
			lifecycle, harness, model, context_project, context_purpose, context_tags,
			context_notes, claude_uuid, tmux_session_name, engram_enabled,
			engram_query, engram_ids, engram_loaded_at, engram_count,
			parent_session_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	engramQuery := ""
	engramCount := 0
	if m.EngramMetadata != nil {
		engramQuery = m.EngramMetadata.Query
		engramCount = m.EngramMetadata.Count
	}

	_, err := db.conn.Exec(query,
		m.SessionID, m.Name, m.SchemaVersion, m.CreatedAt, m.UpdatedAt,
		m.Lifecycle, m.Harness, m.Model, m.Context.Project, m.Context.Purpose, tagsJSON,
		m.Context.Notes, m.Claude.UUID, m.Tmux.SessionName, engramEnabled,
		engramQuery, engramIDsJSON, engramLoadedAt, engramCount,
		nil, // parent_session_id
	)
	require.NoError(t, err, "Failed to insert test session")
}

// TestSearchSessions_AND tests AND queries
func TestSearchSessions_AND(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert test data
	sessions := []*manifest.Manifest{
		{
			SessionID:     "session-1",
			Name:          "temporal workflow implementation",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now().Add(-2 * time.Hour),
			UpdatedAt:     time.Now().Add(-1 * time.Hour),
			Lifecycle:     "",
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
				Purpose: "Implement temporal workflows for async execution",
			},
		},
		{
			SessionID:     "session-2",
			Name:          "temporal activities",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now().Add(-3 * time.Hour),
			UpdatedAt:     time.Now().Add(-2 * time.Hour),
			Lifecycle:     "",
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
				Purpose: "Create activity functions",
			},
		},
		{
			SessionID:     "session-3",
			Name:          "database schema design",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now().Add(-4 * time.Hour),
			UpdatedAt:     time.Now().Add(-3 * time.Hour),
			Lifecycle:     "",
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-persistence",
				Purpose: "Design SQLite schema for sessions",
			},
		},
	}

	for _, s := range sessions {
		insertTestSession(t, db, s)
	}

	// Test AND query - should match only session-1
	results, err := db.SearchSessions("temporal AND workflow", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results), "Expected 1 result for AND query")
	if len(results) > 0 {
		assert.Equal(t, "session-1", results[0].SessionID)
	}

	// Test AND query with no matches
	results, err = db.SearchSessions("temporal AND database", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(results), "Expected 0 results for non-matching AND query")
}

// TestSearchSessions_OR tests OR queries
func TestSearchSessions_OR(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert test data
	sessions := []*manifest.Manifest{
		{
			SessionID:     "session-1",
			Name:          "temporal workflow",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
			},
		},
		{
			SessionID:     "session-2",
			Name:          "database schema",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-persistence",
			},
		},
		{
			SessionID:     "session-3",
			Name:          "frontend components",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "ui-redesign",
			},
		},
	}

	for _, s := range sessions {
		insertTestSession(t, db, s)
	}

	// Test OR query - should match sessions 1 and 2
	results, err := db.SearchSessions("temporal OR database", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, len(results), "Expected 2 results for OR query")

	// Verify correct sessions returned
	sessionIDs := make(map[string]bool)
	for _, r := range results {
		sessionIDs[r.SessionID] = true
	}
	assert.True(t, sessionIDs["session-1"], "Expected session-1 in results")
	assert.True(t, sessionIDs["session-2"], "Expected session-2 in results")
}

// TestSearchSessions_Phrase tests phrase searches
func TestSearchSessions_Phrase(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert test data
	sessions := []*manifest.Manifest{
		{
			SessionID:     "session-1",
			Name:          "temporal workflow system",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
				Purpose: "Build a temporal workflow engine",
			},
		},
		{
			SessionID:     "session-2",
			Name:          "workflow temporal design",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
				Purpose: "Design workflow components",
			},
		},
	}

	for _, s := range sessions {
		insertTestSession(t, db, s)
	}

	// Test exact phrase - should only match session-1
	results, err := db.SearchSessions(`"temporal workflow"`, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results), "Expected 1 result for phrase search")
	if len(results) > 0 {
		assert.Equal(t, "session-1", results[0].SessionID)
	}
}

// TestSearchSessions_ColumnSpecific tests column-specific searches
func TestSearchSessions_ColumnSpecific(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert test data
	sessions := []*manifest.Manifest{
		{
			SessionID:     "session-1",
			Name:          "agm session manager",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-core",
				Purpose: "Core session management",
			},
		},
		{
			SessionID:     "session-2",
			Name:          "temporal workflow",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
				Purpose: "Implement workflows",
			},
		},
	}

	for _, s := range sessions {
		insertTestSession(t, db, s)
	}

	// Test name-specific search
	results, err := db.SearchSessions("name:agm", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results), "Expected 1 result for name-specific search")
	if len(results) > 0 {
		assert.Equal(t, "session-1", results[0].SessionID)
	}

	// Test context_project-specific search
	results, err = db.SearchSessions("context_project:temporal", nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(results), 1, "Expected at least 1 result for project-specific search")
	// Both sessions have "temporal" in project name, so we just check we got results
}

// TestSearchSessions_Pagination tests pagination (limit/offset)
func TestSearchSessions_Pagination(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert 10 test sessions
	for i := 0; i < 10; i++ {
		session := &manifest.Manifest{
			SessionID:     fmt.Sprintf("session-%d", i),
			Name:          fmt.Sprintf("temporal session %d", i),
			SchemaVersion: "2.0",
			CreatedAt:     time.Now().Add(time.Duration(-i) * time.Hour),
			UpdatedAt:     time.Now().Add(time.Duration(-i) * time.Hour),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
			},
		}
		insertTestSession(t, db, session)
	}

	// Test limit
	opts := &SearchOptions{Limit: 5, Offset: 0}
	results, err := db.SearchSessions("temporal", opts)
	require.NoError(t, err)
	assert.Equal(t, 5, len(results), "Expected 5 results with limit=5")

	// Test offset
	opts = &SearchOptions{Limit: 5, Offset: 5}
	results, err = db.SearchSessions("temporal", opts)
	require.NoError(t, err)
	assert.Equal(t, 5, len(results), "Expected 5 results with limit=5, offset=5")

	// Test offset beyond results
	opts = &SearchOptions{Limit: 5, Offset: 15}
	results, err = db.SearchSessions("temporal", opts)
	require.NoError(t, err)
	assert.Equal(t, 0, len(results), "Expected 0 results with offset beyond data")
}

// TestSearchSessions_Performance tests performance with large dataset
func TestSearchSessions_Performance(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert 1000+ sessions
	numSessions := 1000
	t.Logf("Inserting %d test sessions...", numSessions)

	start := time.Now()
	for i := 0; i < numSessions; i++ {
		session := &manifest.Manifest{
			SessionID:     fmt.Sprintf("session-%d", i),
			Name:          fmt.Sprintf("temporal workflow %d", i),
			SchemaVersion: "2.0",
			CreatedAt:     time.Now().Add(time.Duration(-i) * time.Minute),
			UpdatedAt:     time.Now().Add(time.Duration(-i) * time.Minute),
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: fmt.Sprintf("project-%d", i%10),
				Purpose: fmt.Sprintf("Implement feature %d", i),
				Notes:   fmt.Sprintf("Session notes for session %d with various keywords like temporal, workflow, database", i),
			},
		}
		insertTestSession(t, db, session)
	}
	insertDuration := time.Since(start)
	t.Logf("Inserted %d sessions in %v", numSessions, insertDuration)

	// Test various query types
	testCases := []struct {
		name  string
		query string
	}{
		{"Simple term", "temporal"},
		{"AND query", "temporal AND workflow"},
		{"OR query", "temporal OR database"},
		{"Phrase search", `"temporal workflow"`},
		{"Column-specific", "name:temporal"},
		{"Complex query", "temporal AND workflow OR database"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			results, err := db.SearchSessions(tc.query, DefaultSearchOptions())
			duration := time.Since(start)

			require.NoError(t, err, "Search query failed")
			t.Logf("Query '%s' returned %d results in %v", tc.query, len(results), duration)

			// Verify search completes in <100ms
			assert.Less(t, duration.Milliseconds(), int64(100),
				"Search should complete in <100ms, took %v", duration)
		})
	}
}

// TestBuildFTS5Query tests the query builder
func TestBuildFTS5Query(t *testing.T) {
	testCases := []struct {
		name     string
		terms    []string
		operator string
		expected string
	}{
		{
			name:     "AND operator",
			terms:    []string{"temporal", "workflow"},
			operator: "AND",
			expected: "temporal AND workflow",
		},
		{
			name:     "OR operator",
			terms:    []string{"temporal", "workflow"},
			operator: "OR",
			expected: "temporal OR workflow",
		},
		{
			name:     "Default to AND",
			terms:    []string{"temporal", "workflow"},
			operator: "",
			expected: "temporal AND workflow",
		},
		{
			name:     "Phrase search preserved",
			terms:    []string{`"temporal workflow"`, "database"},
			operator: "AND",
			expected: `"temporal workflow" AND database`,
		},
		{
			name:     "Column-specific preserved",
			terms:    []string{"name:agm", "temporal"},
			operator: "AND",
			expected: "name:agm AND temporal",
		},
		{
			name:     "Single term",
			terms:    []string{"temporal"},
			operator: "AND",
			expected: "temporal",
		},
		{
			name:     "Empty terms",
			terms:    []string{},
			operator: "AND",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := BuildFTS5Query(tc.terms, tc.operator)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSearchSessions_EmptyQuery tests error handling for empty queries
func TestSearchSessions_EmptyQuery(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	_, err := db.SearchSessions("", nil)
	assert.Error(t, err, "Expected error for empty query")
	assert.Contains(t, err.Error(), "empty", "Error should mention empty query")
}

// TestSearchSessions_InvalidQuery tests error handling for invalid queries
func TestSearchSessions_InvalidQuery(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Unbalanced quotes
	_, err := db.SearchSessions(`temporal "workflow`, nil)
	assert.Error(t, err, "Expected error for unbalanced quotes")
}

// TestSearchSessions_NoResults tests handling of no results
func TestSearchSessions_NoResults(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert one session
	session := &manifest.Manifest{
		SessionID:     "session-1",
		Name:          "temporal workflow",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "agm-temporal",
		},
	}
	insertTestSession(t, db, session)

	// Search for non-existent term
	results, err := db.SearchSessions("nonexistent", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, len(results), "Expected 0 results for non-matching query")
	assert.NotNil(t, results, "Results should be non-nil empty slice")
}

// TestSearchSessions_Filters tests filter options
func TestSearchSessions_Filters(t *testing.T) {
	db := setupSearchTestDB(t)
	defer db.Close()

	// Insert test data
	sessions := []*manifest.Manifest{
		{
			SessionID:     "session-1",
			Name:          "temporal workflow",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now().Add(-2 * time.Hour),
			UpdatedAt:     time.Now(),
			Lifecycle:     "",
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
			},
		},
		{
			SessionID:     "session-2",
			Name:          "temporal activities",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now().Add(-1 * time.Hour),
			UpdatedAt:     time.Now(),
			Lifecycle:     "archived",
			Harness:       "claude-code",
			Context: manifest.Context{
				Project: "agm-temporal",
			},
		},
		{
			SessionID:     "session-3",
			Name:          "temporal integration",
			SchemaVersion: "2.0",
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "",
			Harness:       "gemini-cli",
			Context: manifest.Context{
				Project: "agm-temporal",
			},
		},
	}

	for _, s := range sessions {
		insertTestSession(t, db, s)
	}

	// Test lifecycle filter
	opts := &SearchOptions{
		Filter: Filter{
			Lifecycle: "archived",
		},
	}
	results, err := db.SearchSessions("temporal", opts)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results), "Expected 1 archived session")
	if len(results) > 0 {
		assert.Equal(t, "session-2", results[0].SessionID)
	}

	// Test harness filter
	opts = &SearchOptions{
		Filter: Filter{
			Harness: "gemini-cli",
		},
	}
	results, err = db.SearchSessions("temporal", opts)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results), "Expected 1 gemini session")
	if len(results) > 0 {
		assert.Equal(t, "session-3", results[0].SessionID)
	}

	// Test date filter - sessions created after 90 minutes ago
	// session-2 (1hr ago) and session-3 (now) should match
	opts = &SearchOptions{
		Filter: Filter{
			CreatedAfter: time.Now().Add(-90 * time.Minute),
		},
	}
	results, err = db.SearchSessions("temporal", opts)
	require.NoError(t, err)
	assert.Equal(t, 2, len(results), "Expected 2 recent sessions (created in last 90 minutes)")

	// Test more restrictive date filter - only session-3 should match
	opts = &SearchOptions{
		Filter: Filter{
			CreatedAfter: time.Now().Add(-30 * time.Minute),
		},
	}
	results, err = db.SearchSessions("temporal", opts)
	require.NoError(t, err)
	assert.Equal(t, 1, len(results), "Expected 1 very recent session")
	if len(results) > 0 {
		assert.Equal(t, "session-3", results[0].SessionID)
	}
}
