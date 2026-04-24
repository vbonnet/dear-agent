package wikibrain_test

import (
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

func TestLint_BrokenWikiLink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "01-decisions/ADR-001.md", `# ADR-001

- **Last updated:** 2026-01-15

[[ADR-999]]
`)

	report, err := wikibrain.Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := findIssue(report.Issues, wikibrain.CodeBrokenLink)
	if !found {
		t.Error("expected BROKEN_LINK issue for [[ADR-999]]")
	}
	if report.Stats.ErrorCount == 0 {
		t.Error("expected ErrorCount > 0")
	}
}

func TestLint_OrphanPage(t *testing.T) {
	dir := t.TempDir()
	// Two pages, neither links to the other
	writeFile(t, dir, "01-decisions/ADR-001.md", "# ADR-001\n\n- **Last updated:** 2026-01-15\n")
	writeFile(t, dir, "01-decisions/ADR-002.md", "# ADR-002\n\n- **Last updated:** 2026-01-15\n")

	report, err := wikibrain.Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := countCode(report.Issues, wikibrain.CodeOrphanPage)
	if count < 2 {
		t.Errorf("expected at least 2 ORPHAN_PAGE issues, got %d", count)
	}
}

func TestLint_StalePage(t *testing.T) {
	dir := t.TempDir()
	// Deliberately old date
	writeFile(t, dir, "01-decisions/ADR-001.md", `# ADR-001

- **Last updated:** 2020-01-01

[[ADR-001]]
`)

	report, err := wikibrain.Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !findIssue(report.Issues, wikibrain.CodeStalePage) {
		t.Error("expected STALE_PAGE issue")
	}
}

func TestLint_MissingMeta(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "02-research-index/topic-foo.md", "# Foo\n\nNo metadata here.\n")

	report, err := wikibrain.Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !findIssue(report.Issues, wikibrain.CodeMissingMeta) {
		t.Error("expected MISSING_META issue")
	}
}

func TestLint_CleanKB(t *testing.T) {
	dir := t.TempDir()
	// Two well-formed pages that link to each other
	writeFile(t, dir, "01-decisions/ADR-001.md", `# ADR-001

- **Last updated:** `+time.Now().Format("2006-01-02")+`

[[ADR-002]] and [ADR-002](ADR-002.md)
`)
	writeFile(t, dir, "01-decisions/ADR-002.md", `# ADR-002

- **Last updated:** `+time.Now().Format("2006-01-02")+`

[[ADR-001]] references [[ADR-001]] again.
`)

	report, err := wikibrain.Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Stats.ErrorCount != 0 {
		t.Errorf("expected no errors, got %d", report.Stats.ErrorCount)
	}
}

func findIssue(issues []wikibrain.LintIssue, code wikibrain.LintCode) bool {
	for _, iss := range issues {
		if iss.Code == code {
			return true
		}
	}
	return false
}

func countCode(issues []wikibrain.LintIssue, code wikibrain.LintCode) int {
	n := 0
	for _, iss := range issues {
		if iss.Code == code {
			n++
		}
	}
	return n
}
