package ecphory

import (
	"testing"
)

// TestApplyTriggerBoosting verifies that engrams with active triggers get boosted.
func TestApplyTriggerBoosting(t *testing.T) {
	ranked := []RankingResult{
		{Path: "/engrams/a.ai.md", Relevance: 50.0},
		{Path: "/engrams/b.ai.md", Relevance: 40.0},
		{Path: "/engrams/c.ai.md", Relevance: 30.0},
	}

	triggerPaths := map[string]bool{
		"/engrams/a.ai.md": true,
		"/engrams/c.ai.md": true,
	}

	applyTriggerBoosting(ranked, triggerPaths)

	// a should be boosted: 50 + 20 = 70
	if ranked[0].Relevance != 70.0 {
		t.Errorf("ranked[0].Relevance = %f, want 70.0 (50.0 + 20.0 boost)", ranked[0].Relevance)
	}

	// b is NOT in triggerPaths, should stay at 40
	if ranked[1].Relevance != 40.0 {
		t.Errorf("ranked[1].Relevance = %f, want 40.0 (no boost)", ranked[1].Relevance)
	}

	// c should be boosted: 30 + 20 = 50
	if ranked[2].Relevance != 50.0 {
		t.Errorf("ranked[2].Relevance = %f, want 50.0 (30.0 + 20.0 boost)", ranked[2].Relevance)
	}
}

// TestApplyTriggerBoostingEmpty verifies no-op when triggerPaths is nil or empty.
func TestApplyTriggerBoostingEmpty(t *testing.T) {
	ranked := []RankingResult{
		{Path: "/engrams/a.ai.md", Relevance: 50.0},
		{Path: "/engrams/b.ai.md", Relevance: 40.0},
	}

	// Test with nil map
	applyTriggerBoosting(ranked, nil)

	if ranked[0].Relevance != 50.0 {
		t.Errorf("nil triggerPaths: ranked[0].Relevance = %f, want 50.0", ranked[0].Relevance)
	}
	if ranked[1].Relevance != 40.0 {
		t.Errorf("nil triggerPaths: ranked[1].Relevance = %f, want 40.0", ranked[1].Relevance)
	}

	// Test with empty map
	applyTriggerBoosting(ranked, map[string]bool{})

	if ranked[0].Relevance != 50.0 {
		t.Errorf("empty triggerPaths: ranked[0].Relevance = %f, want 50.0", ranked[0].Relevance)
	}
	if ranked[1].Relevance != 40.0 {
		t.Errorf("empty triggerPaths: ranked[1].Relevance = %f, want 40.0", ranked[1].Relevance)
	}
}

// TestApplyTriggerBoostingCap verifies that relevance is capped at 100.0.
func TestApplyTriggerBoostingCap(t *testing.T) {
	ranked := []RankingResult{
		{Path: "/engrams/high.ai.md", Relevance: 90.0},
		{Path: "/engrams/exact.ai.md", Relevance: 80.0},
		{Path: "/engrams/under.ai.md", Relevance: 70.0},
	}

	triggerPaths := map[string]bool{
		"/engrams/high.ai.md":  true,
		"/engrams/exact.ai.md": true,
		"/engrams/under.ai.md": true,
	}

	applyTriggerBoosting(ranked, triggerPaths)

	// 90 + 20 = 110 -> capped at 100
	if ranked[0].Relevance != 100.0 {
		t.Errorf("ranked[0].Relevance = %f, want 100.0 (capped from 110.0)", ranked[0].Relevance)
	}

	// 80 + 20 = 100 -> exactly at cap
	if ranked[1].Relevance != 100.0 {
		t.Errorf("ranked[1].Relevance = %f, want 100.0 (80.0 + 20.0)", ranked[1].Relevance)
	}

	// 70 + 20 = 90 -> under cap
	if ranked[2].Relevance != 90.0 {
		t.Errorf("ranked[2].Relevance = %f, want 90.0 (70.0 + 20.0)", ranked[2].Relevance)
	}
}
