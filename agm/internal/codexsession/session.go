// Package codexsession resolves Codex CLI saved-session metadata.
package codexsession

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Metadata is the session_meta payload AGM needs to import or resume a Codex
// conversation.
type Metadata struct {
	SessionID string
	CWD       string
	Path      string
	ModTime   time.Time
	Archived  bool
}

// FindByID locates a Codex saved session by its session_meta.session_id.
func FindByID(homeDir, sessionID string) (*Metadata, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("codex session ID cannot be empty")
	}
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to determine home directory: %w", err)
		}
	}

	codexHome := filepath.Join(homeDir, ".codex")
	for _, tree := range []struct {
		root     string
		archived bool
	}{
		{root: filepath.Join(codexHome, "sessions")},
		{root: filepath.Join(codexHome, "archived_sessions"), archived: true},
	} {
		match, err := findInTree(tree.root, sessionID, tree.archived)
		if err != nil {
			return nil, err
		}
		if match != nil {
			return match, nil
		}
	}

	return nil, fmt.Errorf("no Codex saved session found for ID: %s", sessionID)
}

func findInTree(root, sessionID string, archived bool) (*Metadata, error) {
	var best *Metadata
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable Codex cache entries are ignored.
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if !strings.Contains(filepath.Base(path), sessionID) {
			// Older or renamed files may not include the UUID in the filename,
			// so this is only a fast path skip for ordinary rollout files.
			if strings.HasPrefix(filepath.Base(path), "rollout-") {
				return nil
			}
		}
		meta, ok := readMetadata(path)
		if !ok || meta.SessionID != sessionID {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil //nolint:nilerr // ignore unreadable entries.
		}
		meta.Path = path
		meta.ModTime = info.ModTime()
		meta.Archived = archived
		if best == nil || meta.ModTime.After(best.ModTime) {
			best = meta
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return best, err
}

func readMetadata(path string) (*Metadata, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Payload struct {
				SessionID string `json:"session_id"`
				ID        string `json:"id"`
				CWD       string `json:"cwd"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type != "session_meta" {
			continue
		}
		id := firstNonEmpty(entry.Payload.SessionID, entry.Payload.ID)
		if id == "" || entry.Payload.CWD == "" {
			return nil, false
		}
		return &Metadata{
			SessionID: id,
			CWD:       filepath.Clean(entry.Payload.CWD),
		}, true
	}
	return nil, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
