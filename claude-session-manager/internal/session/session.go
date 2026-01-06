package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// ResolveIdentifier finds a manifest by tmux name, workspace ID, or session ID
func ResolveIdentifier(identifier string, sessionsDir string) (*manifest.Manifest, string, error) {
	// Try as session ID (workspace ID) first
	manifestPath := filepath.Join(sessionsDir, identifier, "manifest.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		m, err := manifest.Read(manifestPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read manifest: %w", err)
		}
		return m, manifestPath, nil
	}

	// Try as tmux name or manifest name by scanning filesystem
	// This handles legacy session directories that don't match session_id
	manifests, err := manifest.List(sessionsDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list manifests: %w", err)
	}

	// First pass: try matching by tmux name (skip archived sessions)
	for _, m := range manifests {
		// Skip archived sessions - we want to resolve to active sessions only
		if m.Lifecycle == manifest.LifecycleArchived {
			continue
		}

		if m.Tmux.SessionName == identifier {
			// Find actual directory path (don't assume it matches session ID)
			actualPath, err := findManifestPath(sessionsDir, m.SessionID)
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
			actualPath, err := findManifestPath(sessionsDir, m.SessionID)
			if err != nil {
				return nil, "", fmt.Errorf("found session but couldn't locate manifest: %w", err)
			}
			return m, actualPath, nil
		}
	}

	return nil, "", fmt.Errorf("session not found: %s", identifier)
}

// findManifestPath scans the sessions directory to find the actual path
// to a manifest with the given session ID. This handles legacy directories
// that are named differently than their session_id field.
func findManifestPath(sessionsDir string, sessionID string) (string, error) {
	// First try the expected path (for new-style directories)
	expectedPath := filepath.Join(sessionsDir, sessionID, "manifest.yaml")
	if _, err := os.Stat(expectedPath); err == nil {
		return expectedPath, nil
	}

	// Scan all directories to find where this session actually lives
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read sessions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip archive directory
		if entry.Name() == ".archive-old-format" {
			continue
		}

		manifestPath := filepath.Join(sessionsDir, entry.Name(), "manifest.yaml")
		if _, err := os.Stat(manifestPath); err == nil {
			// Read the manifest to check session ID
			m, err := manifest.Read(manifestPath)
			if err == nil && m.SessionID == sessionID {
				return manifestPath, nil
			}
		}
	}

	return "", fmt.Errorf("could not find manifest for session ID: %s", sessionID)
}

// HealthReport contains health check results
type HealthReport struct {
	WorktreeExists bool
	Issues         []string
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
	SessionID  string
	Name       string
	ArchivedAt string // Formatted date
	Tags       []string
	Project    string
	ManifestPath string // Full path to manifest.yaml
}

// FindArchived searches for archived sessions matching the given glob pattern
func FindArchived(sessionsDir string, pattern string) ([]*ArchivedSession, error) {
	// Validate glob pattern
	if _, err := filepath.Match(pattern, ""); err != nil {
		return nil, fmt.Errorf("invalid glob pattern: %w", err)
	}

	// Get all manifests from both locations
	var allManifests []*manifest.Manifest
	var manifestPaths []string

	// Location 1: In-place archived sessions (Lifecycle: "archived")
	manifests, err := manifest.List(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list manifests: %w", err)
	}

	for _, m := range manifests {
		if m.Lifecycle == manifest.LifecycleArchived {
			// Find the actual manifest path
			actualPath, err := findManifestPath(sessionsDir, m.SessionID)
			if err != nil {
				continue // Skip if can't find path
			}
			allManifests = append(allManifests, m)
			manifestPaths = append(manifestPaths, actualPath)
		}
	}

	// Location 2: .archive-old-format subdirectory
	archiveDir := filepath.Join(sessionsDir, ".archive-old-format")
	if info, err := os.Stat(archiveDir); err == nil && info.IsDir() {
		archiveManifests, err := manifest.List(archiveDir)
		if err == nil {
			for _, m := range archiveManifests {
				actualPath, err := findManifestPath(archiveDir, m.SessionID)
				if err != nil {
					continue
				}
				allManifests = append(allManifests, m)
				manifestPaths = append(manifestPaths, actualPath)
			}
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
