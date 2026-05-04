// Package uuid provides uuid functionality.
package uuid

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// Package uuid provides UUID discovery functions for AGM sessions.
// This package implements a 3-level fallback chain:
// - Level 1: AGM manifest lookup (verified against /rename if available)
// - Level 2: History search by rename (/rename command - strong signal)
// - Level 3: JSONL fallback (most recent .jsonl file in projects dir)
//
// Note: Timestamp-based search has been removed as it's unreliable and can
// return wrong UUIDs. We prefer to fail with "don't know" rather than
// return incorrect associations.

// SearchHistoryByRename searches the Claude history.jsonl for sessions renamed
// with the /rename command. Returns the UUID of the most recent session with
// the given name.
//
// This function uses the NEW history format (ConversationEntry) which is what
// modern Claude Code writes. The old format (Entry with name field) is deprecated.
//
// Parameters:
//   - sessionName: The name to search for (from /rename command)
//
// Returns:
//   - UUID of the most recent matching session
//   - Error if sessionName is empty or no match found
func SearchHistoryByRename(sessionName string) (string, error) {
	if sessionName == "" {
		return "", fmt.Errorf("sessionName cannot be empty")
	}

	parser := history.NewParser("")
	// Use ReadConversations instead of ReadAll to get new format entries
	// ReadAll() only returns old-format entries which modern Claude doesn't write
	sessions, err := parser.ReadConversations(1000) // Read last 1000 entries
	if err != nil {
		return "", fmt.Errorf("failed to read history: %w", err)
	}

	// Build a list of all rename commands with their timestamps
	// We need to find the MOST RECENT /rename for this session name
	type renameMatch struct {
		sessionID string
		timestamp int64
	}
	var matches []renameMatch

	renameCmd := "/rename " + sessionName
	for _, session := range sessions {
		for _, entry := range session.Entries {
			// Check if this entry is a rename command for our session
			// Trim whitespace to handle trailing spaces from user input
			if strings.TrimSpace(entry.Display) == renameCmd {
				matches = append(matches, renameMatch{
					sessionID: session.SessionID,
					timestamp: entry.Timestamp,
				})
			}
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no rename found for: %s", sessionName)
	}

	// Find the most recent match (highest timestamp)
	mostRecent := matches[0]
	for _, match := range matches[1:] {
		if match.timestamp > mostRecent.timestamp {
			mostRecent = match
		}
	}

	return mostRecent.sessionID, nil
}

// DefaultWindowMinutes is the default time window for timestamp searches
const DefaultWindowMinutes = 10

// SearchHistoryByTimestamp searches the Claude history.jsonl for sessions
// created within a time window around the given timestamp.
//
// This is useful when a manifest exists but lacks a UUID field - we can search
// for sessions created around the same time as the manifest's last modified time.
//
// This function uses the NEW history format (ConversationEntry) which is what
// modern Claude Code writes. The old format (Entry with time.Time timestamp) is deprecated.
//
// Parameters:
//   - timestamp: The reference timestamp to search around
//   - windowMinutes: The search window in minutes (±windowMinutes). Use 0 for default (10 min)
//
// Returns:
//   - UUID of the first session found in the time window
//   - Error if timestamp is zero or no match found
//
// Example:
//
//	// Find session created within ±10 minutes of manifest modification time
//	uuid, err := SearchHistoryByTimestamp(manifestModTime, 10)
func SearchHistoryByTimestamp(timestamp time.Time, windowMinutes int) (string, error) {
	if timestamp.IsZero() {
		return "", fmt.Errorf("timestamp cannot be zero")
	}

	// Use default window if not specified or invalid
	if windowMinutes <= 0 {
		windowMinutes = DefaultWindowMinutes
	}

	windowDuration := time.Duration(windowMinutes) * time.Minute
	startTime := timestamp.Add(-windowDuration)
	endTime := timestamp.Add(windowDuration)

	// Convert to Unix milliseconds for comparison with new format
	startMillis := startTime.UnixMilli()
	endMillis := endTime.UnixMilli()

	parser := history.NewParser("")
	// Use ReadConversations to get new format entries
	sessions, err := parser.ReadConversations(1000) // Read last 1000 entries
	if err != nil {
		return "", fmt.Errorf("failed to read history: %w", err)
	}

	// Find first session with any entry in the time window
	for _, session := range sessions {
		for _, entry := range session.Entries {
			if entry.Timestamp >= startMillis && entry.Timestamp <= endMillis {
				return session.SessionID, nil
			}
		}
	}

	return "", fmt.Errorf("no session found in time window around %s (±%d min)",
		timestamp.Format("2006-01-02 15:04:05"), windowMinutes)
}

// FindMostRecentJSONL scans a project directory for .jsonl files and returns
// the UUID extracted from the most recently modified file.
//
// This is a last-resort fallback when neither AGM manifest nor history lookups
// succeed. It relies on the convention that Claude saves transcripts as
// <uuid>.jsonl in the projects directory.
//
// Parameters:
//   - projectPath: Absolute path to the project directory (e.g., ~/.claude/projects/<session-name>)
//
// Returns:
//   - UUID extracted from the most recent .jsonl filename
//   - Error if directory doesn't exist, contains no .jsonl files, or UUID extraction fails
//
// Example:
//
//	uuid, err := FindMostRecentJSONL("~/.claude/projects/my-session")
func FindMostRecentJSONL(projectPath string) (string, error) {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("project directory does not exist: %s", projectPath)
		}
		return "", fmt.Errorf("failed to read project directory: %w", err)
	}

	// Collect .jsonl files with their FileInfo for sorting
	type jsonlFile struct {
		name    string
		modTime time.Time
	}
	var jsonlFiles []jsonlFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue // Skip files we can't stat
		}

		jsonlFiles = append(jsonlFiles, jsonlFile{
			name:    entry.Name(),
			modTime: info.ModTime(),
		})
	}

	if len(jsonlFiles) == 0 {
		return "", fmt.Errorf("no .jsonl files found in: %s", projectPath)
	}

	// Sort by modification time (most recent first)
	sort.Slice(jsonlFiles, func(i, j int) bool {
		return jsonlFiles[i].modTime.After(jsonlFiles[j].modTime)
	})

	// Extract UUID from filename (remove .jsonl extension)
	mostRecent := jsonlFiles[0].name
	uuid := strings.TrimSuffix(mostRecent, ".jsonl")

	// Validate UUID format (basic check: length 36, 4 dashes)
	if len(uuid) != 36 || strings.Count(uuid, "-") != 4 {
		return "", fmt.Errorf("invalid UUID format in filename: %s", mostRecent)
	}

	return uuid, nil
}

