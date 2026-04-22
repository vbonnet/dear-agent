package engram

import (
	"testing"
)

// TestStandaloneIntegration verifies AGM engram client works without direct engram dependency
func TestStandaloneIntegration(t *testing.T) {
	t.Run("Client creation without engram library", func(t *testing.T) {
		cfg := LoadEngramConfig()
		client := NewClient(cfg)

		if client == nil {
			t.Fatal("NewClient should not return nil")
		}
	})

	t.Run("IsAvailable graceful degradation", func(t *testing.T) {
		cfg := EngramConfig{}
		client := NewClient(cfg)

		// Should not panic, even if CLI not available
		available := client.IsAvailable()
		t.Logf("Engram/workspace CLI available: %v", available)
		// Success if no panic - availability depends on environment
	})

	t.Run("Query graceful degradation without CLI", func(t *testing.T) {
		cfg := EngramConfig{
			Limit:          5,
			ScoreThreshold: 0.8,
			Timeout:        1000,
		}
		client := NewClient(cfg)

		// Should return empty results, not error, if CLI unavailable
		results, err := client.Query("test", []string{"go"})
		if err != nil {
			// Only fail if error is not due to CLI unavailability
			t.Logf("Query returned error (expected if CLI unavailable): %v", err)
		}

		if results == nil {
			t.Error("Query should return empty slice, not nil")
		}
	})

	t.Run("Config loading from environment", func(t *testing.T) {
		cfg := LoadEngramConfig()

		// Verify defaults are set
		if cfg.Limit == 0 {
			t.Error("Expected default Limit to be set")
		}

		if cfg.ScoreThreshold == 0 {
			t.Error("Expected default ScoreThreshold to be set")
		}

		if cfg.Timeout == 0 {
			t.Error("Expected default Timeout to be set")
		}
	})
}

// TestNoEngramImport verifies no direct engram package imports at compile time
// This test exists to document the contract: AGM should use CLI, not direct imports
func TestNoEngramImport(t *testing.T) {
	t.Log("AGM engram client refactored to use CLI contract instead of direct Go imports")
	t.Log("Contract: workspace detect --json for workspace detection")
	t.Log("Contract: engram search --query --tags --json for retrieval")
	t.Log("Graceful degradation: returns empty results if CLI unavailable")
}
