package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// GetCurrentSessionName returns the AGM session name for the current context.
// It auto-detects if running in a AGM-managed tmux session.
//
// Returns:
//   - Session name if in AGM session
//   - Error if not in tmux or not a AGM session
//
// This function is used for auto-detecting sender in agm session send command.
func GetCurrentSessionName(sessionsDir string, adapter *dolt.Adapter) (string, error) {
	// Check if running in tmux
	tmuxSessionName, err := tmux.GetCurrentSessionName()
	if err != nil {
		return "", fmt.Errorf("not in tmux session: %w", err)
	}

	// Look up AGM manifest by tmux session name from Dolt
	if adapter == nil {
		return "", fmt.Errorf("Dolt adapter required")
	}
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return "", fmt.Errorf("failed to list AGM sessions from Dolt: %w", err)
	}

	// Find manifest matching this tmux session
	for _, m := range manifests {
		// Skip archived sessions
		if m.Lifecycle == manifest.LifecycleArchived {
			continue
		}

		if m.Tmux.SessionName == tmuxSessionName {
			// Return the AGM session name (prefer Name field, fallback to tmux name)
			if m.Name != "" {
				return m.Name, nil
			}
			return m.Tmux.SessionName, nil
		}
	}

	return "", fmt.Errorf("tmux session '%s' is not a AGM-managed session", tmuxSessionName)
}

// ResolveIdentifier finds a manifest by tmux name, workspace ID, or session ID
func ResolveIdentifier(identifier string, sessionsDir string, adapter *dolt.Adapter) (*manifest.Manifest, string, error) {
	// Validate identifier to prevent path traversal attacks
	if err := validateIdentifier(identifier); err != nil {
		return nil, "", fmt.Errorf("invalid session identifier: %w", err)
	}

	// Try as session ID first using Dolt
	if adapter == nil {
		return nil, "", fmt.Errorf("Dolt adapter required")
	}

	manifestPath := filepath.Join(sessionsDir, identifier, "manifest.yaml")
	m, err := adapter.GetSession(identifier)
	if err == nil {
		return m, manifestPath, nil
	}

	// Try as tmux name or manifest name by scanning database
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	// First pass: try matching by tmux name (skip archived sessions)
	for _, m := range manifests {
		// Skip archived sessions - we want to resolve to active sessions only
		if m.Lifecycle == manifest.LifecycleArchived {
			continue
		}

		if m.Tmux.SessionName == identifier {
			// Find actual directory path (don't assume it matches session ID)
			actualPath, err := findManifestPath(sessionsDir, m.SessionID, adapter)
			if err != nil {
				return nil, "", fmt.Errorf("found session but couldn't locate manifest: %w", err)
			}
			return m, actualPath, nil
		}
	}

	// Second pass: try matching by manifest Name (v2 field, skip archived)
	for _, m := range manifests {
		// Skip archived sessions - we want to resolve to active sessions only
		if m.Lifecycle == manifest.LifecycleArchived {
			continue
		}

		if m.Name == identifier {
			// Find actual directory path (don't assume it matches session ID)
			actualPath, err := findManifestPath(sessionsDir, m.SessionID, adapter)
			if err != nil {
				return nil, "", fmt.Errorf("found session but couldn't locate manifest: %w", err)
			}
			return m, actualPath, nil
		}
	}

	return nil, "", fmt.Errorf("session not found: %s", identifier)
}

// findManifestPath returns the manifest path for a session ID
// For Dolt-backed sessions, paths are always predictable
func findManifestPath(sessionsDir string, sessionID string, adapter *dolt.Adapter) (string, error) {
	// For Dolt-backed sessions, manifest path is always predictable
	return filepath.Join(sessionsDir, sessionID, "manifest.yaml"), nil
}

// HealthReport contains health check results
type HealthReport struct {
	WorktreeExists bool
	Issues         []string
}

