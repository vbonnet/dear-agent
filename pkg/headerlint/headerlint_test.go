package headerlint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestCheckFile_FlagsRealWorldReviewHeader(t *testing.T) {
	// Mirrors the actual REVIEW.md:3 header block in this repo.
	const content = "# REVIEW.md — Multi-agent PR review protocol\n" +
		"\n" +
		"**Status:** authoritative · **Last updated:** 2026-06-11\n" +
		"\n" +
		"Every PR against this repo goes through the review protocol below.\n" +
		"\n" +
		"---\n" +
		"\n" +
		"## 1. Four-state outcome model\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "REVIEW.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Line != 3 {
		t.Fatalf("want violation on line 3, got line %d", violations[0].Line)
	}
}

func TestCheckFile_FlagsRealWorldBeadHeader(t *testing.T) {
	// Mirrors the three-field "Bead / Status / Date" header variant.
	const content = "# Some doc\n" +
		"\n" +
		"**Bead:** ce-04cv · **Status:** Investigation only (no implementation) · **Date:** 2026-06-20\n" +
		"\n" +
		"## Overview\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "doc.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_FlagsCodeReviewAutomationSetupHeader(t *testing.T) {
	// Mirrors docs/code-review-automation-setup.md:3.
	const content = "# Automated PR code review — Claude + Codex setup\n" +
		"\n" +
		"**Status:** authoritative · **Last audited:** 2026-07-23\n" +
		"\n" +
		"Two independent, advisory review bots comment on PRs.\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "code-review-automation-setup.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagProseWithTwoBoldTermsMidDocument(t *testing.T) {
	// The exact false-positive case called out in the task: two bolded terms
	// inside an ordinary comparison paragraph, well past the header zone.
	const content = "# Design options\n" +
		"\n" +
		"**Status:** authoritative\n" +
		"\n" +
		"## Trade-offs\n" +
		"\n" +
		"Option A is simpler to build. **Complexity:** Low. **Timeline:** Comparable.\n" +
		"Option B needs more infrastructure but scales further.\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "design.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagSingleBoldField(t *testing.T) {
	// A single **Status:** field alone (the ADR-001 style) is fine — there is
	// nothing for it to run together with.
	const content = "# ADR-001: Monorepo Consolidation\n" +
		"\n" +
		"Status: Accepted (2026-04-24)\n" +
		"\n" +
		"## Context\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "adr.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagBoldFieldsOnSeparateLines(t *testing.T) {
	// The recommended replacement format: real line breaks between fields.
	const content = "# Some doc\n" +
		"\n" +
		"- **Status:** authoritative\n" +
		"- **Last updated:** 2026-06-11\n" +
		"\n" +
		"## Overview\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "doc.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagQuotedExampleInsideFence(t *testing.T) {
	// Documentation that quotes the anti-pattern as a "before" example inside
	// a fenced code block, near the top of the file, must not self-trigger.
	const content = "# Doc-header format\n" +
		"\n" +
		"Before:\n" +
		"\n" +
		"```\n" +
		"**Status:** authoritative · **Last updated:** 2026-06-11\n" +
		"```\n" +
		"\n" +
		"## Rationale\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotCloseLongFenceWithShortOrDifferentDelimiter(t *testing.T) {
	const content = "# Doc-header format\n" +
		"\n" +
		"````markdown\n" +
		"```go\n" +
		"~~~\n" +
		"**Status:** authoritative · **Last updated:** 2026-06-11\n" +
		"```\n" +
		"````\n" +
		"\n" +
		"## Rationale\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagFencesNestedInMarkdownContainers(t *testing.T) {
	tests := map[string]string{
		"blockquote":                    "# Doc\n\n> ```markdown\n> **Status:** draft · **Owner:** docs\n> ```\n\n## Body\n",
		"list":                          "# Doc\n\n- ~~~markdown\n  **Status:** draft · **Owner:** docs\n  ~~~\n\n## Body\n",
		"nested":                        "# Doc\n\n> - ````markdown\n>   **Status:** draft · **Owner:** docs\n>   ````\n\n## Body\n",
		"list-continuation-four-spaces": "# Doc\n\n- example\n    ~~~markdown\n    **Status:** draft · **Owner:** docs\n    ~~~\n\n## Body\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTemp(t, dir, "format.md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 0 {
				t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
			}
		})
	}
}

func TestCheckFile_InlineCodeDoesNotCrossMarkdownBlockBoundaries(t *testing.T) {
	tests := map[string]string{
		"blank-line":             "# Doc\n\nUnmatched ` opener\n\n**Status:** draft · **Owner:** docs\n` later\n",
		"blockquote":             "# Doc\n\nUnmatched ` opener\n> **Status:** draft · **Owner:** docs\n> ` later\n",
		"fenced-block":           "# Doc\n\nUnmatched ` opener\n```text\n`\n```\n**Status:** draft · **Owner:** docs\n",
		"nested-list-blockquote": "# Doc\n\n- Unmatched ` opener\n  > **Status:** draft · **Owner:** docs `\n",
		"nested-sublist":         "# Doc\n\n- Unmatched ` opener\n  - **Status:** draft · **Owner:** docs `\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTemp(t, dir, "format.md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 1 {
				t.Fatalf("want one visible header-field violation, got %v", violations)
			}
		})
	}
}

func TestCheckFile_InlineCodeContinuesWithinBlockquote(t *testing.T) {
	const content = "# Doc\n\n> Unmatched ` opener\n> **Status:** draft · **Owner:** docs\n> ` closer\n"
	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_InlineCodeDoesNotCrossBodyHeading(t *testing.T) {
	tests := map[string]string{
		"top-level":  "# Doc\n\nUnmatched ` opener\n## Body `\n**Status:** draft · **Owner:** docs\n",
		"blockquote": "# Doc\n\nUnmatched ` opener\n> ## Body `\n> **Status:** draft · **Owner:** docs\n",
		"list":       "# Doc\n\nUnmatched ` opener\n- ## Body `\n  **Status:** draft · **Owner:** docs\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTemp(t, dir, "format.md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 0 {
				t.Fatalf("body fields after heading should not be scanned, got %v", violations)
			}
		})
	}
}

