package discovery

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"gopkg.in/yaml.v3"
)

// SessionLocation represents a session's current location in the filesystem
type SessionLocation struct {
	Workspace       string // e.g., "oss", "[REDACTED_EMPLOYER]"
	SessionID       string // UUID from manifest
	Name            string // Session name (for unified storage path)
	ManifestPath    string // Current manifest.yaml path
	ConversationDir string // Current conversation directory
}

// FindSessionsAcrossWorkspaces discovers all sessions across all workspaces
// Scans ~/src/ws/*/sessions/ for manifest.yaml files
// Returns comprehensive inventory of sessions with their current locations
func FindSessionsAcrossWorkspaces() ([]SessionLocation, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	workspacesPattern := filepath.Join(homeDir, "src", "ws", "*")
	workspaceDirs, err := filepath.Glob(workspacesPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob workspaces: %w", err)
	}

	var locations []SessionLocation

	for _, wsDir := range workspaceDirs {
		workspace := filepath.Base(wsDir)
		sessionsDir := filepath.Join(wsDir, "sessions")

		// Skip if sessions directory doesn't exist
		if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
			continue
		}

		// Find all session directories (each contains manifest.yaml)
		sessionPattern := filepath.Join(sessionsDir, "*", "manifest.yaml")
		manifestPaths, err := filepath.Glob(sessionPattern)
		if err != nil {
			continue // Skip this workspace on error
		}

		for _, manifestPath := range manifestPaths {
			// Read manifest to get session ID and name
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				continue // Skip corrupted manifest
			}

			var m manifest.Manifest
			if err := yaml.Unmarshal(data, &m); err != nil {
				continue // Skip invalid YAML
			}

			sessionDir := filepath.Dir(manifestPath)
			conversationDir := sessionDir // Conversations in same directory (for now)

			location := SessionLocation{
				Workspace:       workspace,
				SessionID:       m.SessionID,
				Name:            m.Name,
				ManifestPath:    manifestPath,
				ConversationDir: conversationDir,
			}

			locations = append(locations, location)
		}
	}

	return locations, nil
}
