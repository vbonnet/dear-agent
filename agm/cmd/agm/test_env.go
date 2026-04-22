package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
)

var testEnvCmd = &cobra.Command{
	Use:   "test-env",
	Short: "Manage test environments",
	Long: `Manage isolated test environments for AGM.

Test environments provide full isolation: separate HOME, tmux socket,
sessions directory, and database. Useful for E2E tests, CI/CD, and
development without polluting production state.

Examples:
  agm test-env create                         # Create with random name
  agm test-env create --name=my-env           # Create with specific name
  agm test-env create --auth-mode=none        # Create without auth forwarding
  agm test-env destroy my-env                 # Remove test environment
  agm test-env list                           # List active test environments`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var testEnvCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create isolated test environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		authModeStr, _ := cmd.Flags().GetString("auth-mode")

		var tc *testcontext.TestContext
		if name == "" {
			tc = testcontext.New()
		} else {
			tc = testcontext.NewNamed(name)
		}

		if err := tc.EnsureDirs(); err != nil {
			return fmt.Errorf("failed to create test environment dirs: %w", err)
		}

		hostHome, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get host home directory: %w", err)
		}

		authMode := testcontext.AuthMode(authModeStr)
		if err := tc.ForwardAuth(hostHome, authMode); err != nil {
			return fmt.Errorf("failed to forward auth: %w", err)
		}

		fmt.Printf("AGM_TEST_ENV=%s\n", tc.RunID)
		fmt.Printf("Base: %s\n", tc.BaseDir)
		fmt.Printf("Home: %s\n", tc.HomeDir)
		fmt.Printf("Auth: %s\n", authModeStr)
		return nil
	},
}

var testEnvDestroyCmd = &cobra.Command{
	Use:   "destroy [name]",
	Short: "Destroy a test environment",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		tc := testcontext.LoadNamed(name)

		if err := tc.Cleanup(); err != nil {
			return fmt.Errorf("failed to destroy test environment %q: %w", name, err)
		}

		fmt.Printf("Destroyed test environment: %s\n", name)
		return nil
	},
}

var testEnvListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active test environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		tmpDir := os.TempDir()
		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			return fmt.Errorf("failed to read temp directory: %w", err)
		}

		found := false
		fmt.Printf("%-20s  %s\n", "NAME", "PATH")
		fmt.Printf("%-20s  %s\n", "----", "----")
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !strings.HasPrefix(entry.Name(), "agm-test-") {
				continue
			}
			name := strings.TrimPrefix(entry.Name(), "agm-test-")
			fullPath := filepath.Join(tmpDir, entry.Name())
			fmt.Printf("%-20s  %s\n", name, fullPath)
			found = true
		}

		if !found {
			fmt.Println("No active test environments found.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(testEnvCmd)

	testEnvCmd.AddCommand(testEnvCreateCmd)
	testEnvCreateCmd.Flags().String("name", "", "Name for the test environment (default: random)")
	testEnvCreateCmd.Flags().String("auth-mode", "inherit", "Auth forwarding mode: inherit, env, none")

	testEnvCmd.AddCommand(testEnvDestroyCmd)
	testEnvCmd.AddCommand(testEnvListCmd)
}
