package intake

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validTestItem() *WorkItem {
	return &WorkItem{
		ID:                 "intake-20260328-001",
		Version:            1,
		CreatedAt:          "2026-03-28T10:00:00Z",
		Source:             WorkItemSource{Stream: "errors", Trigger: "threshold", AgentSession: "sess-1"},
		Title:              "Fix flaky test",
		Description:        "Test X fails intermittently",
		Priority:           "P1",
		Scope:              "S",
		Status:             "pending",
		AcceptanceCriteria: []string{"test passes 10x in a row"},
		Evidence:           WorkItemEvidence{PatternID: "err-001", IncidentCount: 5},
		Guardrails:         WorkItemGuardrails{MaxScope: "S", RequiresHumanApproval: true, TestGateRequired: true},
		Execution:          WorkItemExecution{},
	}
}

func TestParseWorkItem_Valid(t *testing.T) {
	item := validTestItem()
	data, err := json.Marshal(item)
	require.NoError(t, err)

	parsed, err := ParseWorkItem(data)
	require.NoError(t, err)
	assert.Equal(t, "intake-20260328-001", parsed.ID)
	assert.Equal(t, 1, parsed.Version)
	assert.Equal(t, "P1", parsed.Priority)
	assert.Equal(t, "pending", parsed.Status)
	assert.Equal(t, "errors", parsed.Source.Stream)
	assert.Equal(t, "err-001", parsed.Evidence.PatternID)
	assert.True(t, parsed.Guardrails.RequiresHumanApproval)
}

func TestParseWorkItem_InvalidJSON(t *testing.T) {
	_, err := ParseWorkItem([]byte(`{not json}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse work item")
}

func TestParseWorkItem_MissingID(t *testing.T) {
	item := validTestItem()
	item.ID = ""
	data, _ := json.Marshal(item)

	_, err := ParseWorkItem(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id is required")
}

func TestParseWorkItem_InvalidPriority(t *testing.T) {
	item := validTestItem()
	item.Priority = "P5"
	data, _ := json.Marshal(item)

	_, err := ParseWorkItem(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid priority")
}

func TestParseWorkItem_InvalidScope(t *testing.T) {
	item := validTestItem()
	item.Scope = "XXL"
	data, _ := json.Marshal(item)

	_, err := ParseWorkItem(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid scope")
}

func TestParseWorkItem_InvalidStatus(t *testing.T) {
	item := validTestItem()
	item.Status = "unknown"
	data, _ := json.Marshal(item)

	_, err := ParseWorkItem(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status")
}

func TestParseWorkItem_InvalidCreatedAt(t *testing.T) {
	item := validTestItem()
	item.CreatedAt = "not-a-date"
	data, _ := json.Marshal(item)

	_, err := ParseWorkItem(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "created_at must be RFC3339")
}

func TestParseWorkItems_JSONL(t *testing.T) {
	item1 := validTestItem()
	item2 := validTestItem()
	item2.ID = "intake-20260328-002"
	item2.Title = "Second item"

	line1, _ := json.Marshal(item1)
	line2, _ := json.Marshal(item2)
	var data []byte
	data = append(data, line1...)
	data = append(data, '\n')
	data = append(data, line2...)
	data = append(data, '\n')

	items, err := ParseWorkItems(data)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "intake-20260328-001", items[0].ID)
	assert.Equal(t, "intake-20260328-002", items[1].ID)
}

func TestParseWorkItems_EmptyLines(t *testing.T) {
	item := validTestItem()
	line, _ := json.Marshal(item)
	data := append([]byte("\n"), line...)
	data = append(data, []byte("\n\n")...)

	items, err := ParseWorkItems(data)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestWorkItem_MarshalJSONL_Roundtrip(t *testing.T) {
	item := validTestItem()
	data, err := item.MarshalJSONL()
	require.NoError(t, err)

	parsed, err := ParseWorkItem(data)
	require.NoError(t, err)
	assert.Equal(t, item.ID, parsed.ID)
	assert.Equal(t, item.Title, parsed.Title)
	assert.Equal(t, item.AcceptanceCriteria, parsed.AcceptanceCriteria)
}

func TestValidate_VersionZero(t *testing.T) {
	item := validTestItem()
	item.Version = 0
	assert.Error(t, item.Validate())
}

func TestValidate_MissingTitle(t *testing.T) {
	item := validTestItem()
	item.Title = ""
	assert.Error(t, item.Validate())
}

func TestValidate_MissingCreatedAt(t *testing.T) {
	item := validTestItem()
	item.CreatedAt = ""
	assert.Error(t, item.Validate())
}
