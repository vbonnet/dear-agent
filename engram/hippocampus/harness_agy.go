package hippocampus

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// AgyAdapter discovers Antigravity CLI transcript artifacts.
type AgyAdapter struct {
	appDir string
}

var agyConversationIDPattern = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

const agyWorkspaceMarker = "Initializing CLI store manager for workspace "

// NewAgyAdapter creates an Antigravity CLI transcript adapter.
func NewAgyAdapter(appDir string) *AgyAdapter {
	if appDir == "" {
		appDir = defaultHomeSubdir(".gemini", "antigravity-cli")
	}
	return &AgyAdapter{appDir: appDir}
}

// Name returns the canonical Antigravity harness identifier.
func (a *AgyAdapter) Name() string { return "agy" }

// GetMemoryDir returns the shared Engram memory directory for a project.
func (a *AgyAdapter) GetMemoryDir(projectPath string) (string, error) {
	return existingCanonicalMemoryDir(projectPath)
}

// DiscoverSessions finds Antigravity transcript artifacts for a project.
func (a *AgyAdapter) DiscoverSessions(ctx context.Context, projectPath string, since time.Time) ([]SessionInfo, error) {
	projectByID := a.projectByConversationID()
	wantedProject := ""
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return nil, fmt.Errorf("resolve project path: %w", err)
		}
		wantedProject = filepath.Clean(abs)
	}

	brainDir := filepath.Join(a.appDir, "brain")
	entries, err := os.ReadDir(brainDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read Antigravity brain directory: %w", err)
	}
	var sessions []SessionInfo
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		project := projectByID[id]
		if wantedProject != "" && filepath.Clean(project) != wantedProject {
			continue
		}
		logsDir := filepath.Join(brainDir, id, ".system_generated", "logs")
		transcript := filepath.Join(logsDir, "transcript.jsonl")
		info, err := os.Stat(transcript)
		if err != nil {
			transcript = filepath.Join(logsDir, "transcript_full.jsonl")
			info, err = os.Stat(transcript)
		}
		if err != nil || info.ModTime().Before(since) {
			continue
		}
		start := readAgyStartTime(transcript)
		if start.IsZero() {
			start = info.ModTime()
		}
		sessions = append(sessions, SessionInfo{
			ID: id, StartTime: start, EndTime: info.ModTime(), Project: project, FilePath: transcript,
		})
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartTime.Before(sessions[j].StartTime) })
	return sessions, nil
}

// ReadTranscript extracts user and assistant text from an Antigravity transcript.
func (a *AgyAdapter) ReadTranscript(ctx context.Context, session SessionInfo) (string, error) {
	file, err := os.Open(session.FilePath)
	if err != nil {
		return "", fmt.Errorf("open Antigravity transcript: %w", err)
	}
	defer file.Close()
	var texts []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if text := extractAgyText(scanner.Bytes()); text != "" {
			texts = append(texts, text)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan Antigravity transcript: %w", err)
	}
	return strings.Join(texts, "\n"), nil
}

func (a *AgyAdapter) projectByConversationID() map[string]string {
	data, err := os.ReadFile(filepath.Join(a.appDir, "cache", "last_conversations.json"))
	byID := make(map[string]string)
	var byProject map[string]string
	if err == nil && json.Unmarshal(data, &byProject) == nil {
		for project, id := range byProject {
			abs, absErr := filepath.Abs(project)
			if absErr == nil {
				byID[id] = filepath.Clean(abs)
			}
		}
	}
	a.addProjectsFromLogs(byID)
	return byID
}

func (a *AgyAdapter) addProjectsFromLogs(byID map[string]string) {
	entries, err := os.ReadDir(filepath.Join(a.appDir, "log"))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		a.addProjectsFromLog(filepath.Join(a.appDir, "log", entry.Name()), byID)
	}
}

func (a *AgyAdapter) addProjectsFromLog(path string, byID map[string]string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	workspace := ""
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if _, detectedWorkspace, found := strings.Cut(line, agyWorkspaceMarker); found {
			workspace = strings.TrimSpace(detectedWorkspace)
			continue
		}
		if workspace == "" || (!strings.Contains(line, "Created conversation ") &&
			!strings.Contains(line, "Resuming conversation ") &&
			!strings.Contains(line, "GetConversationDetail: found conversation ")) {
			continue
		}
		if id := agyConversationIDPattern.FindString(line); id != "" {
			if abs, absErr := filepath.Abs(workspace); absErr == nil {
				byID[id] = filepath.Clean(abs)
			}
		}
	}
}

func readAgyStartTime(path string) time.Time {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var entry struct {
			CreatedAt string `json:"created_at"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.CreatedAt != "" {
			started, _ := time.Parse(time.RFC3339Nano, entry.CreatedAt)
			return started
		}
	}
	return time.Time{}
}

func extractAgyText(line []byte) string {
	var entry struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Content == "" {
		return ""
	}
	switch entry.Type {
	case "USER_INPUT":
		return "user: " + entry.Content
	case "PLANNER_RESPONSE":
		return "assistant: " + entry.Content
	default:
		return ""
	}
}
