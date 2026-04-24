package ops

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestSearchSessions_ExactMatch(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-project", "/project"),
		newManifest("id-2", "other-thing", "/project"),
	}
	ctx := testCtx(sessions, "my-project", "other-thing")

	result, err := SearchSessions(ctx, &SearchSessionsRequest{
		Query:  "my-project",
		Status: "all",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalMatches != 1 {
		t.Errorf("expected 1 match, got %d", result.TotalMatches)
	}
	if result.Matches[0].RelevanceScore != 1.0 {
		t.Errorf("exact match should have score 1.0, got %f", result.Matches[0].RelevanceScore)
	}
}

func TestSearchSessions_PrefixMatch(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "my-project-alpha", "/project"),
		newManifest("id-2", "my-project-beta", "/project"),
		newManifest("id-3", "other-thing", "/project"),
	}
	ctx := testCtx(sessions)

	result, err := SearchSessions(ctx, &SearchSessionsRequest{
		Query:  "my-project",
		Status: "all",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalMatches != 2 {
		t.Errorf("expected 2 matches, got %d", result.TotalMatches)
	}
}

func TestSearchSessions_CaseInsensitive(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "My-Project", "/project"),
	}
	ctx := testCtx(sessions)

	result, err := SearchSessions(ctx, &SearchSessionsRequest{
		Query:  "my-project",
		Status: "all",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalMatches != 1 {
		t.Errorf("expected 1 match, got %d", result.TotalMatches)
	}
}

func TestSearchSessions_EmptyQuery(t *testing.T) {
	ctx := testCtx(nil)
	_, err := SearchSessions(ctx, &SearchSessionsRequest{Query: ""})
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchSessions_LimitValidation(t *testing.T) {
	ctx := testCtx(nil)
	_, err := SearchSessions(ctx, &SearchSessionsRequest{
		Query: "test",
		Limit: 51,
	})
	if err == nil {
		t.Fatal("expected error for limit > 50")
	}
}

func TestSearchSessions_SortedByRelevance(t *testing.T) {
	sessions := []*manifest.Manifest{
		newManifest("id-1", "contains-api-thing", "/project"),
		newManifest("id-2", "api", "/project"),
		newManifest("id-3", "api-first", "/project"),
	}
	ctx := testCtx(sessions)

	result, err := SearchSessions(ctx, &SearchSessionsRequest{
		Query:  "api",
		Status: "all",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalMatches != 3 {
		t.Errorf("expected 3 matches, got %d", result.TotalMatches)
	}
	// Exact match first, then prefix, then contains
	if result.Matches[0].Name != "api" {
		t.Errorf("expected exact match first, got %s", result.Matches[0].Name)
	}
	if result.Matches[1].Name != "api-first" {
		t.Errorf("expected prefix match second, got %s", result.Matches[1].Name)
	}
}