// Discover orchestrates the 3-level UUID discovery fallback chain.
//
// Discovery levels:
//  1. AGM manifest lookup (via manifestSearchFunc), verified against /rename if available
//  2. History search by rename (/rename command - strong signal)
//  3. JSONL fallback (scan ~/.claude/projects/<sessionName>/ for recent .jsonl)
//
// Parameters:
//   - sessionName: The session name to discover UUID for
//   - manifestSearchFunc: Function that searches AGM manifests. Should return
//     manifest if found, or error if not found. Pass nil to skip Level 1.
//   - verbose: If true, prints diagnostic output to stderr showing discovery path
//
// Returns:
//   - UUID if found via any level
//   - Aggregated error if all levels fail
//
// Example:
//
//	findInManifests := func(name string) (*manifest.Manifest, error) {
//	    manifests, _ := manifest.List(cfg.SessionsDir)
//	    for _, m := range manifests {
//	        if m.Tmux.SessionName == name || m.Name == name {
//	            return m, nil
//	        }
//	    }
//	    return nil, fmt.Errorf("no AGM session found")
//	}
//	uuid, err := Discover("my-session", findInManifests, false)
func Discover(sessionName string, manifestSearchFunc func(string) (*manifest.Manifest, error), verbose bool) (string, error) {
	logf := func(format string, args ...interface{}) {
		if verbose {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	var errors []string

	// Level 1: AGM manifest search
	if manifestSearchFunc != nil {
		logf("Level 1: AGM manifest search...")
		m, err := manifestSearchFunc(sessionName)
		if err == nil && m != nil {
			if m.Claude.UUID != "" {
				logf("  ✓ found: %s", m.Claude.UUID)
				// Verify manifest UUID matches /rename search (stronger signal)
				verifyUUID, verifyErr := SearchHistoryByRename(sessionName)
				if verifyErr == nil {
					if verifyUUID == m.Claude.UUID {
						// Manifest UUID verified via /rename
						return m.Claude.UUID, nil
					}
					// Manifest has wrong UUID - trust /rename instead
					logf("  - manifest UUID mismatch, using /rename result: %s", verifyUUID)
					return verifyUUID, nil
				}
				// Can't verify via /rename, but manifest UUID exists - trust it
				logf("  - manifest UUID not verified (no /rename found), using manifest value")
				return m.Claude.UUID, nil
			}
			logf("  - manifest found but has no UUID")
		} else {
			logf("  - not found: %v", err)
			errors = append(errors, fmt.Sprintf("Level 1 (manifest): %v", err))
		}
	}

	// Level 2a: History search by rename
	logf("Level 2a: History search by rename...")
	uuid, err := SearchHistoryByRename(sessionName)
	if err == nil {
		logf("  ✓ found: %s", uuid)
		return uuid, nil
	}
	logf("  - not found: %v", err)
	errors = append(errors, fmt.Sprintf("Level 2a (rename): %v", err))

	// Level 3: JSONL fallback
	logf("Level 3: JSONL fallback...")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		errMsg := fmt.Sprintf("failed to get home directory: %v", err)
		logf("  - failed: %s", errMsg)
		errors = append(errors, fmt.Sprintf("Level 3 (JSONL): %s", errMsg))
	} else {
		projectPath := filepath.Join(homeDir, ".claude", "projects", sessionName)
		uuid, err := FindMostRecentJSONL(projectPath)
		if err == nil {
			logf("  ✓ found: %s", uuid)
			return uuid, nil
		}
		logf("  - not found: %v", err)
		errors = append(errors, fmt.Sprintf("Level 3 (JSONL): %v", err))
	}

	// All levels failed
	return "", fmt.Errorf("UUID discovery failed for '%s':\n  %s",
		sessionName, strings.Join(errors, "\n  "))
}
