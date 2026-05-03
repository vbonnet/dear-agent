// Package interrupt provides a flag-based interrupt mechanism for AGM sessions.
//
// Instead of injecting keystrokes via tmux (which is destructive and can corrupt
// input buffers), this package uses flag files that hooks check before each tool call.
// The interrupt flow is:
//  1. Orchestrator/user writes flag file via agm interrupt command
//  2. Pre-tool hook reads flag file before each tool call
//  3. Hook blocks (stop/kill) or injects guidance (steer) based on flag type
//  4. Flag is consumed (deleted) after being read
package interrupt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Type represents the kind of interrupt.
type Type string

const (
	// TypeStop requests the session to stop after current tool completes.
	TypeStop Type = "stop"
	// TypeSteer injects guidance without stopping — session continues with new direction.
	TypeSteer Type = "steer"
	// TypeKill requests immediate termination — hook blocks all subsequent tool calls.
	TypeKill Type = "kill"
)

// Flag represents an interrupt flag written to disk.
type Flag struct {
	Type     Type              `json:"type"`
	Reason   string            `json:"reason"`
	IssuedBy string            `json:"issued_by"`
	IssuedAt time.Time         `json:"issued_at"`
	Context  map[string]string `json:"context,omitempty"`
}

// DefaultDir returns the default directory for interrupt flag files.
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", ".agm", "interrupts")
	}
	return filepath.Join(home, ".agm", "interrupts")
}

// FlagPath returns the path to the interrupt flag file for a session.
func FlagPath(dir, sessionName string) string {
	return filepath.Join(dir, sessionName+".json")
}

// ValidateType checks if a string is a valid interrupt type.
func ValidateType(t string) (Type, error) {
	switch Type(t) {
	case TypeStop, TypeSteer, TypeKill:
		return Type(t), nil
	default:
		return "", fmt.Errorf("invalid interrupt type %q: must be stop, steer, or kill", t)
	}
}

// Write atomically writes an interrupt flag file using temp+rename.
func Write(dir, sessionName string, flag *Flag) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create interrupts directory: %w", err)
	}

	data, err := json.MarshalIndent(flag, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal interrupt flag: %w", err)
	}

	target := FlagPath(dir, sessionName)

	// Atomic write: write to temp file, then rename
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("failed to write temp interrupt file: %w", err)
	}

	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp) // cleanup on failure
		return fmt.Errorf("failed to rename interrupt file: %w", err)
	}

	return nil
}

// Read reads and returns the interrupt flag for a session, if one exists.
// Returns nil, nil if no flag file exists.
func Read(dir, sessionName string) (*Flag, error) {
	path := FlagPath(dir, sessionName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read interrupt flag: %w", err)
	}

	var flag Flag
	if err := json.Unmarshal(data, &flag); err != nil {
		return nil, fmt.Errorf("failed to parse interrupt flag: %w", err)
	}

	return &flag, nil
}

// Consume reads and deletes the interrupt flag for a session.
// Returns nil, nil if no flag file exists.
func Consume(dir, sessionName string) (*Flag, error) {
	flag, err := Read(dir, sessionName)
	if err != nil {
		return nil, err
	}
	if flag == nil {
		return nil, nil
	}

	// Delete the flag file after reading
	path := FlagPath(dir, sessionName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return flag, fmt.Errorf("failed to remove interrupt flag: %w", err)
	}

	return flag, nil
}

// Clear removes the interrupt flag for a session.
func Clear(dir, sessionName string) error {
	path := FlagPath(dir, sessionName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clear interrupt flag: %w", err)
	}
	return nil
}

// ClearAll removes all interrupt flags in the directory.
func ClearAll(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read interrupts directory: %w", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" && !entry.IsDir() {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

// ClearStale removes interrupt flags older than maxAge.
func ClearStale(dir string, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read interrupts directory: %w", err)
	}

	cleared := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" || entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if time.Since(info.ModTime()) > maxAge {
			if err := os.Remove(path); err == nil {
				cleared++
			}
		}
	}
	return cleared, nil
}
