package dolt

import (
	"testing"
)

// GetTestAdapter creates a Dolt adapter connected to the test database
// Returns nil if Dolt server is not available (test should skip)
func GetTestAdapter(t *testing.T) *Adapter {
	t.Helper()

	config := &Config{
		Workspace: "test",
		Port:      "3307",
		Host:      "127.0.0.1",
		Database:  "agm_test", // Use test database
		User:      "root",
		Password:  "",
	}

	adapter, err := New(config)
	if err != nil {
		t.Logf("Dolt server not available: %v", err)
		return nil
	}

	// Apply migrations to test database
	if err := adapter.ApplyMigrations(); err != nil {
		adapter.Close()
		t.Fatalf("Failed to apply migrations to test database: %v", err)
	}

	// Clean test database before each test
	if err := CleanTestDatabase(adapter); err != nil {
		adapter.Close()
		t.Fatalf("Failed to clean test database: %v", err)
	}

	return adapter
}

// CleanTestDatabase removes all sessions from test database
// Exported for use in test suites that need manual cleanup
func CleanTestDatabase(adapter *Adapter) error {
	_, err := adapter.conn.Exec("DELETE FROM agm_sessions")
	return err
}
