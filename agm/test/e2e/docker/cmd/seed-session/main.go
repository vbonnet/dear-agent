// Command seed-session creates the AGM lifecycle record an E2E fixture needs.
//
// The reaper resolves its target through the lifecycle store, not through a
// manifest.yaml on disk: ops.ArchiveSession looks the session up by identifier
// in storage, and the reaper's archive preflight runs before it touches the
// pane. A fixture that only writes a manifest file therefore fails at
// preflight with AGM-001 before any reaping happens.
//
// This seeds the same record `adapter.CreateSession` produces for the reaper's
// own unit tests, using the product's storage API rather than hand-written SQL,
// so the fixture cannot drift from the schema. It deliberately does not go
// through `agm session new`: that spawns a real harness and runs the workspace
// detection and spawn circuit breaker, none of which this test is about.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed-session: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	sessionID := flag.String("session-id", "", "Stable AGM session ID to create")
	name := flag.String("name", "", "Session name")
	tmuxSession := flag.String("tmux-session", "", "tmux session name (defaults to --name)")
	harness := flag.String("harness", "claude-code", "Harness identity for the graceful-exit command")
	project := flag.String("project", "", "Project directory recorded on the session")
	flag.Parse()

	if *sessionID == "" || *name == "" {
		return fmt.Errorf("--session-id and --name are required")
	}
	dbPath := os.Getenv("AGM_DB_PATH")
	if dbPath == "" {
		return fmt.Errorf("AGM_DB_PATH must be set so the fixture writes to the isolated store")
	}

	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		return fmt.Errorf("open lifecycle storage at %s: %w", dbPath, err)
	}
	defer func() { _ = adapter.Close() }()
	if err := adapter.ApplyMigrations(); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	tmux := *tmuxSession
	if tmux == "" {
		tmux = *name
	}
	now := time.Now().UTC()
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     *sessionID,
		Name:          *name,
		Harness:       *harness,
		CreatedAt:     now,
		UpdatedAt:     now,
		Context:       manifest.Context{Project: *project},
		Tmux:          manifest.Tmux{SessionName: tmux},
	}
	if err := adapter.CreateSession(m); err != nil {
		return fmt.Errorf("create session %s: %w", *sessionID, err)
	}
	fmt.Printf("seeded session %s (name=%s, tmux=%s, harness=%s)\n", *sessionID, *name, tmux, *harness)
	return nil
}
