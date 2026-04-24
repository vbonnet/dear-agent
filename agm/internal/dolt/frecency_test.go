package dolt

import (
	"testing"
	"time"
)

func TestFrecencyScore_Brackets(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		accessCount int
		hoursAgo    float64
		wantScore   float64
	}{
		{"recent (<1h) x10", 10, 0.5, 40.0},         // 10 * 4.0
		{"today (<24h) x10", 10, 12, 20.0},          // 10 * 2.0
		{"this week (<7d) x10", 10, 72, 10.0},       // 10 * 1.0
		{"this month (<30d) x10", 10, 24 * 15, 5.0}, // 10 * 0.5
		{"old (>30d) x10", 10, 24 * 60, 2.5},        // 10 * 0.25
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastAccess := now.Add(-time.Duration(tt.hoursAgo * float64(time.Hour)))
			got := FrecencyScore(tt.accessCount, &lastAccess, now)
			if got != tt.wantScore {
				t.Errorf("FrecencyScore(%d, %v ago) = %f, want %f",
					tt.accessCount, time.Duration(tt.hoursAgo*float64(time.Hour)), got, tt.wantScore)
			}
		})
	}
}

func TestFrecencyScore_ZeroAndNil(t *testing.T) {
	now := time.Now()
	lastAccess := now.Add(-time.Minute)

	if score := FrecencyScore(0, &lastAccess, now); score != 0 {
		t.Errorf("FrecencyScore(0, non-nil) = %f, want 0", score)
	}

	if score := FrecencyScore(5, nil, now); score != 0 {
		t.Errorf("FrecencyScore(5, nil) = %f, want 0", score)
	}
}

func TestFrecencyScore_Decay(t *testing.T) {
	now := time.Now()

	recentAccess := now.Add(-30 * time.Minute) // < 1h bracket
	oldAccess := now.Add(-24 * 45 * time.Hour) // > 30d bracket

	recentScore := FrecencyScore(5, &recentAccess, now) // 5 * 4.0 = 20
	oldScore := FrecencyScore(5, &oldAccess, now)       // 5 * 0.25 = 1.25

	if recentScore <= oldScore {
		t.Errorf("recent score (%f) should be > old score (%f)", recentScore, oldScore)
	}

	// Exact values
	if recentScore != 20.0 {
		t.Errorf("recent score = %f, want 20.0", recentScore)
	}
	if oldScore != 1.25 {
		t.Errorf("old score = %f, want 1.25", oldScore)
	}
}

func TestMockAdapter_UpdateAccess(t *testing.T) {
	mock := NewMockAdapter()

	s := NewTestManifest("sess-1", "test-session")
	if err := mock.CreateSession(s); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// First access
	if err := mock.UpdateAccess("sess-1"); err != nil {
		t.Fatalf("UpdateAccess: %v", err)
	}

	// Second access
	if err := mock.UpdateAccess("sess-1"); err != nil {
		t.Fatalf("UpdateAccess: %v", err)
	}

	// Verify via GetByFrecency
	results, err := mock.GetByFrecency(0)
	if err != nil {
		t.Fatalf("GetByFrecency: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// access_count=2, last_accessed_at is recent (<1h), so score = 2 * 4.0 = 8.0
	if results[0].Score != 8.0 {
		t.Errorf("score = %f, want 8.0", results[0].Score)
	}

	// Error on non-existent session
	if err := mock.UpdateAccess("nonexistent"); err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestMockAdapter_GetByFrecency_Ranking(t *testing.T) {
	mock := NewMockAdapter()

	// Create 3 sessions
	for _, id := range []string{"low", "high", "mid"} {
		s := NewTestManifest(id, id+"-session")
		if err := mock.CreateSession(s); err != nil {
			t.Fatalf("CreateSession(%s): %v", id, err)
		}
	}

	// "high" gets 10 accesses
	for i := 0; i < 10; i++ {
		if err := mock.UpdateAccess("high"); err != nil {
			t.Fatalf("UpdateAccess(high): %v", err)
		}
	}

	// "mid" gets 3 accesses
	for i := 0; i < 3; i++ {
		if err := mock.UpdateAccess("mid"); err != nil {
			t.Fatalf("UpdateAccess(mid): %v", err)
		}
	}

	// "low" gets 0 accesses (score = 0)

	results, err := mock.GetByFrecency(0)
	if err != nil {
		t.Fatalf("GetByFrecency: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Verify ordering: high > mid > low
	if results[0].Session.SessionID != "high" {
		t.Errorf("rank 1: got %s, want high", results[0].Session.SessionID)
	}
	if results[1].Session.SessionID != "mid" {
		t.Errorf("rank 2: got %s, want mid", results[1].Session.SessionID)
	}
	if results[2].Session.SessionID != "low" {
		t.Errorf("rank 3: got %s, want low", results[2].Session.SessionID)
	}

	// Verify scores (all recent, weight=4.0)
	if results[0].Score != 40.0 { // 10 * 4.0
		t.Errorf("high score = %f, want 40.0", results[0].Score)
	}
	if results[1].Score != 12.0 { // 3 * 4.0
		t.Errorf("mid score = %f, want 12.0", results[1].Score)
	}
	if results[2].Score != 0.0 { // 0 accesses
		t.Errorf("low score = %f, want 0.0", results[2].Score)
	}
}

func TestMockAdapter_GetByFrecency_ExcludesArchived(t *testing.T) {
	mock := NewMockAdapter()

	active := NewTestManifest("active-1", "active")
	if err := mock.CreateSession(active); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := mock.UpdateAccess("active-1"); err != nil {
		t.Fatalf("UpdateAccess: %v", err)
	}

	archived := NewTestManifest("archived-1", "archived")
	archived.Lifecycle = "archived"
	if err := mock.CreateSession(archived); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := mock.UpdateAccess("archived-1"); err != nil {
		t.Fatalf("UpdateAccess: %v", err)
	}

	results, err := mock.GetByFrecency(0)
	if err != nil {
		t.Fatalf("GetByFrecency: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result (archived excluded), got %d", len(results))
	}

	if results[0].Session.SessionID != "active-1" {
		t.Errorf("expected active-1, got %s", results[0].Session.SessionID)
	}
}

func TestMockAdapter_GetByFrecency_Limit(t *testing.T) {
	mock := NewMockAdapter()

	for i := 0; i < 5; i++ {
		s := NewTestManifest("s"+string(rune('a'+i)), "session")
		if err := mock.CreateSession(s); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
	}

	results, err := mock.GetByFrecency(2)
	if err != nil {
		t.Fatalf("GetByFrecency: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(results))
	}
}

func TestRoundScore(t *testing.T) {
	if got := RoundScore(3.14159, 2); got != 3.14 {
		t.Errorf("RoundScore(3.14159, 2) = %f, want 3.14", got)
	}
	if got := RoundScore(2.5, 0); got != 3.0 {
		t.Errorf("RoundScore(2.5, 0) = %f, want 3.0", got)
	}
}
