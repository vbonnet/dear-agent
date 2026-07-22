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

const piTranscriptLineLimit = 4 * 1024 * 1024

// PiAdapter discovers Pi JSONL sessions from native and AGM-private stores.
type PiAdapter struct {
	roots []string
}

// NewPiAdapter creates a Pi transcript adapter. A non-empty dataDir is treated
// as an exact session root; the default covers both Pi-native and AGM-managed
// sessions and deduplicates them by native header ID.
func NewPiAdapter(dataDir string) *PiAdapter {
	if dataDir != "" {
		return &PiAdapter{roots: []string{dataDir}}
	}
	return &PiAdapter{roots: []string{
		defaultHomeSubdir(".agm", "pi", "sessions"),
		defaultHomeSubdir(".pi", "agent", "sessions"),
	}}
}

// Name returns Pi's canonical AGM harness identifier.
func (p *PiAdapter) Name() string { return "pi-cli" }

// GetMemoryDir returns the canonical Engram memory directory for a project.
func (p *PiAdapter) GetMemoryDir(projectPath string) (string, error) {
	return existingCanonicalMemoryDir(projectPath)
}

// DiscoverSessions scans Pi-native and AGM-managed roots without following symlinks.
//
//nolint:gocyclo // reason: bounded multi-root walk keeps filtering, deduplication, and cancellation in one callback
func (p *PiAdapter) DiscoverSessions(ctx context.Context, projectPath string, since time.Time) ([]SessionInfo, error) {
	wanted := ""
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return nil, fmt.Errorf("resolve project path: %w", err)
		}
		wanted = filepath.Clean(abs)
	}
	seen := make(map[string]bool)
	var sessions []SessionInfo
	for _, root := range p.roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil //nolint:nilerr // one unreadable native record must not disable discovery
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				return nil
			}
			meta, headerErr := readPiHeader(path)
			if headerErr == nil && meta.ID != "" && !seen[meta.ID] &&
				(wanted == "" || filepath.Clean(meta.Project) == wanted) {
				info, infoErr := entry.Info()
				if infoErr == nil && !info.ModTime().Before(since) {
					seen[meta.ID] = true
					sessions = append(sessions, SessionInfo{
						ID: meta.ID, StartTime: meta.Started, EndTime: info.ModTime(), Project: meta.Project, FilePath: path,
					})
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("scan Pi sessions: %w", err)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].StartTime.Before(sessions[j].StartTime) })
	return sessions, nil
}

type piHeader struct {
	ID      string
	Project string
	Started time.Time
}

func readPiHeader(path string) (piHeader, error) {
	file, err := os.Open(path)
	if err != nil {
		return piHeader{}, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return piHeader{}, fmt.Errorf("pi session header not found")
	}
	var header struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CWD       string `json:"cwd"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil || header.Type != "session" || !filepath.IsAbs(header.CWD) {
		return piHeader{}, fmt.Errorf("invalid Pi session header")
	}
	started, _ := time.Parse(time.RFC3339Nano, header.Timestamp)
	if started.IsZero() {
		if info, statErr := file.Stat(); statErr == nil {
			started = info.ModTime()
		}
	}
	return piHeader{ID: header.ID, Project: header.CWD, Started: started}, nil
}

// ReadTranscript extracts only user and assistant text from Pi message entries.
func (p *PiAdapter) ReadTranscript(ctx context.Context, session SessionInfo) (string, error) {
	file, err := os.Open(session.FilePath)
	if err != nil {
		return "", fmt.Errorf("open Pi transcript: %w", err)
	}
	defer file.Close()
	var texts []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), piTranscriptLineLimit)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "message" ||
			(entry.Message.Role != "user" && entry.Message.Role != "assistant") {
			continue
		}
		text := piMessageText(entry.Message.Content)
		if text != "" {
			texts = append(texts, entry.Message.Role+": "+text)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scan Pi transcript: %w", err)
	}
	return strings.Join(texts, "\n"), nil
}

func piMessageText(raw json.RawMessage) string {
	var direct string
	if json.Unmarshal(raw, &direct) == nil {
		return strings.TrimSpace(direct)
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, part := range parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}