func TestCheckFile_UnclosedNestedFenceEndsWithItsContainer(t *testing.T) {
	tests := map[string]string{
		"blockquote": "# Doc\n\n> ```markdown\n> quoted code\n**Status:** draft · **Owner:** docs\n",
		"list":       "# Doc\n\n- ~~~markdown\n  listed code\n**Status:** draft · **Owner:** docs\n",
		"nested":     "# Doc\n\n> - ````markdown\n>   nested code\n**Status:** draft · **Owner:** docs\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTemp(t, dir, "format.md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 1 || violations[0].Line != 5 {
				t.Fatalf("want violation on line 5, got %v", violations)
			}
		})
	}
}

func TestCheckFile_UnclosedListFenceEndsAtSiblingItem(t *testing.T) {
	const content = "# Doc\n\n" +
		"- ```markdown\n" +
		"  quoted code\n" +
		"- **Status:** draft · **Owner:** docs\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 5 {
		t.Fatalf("want violation on sibling list item at line 5, got %v", violations)
	}
}

func TestCheckFile_UnclosedContinuationListFenceEndsAtSiblingItem(t *testing.T) {
	const content = "# Doc\n\n" +
		"- Example:\n" +
		"  ```markdown\n" +
		"  quoted code\n" +
		"- **Status:** draft · **Owner:** docs\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 6 {
		t.Fatalf("want violation on sibling list item at line 6, got %v", violations)
	}
}

