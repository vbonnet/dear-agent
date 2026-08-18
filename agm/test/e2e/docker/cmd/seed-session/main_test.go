package main

import (
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

// The fixture exists to satisfy the reaper's archive preflight, which resolves
// the session out of the lifecycle store. If the seeded record stops being
// readable by that store the E2E regresses to the AGM-001 failure this command
// was written to remove, so the round-trip is the contract worth pinning.
func TestRun_SeedsAReadableLifecycleRecord(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agm.db")
	t.Setenv("AGM_DB_PATH", dbPath)
	args := []string{"--session-id", "seeded-id", "--name", "seeded-name",
		"--harness", "claude-code", "--project", t.TempDir()}

	if err := run(args); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	adapter, err := dolt.NewSQLiteAdapter(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteAdapter() error: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	stored, err := adapter.GetSession("seeded-id")
	if err != nil {
		t.Fatalf("GetSession() error: %v", err)
	}
	if stored.Name != "seeded-name" {
		t.Errorf("Name = %q, want %q", stored.Name, "seeded-name")
	}
	// The reaper stops the pane by tmux identity, so an unset tmux session
	// name would leave it reaping nothing.
	if stored.Tmux.SessionName != "seeded-name" {
		t.Errorf("Tmux.SessionName = %q, want it to default to the name", stored.Tmux.SessionName)
	}
	// GracefulExitCommand switches on harness; an empty one sends the wrong
	// shutdown command.
	if stored.Harness != "claude-code" {
		t.Errorf("Harness = %q, want %q", stored.Harness, "claude-code")
	}
}

func TestRun_RejectsIncompleteInvocations(t *testing.T) {
	tests := []struct {
		name  string
		dbEnv string
		args  []string
	}{
		{"no session id", "set", []string{"--name", "n"}},
		{"no name", "set", []string{"--session-id", "i"}},
		{"no AGM_DB_PATH", "", []string{"--session-id", "i", "--name", "n"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := ""
			if tc.dbEnv != "" {
				dbPath = filepath.Join(t.TempDir(), "agm.db")
			}
			t.Setenv("AGM_DB_PATH", dbPath)
			// Seeding into the wrong store, or under a name the reaper cannot
			// resolve, fails later and much less legibly than refusing here.
			if err := run(tc.args); err == nil {
				t.Fatal("run() succeeded, want an error")
			}
		})
	}
}
