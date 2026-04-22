package git

import (
	"strings"
	"testing"
)

func TestGetBranchAbbrev(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   string
	}{
		{"long branch", "feature/implement-auto-bead", "-bead"},
		{"short branch", "main", "main"},
		{"exact 5 chars", "devel", "devel"},
		{"6 chars", "master", "aster"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBranchAbbrev(tt.branch)
			if got != tt.want {
				t.Errorf("GetBranchAbbrev(%q) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

func TestFormatFileList_Empty(t *testing.T) {
	files := []string{}
	result := FormatFileList(files, 20)

	if result != "(none)" {
		t.Errorf("FormatFileList([]) = %q, want %q", result, "(none)")
	}
}

func TestFormatFileList_BelowMax(t *testing.T) {
	files := []string{"file1.go", "file2.go", "file3.go"}
	result := FormatFileList(files, 20)

	expected := "- file1.go\n- file2.go\n- file3.go\n"
	if result != expected {
		t.Errorf("FormatFileList() = %q, want %q", result, expected)
	}
}

func TestFormatFileList_Truncation(t *testing.T) {
	files := make([]string, 25)
	for i := range files {
		files[i] = "file.go"
	}

	result := FormatFileList(files, 20)

	// Should show first 20 files + footer
	expectedLines := 20 + 2 // 20 files + blank line + footer
	actualLines := len(strings.Split(result, "\n"))

	if actualLines != expectedLines {
		t.Errorf("Expected %d lines, got %d", expectedLines, actualLines)
	}

	if !strings.Contains(result, "... and 5 more file(s)") {
		t.Error("Missing truncation footer")
	}
}

func TestExtractState_GitNotAvailable(t *testing.T) {
	// This test may not work reliably in CI, skip if git is available
	if isGitAvailable() {
		t.Skip("Git is available, skipping unavailable test")
	}

	state, err := ExtractState()

	if err == nil {
		t.Error("Expected error when git unavailable")
	}

	// Should return default values
	if state.Branch != "unknown" {
		t.Errorf("Expected branch 'unknown', got %q", state.Branch)
	}
}

func TestIsGitAvailable(t *testing.T) {
	// This test checks if git is in PATH
	available := isGitAvailable()

	// We expect git to be available in most environments
	// This test is more for coverage than assertion
	_ = available
}

func TestContains(t *testing.T) {
	slice := []string{"foo", "bar", "baz"}

	tests := []struct {
		item string
		want bool
	}{
		{"foo", true},
		{"bar", true},
		{"baz", true},
		{"qux", false},
		{"", false},
	}

	for _, tt := range tests {
		got := contains(slice, tt.item)
		if got != tt.want {
			t.Errorf("contains(%v, %q) = %v, want %v", slice, tt.item, got, tt.want)
		}
	}
}