// checkClaudeBloat detects if a Claude Code session file is bloated
// Returns (true, error message) if bloated, (false, "") if healthy
func checkClaudeBloat(m *manifest.Manifest) (bool, string) {
	// Find Claude session file path
	// Session files are stored at: ~/.claude/projects/<project-hash>/<uuid>.jsonl
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, ""
	}

	// Generate project hash from project path
	projectHash := filepath.Base(m.Context.Project)
	if projectHash == "" || projectHash == "." {
		projectHash = "-home-user-src" // Common default
	}

	// Try to find the session file
	sessionFile := filepath.Join(homeDir, ".claude", "projects", projectHash, fmt.Sprintf("%s.jsonl", m.Claude.UUID))

	// If file doesn't exist in default location, try searching for it
	if _, err := os.Stat(sessionFile); os.IsNotExist(err) {
		// Try searching all project directories
		projectsDir := filepath.Join(homeDir, ".claude", "projects")
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			return false, "" // Can't check, silently skip
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidatePath := filepath.Join(projectsDir, entry.Name(), fmt.Sprintf("%s.jsonl", m.Claude.UUID))
			if _, err := os.Stat(candidatePath); err == nil {
				sessionFile = candidatePath
				break
			}
		}
	}

	// Check if file exists
	info, err := os.Stat(sessionFile)
	if os.IsNotExist(err) {
		return false, "" // File doesn't exist, nothing to check
	}
	if err != nil {
		return false, "" // Can't stat, silently skip
	}

	// Check file size (>100MB is suspicious)
	bloatSizeThreshold := contracts.Load().SessionLifecycle.BloatSizeThresholdBytes
	fileSizeMB := float64(info.Size()) / (1024 * 1024)

	if info.Size() > bloatSizeThreshold {
		// Count progress entries to confirm bloat
		progressCount, err := countProgressEntries(sessionFile)
		if err != nil {
			// If we can't count, just report based on file size
			return true, formatBloatError(sessionFile, fileSizeMB, -1)
		}

		// If file is large AND has many progress entries, it's definitely bloated
		if progressCount > contracts.Load().SessionLifecycle.BloatProgressEntryThreshold {
			return true, formatBloatError(sessionFile, fileSizeMB, progressCount)
		}
	}

	return false, ""
}

// countProgressEntries counts the number of progress entries in a session file
func countProgressEntries(sessionFile string) (int, error) {
	file, err := os.Open(sessionFile)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)

	// Increase buffer size for large lines (progress entries can be huge)
	const maxScanTokenSize = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	for scanner.Scan() {
		var entry map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil {
			if entryType, ok := entry["type"].(string); ok && entryType == "progress" {
				count++
			}
		}
	}

	return count, scanner.Err()
}

// formatBloatError generates a helpful error message for bloated sessions
func formatBloatError(sessionFile string, sizeMB float64, progressCount int) string {
	var msg strings.Builder

	fmt.Fprintf(&msg, "Claude Code session file is bloated (%.0fMB", sizeMB)
	if progressCount > 0 {
		fmt.Fprintf(&msg, ", %d progress entries", progressCount)
	}
	msg.WriteString(")\n")
	msg.WriteString("File: " + sessionFile + "\n")
	msg.WriteString("\n")
	msg.WriteString("This is a known bug in Claude Code 2.1.12-2.1.14 (GitHub Issue #19040).\n")
	msg.WriteString("Session files grow to multi-GB sizes with heavy subagent usage due to\n")
	msg.WriteString("normalizedMessages duplication in progress entries.\n")
	msg.WriteString("\n")
	msg.WriteString("To fix:\n")
	msg.WriteString("1. Create timestamped backup:\n")
	fmt.Fprintf(&msg, "   cp \"%s\" \"%s.backup-$(date +%%Y%%m%%d-%%H%%M%%S)\"\n", sessionFile, sessionFile)
	msg.WriteString("\n")
	msg.WriteString("2. Run cleanup script (removes normalizedMessages from progress entries):\n")
	msg.WriteString("   wget -O /tmp/fix-claude-sessions.py https://raw.githubusercontent.com/anthropics/claude-code/main/scripts/fix-claude-sessions.py\n")
	fmt.Fprintf(&msg, "   python3 /tmp/fix-claude-sessions.py \"%s\"\n", sessionFile)
	msg.WriteString("\n")
	msg.WriteString("3. Ensure custom-title is first line (required for Claude Code 2.1.14+):\n")
	msg.WriteString("   wget -O /tmp/move-custom-title-to-first.py https://raw.githubusercontent.com/anthropics/claude-code/main/scripts/move-custom-title-to-first.py\n")
	fmt.Fprintf(&msg, "   python3 /tmp/move-custom-title-to-first.py \"%s\"\n", sessionFile)
	msg.WriteString("\n")
	msg.WriteString("After fixing, try resuming again with: agm session resume\n")
	msg.WriteString("\n")
	msg.WriteString("For more details: https://github.com/anthropics/claude-code/issues/19040")

	return msg.String()
}

// CheckHealth validates that all paths in manifest exist
func CheckHealth(m *manifest.Manifest) (*HealthReport, error) {
	report := &HealthReport{
		Issues: []string{},
	}

	// Check working directory (v2: Context.Project)
	if _, err := os.Stat(m.Context.Project); err != nil {
		report.WorktreeExists = false
		report.Issues = append(report.Issues, fmt.Sprintf("Working directory does not exist: %s", m.Context.Project))
	} else {
		report.WorktreeExists = true
	}

	// Check for Claude Code session bloat (only for Claude Code harness)
	harnessName := m.Harness
	if harnessName == "" {
		harnessName = "claude-code" // Default for backward compatibility
	}

	if harnessName == "claude-code" && m.Claude.UUID != "" {
		if bloated, info := checkClaudeBloat(m); bloated {
			report.Issues = append(report.Issues, info)
		}
	}

	return report, nil
}

