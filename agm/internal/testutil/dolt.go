// Package testutil provides testutil functionality.
package testutil

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

// GetTestDoltAdapter returns a Dolt adapter for tests using the OSS workspace
// Requires DOLT server running on port 3307
func GetTestDoltAdapter(t testing.TB) *dolt.Adapter {
	t.Helper()

	config := &dolt.Config{
		Host:      "localhost",
		Port:      "3307",
		User:      "root",
		Password:  "",
		Database:  "oss",
		Workspace: "oss",
	}

	adapter, err := dolt.New(config)
	if err != nil {
		t.Skipf("Dolt adapter unavailable (is Dolt server running?): %v", err)
	}

	// Ensure migrations applied
	if err := adapter.ApplyMigrations(); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	return adapter
}

// CleanupTestSession removes a test session from Dolt
func CleanupTestSession(t testing.TB, adapter *dolt.Adapter, sessionID string) {
	t.Helper()

	if err := adapter.DeleteSession(sessionID); err != nil {
		t.Logf("Failed to cleanup test session %s: %v", sessionID, err)
	}
}