func TestCheckFile_DoesNotFlagFourSpaceListFenceContent(t *testing.T) {
	const content = "# Doc\n\n" +
		"- ```markdown\n" +
		"    **Status:** draft · **Owner:** docs\n" +
		"    ```\n" +
		"\n## Body\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagBoldFieldsInsideInlineCode(t *testing.T) {
	const content = "# Doc\n\n" +
		"The old form is `**Status:** draft · **Owner:** docs`.\n" +
		"A real **Status:** field beside that example is still only one field.\n" +
		"\n## Body\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagBoldFieldsInsideMultilineCodeSpan(t *testing.T) {
	const content = "# Doc\n\n" +
		"The old form is `example\n" +
		"**Status:** draft · **Owner:** docs\n" +
		"continues here` and is code.\n" +
		"\n## Body\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "format.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_UnmatchedBacktickIsLiteralText(t *testing.T) {
	const content = "# Doc\n\n" +
		"The unmatched ` opener\n" +
		"**Status:** draft · **Owner:** docs\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "doc.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 4 {
		t.Fatalf("want violation on line 4, got %v", violations)
	}
}

func TestCheckFile_BackslashDoesNotEscapeCodeSpanCloser(t *testing.T) {
	const content = "# Doc\n\n" +
		"`literal \\` **Status:** draft · **Owner:** docs\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "doc.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 3 {
		t.Fatalf("want violation on line 3, got %v", violations)
	}
}

func TestCheckFile_UnmarkedBlankEndsQuotedFence(t *testing.T) {
	const content = "# Doc\n\n" +
		"> ```markdown\n" +
		"> quoted code\n" +
		"\n" +
		"> **Status:** draft · **Owner:** docs\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "doc.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 6 {
		t.Fatalf("want violation on line 6, got %v", violations)
	}
}

func TestCheckFile_IndentedHeadingEndsHeaderZone(t *testing.T) {
	const content = "# Design options\n" +
		"\n" +
		"   ## Trade-offs\n" +
		"\n" +
		"**Complexity:** Low. **Timeline:** Comparable.\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "design.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_DoesNotFlagPastHeaderZoneLineCap(t *testing.T) {
	// No heading at all, but the violation line is well past the header-zone
	// line cap — treated as body text, not a metadata block.
	var builder strings.Builder
	builder.WriteString("# Doc\n\n")
	for range 20 {
		builder.WriteString("Filler line.\n")
	}
	builder.WriteString("**Complexity:** Low. **Timeline:** Comparable.\n")
	content := builder.String()

	dir := t.TempDir()
	path := writeTemp(t, dir, "doc.md", content)

	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want 0 violations, got %d: %v", len(violations), violations)
	}
}

func TestCheckFile_ReadError(t *testing.T) {
	if _, err := CheckFile(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatal("want error for missing file")
	}
}

func TestCheckDir_WalksMarkdownOnly(t *testing.T) {
	dir := t.TempDir()
	writeTemp(t, dir, "a.md", "# A\n\n**X:** 1 · **Y:** 2\n")
	writeTemp(t, dir, "sub/b.md", "# B\n\nfine\n")
	writeTemp(t, dir, "notes.txt", "**X:** 1 · **Y:** 2\n")

	violations, err := CheckDir(dir)
	if err != nil {
		t.Fatalf("CheckDir: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %d: %v", len(violations), violations)
	}
}

func TestCheckRepository_TrackedFilesOnly(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")

	writeTemp(t, dir, "tracked.md", "# Doc\n\n**X:** 1 · **Y:** 2\n")
	writeTemp(t, dir, "untracked.md", "# Doc\n\n**X:** 1 · **Y:** 2\n")
	runGit(t, dir, "add", "tracked.md")
	runGit(t, dir, "commit", "-q", "-m", "init")

	violations, err := CheckRepository(context.Background(), dir)
	if err != nil {
		t.Fatalf("CheckRepository: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Path != "tracked.md" {
		t.Fatalf("want tracked.md, got %s", violations[0].Path)
	}

	if err := os.Remove(filepath.Join(dir, "tracked.md")); err != nil {
		t.Fatal(err)
	}
	violations, err = CheckRepository(context.Background(), dir)
	if err != nil {
		t.Fatalf("CheckRepository with unstaged deletion: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want no violations after unstaged deletion, got %v", violations)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestViolation_String(t *testing.T) {
	v := Violation{Path: "REVIEW.md", Line: 3, Text: "**Status:** authoritative · **Last updated:** 2026-06-11"}
	got := v.String()
	if got == "" {
		t.Fatal("want non-empty string")
	}
}
