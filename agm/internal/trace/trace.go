// Package trace provides file provenance tracking by searching history.jsonl
// for conversations that modified specific files.
package trace

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// HistoryEntry represents a single line from history.jsonl with file modifications
type HistoryEntry struct {
	SessionID     string   `json:"sessionId"`
	Project       string   `json:"project"`
	Timestamp     int64    `json:"timestamp"` // Unix milliseconds
	FilesModified []string `json:"files_modified,omitempty"`
}

// FileModification represents a single file modification event
type FileModification struct {
	FilePath  string
	SessionID string
	Timestamp time.Time
}

// SessionTrace groups modifications by session
type SessionTrace struct {
	SessionID     string
	SessionName   string // From manifest, or "<no manifest>" if orphaned
	Workspace     string
	Project       string
	Modifications []FileModification
}

// TraceResult contains all traces for a single file
type TraceResult struct {
	FilePath string
	Sessions []*SessionTrace
}

// TraceOptions configures the trace operation
type TraceOptions struct {
	FilePaths   []string
	Since       *time.Time
	Workspace   string
	SessionsDir string
}

// Tracer performs file provenance tracking
type Tracer struct {
	sessionsDir string
}

// NewTracer creates a new file tracer
func NewTracer(sessionsDir string) *Tracer {
	return &Tracer{
		sessionsDir: sessionsDir,
	}
}

// TraceFiles searches history.jsonl files for file modifications
func (t *Tracer) TraceFiles(opts TraceOptions) ([]*TraceResult, error) {
	// Collect all history.jsonl files from all workspaces
	historyFiles, err := t.findHistoryFiles()
	if err != nil {
		return nil, fmt.Errorf("failed to find history files: %w", err)
	}

	// Parse all history entries
	allEntries, err := t.parseAllHistoryFiles(historyFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to parse history files: %w", err)
	}

	// Load manifests for session name lookup
	manifests, err := t.loadManifests()
	if err != nil {
		// Non-fatal: we can still show UUIDs without names
		manifests = make(map[string]SessionInfo)
	}

	// Trace each file
	results := make([]*TraceResult, 0, len(opts.FilePaths))
	for _, filePath := range opts.FilePaths {
		// Normalize to absolute path
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			absPath = filePath
		}

		result := t.traceFile(absPath, allEntries, manifests, opts)
		results = append(results, result)
	}

	return results, nil
}

