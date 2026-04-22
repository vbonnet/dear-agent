package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var cleanupSettingsDryRun bool

var adminCleanupSettingsCmd = &cobra.Command{
	Use:   "cleanup-settings",
	Short: "Remove stale additionalDirectories from Claude settings",
	Long: `Remove additionalDirectories entries in ~/.claude/settings.json that point
to non-existent paths. These accumulate from archived sandbox sessions.

Examples:
  agm admin cleanup-settings            # Clean up stale paths
  agm admin cleanup-settings --dry-run  # Preview what would be removed`,
	RunE: runAdminCleanupSettings,
}

func runAdminCleanupSettings(cmd *cobra.Command, args []string) error {
	// Find configure-claude-settings binary
	binPath, err := findConfigureBinary()
	if err != nil {
		return err
	}

	cmdArgs := []string{"cleanup-dirs"}
	if cleanupSettingsDryRun {
		cmdArgs = append(cmdArgs, "--dry-run")
	}

	proc := exec.Command(binPath, cmdArgs...)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	return proc.Run()
}

// findConfigureBinary locates the configure-claude-settings binary.
func findConfigureBinary() (string, error) {
	// Check PATH first
	if p, err := exec.LookPath("configure-claude-settings"); err == nil {
		return p, nil
	}

	// Check alongside agm binary
	agmPath, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(agmPath), "configure-claude-settings")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("configure-claude-settings binary not found in PATH or alongside agm")
}

func init() {
	adminCleanupSettingsCmd.Flags().BoolVar(&cleanupSettingsDryRun, "dry-run", false,
		"Preview what would be removed without writing")
	adminCmd.AddCommand(adminCleanupSettingsCmd)
}
