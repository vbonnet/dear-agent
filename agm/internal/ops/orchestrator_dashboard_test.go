package ops

import (
	"testing"
	"time"
)

func TestExtractTrustSummary_Empty(t *testing.T) {
	lb := &TrustLeaderboardResult{Entries: []TrustLeaderboardEntry{}}
	summary := extractTrustSummary(lb)

	if summary.Total != 0 {
		t.Errorf("expected Total=0, got %d", summary.Total)
	}
	if len(summary.Top) != 0 {
		t.Errorf("expected Top to be empty, got %d entries", len(summary.Top))
	}
	if len(summary.Bottom) != 0 {
		t.Errorf("expected Bottom to be empty, got %d entries", len(summary.Bottom))
	}
}

func TestExtractTrustSummary_SingleEntry(t *testing.T) {
	entries := []TrustLeaderboardEntry{
		{SessionName: "session-1", Score: 75, TotalEvents: 10},
	}
	lb := &TrustLeaderboardResult{Entries: entries}
	summary := extractTrustSummary(lb)

	if summary.Total != 1 {
		t.Errorf("expected Total=1, got %d", summary.Total)
	}
	if len(summary.Top) != 1 {
		t.Errorf("expected Top to have 1 entry, got %d", len(summary.Top))
	}
	// Bottom is empty because with only 1 entry, it only goes to Top
	if len(summary.Bottom) != 0 {
		t.Errorf("expected Bottom to be empty with single entry, got %d", len(summary.Bottom))
	}
	if summary.Top[0].SessionName != "session-1" {
		t.Errorf("expected Top[0] to be session-1, got %s", summary.Top[0].SessionName)
	}
}

func TestExtractTrustSummary_TenEntries(t *testing.T) {
	var entries []TrustLeaderboardEntry
	for i := 1; i <= 10; i++ {
		entries = append(entries, TrustLeaderboardEntry{
			SessionName: "session-" + string(rune('0'+i)),
			Score:       100 - i*5,
			TotalEvents: i,
		})
	}
	lb := &TrustLeaderboardResult{Entries: entries}
	summary := extractTrustSummary(lb)

	if summary.Total != 10 {
		t.Errorf("expected Total=10, got %d", summary.Total)
	}
	if len(summary.Top) != 5 {
		t.Errorf("expected Top to have 5 entries, got %d", len(summary.Top))
	}
	if len(summary.Bottom) != 5 {
		t.Errorf("expected Bottom to have 5 entries, got %d", len(summary.Bottom))
	}

	// Top should be sorted descending (highest scores first)
	if summary.Top[0].Score < summary.Top[len(summary.Top)-1].Score {
		t.Error("Top entries should be sorted by descending score")
	}
}

func TestExtractBacklogSummary_Empty(t *testing.T) {
	backlog := &TaskListResult{Tasks: []Task{}, Total: 0}
	summary := extractBacklogSummary(backlog)

	if summary.Total != 0 {
		t.Errorf("expected Total=0, got %d", summary.Total)
	}
	if len(summary.Next) != 0 {
		t.Errorf("expected Next to be empty, got %d tasks", len(summary.Next))
	}
}

func TestExtractBacklogSummary_SkipsDoneTasks(t *testing.T) {
	now := time.Now()
	tasks := []Task{
		{ID: "1", Status: "done", Description: "task 1"},
		{ID: "2", Status: "queued", Description: "task 2"},
		{ID: "3", Status: "in-progress", Description: "task 3"},
		{ID: "4", Status: "done", Description: "task 4"},
	}
	backlog := &TaskListResult{Tasks: tasks, Total: 4}
	summary := extractBacklogSummary(backlog)

	if summary.Total != 4 {
		t.Errorf("expected Total=4, got %d", summary.Total)
	}
	if len(summary.Next) != 2 {
		t.Errorf("expected Next to have 2 non-done tasks, got %d", len(summary.Next))
	}

	// Check that no "done" tasks are in Next
	for _, task := range summary.Next {
		if task.Status == "done" {
			t.Errorf("Next should not contain done tasks, got %s", task.ID)
		}
	}

	_ = now // suppress unused warning
}

func TestExtractBacklogSummary_LimitThree(t *testing.T) {
	tasks := []Task{
		{ID: "1", Status: "queued", Description: "task 1"},
		{ID: "2", Status: "queued", Description: "task 2"},
		{ID: "3", Status: "queued", Description: "task 3"},
		{ID: "4", Status: "queued", Description: "task 4"},
		{ID: "5", Status: "queued", Description: "task 5"},
	}
	backlog := &TaskListResult{Tasks: tasks, Total: 5}
	summary := extractBacklogSummary(backlog)

	if len(summary.Next) != 3 {
		t.Errorf("expected Next to have max 3 tasks, got %d", len(summary.Next))
	}
}

func TestOrchestratorDashboardRequest_DefaultWindow(t *testing.T) {
	req := &OrchestratorDashboardRequest{}
	if req.Window != 0 {
		t.Errorf("expected Window to be zero-value, got %v", req.Window)
	}
}
