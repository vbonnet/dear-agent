package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var (
	tmuxInstallInterval int
	tmuxInstallFormat   string
	tmuxInstallSocket   string
)

var installTmuxStatusCmd = &cobra.Command{
	Use:   "install-tmux-status",
	Short: "Install AGM status line in tmux configuration",
	Long: `Install AGM status line in your tmux configuration.

This command:
1. Backs up your existing ~/.tmux.conf to ~/.tmux.conf.bak
2. Appends AGM status line configuration
3. Applies status-right and status-interval directly on the AGM tmux socket
4. Reloads tmux configuration on the AGM socket

Examples:
  agm session install-tmux-status
  agm session install-tmux-status --interval 5
  agm session install-tmux-status --socket ~/.agm/agm.sock`,
	RunE: runInstallTmuxStatus,
}

func init() {
	installTmuxStatusCmd.Flags().IntVar(
		&tmuxInstallInterval,
		"interval",
		10,
		"Status line refresh interval in seconds",
	)
	installTmuxStatusCmd.Flags().StringVar(
		&tmuxInstallFormat,
		"format",
		"default",
		"Status line format template (default, minimal, compact, multi-agent, full)",
	)
	installTmuxStatusCmd.Flags().StringVar(
		&tmuxInstallSocket,
		"socket",
		"",
		"AGM tmux socket path (default: $AGM_TMUX_SOCKET or ~/.agm/agm.sock)",
	)
	sessionCmd.AddCommand(installTmuxStatusCmd)
}

func resolveSocketPath() string {
	if tmuxInstallSocket != "" {
		return tmuxInstallSocket
	}
	return tmux.GetSocketPath()
}

func runInstallTmuxStatus(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	socketPath := resolveSocketPath()

	tmuxConf := filepath.Join(homeDir, ".tmux.conf")
	tmuxBackup := filepath.Join(homeDir, ".tmux.conf.bak")

	// Check if already installed
	if data, err := os.ReadFile(tmuxConf); err == nil {
		if strings.Contains(string(data), "agm session status-line") {
			fmt.Println("⚠️  AGM status line appears to be already installed")
			fmt.Println("   Check ~/.tmux.conf manually if you want to reconfigure")
			return nil
		}
	}

	// Backup existing config
	if _, err := os.Stat(tmuxConf); err == nil {
		if err := copyFile(tmuxConf, tmuxBackup); err != nil {
			return fmt.Errorf("failed to backup tmux.conf: %w", err)
		}
		fmt.Printf("✓ Backed up ~/.tmux.conf to ~/.tmux.conf.bak\n")
	}

	// Prepare status line configuration
	// Use #{session_name} so each tmux session gets its own status data
	statusLineConfig := fmt.Sprintf(`
# AGM Status Line (installed %s)
set -g status-interval %d
set -g status-right "#(agm session status-line -s '#{session_name}')"
set -g status-right-length 80
`,
		time.Now().Format("2006-01-02 15:04:05"),
		tmuxInstallInterval,
	)

	// Append to tmux.conf
	f, err := os.OpenFile(tmuxConf, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open tmux.conf: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(statusLineConfig); err != nil {
		return fmt.Errorf("failed to write to tmux.conf: %w", err)
	}

	fmt.Printf("✓ Added AGM status line to ~/.tmux.conf\n")
	fmt.Printf("  Refresh interval: %d seconds\n", tmuxInstallInterval)

	// Apply settings directly on the AGM tmux socket
	// This ensures the running AGM server picks up changes immediately,
	// even if it was started with a different config or no config at all.
	fmt.Printf("  Socket: %s\n", socketPath)

	statusRight := `#(agm session status-line -s '#{session_name}')`
	intervalStr := fmt.Sprintf("%d", tmuxInstallInterval)

	if err := exec.Command("tmux", "-S", socketPath, "set-option", "-g", "status-right", statusRight).Run(); err != nil {
		fmt.Printf("⚠️  Could not set status-right on AGM socket (%s)\n", socketPath)
		fmt.Println("   The AGM tmux server may not be running yet")
	} else {
		fmt.Printf("✓ Set status-right on AGM socket\n")
	}

	if err := exec.Command("tmux", "-S", socketPath, "set-option", "-g", "status-interval", intervalStr).Run(); err != nil {
		fmt.Printf("⚠️  Could not set status-interval on AGM socket (%s)\n", socketPath)
	} else {
		fmt.Printf("✓ Set status-interval to %s on AGM socket\n", intervalStr)
	}

	if err := exec.Command("tmux", "-S", socketPath, "set-option", "-g", "status-right-length", "80").Run(); err != nil {
		// Non-critical, don't warn
	}

	// Reload tmux configuration on the AGM socket
	if err := exec.Command("tmux", "-S", socketPath, "source-file", tmuxConf).Run(); err != nil {
		fmt.Printf("⚠️  Could not reload tmux configuration on AGM socket (%s)\n", socketPath)
		fmt.Printf("   Run: tmux -S %s source-file ~/.tmux.conf\n", socketPath)
	} else {
		fmt.Println("✓ Reloaded tmux configuration on AGM socket")
	}

	// Also reload default tmux server if running (for non-AGM sessions)
	if err := exec.Command("tmux", "source-file", tmuxConf).Run(); err != nil {
		// Not an error -- default server may not be running
		fmt.Println("  (default tmux server not running or reload skipped)")
	} else {
		fmt.Println("✓ Reloaded default tmux configuration")
	}

	fmt.Println("\n✅ Installation complete!")
	fmt.Println("\nYour tmux status line should now display AGM session information.")
	fmt.Println("To customize the format, edit ~/.config/csm/config.yaml")

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
