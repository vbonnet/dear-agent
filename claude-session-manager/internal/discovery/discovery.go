package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/claude"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// MatchResult contains results of matching Claude sessions to manifests
type MatchResult struct {
	Matched          map[string]*manifest.Manifest // UUID → manifest
	OrphanedClaude   []*claude.HistoryEntry        // In history.jsonl, no manifest
	OrphanedManifest []*manifest.Manifest          // Manifest exists, UUID not in history
}

// MatchToManifests matches Claude sessions to existing manifests
func MatchToManifests(sessions []*claude.HistoryEntry, manifests []*manifest.Manifest) *MatchResult {
	result := &MatchResult{
		Matched:          make(map[string]*manifest.Manifest),
		OrphanedClaude:   []*claude.HistoryEntry{},
		OrphanedManifest: []*manifest.Manifest{},
	}

	// Build UUID → manifest map
	manifestsByUUID := make(map[string]*manifest.Manifest)
	for _, m := range manifests {
		manifestsByUUID[m.Claude.SessionID] = m
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
		if !sessionUUIDs[m.Claude.SessionID] {
			result.OrphanedManifest = append(result.OrphanedManifest, m)
		}
	}

	return result
}

// CreateManifest creates a new manifest for orphaned Claude session
func CreateManifest(session *claude.HistoryEntry, sessionsDir string, tmuxName string, sessionID string) (*manifest.Manifest, error) {
	homeDir, _ := os.UserHomeDir()

	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Status:        manifest.StatusDiscovered,
		CreatedAt:     time.Now(),
		LastActivity:  session.Timestamp,
		Worktree: manifest.Worktree{
			Path:   session.WorkDir,
			Branch: "unknown",
			Repo:   "",
		},
		Claude: manifest.Claude{
			SessionID:       session.UUID,
			SessionEnvPath:  filepath.Join(homeDir, ".claude", "session-env", session.UUID),
			FileHistoryPath: filepath.Join(homeDir, ".claude", "file-history", session.UUID),
			StartedAt:       session.Timestamp,
			LastActivity:    session.Timestamp,
		},
		Tmux: manifest.Tmux{
			SessionName: tmuxName,
			WindowName:  "main",
			CreatedAt:   time.Now(),
		},
	}

	// Create manifest directory
	manifestDir := filepath.Join(sessionsDir, sessionID)
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create manifest directory: %w", err)
	}

	// Write manifest
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	return m, nil
}
