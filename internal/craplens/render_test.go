package craplens

import (
	"strings"
	"testing"
)

// TestRenderSilentWhenNotFlagged pins that an unflagged report produces no
// comment body at all, so the workflow cannot post an empty comment.
func TestRenderSilentWhenNotFlagged(t *testing.T) {
	if got := (Report{Scored: 12, WithinAgentTarget: 12}).Render(); got != "" {
		t.Errorf("Render() = %q, want empty", got)
	}
}

// TestRenderNamesTheProblem pins the parts of the comment a reader acts on:
// which function, its score, and the fact that the signal cannot block.
func TestRenderNamesTheProblem(t *testing.T) {
	body := Report{
		Threshold: 30,
		Scored:    4,
		Over: []Function{
			{File: "agm/cmd/agm-bus/main.go", Line: 99, Name: "cmdServe", Complexity: 46, Coverage: 0},
		},
		Untested: []Package{{ImportPath: "agm/cmd/agm-bus", New: true}},
	}.Render()

	for _, want := range []string{"cmdServe", "agm/cmd/agm-bus/main.go:99", "2162", "new package", "advisory", "never fails a check", "parity verification", "hard gate"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body is missing %q:\n%s", want, body)
		}
	}
}

// TestRenderDoesNotDuplicateOwnedChecks pins the harness-hygiene property that
// this signal states what it does NOT own, so a reader does not expect it to
// replace errcheck or gocyclo.
func TestRenderDoesNotDuplicateOwnedChecks(t *testing.T) {
	body := Report{Threshold: 30, Untested: []Package{{ImportPath: "x"}}}.Render()
	for _, want := range []string{"errcheck", "gocyclo"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body should name %q as the owner it defers to:\n%s", want, body)
		}
	}
}

// TestRenderBoundsEachList pins that a diff touching hundreds of functions
// still produces a comment somebody will read.
func TestRenderBoundsEachList(t *testing.T) {
	var over []Function
	for i := range 40 {
		over = append(over, Function{File: "a.go", Line: i, Name: "f", Complexity: 20, Coverage: 0})
	}
	body := Report{Threshold: 30, Over: over}.Render()

	if strings.Count(body, "| 420 |") > maxListed {
		t.Errorf("rendered %d rows, want at most %d", strings.Count(body, "| 420 |"), maxListed)
	}
	if !strings.Contains(body, "and 30 more") {
		t.Errorf("truncation was not disclosed:\n%s", body)
	}
}

// TestMdCodeSurvivesMarkdownMetacharacters covers rendering a path that
// contains characters legal in a filename but structural in a Markdown table.
func TestMdCodeSurvivesMarkdownMetacharacters(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "pipe", in: "pkg/a|b.go"},
		{name: "backtick", in: "pkg/a`b.go"},
		{name: "double backtick", in: "pkg/a``b.go"},
		{name: "leading backtick", in: "`pkg/a.go"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mdCode(tc.in)
			if strings.Contains(got, "|") && !strings.Contains(got, `\|`) {
				t.Errorf("unescaped pipe would add a table column: %q", got)
			}
			// The fence must be longer than any backtick run in the content,
			// or the span terminates early.
			fence := 0
			for fence < len(got) && got[fence] == '`' {
				fence++
			}
			inner := strings.Trim(got, "`")
			longest, run := 0, 0
			for _, r := range inner {
				if r == '`' {
					run++
					if run > longest {
						longest = run
					}
					continue
				}
				run = 0
			}
			if fence <= longest {
				t.Errorf("fence of %d backticks does not clear a run of %d in %q", fence, longest, got)
			}
		})
	}
}

func TestRenderEscapesUnknownPackagePaths(t *testing.T) {
	body := Report{Threshold: 30, Over: []Function{{File: "x.go", Name: "f", Complexity: 40}}, Unknown: []string{"pkg/a`b\n**injected**"}}.Render()
	if !strings.Contains(body, "``pkg/a`b **injected**``") {
		t.Fatalf("unknown path was not safely code-spanned: %q", body)
	}
}

// TestMdCodeCollapsesNewlines guards against a real gap: a widened backtick
// fence alone does not make a literal newline safe, because Markdown's
// block-level parsing (a heading, a list) can still trigger on content after
// a line break even inside what reads as one inline code span. A path
// containing a newline followed by heading syntax must not let that syntax
// survive into the rendered comment.
func TestMdCodeCollapsesNewlines(t *testing.T) {
	got := mdCode("before\n## injected")
	if strings.Contains(got, "\n") {
		t.Fatalf("mdCode contains a raw newline: %q", got)
	}
	if !strings.Contains(got, "before ## injected") {
		t.Fatalf("mdCode lost content: %q", got)
	}
}