// findHistoryFiles finds all history.jsonl files in workspace directories
func (t *Tracer) findHistoryFiles() ([]string, error) {
	var historyFiles []string

	// Walk the sessions directory
	err := filepath.Walk(t.sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}

		if !info.IsDir() && info.Name() == "history.jsonl" {
			historyFiles = append(historyFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return historyFiles, nil
}

// parseAllHistoryFiles parses all history.jsonl files and returns entries
func (t *Tracer) parseAllHistoryFiles(historyFiles []string) ([]HistoryEntry, error) {
	var allEntries []HistoryEntry

	for _, historyFile := range historyFiles {
		entries, err := t.parseHistoryFile(historyFile)
		if err != nil {
			// Log warning but continue with other files
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", historyFile, err)
			continue
		}
		allEntries = append(allEntries, entries...)
	}

	return allEntries, nil
}

// parseHistoryFile parses a single history.jsonl file with null-byte resilience
func (t *Tracer) parseHistoryFile(path string) ([]HistoryEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	var entries []HistoryEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Handle null-byte corruption: remove null bytes
		if bytes.Contains(line, []byte{0}) {
			line = bytes.ReplaceAll(line, []byte{0}, []byte{})
		}

		var entry HistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Log warning and skip corrupted line
			fmt.Fprintf(os.Stderr, "Warning: skipped malformed line %d in %s: %v\n", lineNum, path, err)
			continue
		}

		// Only include entries with file modifications
		if len(entry.FilesModified) > 0 && entry.SessionID != "" {
			entries = append(entries, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading history file: %w", err)
	}

	return entries, nil
}

// SessionInfo holds session metadata from manifest
type SessionInfo struct {
	Name      string
	Workspace string
	Project   string
	UUID      string
}

// loadManifests loads session metadata from manifest.yaml files
func (t *Tracer) loadManifests() (map[string]SessionInfo, error) {
	manifests := make(map[string]SessionInfo)

	err := filepath.Walk(t.sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip directories we can't read
		}

		if !info.IsDir() && info.Name() == "manifest.yaml" {
			// Extract session info from manifest
			sessionInfo, err := t.parseManifest(path)
			if err != nil {
				// Skip invalid manifests
				return nil
			}

			// Map Claude UUID to session info
			if sessionInfo.UUID != "" {
				manifests[sessionInfo.UUID] = SessionInfo{
					Name:      sessionInfo.Name,
					Workspace: sessionInfo.Workspace,
					Project:   sessionInfo.Project,
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return manifests, nil
}

// ManifestData holds minimal manifest fields needed for tracing
type ManifestData struct {
	Name      string `yaml:"name"`
	Workspace string `yaml:"workspace"`
	Claude    struct {
		UUID string `yaml:"uuid"`
	} `yaml:"claude"`
	Context struct {
		Project string `yaml:"project"`
	} `yaml:"context"`
}

// parseManifest extracts session info from a manifest.yaml file
func (t *Tracer) parseManifest(path string) (*SessionInfo, error) {
	// We'll use a simple YAML parser approach
	// For now, just extract the UUID field
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Simple parsing: look for "uuid:" line
	lines := strings.Split(string(data), "\n")
	var uuid, name, workspace, project string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			name = strings.Trim(name, `"'`)
		} else if strings.HasPrefix(line, "workspace:") {
			workspace = strings.TrimSpace(strings.TrimPrefix(line, "workspace:"))
			workspace = strings.Trim(workspace, `"'`)
		} else if strings.HasPrefix(line, "uuid:") {
			uuid = strings.TrimSpace(strings.TrimPrefix(line, "uuid:"))
			uuid = strings.Trim(uuid, `"'`)
		} else if strings.HasPrefix(line, "project:") {
			project = strings.TrimSpace(strings.TrimPrefix(line, "project:"))
			project = strings.Trim(project, `"'`)
		}
	}

	if uuid == "" && name == "" {
		return nil, fmt.Errorf("invalid manifest: missing required fields")
	}

	return &SessionInfo{
		Name:      name,
		Workspace: workspace,
		Project:   project,
		UUID:      uuid,
	}, nil
}

// traceFile finds all modifications for a specific file
func (t *Tracer) traceFile(filePath string, entries []HistoryEntry, manifests map[string]SessionInfo, opts TraceOptions) *TraceResult {
	result := &TraceResult{
		FilePath: filePath,
		Sessions: []*SessionTrace{},
	}

	// Group modifications by session
	sessionMods := make(map[string][]FileModification)

	for _, entry := range entries {
		// Check if this entry modified the target file
		for _, modifiedFile := range entry.FilesModified {
			// Match exact path or substring
			if matchesPath(modifiedFile, filePath) {
				timestamp := time.UnixMilli(entry.Timestamp)

				// Apply date filter
				if opts.Since != nil && timestamp.Before(*opts.Since) {
					continue
				}

				mod := FileModification{
					FilePath:  modifiedFile,
					SessionID: entry.SessionID,
					Timestamp: timestamp,
				}

				sessionMods[entry.SessionID] = append(sessionMods[entry.SessionID], mod)
			}
		}
	}

	// Convert to SessionTrace structs
	for sessionID, mods := range sessionMods {
		info, ok := manifests[sessionID]
		if !ok {
			info = SessionInfo{
				Name:      "<no manifest>",
				Workspace: "",
				Project:   "",
			}
		}

		// Apply workspace filter
		if opts.Workspace != "" && info.Workspace != opts.Workspace {
			continue
		}

		// Sort modifications by timestamp
		sort.Slice(mods, func(i, j int) bool {
			return mods[i].Timestamp.Before(mods[j].Timestamp)
		})

		trace := &SessionTrace{
			SessionID:     sessionID,
			SessionName:   info.Name,
			Workspace:     info.Workspace,
			Project:       info.Project,
			Modifications: mods,
		}

		result.Sessions = append(result.Sessions, trace)
	}

	// Sort sessions by first modification time
	sort.Slice(result.Sessions, func(i, j int) bool {
		if len(result.Sessions[i].Modifications) == 0 {
			return false
		}
		if len(result.Sessions[j].Modifications) == 0 {
			return true
		}
		return result.Sessions[i].Modifications[0].Timestamp.Before(
			result.Sessions[j].Modifications[0].Timestamp,
		)
	})

	return result
}

// matchesPath checks if a file path matches the target (exact or substring)
func matchesPath(modifiedPath, targetPath string) bool {
	// Exact match
	if modifiedPath == targetPath {
		return true
	}

	// Substring match (case-sensitive)
	if strings.Contains(modifiedPath, targetPath) {
		return true
	}

	// Also check if target is a suffix (for relative path matching)
	if strings.HasSuffix(modifiedPath, targetPath) {
		return true
	}

	return false
}
