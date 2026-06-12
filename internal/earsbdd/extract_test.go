package earsbdd

import (
	"strings"
	"testing"
)

func TestExtract(t *testing.T) {
	input := `# Test SPEC

## Requirements

**FSG-01** When a path falls under ~/worktrees/, the system shall allow the write.

**FSG-02** While the agent is in read-only mode, the system shall reject all writes.

The system shall not write outside the declared writable set.

Some prose that contains the word shall in a non-requirement context without a bold ID.
`
	reqs, err := Extract("test.md", strings.NewReader(input))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(reqs) != 4 {
		t.Fatalf("want 4 requirements, got %d: %v", len(reqs), reqs)
	}

	tests := []struct {
		wantID        string
		wantCondition string
		wantAction    string
	}{
		{"FSG-01", "When a path falls under ~/worktrees/", "allow the write"},
		{"FSG-02", "While the agent is in read-only mode", "reject all writes"},
		{"", "The system", "not write outside the declared writable set"},
		{"", "", ""},
	}
	for i, tt := range tests {
		r := reqs[i]
		if r.ID != tt.wantID {
			t.Errorf("req[%d] ID: want %q, got %q", i, tt.wantID, r.ID)
		}
		if tt.wantCondition != "" && r.Condition != tt.wantCondition {
			t.Errorf("req[%d] Condition: want %q, got %q", i, tt.wantCondition, r.Condition)
		}
		if tt.wantAction != "" && r.Action != tt.wantAction {
			t.Errorf("req[%d] Action: want %q, got %q", i, tt.wantAction, r.Action)
		}
	}
}

func TestExtract_SkipsFencedCode(t *testing.T) {
	input := "```\nThe system shall not appear here.\n```\n\n**A-01** The system shall appear here.\n"
	reqs, err := Extract("test.md", strings.NewReader(input))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 requirement (fenced skipped), got %d", len(reqs))
	}
	if reqs[0].ID != "A-01" {
		t.Errorf("want ID A-01, got %q", reqs[0].ID)
	}
}

func TestExtract_Empty(t *testing.T) {
	reqs, err := Extract("empty.md", strings.NewReader("# No requirements here\n\nJust prose.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 0 {
		t.Fatalf("want 0 requirements, got %d", len(reqs))
	}
}

func TestSplitShall(t *testing.T) {
	tests := []struct {
		input         string
		wantCondition string
		wantAction    string
	}{
		{
			"When a path falls under ~/worktrees/, the system shall allow the write.",
			"When a path falls under ~/worktrees/",
			"allow the write",
		},
		{
			"While the agent is running, the write-guard shall reject the write.",
			"While the agent is running",
			"reject the write",
		},
		{
			"The system shall not write outside the declared writable set.",
			"The system",
			"not write outside the declared writable set",
		},
		{
			"No requirement keyword found here.",
			"No requirement keyword found here.",
			"",
		},
	}
	for _, tt := range tests {
		cond, act := splitShall(tt.input)
		if cond != tt.wantCondition {
			t.Errorf("splitShall(%q) condition: want %q, got %q", tt.input, tt.wantCondition, cond)
		}
		if act != tt.wantAction {
			t.Errorf("splitShall(%q) action: want %q, got %q", tt.input, tt.wantAction, act)
		}
	}
}

func TestGenerate_Smoke(t *testing.T) {
	reqs := []Requirement{
		{
			ID:        "FSG-01",
			FullText:  "When a path falls under ~/worktrees/, the system shall allow the write.",
			Condition: "When a path falls under ~/worktrees/",
			Action:    "allow the write",
			File:      "internal/fsguard/SPEC.md",
			Line:      10,
		},
	}
	ff := Generate("internal/fsguard/SPEC.md", reqs)
	if ff.Content == "" {
		t.Fatal("Generate returned empty content")
	}
	for _, want := range []string{
		"Feature:",
		"@ears",
		"@FSG-01",
		"Scenario:",
		"Given the system is configured",
		"When a path falls under ~/worktrees/",
		"Then the system shall allow the write",
	} {
		if !strings.Contains(ff.Content, want) {
			t.Errorf("Generate output missing %q\n---\n%s", want, ff.Content)
		}
	}
}

func TestGenerate_Empty(t *testing.T) {
	ff := Generate("internal/ci/SPEC.md", nil)
	if ff.Content != "" {
		t.Errorf("Generate with nil reqs returned non-empty content: %q", ff.Content)
	}
	if ff.FeatureName == "" {
		t.Error("Generate with nil reqs returned empty FeatureName")
	}
}

func TestLcFirst(t *testing.T) {
	tests := []struct{ in, want string }{
		{"When X happens", "X happens"},
		{"While running", "running"},
		{"Where enabled", "enabled"},
		{"If condition", "condition"},
		{"The system shall", "The system shall"},
		{"", ""},
	}
	for _, tt := range tests {
		got := lcFirst(tt.in)
		if got != tt.want {
			t.Errorf("lcFirst(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
