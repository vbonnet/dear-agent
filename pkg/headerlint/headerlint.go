// Package headerlint detects the single-line bold metadata "header block"
// anti-pattern in Markdown documents: two or more **Label:** bold-field
// markers crammed onto one physical line, with no real line break between
// distinct metadata fields, near the top of a file. For example:
//
//	**Status:** authoritative · **Last updated:** 2026-06-11
//
// Because there is no line break, renderers display the fields as one
// unbroken run of text — hard to scan, and easy to get wrong (inconsistent
// separators, inconsistent key names). See docs/doc-header-format.md for the
// canonical replacement format and the reasoning behind it.
//
// The check is deliberately scoped to the "header zone" — from the top of a
// file up to (but not including) the first level-2-or-deeper heading, or the
// first headerZoneMaxLines lines, whichever comes first. Two bold terms
// appearing later in a document, as part of ordinary prose (e.g. "**Low.**
// **Comparable.**" inside a comparison paragraph), are not a header-block
// violation and are not flagged.
package headerlint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Violation describes one detected header-block line.
type Violation struct {
	Path string
	Line int
	Text string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d: header-block: two or more bold fields on one line (use a real line break between fields instead): %q", v.Path, v.Line, v.Text)
}

// headerZoneMaxLines bounds how far from the top of a file the header zone
// extends when no heading is found first. Metadata headers live in the first
// handful of lines of a document; this cap keeps ordinary prose, many lines
// down, from ever being scanned.
const headerZoneMaxLines = 15

// CheckFile validates one Markdown file. Content defects are returned as
// violations; read failures are operational errors.
func CheckFile(path string) ([]Violation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return checkData(path, data), nil
}

// CheckDir recursively validates every Markdown file under root.
func CheckDir(root string) ([]Violation, error) {
	var violations []Violation
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !isMarkdown(path) {
			return nil
		}
		//nolint:gosec // G122: path is produced by this same filepath.Walk over a
		// caller-supplied local root, not attacker-controlled input.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", path, readErr)
		}
		violations = append(violations, checkData(path, data)...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortViolations(violations)
	return violations, nil
}

// CheckRepository validates every Git-tracked Markdown file in the repository
// containing root. Violation paths are relative to the repository top level
// so local and CI output is stable.
func CheckRepository(ctx context.Context, root string) ([]Violation, error) {
	top, err := gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(top))
	if repoRoot == "" {
		return nil, fmt.Errorf("resolve repository root: git returned an empty path")
	}

	tracked, err := gitOutput(ctx, repoRoot, "ls-files", "-z", "--full-name")
	if err != nil {
		return nil, fmt.Errorf("list tracked files: %w", err)
	}

	var violations []Violation
	for path := range strings.SplitSeq(string(tracked), "\x00") {
		if path == "" || !isMarkdown(path) {
			continue
		}
		absolute := filepath.Join(repoRoot, filepath.FromSlash(path))
		data, err := os.ReadFile(absolute)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// The index still reports an unstaged deletion. Validate the
				// files that actually remain in the working tree.
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		violations = append(violations, checkData(filepath.ToSlash(path), data)...)
	}
	sortViolations(violations)
	return violations, nil
}

func isMarkdown(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".md")
}

func gitOutput(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		message := strings.TrimSpace(string(output))
		if message == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, message)
	}
	return output, nil
}

// checkData scans the header zone of one Markdown document for lines
// carrying two or more **Label:** bold-field markers. See the package doc
// comment for the header-zone definition and the rationale.
func checkData(path string, data []byte) []Violation {
	lines, source, lineStarts := normalizedMarkdown(data)
	source = goldmarkAnalysisSource(source)
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	headerEndLine := firstATXHeaderEnd(document, source, lines, lineStarts)
	counts := headerBoldFieldCounts(document, source, lineStarts, headerEndLine)

	var violations []Violation
	for lineNumber, count := range counts {
		if count >= 2 {
			violations = append(violations, Violation{
				Path: path,
				Line: lineNumber,
				Text: strings.TrimSpace(lines[lineNumber-1]),
			})
		}
	}
	sortViolations(violations)
	return violations
}

func normalizedMarkdown(data []byte) ([]string, []byte, []int) {
	lines := strings.Split(string(data), "\n")
	for index := range lines {
		lines[index] = strings.TrimSuffix(lines[index], "\r")
	}
	source := []byte(strings.Join(lines, "\n"))
	lineStarts := make([]int, len(lines))
	offset := 0
	for index, line := range lines {
		lineStarts[index] = offset
		offset += len(line) + 1
	}
	return lines, source, lineStarts
}

func firstATXHeaderEnd(
	document ast.Node,
	source []byte,
	lines []string,
	lineStarts []int,
) int {
	headerEndLine := headerZoneMaxLines + 1
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := node.(*ast.Heading)
		if !entering || !ok || heading.Level < 2 {
			return ast.WalkContinue, nil
		}
		if !isATXHeading(heading, source, lines, lineStarts) {
			// Setext headings do not terminate this linter's historical header
			// zone; only explicit level-2-or-deeper ATX headings do.
			return ast.WalkContinue, nil
		}
		lineIndex := sourceLineIndex(lineStarts, heading.Pos())
		lineNumber := lineIndex + 1
		if lineNumber < headerEndLine {
			headerEndLine = lineNumber
		}
		return ast.WalkContinue, nil
	})
	return headerEndLine
}

func isATXHeading(
	heading *ast.Heading,
	source []byte,
	lines []string,
	lineStarts []int,
) bool {
	if heading.Lines().Len() > 0 {
		return heading.Lines().At(0).Start > heading.Pos()
	}
	lineIndex := sourceLineIndex(lineStarts, heading.Pos())
	lineEnd := lineStarts[lineIndex] + len(lines[lineIndex])
	return strings.Contains(string(source[heading.Pos():lineEnd]), "#")
}

