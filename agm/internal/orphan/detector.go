// Package orphan provides orphan functionality.
package orphan

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
)

// OrphanedSession represents a conversation without AGM manifest
type OrphanedSession struct {
	// Conversation metadata
	UUID             string    // Claude conversation UUID
	ProjectPath      string    // Inferred from conversation file location
	LastModified     time.Time // From .jsonl file or history.jsonl entry
	ConversationPath string    // Path to .jsonl file (if found)

	// Detection metadata
	DetectedAt      time.Time // When orphan was discovered
	DetectionMethod string    // "history_scan", "projects_scan", "both"

	// Session inference
	InferredName    string // Suggested session name from directory
	HasConversation bool   // .jsonl file exists in ~/.claude/projects/
	Workspace       string // Workspace name (if detected)

	// Status
	Status     string     // "orphaned", "imported", "ignored"
	ImportedAt *time.Time // When manifest was created (nil if not imported)
}

// DetectionMethod constants
const (
	DetectionMethodHistory  = "history_scan"  // Found via history.jsonl
	DetectionMethodProjects = "projects_scan" // Found via ~/.claude/projects scan
	DetectionMethodBoth     = "both"          // Found via both methods
)

// Status constants
const (
	StatusOrphaned = "orphaned" // No manifest exists
	StatusImported = "imported" // Manifest created
	StatusIgnored  = "ignored"  // User chose to skip
)

// OrphanDetectionReport summarizes orphan detection across workspaces
type OrphanDetectionReport struct {
	// Scan metadata
	ScanStarted       time.Time
	ScanCompleted     time.Time
	WorkspacesScanned []string

	// Results
	Orphans      []*OrphanedSession
	TotalOrphans int
	ByWorkspace  map[string]int // workspace -> count

	// Statistics
	HistoryEntries int // Total history.jsonl entries scanned
	ProjectsFound  int // Total .jsonl files in ~/.claude/projects/
	ManifestsFound int // Total existing manifests

	// Errors
	Errors []DetectionError
}

// DetectionError captures non-fatal errors during scan
type DetectionError struct {
	Path      string
	Error     error
	Timestamp time.Time
}

// DetectOrphans scans for orphaned conversations across workspaces
// Returns orphaned sessions that exist in history.jsonl but have no AGM manifest
func DetectOrphans(sessionsDir string, workspaceFilter string, adapter *dolt.Adapter) (*OrphanDetectionReport, error) {
	return DetectOrphansWithAdapter(sessionsDir, workspaceFilter, adapter)
}

// DetectOrphansWithAdapter scans for orphaned conversations using provided Dolt adapter
// If adapter is nil, falls back to YAML manifest scanning
func DetectOrphansWithAdapter(sessionsDir string, workspaceFilter string, adapter *dolt.Adapter) (*OrphanDetectionReport, error) {
	report := &OrphanDetectionReport{
		ScanStarted:       time.Now(),
		WorkspacesScanned: []string{},
		Orphans:           []*OrphanedSession{},
		ByWorkspace:       make(map[string]int),
		Errors:            []DetectionError{},
	}

	// Step 1: Scan history.jsonl for all UUIDs
	historyUUIDs, historySessionMap, err := scanHistoryForUUIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to scan history: %w", err)
	}
	report.HistoryEntries = len(historySessionMap)

	// Step 2: Load all manifests from Dolt database
	if adapter == nil {
		return nil, fmt.Errorf("Dolt adapter required")
	}
	manifestUUIDs, err := loadManifestUUIDsWithAdapter(adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to load manifests: %w", err)
	}
	report.ManifestsFound = len(manifestUUIDs)

	// Step 3: Detect orphans (UUIDs in history but not in manifests)
	for uuid := range historyUUIDs {
		if _, tracked := manifestUUIDs[uuid]; !tracked {
			// Found an orphan
			sessionInfo := historySessionMap[uuid]

			// Apply workspace filter if specified
			if workspaceFilter != "" && sessionInfo.Workspace != workspaceFilter {
				continue
			}

			orphan := &OrphanedSession{
				UUID:            uuid,
				ProjectPath:     sessionInfo.ProjectPath,
				LastModified:    sessionInfo.LastModified,
				DetectedAt:      time.Now(),
				DetectionMethod: DetectionMethodHistory,
				InferredName:    inferSessionName(sessionInfo.ProjectPath),
				HasConversation: false,
				Workspace:       sessionInfo.Workspace,
				Status:          StatusOrphaned,
			}

			// Check if conversation file exists
			conversationPath := findConversationFile(uuid)
			if conversationPath != "" {
				orphan.ConversationPath = conversationPath
				orphan.HasConversation = true
			}

			report.Orphans = append(report.Orphans, orphan)
			report.ByWorkspace[orphan.Workspace]++
		}
	}

	report.TotalOrphans = len(report.Orphans)
	report.ScanCompleted = time.Now()

	return report, nil
}

