package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/claude"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"gopkg.in/yaml.v3"
)

// MatchResult contains results of matching Claude sessions to manifests
type MatchResult struct {
	Matched          map[string]*manifest.Manifest // UUID → manifest
	OrphanedClaude   []*claude.Session             // In history.jsonl, no manifest
	OrphanedManifest []*manifest.Manifest          // Manifest exists, UUID not in history
}

// MatchToManifests matches Claude sessions to existing manifests
func MatchToManifests(sessions []*claude.Session, manifests []*manifest.Manifest) *MatchResult {
	result := &MatchResult{
		Matched:          make(map[string]*manifest.Manifest),
		OrphanedClaude:   []*claude.Session{},
		OrphanedManifest: []*manifest.Manifest{},
	}

	// Build UUID → manifest map (v2: SessionID is top-level, not Claude.SessionID)
	manifestsByUUID := make(map[string]*manifest.Manifest)
	for _, m := range manifests {
		manifestsByUUID[m.SessionID] = m
	}

	// Match sessions to manifests
	for _, session := range sessions {
		if m, found := manifestsByUUID[session.UUID]; found {
			result.Matched[session.UUID] = m
		} else {
			result.OrphanedClaude = append(result.OrphanedClaude, session)
		}
	}

	// Find orphaned manifests (not in session list)
	sessionUUIDs := make(map[string]bool)
	for _, session := range sessions {
		sessionUUIDs[session.UUID] = true
	}

	for _, m := range manifests {
		if !sessionUUIDs[m.SessionID] {
			result.OrphanedManifest = append(result.OrphanedManifest, m)
		}
	}

	return result
}

// CreateManifest creates a new manifest for orphaned Claude session (v2 schema)
func CreateManifest(session *claude.Session, sessionsDir string, tmuxName string, sessionID string, adapter *dolt.Adapter) (*manifest.Manifest, error) {
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          tmuxName, // Use tmux name as manifest name
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // Empty = active/stopped (not archived)
		Context: manifest.Context{
			Project: session.Project,
			Purpose: "", // Unknown for discovered sessions
			Tags:    nil,
			Notes:   "",
		},
		Claude: manifest.Claude{
			UUID: session.UUID, // Store the actual Claude session UUID
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
		},
	}

	// Write to Dolt database
	if adapter == nil {
		return nil, fmt.Errorf("Dolt adapter required")
	}
	if err := adapter.CreateSession(m); err != nil {
		return nil, fmt.Errorf("failed to create session in Dolt: %w", err)
	}

	return m, nil
}

// GetTmuxMapping returns a map of Claude UUID → tmux session name
// by reading all manifests from Dolt database or sessions directory.
// This function is lenient and skips manifests that fail to parse.
func GetTmuxMapping(sessionsDir string, adapter *dolt.Adapter) (map[string]string, error) {
	return GetTmuxMappingWithAdapter(sessionsDir, adapter)
}

// GetTmuxMappingWithAdapter returns a map of SessionID → tmux session name
// using provided Dolt adapter or falling back to YAML manifests
func GetTmuxMappingWithAdapter(sessionsDir string, adapter *dolt.Adapter) (map[string]string, error) {
	mapping := make(map[string]string)

	// If adapter is provided, query Dolt database
	if adapter != nil {
		manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			return mapping, fmt.Errorf("failed to list sessions from Dolt: %w", err)
		}

		for _, m := range manifests {
			if m.SessionID != "" && m.Tmux.SessionName != "" {
				mapping[m.SessionID] = m.Tmux.SessionName
			}
		}

		return mapping, nil
	}

	// Fallback to YAML manifests if no adapter
	// Check if directory exists
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		// If directory doesn't exist, return empty map (not an error)
		if os.IsNotExist(err) {
			return mapping, nil
		}
		return mapping, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	// Read each manifest, skipping invalid ones
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(sessionsDir, entry.Name(), "manifest.yaml")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // Skip if can't read
		}

		// Parse manifest without validation
		var m manifest.Manifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			continue // Skip if can't parse
		}

		// Add to mapping if both SessionID and tmux name exist (v2: SessionID is top-level)
		if m.SessionID != "" && m.Tmux.SessionName != "" {
			mapping[m.SessionID] = m.Tmux.SessionName
		}
	}

	return mapping, nil
}
