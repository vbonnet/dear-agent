package simple

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

const (
	workingContextFilename = "context.json"
	sessionHistoryFilename = "history.json"
)

// GetWorkingContext retrieves the persisted working context for a session.
func (p *SimpleFileProvider) GetWorkingContext(ctx context.Context, sessionID string) (*consolidation.WorkingContext, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	path, err := p.sessionFilePath(sessionID, workingContextFilename)
	if err != nil {
		return nil, fmt.Errorf("get working context: %w", err)
	}
	var workingContext consolidation.WorkingContext
	if err := readSessionJSON(path, &workingContext); err != nil {
		return nil, fmt.Errorf("get working context: %w", err)
	}
	return &workingContext, nil
}

// UpdateWorkingContext applies a partial update and persists it atomically per provider instance.
func (p *SimpleFileProvider) UpdateWorkingContext(ctx context.Context, sessionID string, updates consolidation.ContextUpdate) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	path, err := p.sessionFilePath(sessionID, workingContextFilename)
	if err != nil {
		return fmt.Errorf("update working context: %w", err)
	}
	workingContext := consolidation.WorkingContext{SessionID: sessionID}
	if err := readSessionJSON(path, &workingContext); err != nil && !errorsIsNotFound(err) {
		return fmt.Errorf("update working context: %w", err)
	}
	applyContextUpdate(&workingContext, updates)
	if err := writeSessionJSON(path, &workingContext); err != nil {
		return fmt.Errorf("update working context: %w", err)
	}
	return nil
}

// GetSessionHistory retrieves the append-only history for a session.
func (p *SimpleFileProvider) GetSessionHistory(ctx context.Context, sessionID string) (*consolidation.SessionHistory, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	path, err := p.sessionFilePath(sessionID, sessionHistoryFilename)
	if err != nil {
		return nil, fmt.Errorf("get session history: %w", err)
	}
	var history consolidation.SessionHistory
	if err := readSessionJSON(path, &history); err != nil {
		return nil, fmt.Errorf("get session history: %w", err)
	}
	return &history, nil
}

// AppendSessionEvent appends one event to the session history.
func (p *SimpleFileProvider) AppendSessionEvent(ctx context.Context, sessionID string, event consolidation.SessionEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	path, err := p.sessionFilePath(sessionID, sessionHistoryFilename)
	if err != nil {
		return fmt.Errorf("append session event: %w", err)
	}
	history := consolidation.SessionHistory{SessionID: sessionID}
	if err := readSessionJSON(path, &history); err != nil && !errorsIsNotFound(err) {
		return fmt.Errorf("append session event: %w", err)
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if history.StartTime.IsZero() {
		history.StartTime = event.Timestamp
	}
	history.Events = append(history.Events, event)
	if err := writeSessionJSON(path, &history); err != nil {
		return fmt.Errorf("append session event: %w", err)
	}
	return nil
}

// PersistSession marks a session complete and publishes its lifecycle event.
func (p *SimpleFileProvider) PersistSession(ctx context.Context, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	path, err := p.sessionFilePath(sessionID, sessionHistoryFilename)
	if err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	var history consolidation.SessionHistory
	if err := readSessionJSON(path, &history); err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	completedAt := time.Now()
	history.EndTime = &completedAt
	if err := writeSessionJSON(path, &history); err != nil {
		return fmt.Errorf("persist session: %w", err)
	}
	consolidation.PublishMemoryEvent(ctx, consolidation.GetEventBus(ctx), consolidation.TopicSessionPersisted, map[string]any{
		"provider":   "simple",
		"session_id": sessionID,
	})
	return nil
}

func (p *SimpleFileProvider) sessionFilePath(sessionID, filename string) (string, error) {
	if err := validateMemoryID(sessionID); err != nil {
		return "", err
	}
	return filepath.Join(p.storagePath, "_sessions", sessionID, filename), nil
}

func readSessionJSON(path string, destination any) error {
	// #nosec G304 -- callers pass paths from sessionFilePath, which validates the session ID before joining under storagePath/_sessions.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return consolidation.ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, destination)
}

func writeSessionJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".session-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, consolidation.ErrNotFound)
}

func applyContextUpdate(workingContext *consolidation.WorkingContext, updates consolidation.ContextUpdate) {
	if updates.SetPhase != nil {
		workingContext.CurrentPhase = *updates.SetPhase
	}
	workingContext.ActiveTasks = append(workingContext.ActiveTasks, updates.AddTasks...)
	for i := range workingContext.ActiveTasks {
		if slices.Contains(updates.CompleteTasks, workingContext.ActiveTasks[i].ID) {
			workingContext.ActiveTasks[i].Status = "completed"
		}
	}
	for _, memoryID := range updates.PinMemories {
		for _, memory := range workingContext.RelevantMemory {
			if memory.ID == memoryID && !containsMemory(workingContext.PinnedItems, memoryID) {
				workingContext.PinnedItems = append(workingContext.PinnedItems, memory)
			}
		}
	}
	for _, memoryID := range updates.UnpinMemories {
		workingContext.PinnedItems = slices.DeleteFunc(workingContext.PinnedItems, func(memory consolidation.Memory) bool {
			return memory.ID == memoryID
		})
	}
}

func containsMemory(memories []consolidation.Memory, memoryID string) bool {
	return slices.ContainsFunc(memories, func(memory consolidation.Memory) bool {
		return memory.ID == memoryID
	})
}