// sessionInfo holds metadata extracted from history.jsonl
type sessionInfo struct {
	ProjectPath  string
	LastModified time.Time
	Workspace    string
}

// scanHistoryForUUIDs reads history.jsonl and extracts all session UUIDs
func scanHistoryForUUIDs() (map[string]bool, map[string]*sessionInfo, error) {
	parser := history.NewParser("")
	sessions, err := parser.ReadConversations(10000) // Read last 10k entries
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read conversations: %w", err)
	}

	uuidSet := make(map[string]bool)
	sessionMap := make(map[string]*sessionInfo)

	for _, session := range sessions {
		if session.SessionID == "" {
			continue
		}

		uuidSet[session.SessionID] = true

		// Extract metadata from most recent entry
		var lastModified time.Time
		if len(session.Entries) > 0 {
			// Timestamp is in milliseconds
			lastModified = time.Unix(0, session.Entries[0].Timestamp*int64(time.Millisecond))
		}

		// Infer workspace from project path
		workspace := inferWorkspaceFromPath(session.Project)

		sessionMap[session.SessionID] = &sessionInfo{
			ProjectPath:  session.Project,
			LastModified: lastModified,
			Workspace:    workspace,
		}
	}

	return uuidSet, sessionMap, nil
}

// loadManifestUUIDsWithAdapter loads manifests from Dolt database and extracts their Claude UUIDs
func loadManifestUUIDsWithAdapter(adapter *dolt.Adapter) (map[string]bool, error) {
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	uuidSet := make(map[string]bool)
	for _, m := range manifests {
		if m.Claude.UUID != "" {
			uuidSet[m.Claude.UUID] = true
		}
	}

	return uuidSet, nil
}

// findConversationFile searches for conversation .jsonl file in ~/.claude/projects/
func findConversationFile(uuid string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	projectsDir := filepath.Join(homeDir, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		jsonlPath := filepath.Join(projectsDir, entry.Name(), uuid+".jsonl")
		if _, err := os.Stat(jsonlPath); err == nil {
			return jsonlPath
		}
	}

	return ""
}

// inferSessionName suggests a session name from project path
func inferSessionName(projectPath string) string {
	if projectPath == "" {
		return "unknown-session"
	}

	// Use last directory component
	base := filepath.Base(projectPath)
	if base == "." || base == "/" {
		return "unknown-session"
	}

	return base
}

// inferWorkspaceFromPath attempts to extract workspace name from project path
// Examples:
//
//	~/src/ws/oss -> "oss"
//	~/src/ws/acme -> "acme"
//	~/src/ws/research -> "research"
func inferWorkspaceFromPath(projectPath string) string {
	if projectPath == "" {
		return ""
	}

	// Split path by separator
	parts := filepath.SplitList(projectPath)
	if len(parts) == 1 {
		// SplitList didn't split, try splitting manually
		parts = splitPath(projectPath)
	}

	// Look for "/ws/" pattern
	for i, part := range parts {
		if part == "ws" && i+1 < len(parts) {
			return parts[i+1]
		}
	}

	return ""
}

// splitPath splits a file path into components
func splitPath(path string) []string {
	var parts []string
	for {
		dir, file := filepath.Split(path)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		if dir == "" || dir == "/" {
			break
		}
		path = filepath.Clean(dir)
	}
	return parts
}
