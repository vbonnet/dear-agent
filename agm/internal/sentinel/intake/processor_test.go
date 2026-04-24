package intake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestQueue(t *testing.T, path string, items ...*WorkItem) {
	t.Helper()
	var data []byte
	for _, item := range items {
		line, err := json.Marshal(item)
		require.NoError(t, err)
		data = append(data, line...)
		data = append(data, '\n')
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0750))
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func readTestQueue(t *testing.T, path string) []*WorkItem {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	items, err := ParseWorkItems(data)
	require.NoError(t, err)
	return items
}

func TestIsValidTransition_AllValid(t *testing.T) {
	valid := []struct{ from, to string }{
		{"pending", "approved"},
		{"pending", "rejected"},
		{"approved", "claimed"},
		{"approved", "rejected"},
		{"claimed", "in_progress"},
		{"claimed", "rejected"},
		{"in_progress", "completed"},
		{"in_progress", "rejected"},
	}
	for _, tt := range valid {
		assert.True(t, IsValidTransition(tt.from, tt.to),
			"expected valid: %s -> %s", tt.from, tt.to)
	}
}

func TestIsValidTransition_Invalid(t *testing.T) {
	invalid := []struct{ from, to string }{
		{"pending", "completed"},
		{"pending", "in_progress"},
		{"pending", "claimed"},
		{"completed", "pending"},
		{"rejected", "pending"},
		{"completed", "in_progress"},
		{"in_progress", "approved"},
	}
	for _, tt := range invalid {
		assert.False(t, IsValidTransition(tt.from, tt.to),
			"expected invalid: %s -> %s", tt.from, tt.to)
	}
}

func TestProcessor_TransitionStatus_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	writeTestQueue(t, queuePath, item)

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})

	err := p.TransitionStatus("intake-20260328-001", "approved")
	require.NoError(t, err)

	items := readTestQueue(t, queuePath)
	require.Len(t, items, 1)
	assert.Equal(t, "approved", items[0].Status)
}

func TestProcessor_TransitionStatus_Invalid(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	writeTestQueue(t, queuePath, item)

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})

	err := p.TransitionStatus("intake-20260328-001", "completed")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid transition: pending -> completed")

	items := readTestQueue(t, queuePath)
	assert.Equal(t, "pending", items[0].Status)
}

func TestProcessor_TransitionStatus_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	writeTestQueue(t, queuePath, item)

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})

	err := p.TransitionStatus("nonexistent-id", "approved")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "work item not found")
}

func TestProcessor_TransitionSideEffects_StartedAt(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	item.Status = "claimed"
	writeTestQueue(t, queuePath, item)

	fixedTime := time.Date(2026, 3, 29, 14, 0, 0, 0, time.UTC)
	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})
	p.nowFunc = func() time.Time { return fixedTime }

	require.NoError(t, p.TransitionStatus("intake-20260328-001", "in_progress"))

	items := readTestQueue(t, queuePath)
	assert.Equal(t, "2026-03-29T14:00:00Z", items[0].Execution.StartedAt)
}

func TestProcessor_TransitionSideEffects_CompletedAt(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	item.Status = "in_progress"
	item.Execution.StartedAt = "2026-03-29T14:00:00Z"
	writeTestQueue(t, queuePath, item)

	fixedTime := time.Date(2026, 3, 29, 15, 30, 0, 0, time.UTC)
	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})
	p.nowFunc = func() time.Time { return fixedTime }

	require.NoError(t, p.TransitionStatus("intake-20260328-001", "completed"))

	items := readTestQueue(t, queuePath)
	assert.Equal(t, "2026-03-29T15:30:00Z", items[0].Execution.CompletedAt)
}

func TestProcessor_AppendItem(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "intake", "queue.jsonl")

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})

	item := validTestItem()
	require.NoError(t, p.AppendItem(item))

	items := readTestQueue(t, queuePath)
	require.Len(t, items, 1)
	assert.Equal(t, "intake-20260328-001", items[0].ID)
}

func TestProcessor_AppendItem_Invalid(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})

	item := validTestItem()
	item.ID = ""
	err := p.AppendItem(item)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid work item")
}

func TestProcessor_ClaimItem(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	item.Status = "approved"
	writeTestQueue(t, queuePath, item)

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})

	require.NoError(t, p.ClaimItem("intake-20260328-001", "agent-session-42"))

	items := readTestQueue(t, queuePath)
	assert.Equal(t, "claimed", items[0].Status)
	assert.Equal(t, "agent-session-42", items[0].Execution.AssignedSession)
}

func TestProcessor_GetItem(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	item := validTestItem()
	writeTestQueue(t, queuePath, item)

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})

	got, err := p.GetItem("intake-20260328-001")
	require.NoError(t, err)
	assert.Equal(t, "Fix flaky test", got.Title)
}

func TestProcessor_FullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queue.jsonl")

	p := NewWorkItemProcessor(WorkItemProcessorConfig{FilePath: queuePath})
	p.nowFunc = func() time.Time {
		return time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	}

	item := validTestItem()
	require.NoError(t, p.AppendItem(item))

	require.NoError(t, p.TransitionStatus(item.ID, "approved"))
	require.NoError(t, p.TransitionStatus(item.ID, "claimed"))
	require.NoError(t, p.TransitionStatus(item.ID, "in_progress"))
	require.NoError(t, p.TransitionStatus(item.ID, "completed"))

	got, err := p.GetItem(item.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.NotEmpty(t, got.Execution.StartedAt)
	assert.NotEmpty(t, got.Execution.CompletedAt)
}
