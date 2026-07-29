package headerlint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
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

func TestCheckFile_NonOneOrderedMarkerDoesNotInterruptParagraph(t *testing.T) {
	const content = "# Design options\n" +
		"\n" +
		"Introductory paragraph\n" +
		"2. ## literal text, not a list heading\n" +
		"**Complexity:** Low. **Timeline:** Comparable.\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "design.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 5 {
		t.Fatalf("want violation after noninterrupting ordered marker, got %v", violations)
	}
}

func TestCheckFile_NonOneOrderedListHeadingAfterBlankEndsHeaderZone(t *testing.T) {
	const content = "# Design options\n" +
		"\n" +
		"2. ## Nested body heading\n" +
		"**Complexity:** Low. **Timeline:** Comparable.\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "design.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want ordered-list heading after blank to end header zone, got %v", violations)
	}
}

func TestCheckFile_LeadingZeroOneOrderedMarkerInterruptsParagraph(t *testing.T) {
	const content = "# Design options\n" +
		"\n" +
		"Introductory paragraph\n" +
		"01. ## Nested body heading\n" +
		"**Complexity:** Low. **Timeline:** Comparable.\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "design.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want numeric-one list marker to end header zone, got %v", violations)
	}
}

func TestCheckFile_CRLFHeadingEndsHeaderZone(t *testing.T) {
	const content = "# Design options\r\n" +
		"\r\n" +
		"##\r\n" +
		"**Complexity:** Low. **Timeline:** Comparable.\r\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "design.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want CRLF heading to end header zone, got %v", violations)
	}
}

func TestCheckFile_ListContinuationHeadingEndsHeaderZone(t *testing.T) {
	const content = "# Design options\n" +
		"\n" +
		"- parent\n" +
		"  - child\n" +
		"    ## Details\n" +
		"    **Complexity:** Low. **Timeline:** Comparable.\n"

	dir := t.TempDir()
	path := writeTemp(t, dir, "design.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want nested-list continuation heading to end header zone, got %v", violations)
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
	gittest.Run(t, dir, args...)
}

func TestViolation_String(t *testing.T) {
	v := Violation{Path: "REVIEW.md", Line: 3, Text: "**Status:** authoritative · **Last updated:** 2026-06-11"}
	got := v.String()
	if got == "" {
		t.Fatal("want non-empty string")
	}
}

func TestCheckFile_DoesNotFlagEscapedBoldFieldExamples(t *testing.T) {
	dir := t.TempDir()
	path := writeTemp(t, dir, "escaped.md", "# Doc\n\n\\**Status:** draft · \\**Owner:** docs\n")
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want escaped markers ignored, got %v", violations)
	}
}

func TestCheckFile_DoesNotCloseListFenceWithOverIndentedDelimiter(t *testing.T) {
	const content = "# Doc\n\n- ```markdown\n      ```\n  **Status:** draft · **Owner:** docs\n  ```\n"
	dir := t.TempDir()
	path := writeTemp(t, dir, "list-fence.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want over-indented delimiter treated as code content, got %v", violations)
	}
}

func TestCheckFile_InlineCodeDoesNotCrossSetextHeading(t *testing.T) {
	const content = "# Doc\n\nUnmatched ` opener\n---\n**Status:** draft · **Owner:** docs\n` later\n"
	dir := t.TempDir()
	path := writeTemp(t, dir, "setext.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("want one visible header-field violation, got %v", violations)
	}
}

func TestCheckFile_InlineCodeDoesNotCrossThematicBreak(t *testing.T) {
	for _, delimiter := range []string{"***", "___", "* * *", "_ _ _"} {
		t.Run(delimiter, func(t *testing.T) {
			content := "# Doc\n\nUnmatched ` opener\n" + delimiter +
				"\n**Status:** draft · **Owner:** docs\n` later\n"
			dir := t.TempDir()
			path := writeTemp(t, dir, "thematic-break.md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 1 {
				t.Fatalf("want one visible header-field violation across %q, got %v", delimiter, violations)
			}
		})
	}
}

func TestCheckFile_InlineCodeDoesNotCrossHTMLBlock(t *testing.T) {
	const content = "# Doc\n\n" +
		"Unmatched ` opener\n" +
		"<script></script>\n" +
		"**Status:** draft · **Owner:** docs\n" +
		"` later\n"
	dir := t.TempDir()
	path := writeTemp(t, dir, "html-block.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 5 {
		t.Fatalf("want visible header-field violation after HTML block, got %v", violations)
	}
}