// IsHealthy returns true if all health checks pass
func (r *HealthReport) IsHealthy() bool {
	return r.WorktreeExists
}

// Summary returns a human-readable summary of health issues
func (r *HealthReport) Summary() string {
	if r.IsHealthy() {
		return "All health checks passed"
	}
	return strings.Join(r.Issues, "\n")
}

// ArchivedSession represents metadata for an archived session
type ArchivedSession struct {
	SessionID    string
	Name         string
	ArchivedAt   string // Formatted date
	Tags         []string
	Project      string
	ManifestPath string // Full path to manifest.yaml
}

// FindArchived searches for archived sessions matching the given glob pattern
func FindArchived(sessionsDir string, pattern string, adapter *dolt.Adapter) ([]*ArchivedSession, error) {
	// Validate glob pattern
	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, fmt.Errorf("invalid glob pattern: %w", err)
	}

	// Get all manifests from both locations
	var allManifests []*manifest.Manifest
	var manifestPaths []string

	// Location 1: In-place archived sessions (Lifecycle: "archived")
	if adapter == nil {
		return nil, fmt.Errorf("Dolt adapter required")
	}

	// Query Dolt for archived sessions
	manifests, err := adapter.ListSessions(&dolt.SessionFilter{
		Lifecycle: manifest.LifecycleArchived,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
	}

	for _, m := range manifests {
		if m.Lifecycle == manifest.LifecycleArchived {
			// Find the actual manifest path
			actualPath, err := findManifestPath(sessionsDir, m.SessionID, adapter)
			if err != nil {
				continue // Skip if can't find path
			}
			allManifests = append(allManifests, m)
			manifestPaths = append(manifestPaths, actualPath)
		}
	}

	// Filter by glob pattern (match against session name and tmux name)
	var matches []*ArchivedSession
	for i, m := range allManifests {
		nameMatches := false

		// Try matching against manifest name
		if m.Name != "" {
			if matched, _ := filepath.Match(pattern, m.Name); matched {
				nameMatches = true
			}
		}

		// Try matching against tmux session name
		if m.Tmux.SessionName != "" {
			if matched, _ := filepath.Match(pattern, m.Tmux.SessionName); matched {
				nameMatches = true
			}
		}

		// Try matching against session ID
		if matched, _ := filepath.Match(pattern, m.SessionID); matched {
			nameMatches = true
		}

		if nameMatches {
			// Determine display name
			displayName := m.Name
			if displayName == "" {
				displayName = m.Tmux.SessionName
			}
			if displayName == "" {
				displayName = m.SessionID
			}

			// Format archived date
			archivedAt := "unknown"
			if !m.UpdatedAt.IsZero() {
				archivedAt = m.UpdatedAt.Format("2006-01-02")
			}

			matches = append(matches, &ArchivedSession{
				SessionID:    m.SessionID,
				Name:         displayName,
				ArchivedAt:   archivedAt,
				Tags:         m.Context.Tags,
				Project:      m.Context.Project,
				ManifestPath: manifestPaths[i],
			})
		}
	}

	// Sort by archived date (most recent first)
	// Use UpdatedAt timestamp from manifest
	for i := 0; i < len(matches); i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].ArchivedAt > matches[i].ArchivedAt {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	return matches, nil
}

// validateIdentifier checks if a session identifier is safe to use in file paths
// This prevents path traversal attacks (e.g., "../../etc/passwd")
func validateIdentifier(identifier string) error {
	if identifier == "" {
		return fmt.Errorf("identifier cannot be empty")
	}

	// Reject if identifier contains path separators or parent directory references
	if strings.Contains(identifier, "/") {
		return fmt.Errorf("identifier cannot contain forward slashes")
	}
	if strings.Contains(identifier, "\\") {
		return fmt.Errorf("identifier cannot contain backslashes")
	}
	if strings.Contains(identifier, "..") {
		return fmt.Errorf("identifier cannot contain '..'")
	}

	// Reject if starts with a dot (hidden files/directories)
	if strings.HasPrefix(identifier, ".") {
		return fmt.Errorf("identifier cannot start with '.'")
	}

	// Ensure cleaned path equals original (catches any other path tricks)
	cleaned := filepath.Clean(identifier)
	if cleaned != identifier {
		return fmt.Errorf("identifier contains invalid path components")
	}

	return nil
}
