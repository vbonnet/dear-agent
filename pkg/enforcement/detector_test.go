package enforcement

import (
	"testing"
)

func newTestDB() *PatternDatabase {
	return &PatternDatabase{
		Patterns: []Pattern{
			{
				ID:          "cd-command",
				Order:       20,
				RE2Regex:    `\bcd\s+`,
				Regex:       `\bcd\s+`,
				PatternName: "cd command",
				Remediation: "Use absolute paths",
				Reason:      "Using cd command",
				Alternative: "Use -C flag",
				Severity:    "high",
			},
			{
				ID:          "command-chaining",
				Order:       30,
				RE2Regex:    `&&`,
				Regex:       `&&`,
				PatternName: "command chaining (&&)",
				Remediation: "Run separately",
				Reason:      "Command chaining",
				Alternative: "Make separate tool calls",
				Severity:    "high",
			},
			{
				ID:          "file-operations",
				Order:       200,
				RE2Regex:    `\b(cat|cp|mv|rm|touch)\b`,
				Regex:       `\b(cat|cp|mv|rm|touch)\b`,
				PatternName: "file operations",
				Remediation: "Use Read/Write/Edit tools",
				Reason:      "Using shell file operations",
				Alternative: "Use Read/Write/Edit tools",
				Severity:    "high",
			},
			{
				ID:               "consolidated-pattern",
				Regex:            `\bfoo\b`,
				ConsolidatedInto: "file-operations",
				Reason:           "consolidated",
			},
			{
				ID:      "relaxed-pattern",
				Regex:   `\bbar\b`,
				Relaxed: true,
				Reason:  "relaxed",
			},
		},
	}
}

func TestDetect(t *testing.T) {
	db := newTestDB()
	d, err := NewDetector(db)
	if err != nil {
		t.Fatal(err)
	}

	p := d.Detect("cd /repo")
	if p == nil {
		t.Fatal("expected violation for 'cd /repo'")
	}
	if p.ID != "cd-command" {
		t.Errorf("expected pattern 'cd-command', got %q", p.ID)
	}

	p = d.Detect("git status")
	if p != nil {
		t.Errorf("expected no violation for 'git status', got %q", p.ID)
	}
}

func TestDetectAll(t *testing.T) {
	db := newTestDB()
	d, err := NewDetector(db)
	if err != nil {
		t.Fatal(err)
	}

	violations := d.DetectAll("cd /repo && cat file.txt")
	if len(violations) != 3 {
		t.Fatalf("expected 3 violations, got %d", len(violations))
	}
}

func TestDetectWithContext(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{
				ID:           "worktree-only",
				Regex:        `\bdangerous\b`,
				Reason:       "dangerous in worktrees",
				ContextCheck: "has_worktrees",
			},
		},
	}

	d, err := NewDetector(db)
	if err != nil {
		t.Fatal(err)
	}

	// Should not match without worktrees
	p := d.DetectWithContext("dangerous command", Context{HasWorktrees: false})
	if p != nil {
		t.Error("expected no violation without worktrees")
	}

	// Should match with worktrees
	p = d.DetectWithContext("dangerous command", Context{HasWorktrees: true})
	if p == nil {
		t.Error("expected violation with worktrees")
	}
}

func TestNewDetectorRE2(t *testing.T) {
	db := newTestDB()
	d, err := NewDetectorRE2(db)
	if err != nil {
		t.Fatal(err)
	}

	// Patterns with RE2Regex should be compiled
	p := d.Detect("cd /repo")
	if p == nil {
		t.Fatal("expected violation for 'cd /repo'")
	}

	// consolidated-pattern has no RE2Regex, should not be compiled
	p = d.Detect("foo")
	if p != nil {
		t.Error("expected no violation for 'foo' with RE2 detector")
	}
}

func TestValidateCommand(t *testing.T) {
	db := newTestDB()
	d, err := NewDetectorRE2(db)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		cmd     string
		wantID  string
		wantNil bool
	}{
		{"cd /repo", "cd-command", false},
		{"git add . && git commit", "command-chaining", false},
		{"cat file.txt", "file-operations", false},
		{"git -C /repo status", "", true},
		{"go test ./...", "", true},
	}

	for _, tt := range tests {
		p := d.ValidateCommand(tt.cmd)
		if tt.wantNil {
			if p != nil {
				t.Errorf("ValidateCommand(%q): expected nil, got %q", tt.cmd, p.ID)
			}
		} else {
			if p == nil {
				t.Errorf("ValidateCommand(%q): expected %q, got nil", tt.cmd, tt.wantID)
			} else if p.ID != tt.wantID {
				t.Errorf("ValidateCommand(%q): expected %q, got %q", tt.cmd, tt.wantID, p.ID)
			}
		}
	}
}

func TestValidateCommandOrderMatters(t *testing.T) {
	// cd command (order 20) should match before command-chaining (order 30)
	db := newTestDB()
	d, err := NewDetectorRE2(db)
	if err != nil {
		t.Fatal(err)
	}

	p := d.ValidateCommand("cd /repo && make build")
	if p == nil {
		t.Fatal("expected violation")
	}
	if p.ID != "cd-command" {
		t.Errorf("expected cd-command (order 20) to match first, got %q", p.ID)
	}
}

func TestSkippedPatterns(t *testing.T) {
	db := &PatternDatabase{
		Patterns: []Pattern{
			{ID: "bad-regex", Regex: `(?=lookahead)`, Reason: "unsupported"},
			{ID: "good-regex", Regex: `\btest\b`, Reason: "supported"},
		},
	}

	d, err := NewDetector(db)
	if err != nil {
		t.Fatal(err)
	}

	skipped := d.GetSkippedPatterns()
	if len(skipped) != 1 {
		t.Fatalf("expected 1 skipped pattern, got %d", len(skipped))
	}
	if skipped[0] != "bad-regex" {
		t.Errorf("expected skipped pattern 'bad-regex', got %q", skipped[0])
	}
}
