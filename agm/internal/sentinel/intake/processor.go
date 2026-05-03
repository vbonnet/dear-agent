// Package intake provides intake-related functionality.
package intake

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// validTransitions defines the allowed status state machine.
var validTransitions = map[string]map[string]bool{
	StatusPending:    {StatusApproved: true, StatusRejected: true},
	StatusApproved:   {StatusClaimed: true, StatusRejected: true},
	StatusClaimed:    {StatusInProgress: true, StatusRejected: true},
	StatusInProgress: {StatusCompleted: true, StatusRejected: true},
}

// IsValidTransition checks if moving from oldStatus to newStatus is allowed.
func IsValidTransition(oldStatus, newStatus string) bool {
	allowed, ok := validTransitions[oldStatus]
	if !ok {
		return false
	}
	return allowed[newStatus]
}

// WorkItemProcessor handles status transitions and writes updates back to queue.jsonl.
type WorkItemProcessor struct {
	filePath      string
	logger        *slog.Logger
	mu            sync.Mutex
	nowFunc       func() time.Time
	readFileFunc  func(string) ([]byte, error)
	writeFileFunc func(string, []byte, os.FileMode) error
}

// WorkItemProcessorConfig configures the processor.
type WorkItemProcessorConfig struct {
	FilePath string
	Logger   *slog.Logger
}

// NewWorkItemProcessor creates a new processor.
func NewWorkItemProcessor(cfg WorkItemProcessorConfig) *WorkItemProcessor {
	if cfg.FilePath == "" {
		home, _ := os.UserHomeDir()
		cfg.FilePath = filepath.Join(home, ".agm", "intake", "queue.jsonl")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &WorkItemProcessor{
		filePath:      cfg.FilePath,
		logger:        cfg.Logger,
		nowFunc:       time.Now,
		readFileFunc:  os.ReadFile,
		writeFileFunc: os.WriteFile,
	}
}

// TransitionStatus changes a work item's status and rewrites queue.jsonl.
func (p *WorkItemProcessor) TransitionStatus(itemID, newStatus string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	items, err := p.readItems()
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	found := false
	for _, item := range items {
		if item.ID == itemID {
			found = true
			if !IsValidTransition(item.Status, newStatus) {
				return fmt.Errorf("invalid transition: %s -> %s for item %s",
					item.Status, newStatus, itemID)
			}
			item.Status = newStatus
			p.applyTransitionSideEffects(item, newStatus)
			p.logger.Info("Status transition", "id", itemID, "to", newStatus)
			break
		}
	}

	if !found {
		return fmt.Errorf("work item not found: %s", itemID)
	}

	return p.writeItems(items)
}

// applyTransitionSideEffects sets execution timestamps on relevant transitions.
func (p *WorkItemProcessor) applyTransitionSideEffects(item *WorkItem, newStatus string) {
	now := p.nowFunc().Format(time.RFC3339)
	switch newStatus {
	case StatusInProgress:
		item.Execution.StartedAt = now
	case StatusCompleted, StatusRejected:
		item.Execution.CompletedAt = now
	}
}

// ClaimItem transitions to claimed and sets assigned_session.
func (p *WorkItemProcessor) ClaimItem(itemID, sessionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	items, err := p.readItems()
	if err != nil {
		return fmt.Errorf("failed to read items: %w", err)
	}

	for _, item := range items {
		if item.ID == itemID {
			if !IsValidTransition(item.Status, StatusClaimed) {
				return fmt.Errorf("invalid transition: %s -> claimed for item %s",
					item.Status, itemID)
			}
			item.Status = StatusClaimed
			item.Execution.AssignedSession = sessionID
			p.logger.Info("Item claimed", "id", itemID, "session", sessionID)
			return p.writeItems(items)
		}
	}
	return fmt.Errorf("work item not found: %s", itemID)
}

// AppendItem appends a new work item to queue.jsonl.
func (p *WorkItemProcessor) AppendItem(item *WorkItem) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := item.Validate(); err != nil {
		return fmt.Errorf("invalid work item: %w", err)
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("failed to marshal work item: %w", err)
	}

	dir := filepath.Dir(p.filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.OpenFile(p.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open queue.jsonl: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write item: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}
	return f.Sync()
}

// GetItem returns the item with the given ID.
func (p *WorkItemProcessor) GetItem(itemID string) (*WorkItem, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	items, err := p.readItems()
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == itemID {
			return item, nil
		}
	}
	return nil, fmt.Errorf("work item not found: %s", itemID)
}

func (p *WorkItemProcessor) readItems() ([]*WorkItem, error) {
	data, err := p.readFileFunc(p.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*WorkItem{}, nil
		}
		return nil, err
	}
	return ParseWorkItems(data)
}

func (p *WorkItemProcessor) writeItems(items []*WorkItem) error {
	var buf []byte
	for _, item := range items {
		line, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("failed to marshal item %s: %w", item.ID, err)
		}
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	return p.writeFileFunc(p.filePath, buf, 0600)
}
