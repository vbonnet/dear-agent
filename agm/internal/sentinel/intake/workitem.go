package intake

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SchemaVersion is the current work item schema version.
const SchemaVersion = 1

// Priority levels.
const (
	PriorityP0 = "P0"
	PriorityP1 = "P1"
	PriorityP2 = "P2"
	PriorityP3 = "P3"
)

// Scope levels.
const (
	ScopeXS = "XS"
	ScopeS  = "S"
	ScopeM  = "M"
	ScopeL  = "L"
)

// Status values representing the work item lifecycle.
const (
	StatusPending    = "pending"
	StatusApproved   = "approved"
	StatusClaimed    = "claimed"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusRejected   = "rejected"
)

// WorkItem represents a single work item from queue.jsonl.
type WorkItem struct {
	ID                 string             `json:"id"`
	Version            int                `json:"version"`
	CreatedAt          string             `json:"created_at"`
	Source             WorkItemSource     `json:"source"`
	Title              string             `json:"title"`
	Description        string             `json:"description"`
	Priority           string             `json:"priority"`
	Scope              string             `json:"scope"`
	Status             string             `json:"status"`
	AcceptanceCriteria []string           `json:"acceptance_criteria"`
	Evidence           WorkItemEvidence   `json:"evidence"`
	Guardrails         WorkItemGuardrails `json:"guardrails"`
	Execution          WorkItemExecution  `json:"execution"`
}

// WorkItemSource describes the origin of a work item.
type WorkItemSource struct {
	Stream       string `json:"stream"`
	Trigger      string `json:"trigger"`
	AgentSession string `json:"agent_session"`
}

// WorkItemEvidence contains validation data for a work item.
type WorkItemEvidence struct {
	ToolOutput       string   `json:"tool_output,omitempty"`
	IncidentCount    int      `json:"incident_count,omitempty"`
	AffectedSessions []string `json:"affected_sessions,omitempty"`
	RetroReferences  []string `json:"retro_references,omitempty"`
	PatternID        string   `json:"pattern_id,omitempty"`
}

// WorkItemGuardrails defines safety constraints for execution.
type WorkItemGuardrails struct {
	MaxScope              string   `json:"max_scope"`
	RequiresHumanApproval bool     `json:"requires_human_approval"`
	TestGateRequired      bool     `json:"test_gate_required"`
	FilesInScope          []string `json:"files_in_scope,omitempty"`
}

// WorkItemExecution tracks active/completed work.
type WorkItemExecution struct {
	AssignedSession string   `json:"assigned_session"`
	StartedAt       string   `json:"started_at"`
	CompletedAt     string   `json:"completed_at"`
	CommitSHAs      []string `json:"commit_shas"`
}

var validPriorities = map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
var validScopes = map[string]bool{"XS": true, "S": true, "M": true, "L": true}
var validStatuses = map[string]bool{
	StatusPending: true, StatusApproved: true, StatusClaimed: true,
	StatusInProgress: true, StatusCompleted: true, StatusRejected: true,
}

// ParseWorkItem parses a single JSON line into a WorkItem.
func ParseWorkItem(data []byte) (*WorkItem, error) {
	var item WorkItem
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("failed to parse work item: %w", err)
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("invalid work item: %w", err)
	}
	return &item, nil
}

// ParseWorkItems parses a JSONL byte slice into a slice of WorkItems.
func ParseWorkItems(data []byte) ([]*WorkItem, error) {
	var items []*WorkItem
	for _, line := range splitLines(data) {
		trimmed := strings.TrimSpace(string(line))
		if len(trimmed) == 0 {
			continue
		}
		item, err := ParseWorkItem([]byte(trimmed))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// MarshalJSONL marshals a WorkItem to a single JSON line.
func (w *WorkItem) MarshalJSONL() ([]byte, error) {
	return json.Marshal(w)
}

// Validate checks required fields and value constraints.
func (w *WorkItem) Validate() error {
	if w.ID == "" {
		return fmt.Errorf("id is required")
	}
	if w.Version < 1 {
		return fmt.Errorf("version must be >= 1")
	}
	if w.CreatedAt == "" {
		return fmt.Errorf("created_at is required")
	}
	if _, err := time.Parse(time.RFC3339, w.CreatedAt); err != nil {
		return fmt.Errorf("created_at must be RFC3339: %w", err)
	}
	if w.Title == "" {
		return fmt.Errorf("title is required")
	}
	if !validPriorities[w.Priority] {
		return fmt.Errorf("invalid priority: %s", w.Priority)
	}
	if !validScopes[w.Scope] {
		return fmt.Errorf("invalid scope: %s", w.Scope)
	}
	if !validStatuses[w.Status] {
		return fmt.Errorf("invalid status: %s", w.Status)
	}
	return nil
}

// splitLines splits JSONL data on newlines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
