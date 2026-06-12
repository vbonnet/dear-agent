package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WayfinderSession is the minimal summary returned by engram_list_wayfinder_sessions.
type WayfinderSession struct {
	ID           string `json:"id"`
	ProjectName  string `json:"project_name"`
	Status       string `json:"status"`
	CurrentPhase string `json:"current_phase"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	Repository   string `json:"repository,omitempty"`
}

const defaultWayfinderDir = "~/src/engram-research/wf"
const wayfinderStatusFile = "WAYFINDER-STATUS.md"
const defaultWayfinderLimit = 100

// parseFrontmatter extracts and unmarshals the YAML frontmatter block from a
// Markdown file (the content between the first two --- lines).
func parseFrontmatter(content []byte) (map[string]any, error) {
	// Find first delimiter
	rest := bytes.TrimSpace(content)
	if !bytes.HasPrefix(rest, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}
	rest = rest[3:] // strip leading ---
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}
	// Find closing delimiter
	yamlData, _, ok := bytes.Cut(rest, []byte("\n---"))
	if !ok {
		return nil, fmt.Errorf("unterminated frontmatter")
	}

	var fm map[string]any
	if err := yaml.Unmarshal(yamlData, &fm); err != nil {
		return nil, fmt.Errorf("parse frontmatter yaml: %w", err)
	}
	return fm, nil
}

// fmString extracts a string field from a parsed frontmatter map, trying
// multiple key names (newer schema first, then legacy names).
func fmString(fm map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := fm[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// readWayfinderSession reads and parses WAYFINDER-STATUS.md from sessionDir.
// Returns the raw frontmatter map and the canonical session ID (the dir name).
func readWayfinderSession(sessionDir string) (string, map[string]any, error) {
	id := filepath.Base(sessionDir)
	statusPath := filepath.Join(sessionDir, wayfinderStatusFile)
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return id, nil, err
	}
	fm, err := parseFrontmatter(data)
	if err != nil {
		return id, nil, err
	}
	return id, fm, nil
}

// listWayfinderSessions walks wayfinderDir and returns sessions matching statusFilter.
// An empty statusFilter returns all sessions. limit <= 0 uses defaultWayfinderLimit.
func listWayfinderSessions(wayfinderDir, statusFilter string, limit int) ([]WayfinderSession, error) {
	if limit <= 0 {
		limit = defaultWayfinderLimit
	}
	if limit > 1000 {
		limit = 1000
	}

	entries, err := os.ReadDir(wayfinderDir)
	if err != nil {
		return nil, fmt.Errorf("read wayfinder dir %s: %w", wayfinderDir, err)
	}

	var sessions []WayfinderSession
	for _, entry := range entries {
		if len(sessions) >= limit {
			break
		}
		if !entry.IsDir() {
			continue
		}
		sessionDir := filepath.Join(wayfinderDir, entry.Name())
		id, fm, err := readWayfinderSession(sessionDir)
		if err != nil {
			// Skip dirs without a valid WAYFINDER-STATUS.md
			continue
		}

		status := fmString(fm, "status")
		if statusFilter != "" && status != statusFilter {
			continue
		}

		sessions = append(sessions, WayfinderSession{
			ID:           id,
			ProjectName:  fmString(fm, "project_name", "project"),
			Status:       status,
			CurrentPhase: fmString(fm, "current_phase", "current_waypoint"),
			CreatedAt:    fmString(fm, "created_at", "created"),
			UpdatedAt:    fmString(fm, "updated_at", "updated"),
			Repository:   fmString(fm, "repository"),
		})
	}
	return sessions, nil
}

// getWayfinderSessionDetail returns the full frontmatter of a single session
// as a JSON-encoded string. sessionID is the directory name under wayfinderDir.
func getWayfinderSessionDetail(wayfinderDir, sessionID string) (string, error) {
	// Prevent path traversal
	if strings.ContainsAny(sessionID, "/\\..") {
		return "", fmt.Errorf("invalid session_id")
	}
	sessionDir := filepath.Join(wayfinderDir, sessionID)
	id, fm, err := readWayfinderSession(sessionDir)
	if err != nil {
		return "", fmt.Errorf("session %q: %w", sessionID, err)
	}
	// Merge the canonical ID into the response
	fm["id"] = id
	out, err := json.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshal session: %w", err)
	}
	return string(out), nil
}
