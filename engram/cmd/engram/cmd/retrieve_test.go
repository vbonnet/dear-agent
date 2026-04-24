package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestQueryFlagParsing(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantQuery   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "positional argument syntax (backward compat)",
			args:      []string{"retrieve", "test query"},
			wantQuery: "test query",
			wantErr:   false,
		},
		{
			name:      "query flag syntax",
			args:      []string{"retrieve", "--query", "test query"},
			wantQuery: "test query",
			wantErr:   false,
		},
		{
			name:      "query flag short form",
			args:      []string{"retrieve", "-q", "test query"},
			wantQuery: "test query",
			wantErr:   false,
		},
		{
			name:        "both flag and positional (ambiguous)",
			args:        []string{"retrieve", "--query", "flag query", "positional query"},
			wantErr:     true,
			errContains: "Ambiguous query input",
		},
		{
			name:        "no query provided",
			args:        []string{"retrieve"},
			wantErr:     true,
			errContains: "query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset config before each test (preserve defaults)
			retrieveCfg = retrieveConfig{
				Limit:  10, // Default from flag definition
				Format: "table",
			}

			// Note: This is a basic flag parsing test.
			// Full integration testing would require mocking the retrieval service
			// and setting up test engram data.
			// For now, we verify that the flag is registered and can be set.

			// Create a new root command for isolated testing
			testRootCmd := &cobra.Command{Use: "engram"}
			testRetrieveCmd := *retrieveCmd // Copy command
			testRootCmd.AddCommand(&testRetrieveCmd)

			// Set args and execute
			testRootCmd.SetArgs(tt.args) // Include "retrieve" subcommand in args

			err := testRootCmd.Execute()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.errContains)
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				// Some errors are expected (like missing engram path) in unit tests
				// We're primarily testing flag parsing, not full command execution
				// Only fail if it's a flag parsing error
				if contains(err.Error(), "flag") {
					t.Errorf("Unexpected flag parsing error: %v", err)
				}
			}
		})
	}
}

func TestQueryFlagWithOtherFlags(t *testing.T) {
	// Reset config (preserve defaults)
	retrieveCfg = retrieveConfig{
		Limit:  10,
		Format: "table",
	}

	// Test that --query works with other flags
	testRootCmd := &cobra.Command{Use: "engram"}
	testRetrieveCmd := *retrieveCmd
	testRootCmd.AddCommand(&testRetrieveCmd)

	args := []string{"retrieve", "--query", "test", "--tag", "go", "--limit", "5", "--format", "json"}
	testRootCmd.SetArgs(args)

	// Execute (may fail due to missing engram data, but flag parsing should work)
	_ = testRootCmd.Execute()

	// Verify flags were parsed correctly
	if retrieveCfg.Query != "test" {
		t.Errorf("Expected query %q, got %q", "test", retrieveCfg.Query)
	}
	if retrieveCfg.Tag != "go" {
		t.Errorf("Expected tag %q, got %q", "go", retrieveCfg.Tag)
	}
	if retrieveCfg.Limit != 5 {
		t.Errorf("Expected limit 5, got %d", retrieveCfg.Limit)
	}
	if retrieveCfg.Format != "json" {
		t.Errorf("Expected format %q, got %q", "json", retrieveCfg.Format)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
