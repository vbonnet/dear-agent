package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/corpus"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Engram workspace",
	Long: `Initialize Engram workspace by creating required directories and symlinks.

This command creates workspace-aware directories:
  - {workspace}/.engram/           Workspace directory
  - {workspace}/.engram/user/      User directory for custom engrams
  - {workspace}/.engram/core       Symlink to engram repository
  - {workspace}/.engram/logs/      Logs directory
  - {workspace}/.engram/cache/     Cache directory
  - {workspace}/.engram/user/config.yaml  User configuration (if missing)

Where {workspace} is determined by (in priority order):
  1. --workspace flag
  2. $WORKSPACE environment variable
  3. Auto-detect from current directory (~/src/ws/{name}/*)
  4. Fallback to ~/.engram (backward compatible)

The command is idempotent and safe to run multiple times.

EXAMPLES
  # Initialize in detected workspace
  $ export WORKSPACE=oss
  $ engram init
  # Creates: ~/src/ws/oss/.engram/

  # Initialize in legacy location
  $ engram init
  # Creates: ~/.engram/

  # Verify workspace
  $ engram doctor
`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

//nolint:gocyclo // reason: linear CLI bootstrap with many independent init steps
func runInit(cmd *cobra.Command, args []string) error {
	// Use workspace-aware base path instead of hardcoded ~/.engram
	// This will detect workspace from $WORKSPACE env var or PWD,
	// and fall back to ~/.engram for backward compatibility
	workspaceDir, err := getEngramBasePath()
	if err != nil {
		return fmt.Errorf("failed to determine engram base path: %w", err)
	}

	userDir := filepath.Join(workspaceDir, "user")
	logsDir := filepath.Join(workspaceDir, "logs")
	cacheDir := filepath.Join(workspaceDir, "cache")
	coreSymlink := filepath.Join(workspaceDir, "core")
	userConfig := filepath.Join(userDir, "config.yaml")

	// Step 1: Create workspace directory
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}
	cli.PrintSuccess(fmt.Sprintf("Created workspace at %s", workspaceDir))

	// Step 2: Create user directory
	if err := os.MkdirAll(userDir, 0o700); err != nil {
		return fmt.Errorf("failed to create user directory: %w", err)
	}
	cli.PrintSuccess(fmt.Sprintf("Created user directory at %s", userDir))

	// Step 3: Create logs directory
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Step 4: Create cache directory
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Step 5: Create core symlink (if not exists)
	if _, err := os.Lstat(coreSymlink); err != nil {
		if os.IsNotExist(err) {
			// Try to detect engram repository location
			repoPath, err := detectEngramRepo()
			if err != nil {
				cli.PrintWarning(fmt.Sprintf("Could not detect engram repository: %v", err))
				cli.PrintWarning("Skipping core symlink creation. You can create it manually:")
				fmt.Printf("  ln -s /path/to/engram %s\n", coreSymlink)
			} else {
				// Create symlink
				if err := os.Symlink(repoPath, coreSymlink); err != nil {
					cli.PrintWarning(fmt.Sprintf("Failed to create core symlink: %v", err))
					cli.PrintWarning("You can create it manually:")
					fmt.Printf("  ln -s %s %s\n", repoPath, coreSymlink)
				} else {
					cli.PrintSuccess(fmt.Sprintf("Created core symlink: %s -> %s", coreSymlink, repoPath))
				}
			}
		} else {
			// Symlink already exists, verify it's valid
			target, err := os.Readlink(coreSymlink)
			if err != nil {
				cli.PrintWarning(fmt.Sprintf("Core path exists but is not a symlink: %s", coreSymlink))
			} else {
				// Check if target exists
				if _, err := os.Stat(target); err != nil {
					cli.PrintWarning(fmt.Sprintf("Core symlink points to non-existent path: %s -> %s", coreSymlink, target))
				}
			}
		}
	}

	// Step 6: Create initial user config if missing
	if _, err := os.Stat(userConfig); os.IsNotExist(err) {
		defaultConfig := `# Engram User Configuration
# This file contains user-specific configuration overrides.
# See ~/.engram/core/config.yaml for default configuration options.

# Example configurations:
# ecphory:
#   max_results: 10
#   enable_api_ranking: true
#
# telemetry:
#   enabled: false
`
		if err := os.WriteFile(userConfig, []byte(defaultConfig), 0o600); err != nil {
			cli.PrintWarning(fmt.Sprintf("Failed to create user config: %v", err))
		} else {
			cli.PrintSuccess(fmt.Sprintf("Created user config at %s", userConfig))
		}
	}

	// Step 7: Register with corpus callosum (optional)
	// This allows other tools (AGM, Wayfinder, Swarm) to discover Engram's schemas
	cfg, err := loadConfigWithWorkspace()
	if err == nil && cfg.Workspace != "" {
		if err := corpus.RegisterEngramSchemas(cfg.Workspace); err != nil {
			// Non-fatal - corpus callosum integration is optional
			cli.PrintWarning("Corpus callosum registration skipped (cc not installed or failed)")
		} else {
			cli.PrintSuccess(fmt.Sprintf("Registered with corpus callosum (workspace: %s)", cfg.Workspace))
		}
	}

	// Final success message
	fmt.Println()
	cli.PrintSuccess("Engram workspace initialized successfully")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Run: engram doctor")
	fmt.Println("  2. Create your first pattern: engram create-pattern <name>")
	fmt.Println()

	return nil
}

// detectEngramRepo tries to detect the engram repository location
func detectEngramRepo() (string, error) {
	home := os.Getenv("HOME")

	// Strategy 1: Check if we're running from the repository (development build)
	executable, err := os.Executable()
	if err == nil {
		// Resolve symlinks
		realPath, err := filepath.EvalSymlinks(executable)
		if err == nil {
			// Navigate up from the binary to find repo root
			// Binary is typically in: <repo>/core/engram or <repo>/core/cmd/engram/engram
			dir := filepath.Dir(realPath)

			// Try parent directories up to 5 levels
			for i := 0; i < 5; i++ {
				if isEngramRepo(dir) {
					return dir, nil
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					break // Reached root
				}
				dir = parent
			}
		}
	}

	// Strategy 2: Check common installation locations
	commonPaths := []string{
		filepath.Join(home, "src/engram"),                                // Common dev location
		filepath.Join(home, "engram"),                                    // Simple clone
		filepath.Join(home, "go/src/github.com/vbonnet/dear-agent/engram"), // GOPATH location
		filepath.Join(home, ".local/share/engram"),                       // User install
		"/usr/local/share/engram",                                        // System install
		"/opt/engram",                                                    // Alternative system install
	}

	for _, path := range commonPaths {
		if isEngramRepo(path) {
			return path, nil
		}
	}

	return "", fmt.Errorf("engram repository not found in common locations")
}

// isEngramRepo checks if a path looks like the engram repository
func isEngramRepo(path string) bool {
	// Check for characteristic files/directories
	markers := []string{
		filepath.Join(path, "README.md"),
		filepath.Join(path, "core"),
		filepath.Join(path, "engrams"),
		filepath.Join(path, "plugins"),
	}

	for _, marker := range markers {
		if _, err := os.Stat(marker); err != nil {
			return false
		}
	}

	return true
}
