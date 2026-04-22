package enforcement

import (
	"testing"
)

func TestMatcherMatch(t *testing.T) {
	m, err := NewMatcher(`\bcd\s+`)
	if err != nil {
		t.Fatal(err)
	}

	if !m.Match("cd /repo") {
		t.Error("expected match for 'cd /repo'")
	}
	if m.Match("abcd") {
		t.Error("expected no match for 'abcd'")
	}
}

func TestMatcherFindMatch(t *testing.T) {
	m, err := NewMatcher(`\b(cat|rm)\b`)
	if err != nil {
		t.Fatal(err)
	}

	result := m.FindMatch("rm file.txt")
	if !result.Matched {
		t.Fatal("expected match")
	}
	if result.MatchText != "rm" {
		t.Errorf("expected 'rm', got %q", result.MatchText)
	}
	if result.StartIndex != 0 {
		t.Errorf("expected start 0, got %d", result.StartIndex)
	}

	result = m.FindMatch("git status")
	if result.Matched {
		t.Error("expected no match")
	}
}

func TestMatcherFindAllMatches(t *testing.T) {
	m, err := NewMatcher(`\b\d+\b`)
	if err != nil {
		t.Fatal(err)
	}

	matches := m.FindAllMatches("port 8080 and 3000")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].MatchText != "8080" {
		t.Errorf("expected '8080', got %q", matches[0].MatchText)
	}
	if matches[1].MatchText != "3000" {
		t.Errorf("expected '3000', got %q", matches[1].MatchText)
	}
}

func TestQuickMatch(t *testing.T) {
	ok, err := QuickMatch(`\btest\b`, "go test ./...")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected match")
	}

	ok, err = QuickMatch(`\btest\b`, "go build")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected no match")
	}
}

func TestMatcherReplaceAll(t *testing.T) {
	m, err := NewMatcher(`foo`)
	if err != nil {
		t.Fatal(err)
	}

	result := m.ReplaceAll("foo bar foo", "baz")
	if result != "baz bar baz" {
		t.Errorf("expected 'baz bar baz', got %q", result)
	}
}

func TestMatcherSplit(t *testing.T) {
	m, err := NewMatcher(`,\s*`)
	if err != nil {
		t.Fatal(err)
	}

	parts := m.Split("a, b, c", -1)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
}

func TestMatcherExtractGroups(t *testing.T) {
	m, err := NewMatcher(`(?P<cmd>\w+)\s+(?P<arg>\S+)`)
	if err != nil {
		t.Fatal(err)
	}

	groups := m.ExtractGroups("git status")
	if groups["cmd"] != "git" {
		t.Errorf("expected cmd='git', got %q", groups["cmd"])
	}
	if groups["arg"] != "status" {
		t.Errorf("expected arg='status', got %q", groups["arg"])
	}
}

func TestNewMatcherInvalidRegex(t *testing.T) {
	_, err := NewMatcher(`(?=lookahead)`)
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}
