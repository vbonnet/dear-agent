package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
)

var (
	memoryProvider string
	memoryConfig   string
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage agent memory storage",
	Long: `Memory commands for storing, retrieving, updating, and deleting agent memories.

The memory system provides pluggable storage backends for AI agent memory management,
supporting four tiers: Working Context, Session History, Long-term Memory, and Artifacts.

COMMANDS
  store    - Store a new memory entry
  retrieve - Retrieve memories matching a query
  update   - Update an existing memory entry
  delete   - Delete a memory entry

EXAMPLES
  # Store a memory
  $ engram memory store --namespace user,alice --type episodic \
      --content "Completed D1 validation phase"

  # Retrieve memories
  $ engram memory retrieve --namespace user,alice --type episodic --limit 10

  # Update a memory
  $ engram memory update --namespace user,alice --memory-id mem-123 \
      --append-content " - Review complete"

  # Delete a memory
  $ engram memory delete --namespace user,alice --memory-id mem-123

CONFIGURATION
  Provider: --provider flag > ENGRAM_MEMORY_PROVIDER env > default (simple)
  Config:   --config flag > ENGRAM_MEMORY_CONFIG env > ~/.engram/memory.yaml

  NOTE (v0.1.0): The --config flag specifies the storage directory path, not a
  config file. Full YAML config file support will be added in v0.2.0.`,
}

func init() {
	rootCmd.AddCommand(memoryCmd)

	// Add persistent flags (available to all subcommands)
	memoryCmd.PersistentFlags().StringVar(&memoryProvider, "provider", "", "Memory provider type (default: simple)")
	memoryCmd.PersistentFlags().StringVar(&memoryConfig, "config", "", "Provider config file (default: ~/.engram/memory.yaml)")
}

// getMemoryProvider returns the memory provider with the following priority:
// 1. --provider flag
// 2. ENGRAM_MEMORY_PROVIDER environment variable
// 3. Default: "simple"
func getMemoryProvider() string {
	if memoryProvider != "" {
		return memoryProvider
	}

	if envProvider := os.Getenv("ENGRAM_MEMORY_PROVIDER"); envProvider != "" {
		return envProvider
	}

	return "simple"
}

// getMemoryConfig returns the memory config path with the following priority:
// 1. --config flag
// 2. ENGRAM_MEMORY_CONFIG environment variable
// 3. Default: ~/.engram/memory.yaml
//
// Enhanced with security validation to prevent path traversal attacks.
func getMemoryConfig() (string, error) {
	configPath := memoryConfig
	if configPath == "" {
		if envConfig := os.Getenv("ENGRAM_MEMORY_CONFIG"); envConfig != "" {
			configPath = envConfig
		} else {
			configPath = "$HOME/.engram/memory.yaml"
		}
	}

	// Validate config path is safe (prevent environment variable injection)
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	allowedPaths := []string{
		filepath.Join(home, ".engram"),
		"/tmp/engram-memory-test", // For testing
	}

	// Expand environment variables and validate
	if err := cli.ValidateSafeEnvExpansion("config", configPath, allowedPaths); err != nil {
		return "", err
	}

	return os.ExpandEnv(configPath), nil
}
