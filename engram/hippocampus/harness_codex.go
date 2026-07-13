package hippocampus

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CodexCLIAdapter discovers Codex CLI rollout transcripts.
type CodexCLIAdapter struct {
	codexDir string
}

// NewCodexCLIAdapter creates a Codex CLI transcript adapter.
func NewCodexCLIAdapter(codexDir string) *CodexCLIAdapter {
	if codexDir == "" {
		codexDir = defaultHomeSubdir(".codex")
	}
	return &CodexCLIAdapter{codexDir: codexDir}
}

// Name returns the canonical Codex CLI harness identifier.
func (c *CodexCLIAdapter) Name() string { return "codex-cli" }

// GetMemoryDir returns the shared Engram memory directory for a project.
func (c *CodexCLIAdapter) GetMemoryDir(projectPath string) (string, error) {
	return existingCanonicalMemoryDir(projectPath)
}

// DiscoverSessions scans active and archived Codex rollout trees for a project.
func (c *CodexCLIAdapter) DiscoverSessions(ctx context.Context, projectPath string, since time.Time) ([]SessionInfo, error) {
	wantedProject := ""
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return nil, fmt.Errorf("resolve project path: %w", err)
		}
		wantedProject = filepath.Clean(abs)
	}

	seen := make(map[string]bool)
	var sessions []SessionInfo
	for _, root := range []string{filepath.Join(c.codexDir, "sessions"), filepath.Join(c.codexDir, "archived_sessions")} {
		found, err := discoverCodexRoot(ctx, root, wantedProject, since, seen)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("scan Codex sessions: %w", err)
		}
		sessions = append(sessions, found...)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartTime.Before(sessions[j].StartTime) })
	return sessions, nil
}

func discoverCodexRoot(ctx context.Context, root, wantedProject string, since time.Time, seen map[string]bool) ([]SessionInfo, error) {
	var sessions []SessionInfo
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // unreadable entries do not disable discovery
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		meta, err := readCodexSessionMeta(path)
		if err != nil || meta.ID == "" || seen[meta.ID] {
			return nil //nolint:nilerr // malformed transcripts are skipped independently
		}
		if wantedProject != "" && filepath.Clean(meta.Project) != wantedProject {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil //nolint:nilerr // a single disappearing file must not abort discovery
		}
		start := meta.StartTime
		if start.IsZero() {
			start = info.ModTime()
		}
		if info.ModTime().Before(since) && start.Before(since) {
			return nil
		}
		seen[meta.ID] = true
		sessions = append(sessions, SessionInfo{
			ID: meta.ID, StartTime: start, EndTime: info.ModTime(),
			Project: meta.Project, FilePath: path,
		})
		return nil
	})
	return sessions, err
}

// ReadTranscript extracts user and assistant text from a Codex rollout.
func (c *CodexCLIAdapter) ReadTranscript(ctx context.Context, session SessionInfo) (string, error) {
	file, err := os.Open(session.FilePath)
	if err != nil {
		return "", fmt.Errorf("open Codex transcript: %w", err)
	}
	defer file.Close()

	var texts []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if text := extractCodexText(scanner.Bytes()); text != "" {
			texts = append(texts, text)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan Codex transcript: %w", err)
	}
	return strings.Join(texts, "\n"), nil
}

type codexSessionMeta struct {
	ID        string
	Project   string
	StartTime time.Time
}

func readCodexSessionMeta(path string) (codexSessionMeta, error) {
	file, err := os.Open(path)
	if err != nil {
		return codexSessionMeta{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var line struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &line) != nil || line.Type != "session_meta" {
			continue
		}
		var payload struct {
			ID        string `json:"id"`
			Cwd       string `json:"cwd"`
			Timestamp string `json:"timestamp"`
		}
		if err := json.Unmarshal(line.Payload, &payload); err != nil {
			return codexSessionMeta{}, err
		}
		started, _ := time.Parse(time.RFC3339Nano, payload.Timestamp)
		return codexSessionMeta{ID: payload.ID, Project: payload.Cwd, StartTime: started}, nil
	}
	if err := scanner.Err(); err != nil {
		return codexSessionMeta{}, err
	}
	return codexSessionMeta{}, fmt.Errorf("codex session metadata not found")
}

func extractCodexText(line []byte) string {
	var entry struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "response_item" {
		return ""
	}
	var payload struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(entry.Payload, &payload) != nil || payload.Type != "message" ||
		(payload.Role != "user" && payload.Role != "assistant") {
		return ""
	}
	var parts []string
	for _, content := range payload.Content {
		if (content.Type == "input_text" || content.Type == "output_text" || content.Type == "text") && content.Text != "" {
			parts = append(parts, content.Text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return payload.Role + ": " + strings.Join(parts, " ")
}
