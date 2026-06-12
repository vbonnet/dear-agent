package main

import (
	"testing"
)

func TestContainsWholeToken_Exact(t *testing.T) {
	t.Parallel()
	if !containsWholeToken("fix ce-abc cleanup", "ce-abc") {
		t.Error("exact match should return true")
	}
}

func TestContainsWholeToken_Substring_NoMatch(t *testing.T) {
	t.Parallel()
	if containsWholeToken("fix ce-abcdef cleanup", "ce-abc") {
		t.Error("substring match should return false (word boundary not met)")
	}
}

func TestContainsWholeToken_AtStart(t *testing.T) {
	t.Parallel()
	if !containsWholeToken("ce-abc: title", "ce-abc") {
		t.Error("token at start should match")
	}
}

func TestContainsWholeToken_AtEnd(t *testing.T) {
	t.Parallel()
	if !containsWholeToken("closes ce-abc", "ce-abc") {
		t.Error("token at end should match")
	}
}

func TestCheckBeadDedup_EmptyBeadID(t *testing.T) {
	t.Parallel()
	prs := []openPR{{Number: 1, Title: "fix ce-xyz"}}
	if claims := checkBeadDedup(prs, ""); claims != nil {
		t.Errorf("empty beadID should return nil, got %v", claims)
	}
}

func TestCheckBeadDedup_Found(t *testing.T) {
	t.Parallel()
	prs := []openPR{
		{Number: 10, Title: "fix ce-abc: something", HeadRefName: "fix/ce-abc"},
		{Number: 11, Title: "unrelated", HeadRefName: "fix/other"},
	}
	claims := checkBeadDedup(prs, "ce-abc")
	if len(claims) != 1 || claims[0].Number != 10 {
		t.Errorf("expected [10], got %v", claims)
	}
}

func TestCheckBeadDedup_NotFound(t *testing.T) {
	t.Parallel()
	prs := []openPR{
		{Number: 10, Title: "fix ce-xyz", HeadRefName: "fix/ce-xyz"},
	}
	if claims := checkBeadDedup(prs, "ce-abc"); len(claims) != 0 {
		t.Errorf("expected no claims, got %v", claims)
	}
}

func TestCheckBeadDedup_CaseInsensitive(t *testing.T) {
	t.Parallel()
	prs := []openPR{
		{Number: 5, Title: "Fix CE-ABC things"},
	}
	claims := checkBeadDedup(prs, "ce-abc")
	if len(claims) != 1 {
		t.Errorf("expected case-insensitive match, got %v", claims)
	}
}

func TestCheckBeadDedup_InBody(t *testing.T) {
	t.Parallel()
	prs := []openPR{
		{Number: 7, Title: "misc fix", HeadRefName: "misc", Body: "Closes ce-abc"},
	}
	claims := checkBeadDedup(prs, "ce-abc")
	if len(claims) != 1 {
		t.Errorf("expected body match, got %v", claims)
	}
}
