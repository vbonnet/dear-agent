package interrupt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AuditEntry records a single interrupt event
type AuditEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	Sender       string    `json:"sender"`
	Recipient    string    `json:"recipient"`
	Reason       string    `json:"reason,omitempty"`
	SessionState string    `json:"session_state,omitempty"`
	FlagUsed     string    `json:"flag_used"`
	InterruptNum int       `json:"interrupt_num"`
}

// LogDir returns the directory for interrupt audit logs
func LogDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agm", "logs")
}

// LogPath returns the path to the interrupt audit log
func LogPath() string {
	return filepath.Join(LogDir(), "interrupt-audit.jsonl")
}

// LogInterrupt appends an audit entry to the interrupt log
func LogInterrupt(entry *AuditEntry) error {
	dir := LogDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	entry.Timestamp = time.Now()
	entry.InterruptNum = GetInterruptCount(entry.Recipient) + 1

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	f, err := os.OpenFile(LogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}

	return nil
}

// GetInterruptCount returns the number of interrupts for a given recipient
func GetInterruptCount(recipient string) int {
	data, err := os.ReadFile(LogPath())
	if err != nil {
		return 0
	}

	count := 0
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Recipient == recipient {
			count++
		}
	}
	return count
}