func TestCheckFile_HeadingsInsideRawHTMLDoNotEndHeaderZone(t *testing.T) {
	for name, block := range map[string]string{
		"comment": "<!--\n## literal comment content\n-->\n",
		"script":  "<script>\n## literal script content\n</script>\n",
	} {
		t.Run(name, func(t *testing.T) {
			content := "# Doc\n\n" + block + "**Status:** draft · **Owner:** docs\n"
			dir := t.TempDir()
			path := writeTemp(t, dir, name+".md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 1 {
				t.Fatalf("want header-field violation after raw HTML, got %v", violations)
			}
		})
	}
}

func TestCheckFile_HeadingsInsideContainerRawHTMLDoNotEndHeaderZone(t *testing.T) {
	for name, block := range map[string]string{
		"blockquote": "> <!--\n> ## literal comment content\n> -->\n> ",
		"list":       "- <script>\n  ## literal script content\n  </script>\n  ",
	} {
		t.Run(name, func(t *testing.T) {
			content := "# Doc\n\n" + block + "**Status:** draft · **Owner:** docs\n"
			dir := t.TempDir()
			path := writeTemp(t, dir, name+".md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 1 || violations[0].Line != 6 {
				t.Fatalf("want header-field violation after container raw HTML, got %v", violations)
			}
		})
	}
}

func TestCheckFile_UnclosedRawHTMLEndsWithItsContainer(t *testing.T) {
	for name, block := range map[string]string{
		"blockquote": "> <!--\n\n",
		"list":       "- <script>\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			content := "# Doc\n\n" + block + "**Status:** draft · **Owner:** docs\n"
			dir := t.TempDir()
			path := writeTemp(t, dir, name+"-unclosed-html.md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 1 || violations[0].Line != 5 {
				t.Fatalf("want top-level header-field violation after container ends, got %v", violations)
			}
		})
	}
}

func TestCheckFile_NonInterruptingListShapedFencePreservesInlineCode(t *testing.T) {
	const content = "# Doc\n\n" +
		"Intro ` opener\n" +
		"2. ~~~\n" +
		"**Status:** draft · **Owner:** docs`\n"
	dir := t.TempDir()
	path := writeTemp(t, dir, "noninterrupting-fence.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("want multiline code-span fields ignored, got %v", violations)
	}
}

func TestCheckFile_PreservesLazyContainerInlineCodeContinuation(t *testing.T) {
	for name, content := range map[string]string{
		"blockquote": "# Doc\n\n> old form is `example\n**Status:** draft · **Owner:** docs\n> continues here`\n",
		"list":       "# Doc\n\n- old form is `example\n**Status:** draft · **Owner:** docs\n  continues here`\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTemp(t, dir, name+".md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 0 {
				t.Fatalf("want lazy continuation code span ignored, got %v", violations)
			}
		})
	}
}

func TestCheckFile_RetainsContainerForInlineCodeOpenedOnLazyLine(t *testing.T) {
	for name, content := range map[string]string{
		"blockquote": "# Doc\n\n> intro\nlazy opener `start\n> **Status:** draft · **Owner:** docs\n> end`\n",
		"list":       "# Doc\n\n- intro\nlazy opener `start\n  **Status:** draft · **Owner:** docs\n  end`\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTemp(t, dir, name+"-lazy-opener.md", content)
			violations, err := CheckFile(path)
			if err != nil {
				t.Fatalf("CheckFile: %v", err)
			}
			if len(violations) != 0 {
				t.Fatalf("want code span opened on lazy line ignored, got %v", violations)
			}
		})
	}
}

func TestCheckFile_MarkedContainerBlankEndsInlineCodeParagraph(t *testing.T) {
	const content = "# Doc\n" +
		"> opener `\n" +
		"> \n" +
		"> **Status:** draft · **Owner:** docs\n" +
		"> later`\n"
	dir := t.TempDir()
	path := writeTemp(t, dir, "marked-blank-inline.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 4 {
		t.Fatalf("want field violation after marked paragraph break, got %v", violations)
	}
}

func TestCheckFile_MarkedContainerBlankEndsTypeSixHTMLBlock(t *testing.T) {
	const content = "# Doc\n" +
		"> <div>\n" +
		"> \n" +
		"> **Status:** draft · **Owner:** docs\n"
	dir := t.TempDir()
	path := writeTemp(t, dir, "marked-blank-html.md", content)
	violations, err := CheckFile(path)
	if err != nil {
		t.Fatalf("CheckFile: %v", err)
	}
	if len(violations) != 1 || violations[0].Line != 4 {
		t.Fatalf("want field violation after marked HTML block break, got %v", violations)
	}
}
