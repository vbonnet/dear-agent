package ops

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckHygiene_EmptyPath(t *testing.T) {
	_, err := CheckHygiene("")
	if err == nil {
		t.Fatal("expected error for empty packagePath")
	}
}

func TestCheckHygiene_NotADirectory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file.go")
	os.WriteFile(f, []byte("package main"), 0o644)

	_, err := CheckHygiene(f)
	if err == nil {
		t.Fatal("expected error for non-directory path")
	}
}

func TestCheckHygiene_NonexistentPath(t *testing.T) {
	_, err := CheckHygiene("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestCheckHygiene_CleanPackage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/clean\n\ngo 1.21\n")
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	report, err := CheckHygiene(dir)
	if err != nil {
		t.Fatalf("CheckHygiene failed: %v", err)
	}

	if report.Score != 100 {
		t.Errorf("expected score 100 for clean package, got %d", report.Score)
	}
	if len(report.Issues) != 0 {
		t.Errorf("expected no issues, got %d: %v", len(report.Issues), report.Issues)
	}
	if report.PackagePath != dir {
		t.Errorf("expected packagePath %q, got %q", dir, report.PackagePath)
	}
}

func TestCountTODOs(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "clean.go"), `package foo

func Clean() string { return "clean" }
`)

	writeFile(t, filepath.Join(dir, "dirty.go"), `package foo

// TODO: fix this later
func Dirty() string {
	// FIXME: broken
	// HACK: workaround
	return "dirty"
}

// XXX: this is bad
func Bad() {}
`)

	// Non-Go files should be ignored.
	writeFile(t, filepath.Join(dir, "notes.txt"), "TODO: ignored because not .go\n")

	issues := countTODOs(dir)

	if len(issues) != 4 {
		t.Fatalf("expected 4 TODO issues, got %d: %v", len(issues), issues)
	}

	for _, issue := range issues {
		if issue.Category != "todo" {
			t.Errorf("expected category 'todo', got %q", issue.Category)
		}
		if issue.File != "dirty.go" {
			t.Errorf("expected file 'dirty.go', got %q", issue.File)
		}
		if issue.Line == 0 {
			t.Error("expected non-zero line number")
		}
	}
}

func TestCountTODOs_Empty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "clean.go"), "package foo\n\nfunc F() {}\n")

	issues := countTODOs(dir)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestCountTODOs_NonexistentDir(t *testing.T) {
	issues := countTODOs("/nonexistent/dir")
	if issues != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", issues)
	}
}

func TestParseToolOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		category string
		want     int
	}{
		{
			name:     "empty",
			output:   "",
			category: "govet",
			want:     0,
		},
		{
			name:     "single_issue",
			output:   "foo.go:10:5: unreachable code\n",
			category: "govet",
			want:     1,
		},
		{
			name:     "multiple_issues",
			output:   "foo.go:10:5: unreachable code\nbar.go:20:3: unused variable\n",
			category: "staticcheck",
			want:     2,
		},
		{
			name:     "comment_lines_skipped",
			output:   "# example.com/pkg\nfoo.go:10: error here\n",
			category: "govet",
			want:     1,
		},
		{
			name:     "blank_lines_skipped",
			output:   "\n\nfoo.go:1:1: msg\n\n",
			category: "govet",
			want:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := parseToolOutput(tt.output, tt.category)
			if len(issues) != tt.want {
				t.Errorf("parseToolOutput() returned %d issues; want %d: %v", len(issues), tt.want, issues)
			}
			for _, issue := range issues {
				if issue.Category != tt.category {
					t.Errorf("issue category = %q; want %q", issue.Category, tt.category)
				}
			}
		})
	}
}

func TestParseToolOutput_FieldExtraction(t *testing.T) {
	output := "pkg/foo.go:42:5: something is wrong\n"
	issues := parseToolOutput(output, "govet")

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.File != "foo.go" {
		t.Errorf("file = %q; want %q", issue.File, "foo.go")
	}
	if issue.Line != 42 {
		t.Errorf("line = %d; want 42", issue.Line)
	}
	if issue.Message != "something is wrong" {
		t.Errorf("message = %q; want %q", issue.Message, "something is wrong")
	}
}

func TestCheckHygiene_ScoreDeduction(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/messy\n\ngo 1.21\n")

	// Create a file with 5 TODOs — should deduct 5 points.
	writeFile(t, filepath.Join(dir, "messy.go"), `package messy

// TODO: one
// TODO: two
// TODO: three
// TODO: four
// TODO: five
func F() {}
`)

	report, err := CheckHygiene(dir)
	if err != nil {
		t.Fatalf("CheckHygiene failed: %v", err)
	}

	if report.Summary["todo"] != 5 {
		t.Errorf("expected 5 TODO issues, got %d", report.Summary["todo"])
	}

	// Score should be <= 95 (100 - 5 TODOs, possibly lower from vet/staticcheck).
	if report.Score > 95 {
		t.Errorf("expected score <= 95 with 5 TODOs, got %d", report.Score)
	}
}

func TestCheckHygiene_ScoreFloor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/terrible\n\ngo 1.21\n")

	// 25 TODOs — deduction capped at 20.
	var content string
	content = "package terrible\n\n"
	for i := range 25 {
		content += "// TODO: " + string(rune('a'+i%26)) + "\n"
	}
	content += "func F() {}\n"
	writeFile(t, filepath.Join(dir, "terrible.go"), content)

	report, err := CheckHygiene(dir)
	if err != nil {
		t.Fatalf("CheckHygiene failed: %v", err)
	}

	if report.Score < 0 {
		t.Errorf("score should never go below 0, got %d", report.Score)
	}
	if report.Summary["todo"] != 25 {
		t.Errorf("expected 25 TODO issues, got %d", report.Summary["todo"])
	}
}

func TestCheckHygiene_GoVetFindings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/vettest\n\ngo 1.21\n")

	// Unreachable code: go vet should flag this.
	writeFile(t, filepath.Join(dir, "bad.go"), `package vettest

func Bad() int {
	return 1
	return 2
}
`)

	report, err := CheckHygiene(dir)
	if err != nil {
		t.Fatalf("CheckHygiene failed: %v", err)
	}

	if report.Summary["govet"] == 0 {
		t.Error("expected go vet to find unreachable code")
	}
	if report.Score >= 100 {
		t.Errorf("expected score < 100 with go vet findings, got %d", report.Score)
	}
}
