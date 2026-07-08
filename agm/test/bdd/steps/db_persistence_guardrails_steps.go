package steps

import (
	"context"
	"fmt"
	"time"

	"github.com/cucumber/godog"

	agmdb "github.com/vbonnet/dear-agent/agm/internal/db"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type dbPersistenceState struct {
	db            *agmdb.DB
	schemaObjects map[string]string
	session       *manifest.Manifest
	retrieved     *manifest.Manifest
	searchResults []*manifest.Manifest
}

type dbPersistenceStateKey struct{}

// RegisterDBPersistenceGuardrailSteps registers BDD steps for AGM database persistence.
func RegisterDBPersistenceGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, dbPersistenceStateKey{}, &dbPersistenceState{}), nil
	})
	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		state, ok := ctx.Value(dbPersistenceStateKey{}).(*dbPersistenceState)
		if ok && state != nil && state.db != nil {
			if closeErr := state.db.Close(); closeErr != nil && err == nil {
				return ctx, closeErr
			}
		}
		return ctx, nil
	})

	ctx.Step(`^an AGM in-memory database is open$`, anAGMInMemoryDatabaseIsOpen)
	ctx.Step(`^AGM inspects the database schema$`, agmInspectsTheDatabaseSchema)
	ctx.Step(`^the database should expose table "([^"]*)"$`, theDatabaseShouldExposeTable)
	ctx.Step(`^the database should expose view "([^"]*)"$`, theDatabaseShouldExposeView)
	ctx.Step(`^an AGM session manifest with harness "([^"]*)" and model "([^"]*)"$`, anAGMSessionManifestWithHarnessAndModel)
	ctx.Step(`^AGM stores and retrieves the session manifest$`, agmStoresAndRetrievesTheSessionManifest)
	ctx.Step(`^the retrieved session should preserve harness-neutral metadata$`, theRetrievedSessionShouldPreserveHarnessNeutralMetadata)
	ctx.Step(`^AGM has stored searchable sessions across harnesses$`, agmHasStoredSearchableSessionsAcrossHarnesses)
	ctx.Step(`^AGM searches sessions for "([^"]*)" with harness "([^"]*)"$`, agmSearchesSessionsForWithHarness)
	ctx.Step(`^the search results should include only session "([^"]*)"$`, theSearchResultsShouldIncludeOnlySession)
}

func anAGMInMemoryDatabaseIsOpen(ctx context.Context) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	database, err := agmdb.Open(":memory:")
	if err != nil {
		return err
	}
	state.db = database
	return nil
}

func agmInspectsTheDatabaseSchema(ctx context.Context) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	if state.db == nil {
		return fmt.Errorf("database is not open")
	}
	rows, err := state.db.Conn().Query(`
		SELECT name, type
		FROM sqlite_master
		WHERE type IN ('table', 'view')
	`)
	if err != nil {
		return fmt.Errorf("query sqlite schema: %w", err)
	}
	defer rows.Close()

	state.schemaObjects = make(map[string]string)
	for rows.Next() {
		var name, objectType string
		if err := rows.Scan(&name, &objectType); err != nil {
			return fmt.Errorf("scan sqlite schema: %w", err)
		}
		state.schemaObjects[name] = objectType
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate sqlite schema: %w", err)
	}
	return nil
}

func theDatabaseShouldExposeTable(ctx context.Context, name string) error {
	return databaseShouldExposeObject(ctx, name, "table")
}

func theDatabaseShouldExposeView(ctx context.Context, name string) error {
	return databaseShouldExposeObject(ctx, name, "view")
}

func anAGMSessionManifestWithHarnessAndModel(ctx context.Context, harness, model string) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Truncate(time.Second)
	state.session = &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     "db-bdd-session",
		Name:          "DB BDD Session",
		CreatedAt:     now,
		UpdatedAt:     now,
		Harness:       harness,
		Model:         model,
		Context: manifest.Context{
			Project: "dear-agent",
			Purpose: "Validate database persistence",
			Tags:    []string{"bdd", "db"},
			Notes:   "temporal workflow database coverage",
		},
		Claude: manifest.Claude{UUID: "claude-session-db-bdd"},
		Tmux:   manifest.Tmux{SessionName: "tmux-db-bdd"},
		EngramMetadata: &manifest.EngramMetadata{
			Enabled:   true,
			Query:     "db persistence guardrail",
			EngramIDs: []string{"engram-db-1", "engram-db-2"},
			LoadedAt:  now,
			Count:     2,
		},
	}
	return nil
}

func agmStoresAndRetrievesTheSessionManifest(ctx context.Context) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	if state.db == nil {
		return fmt.Errorf("database is not open")
	}
	if state.session == nil {
		return fmt.Errorf("session manifest is not configured")
	}
	if err := state.db.CreateSession(state.session); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	retrieved, err := state.db.GetSession(state.session.SessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}
	state.retrieved = retrieved
	return nil
}

