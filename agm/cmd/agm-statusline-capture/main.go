// agm-statusline-capture reads CC session JSON from stdin and persists it
// for the AGM status-line renderer. CC pipes full session data (including
// exact total_cost_usd) to the statusLine command after every assistant
// message. This binary captures that data and outputs the default prompt
// format to stdout so the terminal status line remains functional.
//
// Performance target: <50ms — runs after every CC response.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
)

// statusLineDir is where session JSON files are written.
// Matches the directory read by internal/session.ReadStatusLineFile.
// Exported as var (not const) so tests can override it.
var statusLineDir = "/tmp/agm-context"

// sessionData is the subset of CC's statusLine JSON we need for routing.
// The full JSON is persisted as-is; we only decode to extract session_id.
type sessionData struct {
	SessionID string `json:"session_id"`
}

func main() {
	if err := run(); err != nil {
		// Errors must not crash CC's status line — silently fall through
		// to the prompt output below.
		_, _ = fmt.Fprintf(os.Stderr, "agm-statusline-capture: %v\n", err)
	}

	// Always emit the default prompt so the terminal looks normal.
	printPrompt()
}

// run reads stdin, extracts session_id, and writes the full JSON to disk.
func run() error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	if len(raw) == 0 {
		return nil // nothing to do
	}

	var sd sessionData
	if err := json.Unmarshal(raw, &sd); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}
	if sd.SessionID == "" {
		return fmt.Errorf("missing session_id in JSON")
	}

	// Ensure the output directory exists.
	if err := os.MkdirAll(statusLineDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", statusLineDir, err)
	}

	// Atomic-ish write: write to temp, then rename.
	dst := filepath.Join(statusLineDir, sd.SessionID+".json")
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		// Rename failed — clean up temp and report.
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

// printPrompt emits the default bash prompt with ANSI color codes.
func printPrompt() {
	hostname, _ := os.Hostname()
	username := "user"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	cwd, _ := os.Getwd()

	fmt.Printf("\033[01;32m%s@%s\033[00m:\033[01;34m%s\033[00m",
		username, hostname, cwd)
}
