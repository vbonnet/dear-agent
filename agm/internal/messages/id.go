package messages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MessageIDGenerator generates unique, short, sortable message IDs
//
// Format: {unix_ms}-{sender_short}-{seq}
// Example: 1738612345678-agm-send-001
//
// Properties:
//   - Short: ~25-30 chars
//   - Unique: timestamp + sender + sequence prevents collisions
//   - Sortable: Chronological by timestamp
//   - Readable: Can identify sender at a glance
type MessageIDGenerator struct {
	senderName string
	sequence   int
	mu         sync.Mutex
	stateFile  string
}

// sequenceState persists sequence counter to disk
type sequenceState struct {
	Sequence int `json:"sequence"`
}

// NewMessageIDGenerator creates a new ID generator for the given sender
func NewMessageIDGenerator(senderName, stateDir string) (*MessageIDGenerator, error) {
	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	// State file path (one per sender to avoid conflicts)
	stateFile := filepath.Join(stateDir, fmt.Sprintf("msg_seq_%s.json", senderName))

	gen := &MessageIDGenerator{
		senderName: senderName,
		sequence:   0,
		stateFile:  stateFile,
	}

	// Load existing sequence if file exists
	if err := gen.loadSequence(); err != nil {
		// If file doesn't exist, start from 0
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load sequence: %w", err)
		}
	}

	return gen, nil
}

// Next generates the next message ID and increments the sequence
func (g *MessageIDGenerator) Next() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Generate ID components
	unixMs := time.Now().UnixMilli()
	senderShort := g.senderName
	if len(senderShort) > 8 {
		senderShort = senderShort[:8]
	}

	// Increment sequence
	g.sequence++

	// Format ID: {timestamp}-{sender}-{seq}
	messageID := fmt.Sprintf("%d-%s-%03d", unixMs, senderShort, g.sequence)

	// Persist sequence to disk synchronously to prevent race conditions
	// SAFETY: Synchronous save prevents out-of-order writes from concurrent Next() calls
	// The atomic write (temp file + rename) is fast enough (<1ms) to not impact performance
	// Timestamp still provides uniqueness guarantee even if save fails
	if err := g.saveSequence(); err != nil {
		// Log error but don't fail - timestamp ensures uniqueness
		// Sequence will reset to last saved value on restart
		_ = err // Intentionally ignored - non-critical error
	}

	return messageID, nil
}

// loadSequence loads the sequence counter from disk
func (g *MessageIDGenerator) loadSequence() error {
	data, err := os.ReadFile(g.stateFile)
	if err != nil {
		return err
	}

	var state sequenceState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to parse sequence state: %w", err)
	}

	g.sequence = state.Sequence
	return nil
}

// saveSequence persists the sequence counter to disk
// IMPORTANT: Caller must hold g.mu lock
func (g *MessageIDGenerator) saveSequence() error {
	state := sequenceState{Sequence: g.sequence}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal sequence state: %w", err)
	}

	// Write atomically using temp file + rename
	tempFile := g.stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempFile, g.stateFile); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// ValidateMessageID checks if a message ID has the correct format
//
// Valid format: {unix_ms}-{sender}-{seq}
// Example: 1738612345678-sender-001
func ValidateMessageID(messageID string) bool {
	// Basic length check (minimum: 13 digits + 1 dash + 1 char + 1 dash + 3 digits = 19 chars)
	if len(messageID) < 19 {
		return false
	}

	// Quick check: must have at least 2 dashes
	dashCount := 0
	for _, ch := range messageID {
		if ch == '-' {
			dashCount++
		}
	}

	return dashCount >= 2
}
