package main

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

// getStorage returns a Dolt storage adapter for the current workspace
// This function handles:
// - Test sandbox isolation (AGM_DB_PATH skips Dolt, returns nil adapter)
// - Dolt configuration from environment
// - Connection to Dolt server
// - Migration application
func getStorage() (*dolt.Adapter, error) {
	// Test sandbox isolation: when AGM_DB_PATH is set, skip Dolt entirely.
	// The test sandbox uses isolated paths and does not need a Dolt server.
	// TODO(test-sandbox): Implement SQLite adapter for test mode so test
	// sessions can be persisted and queried within the sandbox.
	if dbPath := os.Getenv("AGM_DB_PATH"); dbPath != "" {
		return nil, fmt.Errorf("test sandbox mode: Dolt skipped (AGM_DB_PATH=%s). SQLite adapter not yet implemented", dbPath)
	}

	// Get Dolt configuration from environment
	config, err := dolt.DefaultConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Dolt config: %w\n"+
			"Hint: Ensure WORKSPACE environment variable is set (e.g., export WORKSPACE=oss)", err)
	}

	// Create Dolt adapter
	adapter, err := dolt.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Dolt: %w\n"+
			"Hint: Ensure Dolt server is running on port %s\n"+
			"Start with: dolt sql-server --data-dir ~/src/ws/%s/.dolt/dolt-db -H 127.0.0.1 -P %s",
			err, config.Port, config.Workspace, config.Port)
	}

	// Apply migrations to ensure schema is up to date
	if err := adapter.ApplyMigrations(); err != nil {
		adapter.Close()
		return nil, fmt.Errorf("failed to apply Dolt migrations: %w", err)
	}

	return adapter, nil
}


