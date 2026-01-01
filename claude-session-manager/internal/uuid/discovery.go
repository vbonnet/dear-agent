package uuid

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/history"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// Package uuid provides UUID discovery functions for CSM sessions.
// This package implements a 3-level fallback chain:
// - Level 1: CSM manifest lookup
// - Level 2a: History search by rename (/rename command)
// - Level 2b: History search by timestamp (±10 min window)
// - Level 3: JSONL fallback (most recent .jsonl file in projects dir)

// SearchHistoryByRename searches the Claude history.jsonl for sessions renamed
// with the /rename command. Returns the UUID of the most recent session with
// the given name.
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
	entries, err := parser.ReadAll()
	if err != nil {
		return "", fmt.Errorf("failed to read history: %w", err)
	}

	var latest *history.Entry
	for _, entry := range entries {
		if entry.Name == sessionName {
			if latest == nil || entry.Timestamp.After(latest.Timestamp) {
				latest = entry
			}
		}
	}

	if latest == nil {
		return "", fmt.Errorf("no rename found for: %s", sessionName)
	}

	return latest.UUID, nil
}

// DefaultWindowMinutes is the default time window for timestamp searches
const DefaultWindowMinutes = 10

// SearchHistoryByTimestamp searches the Claude history.jsonl for sessions
// created within a time window around the given timestamp.
//
// This is useful when a manifest exists but lacks a UUID field - we can search
// for sessions created around the same time as the manifest's last modified time.
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

	parser := history.NewParser("")
	entries, err := parser.ReadAll()
	if err != nil {
		return "", fmt.Errorf("failed to read history: %w", err)
	}

	for _, entry := range entries {
		if entry.Timestamp.After(startTime) && entry.Timestamp.Before(endTime) {
			return entry.UUID, nil
		}
	}

	return "", fmt.Errorf("no session found in time window around %s (±%d min)",
		timestamp.Format("2006-01-02 15:04:05"), windowMinutes)
}

// FindMostRecentJSONL scans a project directory for .jsonl files and returns
// the UUID extracted from the most recently modified file.
//
// This is a last-resort fallback when neither CSM manifest nor history lookups
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
//	uuid, err := FindMostRecentJSONL("/home/user/.claude/projects/my-session")
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
//  1. CSM manifest lookup (via manifestSearchFunc)
//  2a. History search by rename (/rename command)
//  2b. History search by timestamp (±10 min window from manifest ModTime)
//  3. JSONL fallback (scan ~/.claude/projects/<sessionName>/ for recent .jsonl)
//
// Parameters:
//   - sessionName: The session name to discover UUID for
//   - manifestSearchFunc: Function that searches CSM manifests. Should return
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
//	    return nil, fmt.Errorf("no CSM session found")
//	}
//	uuid, err := Discover("my-session", findInManifests, false)
func Discover(sessionName string, manifestSearchFunc func(string) (*manifest.Manifest, error), verbose bool) (string, error) {
	logf := func(format string, args ...interface{}) {
		if verbose {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}

	var errors []string

	// Level 1: CSM manifest search
	if manifestSearchFunc != nil {
		logf("Level 1: CSM manifest search...")
		m, err := manifestSearchFunc(sessionName)
		if err == nil && m != nil {
			if m.Claude.UUID != "" {
				logf("  ✓ found: %s", m.Claude.UUID)
				return m.Claude.UUID, nil
			}
			logf("  - manifest found but has no UUID")

			// If manifest exists but no UUID, try Level 2b (timestamp search)
			logf("Level 2b: History search by timestamp (±%d min from manifest)...", DefaultWindowMinutes)
			uuid, err := SearchHistoryByTimestamp(m.UpdatedAt, DefaultWindowMinutes)
			if err == nil {
				logf("  ✓ found: %s", uuid)
				return uuid, nil
			}
			logf("  - not found: %v", err)
			errors = append(errors, fmt.Sprintf("Level 2b (timestamp): %v", err))
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
