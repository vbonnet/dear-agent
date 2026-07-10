package hippocampus

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Register the pure-Go SQLite driver used by OpenCode stores.
)

// OpenCodeAdapter discovers sessions from OpenCode's SQLite store.
type OpenCodeAdapter struct {
	dataDir string
}

// NewOpenCodeAdapter creates an OpenCode transcript adapter.
func NewOpenCodeAdapter(dataDir string) *OpenCodeAdapter {
	if dataDir == "" {
		dataDir = os.Getenv("OPENCODE_DATA_DIR")
	}
	if dataDir == "" {
		dataDir = defaultHomeSubdir(".local", "share", "opencode")
	}
	return &OpenCodeAdapter{dataDir: dataDir}
}

// Name returns the canonical OpenCode harness identifier.
func (o *OpenCodeAdapter) Name() string { return "opencode-cli" }

// GetMemoryDir returns the shared Engram memory directory for a project.
func (o *OpenCodeAdapter) GetMemoryDir(projectPath string) (string, error) {
	return existingCanonicalMemoryDir(projectPath)
}

// DiscoverSessions queries OpenCode session metadata for a project.
func (o *OpenCodeAdapter) DiscoverSessions(ctx context.Context, projectPath string, since time.Time) ([]SessionInfo, error) {
	dbPath := filepath.Join(o.dataDir, "opencode.db")
	db, err := openReadOnlySQLite(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT id, directory, time_created, time_updated
		FROM session WHERE time_updated >= ? ORDER BY time_created, id`, since.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("query OpenCode sessions: %w", err)
	}
	defer rows.Close()
	wantedProject := ""
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return nil, fmt.Errorf("resolve project path: %w", err)
		}
		wantedProject = filepath.Clean(abs)
	}
	var sessions []SessionInfo
	for rows.Next() {
		var id, project string
		var createdMS, updatedMS int64
		if err := rows.Scan(&id, &project, &createdMS, &updatedMS); err != nil {
			return nil, fmt.Errorf("scan OpenCode session: %w", err)
		}
		if wantedProject != "" && filepath.Clean(project) != wantedProject {
			continue
		}
		sessions = append(sessions, SessionInfo{
			ID: id, StartTime: time.UnixMilli(createdMS), EndTime: time.UnixMilli(updatedMS),
			Project: project, FilePath: dbPath,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate OpenCode sessions: %w", err)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartTime.Before(sessions[j].StartTime) })
	return sessions, nil
}

// ReadTranscript extracts user and assistant text parts from an OpenCode session.
func (o *OpenCodeAdapter) ReadTranscript(ctx context.Context, session SessionInfo) (string, error) {
	dbPath := session.FilePath
	if dbPath == "" {
		dbPath = filepath.Join(o.dataDir, "opencode.db")
	}
	db, err := openReadOnlySQLite(dbPath)
	if err != nil {
		return "", err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT message.data, part.data
		FROM message JOIN part ON part.message_id = message.id
		WHERE message.session_id = ?
		ORDER BY message.time_created, message.id, part.time_created, part.id`, session.ID)
	if err != nil {
		return "", fmt.Errorf("query OpenCode transcript: %w", err)
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var messageData, partData string
		if err := rows.Scan(&messageData, &partData); err != nil {
			return "", fmt.Errorf("scan OpenCode transcript: %w", err)
		}
		var message struct {
			Role string `json:"role"`
		}
		var part struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(messageData), &message) != nil ||
			json.Unmarshal([]byte(partData), &part) != nil ||
			(message.Role != "user" && message.Role != "assistant") ||
			part.Type != "text" || strings.TrimSpace(part.Text) == "" {
			continue
		}
		texts = append(texts, message.Role+": "+part.Text)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate OpenCode transcript: %w", err)
	}
	return strings.Join(texts, "\n"), nil
}

func openReadOnlySQLite(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open SQLite %s: %w", path, err)
	}
	return db, nil
}