func theRetrievedSessionShouldPreserveHarnessNeutralMetadata(ctx context.Context) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	if state.session == nil || state.retrieved == nil {
		return fmt.Errorf("session manifest was not round-tripped")
	}
	want := state.session
	got := state.retrieved
	if got.Harness != want.Harness || got.Model != want.Model {
		return fmt.Errorf("retrieved harness/model = %q/%q, want %q/%q", got.Harness, got.Model, want.Harness, want.Model)
	}
	if got.Context.Project != want.Context.Project || got.Context.Purpose != want.Context.Purpose || got.Context.Notes != want.Context.Notes {
		return fmt.Errorf("retrieved context = %#v, want %#v", got.Context, want.Context)
	}
	if len(got.Context.Tags) != len(want.Context.Tags) {
		return fmt.Errorf("retrieved context tags = %v, want %v", got.Context.Tags, want.Context.Tags)
	}
	for i := range got.Context.Tags {
		if got.Context.Tags[i] != want.Context.Tags[i] {
			return fmt.Errorf("retrieved context tags = %v, want %v", got.Context.Tags, want.Context.Tags)
		}
	}
	if got.Claude.UUID != want.Claude.UUID || got.Tmux.SessionName != want.Tmux.SessionName {
		return fmt.Errorf("retrieved native metadata = %q/%q, want %q/%q", got.Claude.UUID, got.Tmux.SessionName, want.Claude.UUID, want.Tmux.SessionName)
	}
	if got.EngramMetadata == nil || want.EngramMetadata == nil {
		return fmt.Errorf("engram metadata was not preserved")
	}
	if got.EngramMetadata.Enabled != want.EngramMetadata.Enabled ||
		got.EngramMetadata.Query != want.EngramMetadata.Query ||
		got.EngramMetadata.Count != want.EngramMetadata.Count ||
		len(got.EngramMetadata.EngramIDs) != len(want.EngramMetadata.EngramIDs) {
		return fmt.Errorf("retrieved engram metadata = %#v, want %#v", got.EngramMetadata, want.EngramMetadata)
	}
	for i := range got.EngramMetadata.EngramIDs {
		if got.EngramMetadata.EngramIDs[i] != want.EngramMetadata.EngramIDs[i] {
			return fmt.Errorf("retrieved engram IDs = %v, want %v", got.EngramMetadata.EngramIDs, want.EngramMetadata.EngramIDs)
		}
	}
	return nil
}

func agmHasStoredSearchableSessionsAcrossHarnesses(ctx context.Context) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	if state.db == nil {
		return fmt.Errorf("database is not open")
	}
	now := time.Now().UTC().Truncate(time.Second)
	sessions := []*manifest.Manifest{
		searchableSession("db-search-codex", "codex-cli", "temporal database workflow", now),
		searchableSession("db-search-claude", "claude-code", "temporal database workflow", now.Add(-time.Minute)),
		searchableSession("db-search-opencode", "opencode-cli", "unrelated plugin marketplace", now.Add(-2*time.Minute)),
	}
	for _, session := range sessions {
		if err := state.db.CreateSession(session); err != nil {
			return fmt.Errorf("create searchable session %s: %w", session.SessionID, err)
		}
	}
	return nil
}

func agmSearchesSessionsForWithHarness(ctx context.Context, query, harness string) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	if state.db == nil {
		return fmt.Errorf("database is not open")
	}
	results, err := state.db.SearchSessions(query, &agmdb.SearchOptions{
		Limit: 10,
		Filter: agmdb.Filter{
			Harness: harness,
		},
	})
	if err != nil {
		return fmt.Errorf("search sessions: %w", err)
	}
	state.searchResults = results
	return nil
}

func theSearchResultsShouldIncludeOnlySession(ctx context.Context, sessionID string) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	if len(state.searchResults) != 1 {
		return fmt.Errorf("search results length = %d, want 1", len(state.searchResults))
	}
	if state.searchResults[0].SessionID != sessionID {
		return fmt.Errorf("search result session = %q, want %q", state.searchResults[0].SessionID, sessionID)
	}
	return nil
}

func databaseShouldExposeObject(ctx context.Context, name, objectType string) error {
	state, err := getDBPersistenceState(ctx)
	if err != nil {
		return err
	}
	gotType, ok := state.schemaObjects[name]
	if !ok {
		return fmt.Errorf("database schema does not expose %s %q", objectType, name)
	}
	if gotType != objectType {
		return fmt.Errorf("database object %q type = %q, want %q", name, gotType, objectType)
	}
	return nil
}

func searchableSession(sessionID, harness, text string, when time.Time) *manifest.Manifest {
	return &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sessionID,
		Name:          text,
		CreatedAt:     when,
		UpdatedAt:     when,
		Harness:       harness,
		Context: manifest.Context{
			Project: "dear-agent",
			Purpose: text,
			Tags:    []string{"bdd", harness},
			Notes:   text,
		},
		Tmux: manifest.Tmux{SessionName: "tmux-" + sessionID},
	}
}

func getDBPersistenceState(ctx context.Context) (*dbPersistenceState, error) {
	state, ok := ctx.Value(dbPersistenceStateKey{}).(*dbPersistenceState)
	if !ok || state == nil {
		return nil, fmt.Errorf("database persistence state not initialized")
	}
	return state, nil
}