func headerBoldFieldCounts(
	document ast.Node,
	source []byte,
	lineStarts []int,
	headerEndLine int,
) map[int]int {
	counts := make(map[int]int)
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		emphasis, ok := node.(*ast.Emphasis)
		if !entering || !ok || emphasis.Level != 2 || insideImage(emphasis) {
			return ast.WalkContinue, nil
		}
		lineNumber, ok := boldFieldLine(emphasis, source, lineStarts)
		if !ok || lineNumber >= headerEndLine || lineNumber > headerZoneMaxLines {
			return ast.WalkContinue, nil
		}
		counts[lineNumber]++
		return ast.WalkContinue, nil
	})
	return counts
}

func goldmarkAnalysisSource(source []byte) []byte {
	masked := append([]byte(nil), source...)
	for {
		document := goldmark.DefaultParser().Parse(text.NewReader(masked))
		changed := false
		_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
			htmlBlock, ok := node.(*ast.HTMLBlock)
			if !entering || !ok ||
				(htmlBlock.HTMLBlockType != ast.HTMLBlockType6 &&
					htmlBlock.HTMLBlockType != ast.HTMLBlockType7) {
				return ast.WalkContinue, nil
			}
			paragraphBefore, paragraphOK := htmlBlock.PreviousSibling().(*ast.Paragraph)
			if !paragraphOK || htmlBlock.Lines().Len() == 0 ||
				paragraphBefore.Lines().Len() == 0 {
				return ast.WalkContinue, nil
			}
			start := htmlBlock.Lines().At(0).Start
			paragraphStop := paragraphBefore.Lines().At(paragraphBefore.Lines().Len() - 1).Stop
			if bytes.Count(source[paragraphStop:start], []byte{'\n'}) >= 2 {
				// Goldmark omits blank lines from the sibling list. A real blank
				// between the paragraph and tag still allows a type-6/7 HTML block
				// to interrupt, so preserve the block and everything it contains.
				return ast.WalkContinue, nil
			}
			stop := htmlBlock.Lines().At(htmlBlock.Lines().Len() - 1).Stop
			tagEnd := inlineHTMLTagEnd(source, start, stop)
			if tagEnd < 0 {
				masked[start] = 'x'
				changed = true
				return ast.WalkContinue, nil
			}
			for index := start; index < tagEnd; index++ {
				if masked[index] != '\n' {
					masked[index] = ' '
				}
			}
			// Keep the downgraded source line nonblank. Otherwise a following
			// type-6/7 tag can become a new HTML-block opener on the next parse,
			// even though both tags belonged to the original open paragraph.
			masked[start] = 'x'
			changed = true
			return ast.WalkContinue, nil
		})
		if !changed {
			return masked
		}
	}
}

func inlineHTMLTagEnd(source []byte, start, stop int) int {
	var quote byte
	for index := start; index < stop && index < len(source); index++ {
		switch {
		case quote != 0 && source[index] == quote:
			quote = 0
		case quote == 0 && (source[index] == '"' || source[index] == '\''):
			quote = source[index]
		case quote == 0 && source[index] == '>':
			return index + 1
		}
	}
	return -1
}

func sourceLineIndex(lineStarts []int, offset int) int {
	index := sort.Search(len(lineStarts), func(index int) bool {
		return lineStarts[index] > offset
	})
	if index == 0 {
		return 0
	}
	return index - 1
}

func insideImage(node ast.Node) bool {
	for parent := node.Parent(); parent != nil; parent = parent.Parent() {
		if _, ok := parent.(*ast.Image); ok {
			return true
		}
	}
	return false
}

func boldFieldLine(
	emphasis *ast.Emphasis,
	source []byte,
	lineStarts []int,
) (int, bool) {
	lineIndex := sourceLineIndex(lineStarts, emphasis.Pos())
	label, ok := emphasisLabel(emphasis, source, lineStarts, lineIndex)
	if !ok || !validBoldFieldLabel(label) {
		return 0, false
	}
	return lineIndex + 1, true
}

func emphasisLabel(
	emphasis *ast.Emphasis,
	source []byte,
	lineStarts []int,
	lineIndex int,
) ([]rune, bool) {
	var rawLabel strings.Builder
	for child := emphasis.FirstChild(); child != nil; child = child.NextSibling() {
		switch inline := child.(type) {
		case *ast.Text:
			if !textSegmentIsOnLine(inline, lineStarts, lineIndex) {
				return nil, false
			}
			rawLabel.Write(inline.Segment.Value(source))
		case *ast.String:
			rawLabel.Write(inline.Value)
		default:
			return nil, false
		}
	}
	return []rune(rawLabel.String()), true
}

func textSegmentIsOnLine(inline *ast.Text, lineStarts []int, lineIndex int) bool {
	return !inline.SoftLineBreak() &&
		!inline.HardLineBreak() &&
		sourceLineIndex(lineStarts, inline.Segment.Start) == lineIndex &&
		sourceLineIndex(lineStarts, max(inline.Segment.Stop-1, inline.Segment.Start)) == lineIndex
}

func validBoldFieldLabel(label []rune) bool {
	if len(label) < 2 || len(label) > 61 || label[len(label)-1] != ':' {
		return false
	}
	for _, character := range label[:len(label)-1] {
		if character == '*' || character == ':' || character == '\n' || character == '\r' {
			return false
		}
	}
	return true
}

func sortViolations(violations []Violation) {
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Path != violations[j].Path {
			return violations[i].Path < violations[j].Path
		}
		return violations[i].Line < violations[j].Line
	})
}
