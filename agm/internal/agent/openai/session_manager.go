package openai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	agmlock "github.com/vbonnet/dear-agent/agm/internal/lock"
)

const sessionLockPollInterval = 25 * time.Millisecond

// SessionManager manages OpenAI conversation sessions with JSONL storage.
type SessionManager struct {
	baseDir  string
	mu       sync.RWMutex
	sessions map[string]*SessionInfo
}

// SessionInfo contains metadata and conversation history for a session.
type SessionInfo struct {
	ID               string                `json:"id"`
	Title            string                `json:"title,omitempty"`
	Model            string                `json:"model"`
	WorkingDirectory string                `json:"working_directory"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	MessageCount     int                   `json:"message_count"`
	RuntimeConfig    *SessionRuntimeConfig `json:"runtime_config,omitempty"`

	// In-memory message cache (not persisted in metadata)
	messages []Message
}

// SessionRuntimeConfig is the non-secret client configuration required to
// deliver future messages with the same semantics as the adapter that created
// the session. API credentials are deliberately excluded and must be supplied
// at runtime.
type SessionRuntimeConfig struct {
	Temperature     float32 `json:"temperature"`
	MaxTokens       int     `json:"max_tokens"`
	BaseURL         string  `json:"base_url,omitempty"`
	IsAzure         bool    `json:"is_azure,omitempty"`
	AzureAPIVersion string  `json:"azure_api_version,omitempty"`
}

// NewSessionManager creates a new session manager.
// If baseDir is empty, defaults to ~/.agm/openai-sessions/
func NewSessionManager(baseDir string) (*SessionManager, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".agm", "openai-sessions")
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	sm := &SessionManager{
		baseDir:  baseDir,
		sessions: make(map[string]*SessionInfo),
	}

	// Load existing sessions
	if err := sm.loadAllSessions(); err != nil {
		return nil, err
	}

	return sm, nil
}

// CreateSession creates a new conversation session.
// If model is empty, it will be read from OPENAI_MODEL environment variable
// or default to gpt-4-turbo-preview.
func (sm *SessionManager) CreateSession(id, model, workingDir string) (*SessionInfo, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.sessions[id]; exists {
		return nil, fmt.Errorf("session %s already exists", id)
	}

	// Set model from environment variable if not provided
	if model == "" {
		model = os.Getenv("OPENAI_MODEL")
		if model == "" {
			model = "gpt-4-turbo-preview"
		}
	}

	// Validate model
	if err := ValidateModel(model); err != nil {
		return nil, fmt.Errorf("invalid model: %w", err)
	}

	now := time.Now()
	info := &SessionInfo{
		ID:               id,
		Model:            model,
		WorkingDirectory: workingDir,
		CreatedAt:        now,
		UpdatedAt:        now,
		MessageCount:     0,
		messages:         []Message{},
	}

	sm.sessions[id] = info

	// Create session directory
	sessionDir := sm.getSessionDir(id)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	// Save metadata
	if err := sm.saveMetadata(id); err != nil {
		return nil, err
	}

	return info, nil
}

// GetSession retrieves session information.
func (sm *SessionManager) GetSession(id string) (*SessionInfo, error) {
	return sm.GetSessionContext(context.Background(), id)
}

// GetSessionContext is the request-aware form of GetSession. It lets adapter
// reconstruction and readiness stop waiting for a contended session lock when
// their owning CLI or MCP request is canceled.
func (sm *SessionManager) GetSessionContext(ctx context.Context, id string) (*SessionInfo, error) {
	var info *SessionInfo
	err := sm.WithSessionLockContext(ctx, id, func() error {
		var err error
		info, err = sm.GetSessionUnderLock(id)
		return err
	})
	return info, err
}

// GetSessionUnderLock reloads authoritative metadata while the caller holds
// the store-scoped session lock across a larger transaction.
func (sm *SessionManager) GetSessionUnderLock(id string) (*SessionInfo, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	info, err := sm.loadMetadataFromFile(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			delete(sm.sessions, id)
			return nil, fmt.Errorf("session %s not found: %w", id, err)
		}
		return nil, err
	}
	if cached := sm.sessions[id]; cached != nil {
		info.messages = append([]Message(nil), cached.messages...)
	}
	sm.sessions[id] = info
	return info, nil
}

// ListSessions returns all session IDs.
func (sm *SessionManager) ListSessions() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	ids := make([]string, 0, len(sm.sessions))
	for id := range sm.sessions {
		ids = append(ids, id)
	}

	return ids
}

// AddMessage appends a message to the session's conversation history.
func (sm *SessionManager) AddMessage(sessionID string, msg Message) error {
	return sm.AddMessages(sessionID, msg)
}

// AddMessages atomically replaces the on-disk history with the prior history
// plus all supplied messages. Callers use this to commit a completed
// user/assistant turn without exposing a provisional user message.
func (sm *SessionManager) AddMessages(sessionID string, messages ...Message) error {
	return sm.WithSessionLock(sessionID, func() error {
		return sm.AddMessagesUnderLock(sessionID, messages...)
	})
}

// AddMessagesUnderLock commits messages while the caller holds the
// store-scoped session lock across a larger transaction such as provider work.
func (sm *SessionManager) AddMessagesUnderLock(sessionID string, messages ...Message) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.sessions[sessionID]; !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}
	if len(messages) == 0 {
		return nil
	}

	info, err := sm.loadMetadataFromFile(sessionID)
	if err != nil {
		return err
	}
	currentMessages, err := sm.loadMessagesFromFile(sessionID)
	if err != nil {
		return err
	}
	committedMessages := append([]Message(nil), currentMessages...)
	for _, message := range messages {
		if message.Timestamp.IsZero() {
			message.Timestamp = time.Now()
		}
		committedMessages = append(committedMessages, message)
	}

	if err := sm.writeMessagesToFile(sessionID, committedMessages); err != nil {
		return err
	}

	info.messages = append([]Message(nil), committedMessages...)
	info.MessageCount = len(committedMessages)
	info.UpdatedAt = time.Now()
	if err := sm.writeMetadata(sessionID, info); err != nil {
		rollbackErr := sm.writeMessagesToFile(sessionID, currentMessages)
		return errors.Join(err, rollbackErr)
	}
	sm.sessions[sessionID] = info

	return nil
}

// ClearMessages atomically empties one session history while preserving every
// metadata field, including the non-secret runtime configuration required for
// later adapter reconstruction.
func (sm *SessionManager) ClearMessages(sessionID string) error {
	return sm.WithSessionLock(sessionID, func() error {
		return sm.ClearMessagesUnderLock(sessionID)
	})
}

// ClearMessagesUnderLock clears history while the caller holds the
// store-scoped lock across a larger adapter command transaction.
func (sm *SessionManager) ClearMessagesUnderLock(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.sessions[sessionID]; !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}
	// The cross-process session lock held by the adapter makes the on-disk
	// metadata authoritative here. A long-lived manager cache may predate
	// title, directory, or runtime-config changes from another process.
	info, err := sm.loadMetadataFromFile(sessionID)
	if err != nil {
		return err
	}
	currentMessages, err := sm.loadMessagesFromFile(sessionID)
	if err != nil {
		return err
	}
	info.messages = append([]Message(nil), currentMessages...)
	if err := sm.writeMessagesToFile(sessionID, nil); err != nil {
		return err
	}

	info.messages = nil
	info.MessageCount = 0
	info.UpdatedAt = time.Now()
	if err := sm.writeMetadata(sessionID, info); err != nil {
		rollbackErr := sm.writeMessagesToFile(sessionID, currentMessages)
		return errors.Join(err, rollbackErr)
	}
	sm.sessions[sessionID] = info
	return nil
}

// GetMessages retrieves all messages for a session.
func (sm *SessionManager) GetMessages(sessionID string) ([]Message, error) {
	var messages []Message
	err := sm.WithSessionLock(sessionID, func() error {
		var err error
		messages, err = sm.GetMessagesUnderLock(sessionID)
		return err
	})
	return messages, err
}

// GetMessagesUnderLock reloads authoritative metadata and history while the
// caller holds the store-scoped session lock across a larger transaction.
func (sm *SessionManager) GetMessagesUnderLock(sessionID string) ([]Message, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	info, err := sm.loadMetadataFromFile(sessionID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			delete(sm.sessions, sessionID)
			return nil, fmt.Errorf("session %s not found: %w", sessionID, err)
		}
		return nil, err
	}

	messages, err := sm.loadMessagesFromFile(sessionID)
	if err != nil {
		return nil, err
	}
	info.messages = append([]Message(nil), messages...)
	info.MessageCount = len(messages)
	sm.sessions[sessionID] = info
	return append([]Message(nil), messages...), nil
}

// WithSessionLock serializes one OpenAI session across adapter instances and
// processes for the complete provider completion and history commit.
func (sm *SessionManager) WithSessionLock(sessionID string, fn func() error) error {
	return sm.WithSessionLockContext(context.Background(), sessionID, fn)
}

// WithSessionLockContext is the request-aware form of WithSessionLock. It
// polls the cross-process file lock so cancellation can stop a contended wait
// without leaving a goroutine that later acquires and abandons the lock.
func (sm *SessionManager) WithSessionLockContext(ctx context.Context, sessionID string, fn func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(sessionID))
	lockPath := filepath.Join(sm.baseDir, ".locks", fmt.Sprintf("%x.lock", digest[:16]))
	sessionLock, err := agmlock.New(lockPath)
	if err != nil {
		return fmt.Errorf("create OpenAI session lock: %w", err)
	}
	for {
		err = sessionLock.TryLock()
		if err == nil {
			break
		}
		var lockErr *agmlock.LockError
		if !errors.As(err, &lockErr) {
			_ = sessionLock.Unlock()
			return fmt.Errorf("acquire OpenAI session lock: %w", err)
		}
		timer := time.NewTimer(sessionLockPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = sessionLock.Unlock()
			return ctx.Err()
		case <-timer.C:
		}
	}
	// Unlock/close cannot safely turn a successfully persisted provider turn
	// into a retryable send failure. The OS releases this advisory lock when the
	// descriptor closes or the process exits.
	defer func() { _ = sessionLock.Unlock() }()
	return fn()
}

// UpdateTitle updates the session title.
func (sm *SessionManager) UpdateTitle(sessionID, title string) error {
	return sm.updateMetadata(sessionID, func(info *SessionInfo) {
		info.Title = title
	})
}

// UpdateWorkingDirectory updates the session's working directory.
func (sm *SessionManager) UpdateWorkingDirectory(sessionID, workingDir string) error {
	return sm.updateMetadata(sessionID, func(info *SessionInfo) {
		info.WorkingDirectory = workingDir
	})
}

// UpdateRuntimeConfig persists the non-secret client settings used by a
// session so another AGM process can reconstruct its delivery adapter.
func (sm *SessionManager) UpdateRuntimeConfig(sessionID string, runtimeConfig SessionRuntimeConfig) error {
	return sm.updateMetadata(sessionID, func(info *SessionInfo) {
		info.RuntimeConfig = &runtimeConfig
	})
}

// updateMetadata serializes every metadata writer with completed-turn and
// history-clear transactions, then applies one field update to the current
// on-disk snapshot so independent managers cannot clobber each other.
func (sm *SessionManager) updateMetadata(sessionID string, update func(*SessionInfo)) error {
	return sm.WithSessionLock(sessionID, func() error {
		sm.mu.Lock()
		defer sm.mu.Unlock()

		cached, exists := sm.sessions[sessionID]
		if !exists {
			return fmt.Errorf("session %s not found", sessionID)
		}
		info, err := sm.loadMetadataFromFile(sessionID)
		if err != nil {
			return err
		}
		info.messages = append([]Message(nil), cached.messages...)
		update(info)
		info.UpdatedAt = time.Now()
		if err := sm.writeMetadata(sessionID, info); err != nil {
			return err
		}
		sm.sessions[sessionID] = info
		return nil
	})
}

// DeleteSession removes a session and its data.
func (sm *SessionManager) DeleteSession(sessionID string) error {
	return sm.WithSessionLock(sessionID, func() error {
		return sm.DeleteSessionUnderLock(sessionID)
	})
}

// DeleteSessionUnderLock removes a session while the caller holds the
// store-scoped lock shared with provider completion and history persistence.
func (sm *SessionManager) DeleteSessionUnderLock(sessionID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.sessions[sessionID]; !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Remove session directory
	sessionDir := sm.getSessionDir(sessionID)
	if err := os.RemoveAll(sessionDir); err != nil {
		return err
	}
	delete(sm.sessions, sessionID)
	return nil
}

// getSessionDir returns the directory path for a session.
func (sm *SessionManager) getSessionDir(sessionID string) string {
	return filepath.Join(sm.baseDir, sessionID)
}

// getMetadataPath returns the metadata file path for a session.
func (sm *SessionManager) getMetadataPath(sessionID string) string {
	return filepath.Join(sm.getSessionDir(sessionID), "metadata.json")
}

// getMessagesPath returns the messages file path for a session.
func (sm *SessionManager) getMessagesPath(sessionID string) string {
	return filepath.Join(sm.getSessionDir(sessionID), "messages.jsonl")
}

// saveMetadata writes session metadata to disk.
func (sm *SessionManager) saveMetadata(sessionID string) error {
	info, exists := sm.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}
	return sm.writeMetadata(sessionID, info)
}

func (sm *SessionManager) writeMetadata(sessionID string, info *SessionInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	metadataPath := sm.getMetadataPath(sessionID)
	tempPath := metadataPath + ".tmp"

	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	if err := os.Rename(tempPath, metadataPath); err != nil {
		return fmt.Errorf("failed to replace metadata file: %w", err)
	}

	return nil
}

// loadMetadataFromFile reads the current cross-process metadata snapshot.
// Callers must hold sm.mu and, for read-modify-write transactions, the
// store-scoped session lock.
func (sm *SessionManager) loadMetadataFromFile(sessionID string) (*SessionInfo, error) {
	data, err := os.ReadFile(sm.getMetadataPath(sessionID))
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata for session %s: %w", sessionID, err)
	}
	var info SessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse metadata for session %s: %w", sessionID, err)
	}
	return &info, nil
}

// writeMessagesToFile atomically replaces the session's JSONL history.
func (sm *SessionManager) writeMessagesToFile(sessionID string, messages []Message) error {
	messagesPath := sm.getMessagesPath(sessionID)
	var buffer bytes.Buffer
	for _, message := range messages {
		data, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
		buffer.Write(data)
		buffer.WriteByte('\n')
	}

	tempPath := messagesPath + ".tmp"
	if err := os.WriteFile(tempPath, buffer.Bytes(), 0o600); err != nil {
		return fmt.Errorf("failed to write messages: %w", err)
	}
	if err := os.Rename(tempPath, messagesPath); err != nil {
		return fmt.Errorf("failed to replace messages: %w", err)
	}
	return nil
}

// loadMessagesFromFile loads messages from the JSONL file.
func (sm *SessionManager) loadMessagesFromFile(sessionID string) ([]Message, error) {
	messagesPath := sm.getMessagesPath(sessionID)

	file, err := os.Open(messagesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Message{}, nil
		}
		return nil, fmt.Errorf("failed to open messages file: %w", err)
	}
	defer file.Close()

	var messages []Message
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return nil, fmt.Errorf("failed to read messages file: %w", readErr)
			}
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("failed to parse message: %w", err)
		}

		messages = append(messages, msg)
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("failed to read messages file: %w", readErr)
		}
	}

	return messages, nil
}

// loadAllSessions loads all existing sessions from disk.
func (sm *SessionManager) loadAllSessions() error {
	entries, err := os.ReadDir(sm.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read sessions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		metadataPath := sm.getMetadataPath(sessionID)

		data, err := os.ReadFile(metadataPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // Skip sessions without metadata
			}
			return fmt.Errorf("failed to read metadata for session %s: %w", sessionID, err)
		}

		var info SessionInfo
		if err := json.Unmarshal(data, &info); err != nil {
			return fmt.Errorf("failed to parse metadata for session %s: %w", sessionID, err)
		}

		// Initialize empty messages slice (will be loaded on demand)
		info.messages = []Message{}

		sm.sessions[sessionID] = &info
	}

	return nil
}
