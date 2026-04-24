// Package readiness provides readiness functionality.
package readiness

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// ReadyFilePayload represents the JSON structure of ready-files.
type ReadyFilePayload struct {
	Status          string   `json:"status"`           // "ready" or "crashed"
	ReadyAt         string   `json:"ready_at"`         // ISO 8601 timestamp
	SessionName     string   `json:"session_name"`     // Tmux session name
	ManifestPath    string   `json:"manifest_path"`    // Path to manifest.yaml
	AGMVersion      string   `json:"agm_version"`      // AGM version string
	SignalsDetected []string `json:"signals_detected"` // List of signals
	// Phase 2 fields (crash detection)
	CrashedAt string `json:"crashed_at,omitempty"` // ISO 8601 timestamp
	Error     string `json:"error,omitempty"`      // Crash error message
	ExitCode  int    `json:"exit_code,omitempty"`  // Process exit code
}

// getStateDir returns the AGM state directory.
// Uses AGM_STATE_DIR environment variable if set (for test isolation),
// otherwise defaults to ~/.agm (production default).
func getStateDir() (string, error) {
	stateDir := os.Getenv("AGM_STATE_DIR")
	if stateDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		stateDir = filepath.Join(homeDir, ".agm")
	}
	return stateDir, nil
}

// WaitForReady waits for an agent to create the ready-file signal.
// It watches $AGM_STATE_DIR/ directory for ready-{sessionName} file creation using fsnotify.
//
// Returns nil when ready-file detected and parsed successfully.
// Returns error on timeout or failure.
func WaitForReady(sessionName string, timeout time.Duration) error {
	agmDir, err := getStateDir()
	if err != nil {
		return err
	}

	readyFile := filepath.Join(agmDir, "ready-"+sessionName)

	// Create ~/.agm/ directory with user-only permissions (0700 for security)
	if err := os.MkdirAll(agmDir, 0700); err != nil {
		return fmt.Errorf("failed to create ~/.agm directory: %w", err)
	}

	// Cleanup stale ready-files before watching
	if err := cleanupStaleReadyFiles(agmDir); err != nil {
		debug.Log("Warning: Failed to cleanup stale files: %v", err)
		// Non-fatal, continue anyway
	}

	// Check if ready-file already exists (race condition mitigation)
	if fileExists(readyFile) {
		debug.Log("Ready-file already exists: %s", readyFile)

		// Parse JSON to verify status (crash detection)
		status, err := parseReadyFile(readyFile)
		if err != nil {
			debug.Log("Failed to parse pre-existing ready-file: %v", err)
			os.Remove(readyFile) // Cleanup malformed file
			return nil
		}

		if status == "crashed" {
			os.Remove(readyFile) // Cleanup
			return fmt.Errorf("Claude crashed during startup")
		}

		os.Remove(readyFile) // Cleanup
		return nil
	}

	// Create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}
	defer watcher.Close()

	// Watch ~/.agm/ directory (1 FD, not individual files)
	if err := watcher.Add(agmDir); err != nil {
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	debug.Log("Watching for ready-file: %s (timeout: %v)", readyFile, timeout)

	// Timeout and periodic fallback check
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("watcher closed unexpectedly")
			}

			// Ignore Chmod events (best practice from S5 research)
			if event.Has(fsnotify.Chmod) {
				continue
			}

			if event.Has(fsnotify.Create) && event.Name == readyFile {
				debug.Log("Ready-file detected: %s", event.Name)

				// Parse JSON to verify status
				status, err := parseReadyFile(readyFile)
				if err != nil {
					debug.Log("Failed to parse ready-file: %v", err)
					continue // Malformed JSON, keep waiting
				}

				if status == "ready" {
					os.Remove(readyFile) // Cleanup
					return nil
				}

				if status == "crashed" {
					os.Remove(readyFile) // Cleanup
					return fmt.Errorf("Claude crashed during startup")
				}

				debug.Log("Unexpected status in ready-file: %s", status)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher error channel closed")
			}
			debug.Log("Watcher error: %v", err)
			// Continue waiting despite errors

		case <-ticker.C:
			// Periodic fallback check for race conditions
			if fileExists(readyFile) {
				debug.Log("Ready-file detected via fallback check")

				// Parse JSON to verify status (crash detection)
				status, err := parseReadyFile(readyFile)
				if err != nil {
					debug.Log("Failed to parse ready-file: %v", err)
					continue // Malformed JSON, keep waiting
				}

				if status == "ready" {
					os.Remove(readyFile)
					return nil
				}

				if status == "crashed" {
					os.Remove(readyFile)
					return fmt.Errorf("Claude crashed during startup")
				}

				debug.Log("Unexpected status in ready-file: %s", status)
			}
		}
	}

	return fmt.Errorf("timeout waiting for ready-file")
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// parseReadyFile reads and parses the JSON ready-file.
// Returns status field ("ready" or "crashed") or error if parsing fails.
func parseReadyFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read ready-file: %w", err)
	}

	var payload ReadyFilePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("invalid JSON in ready-file: %w", err)
	}

	if payload.Status == "" {
		return "", fmt.Errorf("missing status field in ready-file")
	}

	return payload.Status, nil
}

// cleanupStaleReadyFiles removes ready-files older than 10 minutes.
// Prevents false positives from stale files (e.g., AGM crashed before cleanup).
func cleanupStaleReadyFiles(agmDir string) error {
	cutoff := time.Now().Add(-10 * time.Minute)

	files, err := filepath.Glob(filepath.Join(agmDir, "ready-*"))
	if err != nil {
		return fmt.Errorf("failed to list ready-files: %w", err)
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			debug.Log("Failed to stat %s: %v", file, err)
			continue // Non-fatal, continue with other files
		}

		if info.ModTime().Before(cutoff) {
			age := time.Since(info.ModTime())
			debug.Log("Removing stale ready-file: %s (age: %v)", file, age)
			if err := os.Remove(file); err != nil {
				debug.Log("Failed to remove %s: %v", file, err)
				// Non-fatal, continue
			}
		}
	}

	return nil
}

// CreateReadyFile creates a ready-file signal for the specified session.
// Called by agm associate to signal that Claude has been successfully associated.
func CreateReadyFile(sessionName, manifestPath string) error {
	agmDir, err := getStateDir()
	if err != nil {
		return err
	}

	readyFile := filepath.Join(agmDir, "ready-"+sessionName)

	// Create ~/.agm/ directory with user-only permissions
	if err := os.MkdirAll(agmDir, 0700); err != nil {
		return fmt.Errorf("failed to create ~/.agm directory: %w", err)
	}

	// Get AGM version
	agmVersion := "unknown"
	if cmd := exec.Command("agm", "--version"); cmd != nil {
		if output, err := cmd.Output(); err == nil {
			// Extract first line (version info)
			lines := strings.Split(string(output), "\n")
			if len(lines) > 0 {
				agmVersion = strings.TrimSpace(lines[0])
			}
		}
	}

	// Create payload
	payload := ReadyFilePayload{
		Status:          "ready",
		ReadyAt:         time.Now().Format(time.RFC3339),
		SessionName:     sessionName,
		ManifestPath:    manifestPath,
		AGMVersion:      agmVersion,
		SignalsDetected: []string{"association_complete"},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ready-file JSON: %w", err)
	}

	// Write ready-file with user-only permissions
	if err := os.WriteFile(readyFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write ready-file: %w", err)
	}

	debug.Log("Created ready-file: %s", readyFile)
	return nil
}
