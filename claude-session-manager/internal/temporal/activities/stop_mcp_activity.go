package activities

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal/workflows"
)

// StopMCPActivity gracefully stops an MCP HTTP server process
// This activity handles:
// 1. Graceful process termination (SIGTERM)
// 2. Force kill if needed (SIGKILL)
// 3. Cleanup of PID file
func StopMCPActivity(ctx context.Context, input workflows.StopMCPInput) (*workflows.StopMCPResult, error) {
	// Validate input
	if input.Name == "" {
		return nil, fmt.Errorf("MCP service name cannot be empty")
	}
	if input.PID <= 0 {
		return nil, fmt.Errorf("invalid PID: %d", input.PID)
	}

	// Set defaults
	if input.GracePeriod == 0 {
		input.GracePeriod = 10 * time.Second // Default 10s grace period
	}

	output := &workflows.StopMCPResult{
		ProcessKilled: false,
		GracefulExit:  false,
		StoppedAt:     time.Now(),
	}

	// Step 1: Try graceful shutdown (SIGTERM)
	process, err := os.FindProcess(input.PID)
	if err != nil {
		// Process doesn't exist (already terminated)
		output.ProcessKilled = true
		output.GracefulExit = true
	} else {
		// Send SIGTERM for graceful shutdown
		if err := process.Signal(syscall.SIGTERM); err != nil {
			// Process may have already exited
			if err.Error() != "os: process already finished" {
				return nil, fmt.Errorf("failed to send SIGTERM: %w", err)
			}
			output.ProcessKilled = true
			output.GracefulExit = true
		} else {
			// Wait for graceful shutdown
			gracefulDone := make(chan bool, 1)
			go func() {
				// Wait for process to exit
				_, _ = process.Wait()
				gracefulDone <- true
			}()

			select {
			case <-gracefulDone:
				// Process exited gracefully
				output.ProcessKilled = true
				output.GracefulExit = true
			case <-time.After(input.GracePeriod):
				// Grace period expired
				if input.ForceKill {
					// Force kill with SIGKILL
					if err := process.Signal(syscall.SIGKILL); err != nil {
						return nil, fmt.Errorf("failed to send SIGKILL: %w", err)
					}
					// Give it a moment to die
					time.Sleep(500 * time.Millisecond)
					output.ProcessKilled = true
					output.GracefulExit = false
				} else {
					return nil, fmt.Errorf("process did not exit gracefully within %v", input.GracePeriod)
				}
			case <-ctx.Done():
				// Context cancelled
				return nil, ctx.Err()
			}
		}
	}

	// Step 2: Cleanup PID file
	mcpDataDir, err := GetMCPDataDir(input.Name)
	if err == nil {
		pidFile := filepath.Join(mcpDataDir, "mcp-server.pid")
		if err := os.Remove(pidFile); err != nil {
			// Non-fatal error, log but continue
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove PID file: %v\n", err)
			}
		}
	}

	output.StoppedAt = time.Now()
	return output, nil
}

// CleanupMCPDataActivity removes all data for an MCP service (for testing or complete cleanup)
func CleanupMCPDataActivity(ctx context.Context, serviceName string) error {
	if serviceName == "" {
		return fmt.Errorf("service name cannot be empty")
	}

	mcpDir, err := GetMCPDataDir(serviceName)
	if err != nil {
		return err
	}

	// Remove entire MCP data directory
	if err := os.RemoveAll(mcpDir); err != nil {
		if os.IsNotExist(err) {
			// Already removed
			return nil
		}
		return fmt.Errorf("failed to remove MCP data directory: %w", err)
	}

	return nil
}

// CheckMCPProcessActivity checks if an MCP process is running
func CheckMCPProcessActivity(ctx context.Context, pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid PID: %d", pid)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		// Process doesn't exist
		return false, nil
	}

	// Try sending signal 0 (doesn't actually send a signal, just checks if process exists)
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		// Process doesn't exist
		return false, nil
	}

	return true, nil
}
