package ops

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

// AuditEvent represents a single auditable AGM command execution.
type AuditEvent struct {
	Timestamp  time.Time         `json:"timestamp"`
	Command    string            `json:"command"`
	Session    string            `json:"session,omitempty"`
	User       string            `json:"user,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Result     string            `json:"result"`
	DurationMs int64             `json:"duration_ms"`
	Error      string            `json:"error,omitempty"`
}

// AuditLogger writes audit events to a JSONL file.
type AuditLogger struct {
	filePath string
	mu       sync.Mutex
}

// defaultAuditDir returns ~/.agm/logs.
func defaultAuditDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".agm", "logs"), nil
}

// NewAuditLogger creates a new AuditLogger.
// If filePath is empty, it defaults to ~/.agm/logs/audit.jsonl.
func NewAuditLogger(filePath string) (*AuditLogger, error) {
	if filePath == "" {
		dir, err := defaultAuditDir()
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(dir, os.FileMode(contracts.Load().AuditTrail.LogDirectoryPermissions)); err != nil {
			return nil, fmt.Errorf("failed to create audit log directory: %w", err)
		}
		filePath = filepath.Join(dir, "audit.jsonl")
	}

	return &AuditLogger{filePath: filePath}, nil
}

// Log writes an audit event to the JSONL file.
func (l *AuditLogger) Log(event AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	f, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, os.FileMode(contracts.Load().AuditTrail.LogFilePermissions))
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(event)
}

// FilePath returns the path to the audit log file.
func (l *AuditLogger) FilePath() string {
	return l.filePath
}

// AuditSearchParams defines filters for searching audit events.
type AuditSearchParams struct {
	Command string
	Session string
	Limit   int
}

// ReadRecentEvents reads the last N events from the audit log.
func ReadRecentEvents(filePath string, limit int) ([]AuditEvent, error) {
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()

	var all []AuditEvent
	scanner := bufio.NewScanner(f)
	maxBuf := contracts.Load().AuditTrail.MaxLineBufferBytes
	scanner.Buffer(make([]byte, 0, maxBuf), maxBuf)
	for scanner.Scan() {
		var ev AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue // skip malformed lines
		}
		all = append(all, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read audit log: %w", err)
	}

	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// SearchEvents searches audit events matching the given parameters.
func SearchEvents(filePath string, params AuditSearchParams) ([]AuditEvent, error) {
	all, err := ReadRecentEvents(filePath, 0)
	if err != nil {
		return nil, err
	}

	var matched []AuditEvent
	for _, ev := range all {
		if params.Command != "" && !strings.Contains(ev.Command, params.Command) {
			continue
		}
		if params.Session != "" && !strings.Contains(ev.Session, params.Session) {
			continue
		}
		matched = append(matched, ev)
	}

	if params.Limit > 0 && len(matched) > params.Limit {
		matched = matched[len(matched)-params.Limit:]
	}
	return matched, nil
}
