package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/statusread"
	"gopkg.in/yaml.v3"
)

// WayfinderSession is the minimal summary returned by engram_list_wayfinder_sessions.
type WayfinderSession struct {
	ID              string `json:"id"`
	ProjectName     string `json:"project_name"`
	Status          string `json:"status"`
	CurrentWaypoint string `json:"current_waypoint"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
	Repository      string `json:"repository,omitempty"`
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

// fmString extracts one canonical string field from parsed frontmatter.
func fmString(fm map[string]any, key string) string {
	if value, ok := fm[key]; ok {
		switch typed := value.(type) {
		case string:
			return typed
		case time.Time:
			return typed.Format(time.RFC3339)
		}
	}
	return ""
}

func validateWayfinderV2Frontmatter(fm map[string]any) error {
	if fmString(fm, "schema_version") != "2.0" {
		return fmt.Errorf("unsupported Wayfinder schema: require schema_version 2.0")
	}
	for _, key := range []string{"project_name", "status", "current_waypoint"} {
		if fmString(fm, key) == "" {
			return fmt.Errorf("invalid Wayfinder V2 frontmatter: %s is required", key)
		}
	}
	return nil
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
	if _, err := statusread.Parse(data); err != nil {
		return id, nil, fmt.Errorf("invalid canonical status: %w", err)
	}
	fm, err := parseFrontmatter(data)
	if err != nil {
		return id, nil, err
	}
	if err := validateWayfinderV2Frontmatter(fm); err != nil {
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
			ID:              id,
			ProjectName:     fmString(fm, "project_name"),
			Status:          status,
			CurrentWaypoint: fmString(fm, "current_waypoint"),
			CreatedAt:       fmString(fm, "created_at"),
			UpdatedAt:       fmString(fm, "updated_at"),
			Repository:      fmString(fm, "repository"),
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
