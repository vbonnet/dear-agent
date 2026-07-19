package ops

import (
	"testing"
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

func TestOrchestratorDashboardRequest_DefaultWindow(t *testing.T) {
	req := &OrchestratorDashboardRequest{}
	if req.Window != 0 {
		t.Errorf("expected Window to be zero-value, got %v", req.Window)
	}
}
