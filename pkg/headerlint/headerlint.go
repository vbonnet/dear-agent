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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
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

var (
	// boldField matches one **Label:** bold-field marker: bold text that ends
	// with a colon immediately before the closing **. The body is capped at
	// 60 characters and may not itself contain "*" or ":", so this does not
	// match arbitrary bold prose that happens to contain a colon somewhere
	// inside a longer sentence.
	boldField = regexp.MustCompile(`\*\*[^*\n:]{1,60}:\*\*`)

	// headingH2Plus matches a level-2-through-6 ATX heading, including the
	// zero-to-three leading spaces Markdown permits. A level-1 title does not
	// end the zone, since the metadata block conventionally follows it.
	headingH2Plus = regexp.MustCompile(`^ {0,3}#{2,6}([ \t]|$)`)
)

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
	var violations []Violation
	lines := strings.Split(string(data), "\n")
	var fenceByte byte
	fenceLength := 0
	var fenceContainer fenceContainerContext
	for i, line := range lines {
		lineNo := i + 1
		marker, length, trailing, isFence := fenceDelimiter(line)
		if fenceByte != 0 {
			if fenceContainer.contains(line) {
				if isFence && marker == fenceByte && length >= fenceLength && strings.TrimSpace(trailing) == "" {
					fenceByte = 0
					fenceLength = 0
					fenceContainer = fenceContainerContext{}
				}
				continue
			}
			// A fenced block nested in a quote or list ends when that
			// container ends, even if it never supplied a closing fence.
			fenceByte = 0
			fenceLength = 0
			fenceContainer = fenceContainerContext{}
		}
		if isFence {
			fenceByte = marker
			fenceLength = length
			fenceContainer = parseFenceContainerContext(line)
			continue
		}
		if headingH2Plus.MatchString(line) {
			break
		}
		if lineNo > headerZoneMaxLines {
			break
		}
		if matches := boldField.FindAllString(line, -1); len(matches) >= 2 {
			violations = append(violations, Violation{Path: path, Line: lineNo, Text: strings.TrimSpace(line)})
		}
	}
	return violations
}

type fenceContainerContext struct {
	contentOffset int
	quoteDepth    int
	hasList       bool
}

func (c fenceContainerContext) contains(line string) bool {
	if c.quoteDepth == 0 && !c.hasList {
		return true
	}
	if strings.TrimSpace(line) == "" {
		return true
	}
	current := parseFenceContainerContext(line)
	if current.quoteDepth < c.quoteDepth {
		return false
	}
	return !c.hasList || current.contentOffset >= c.contentOffset
}

func parseFenceContainerContext(line string) fenceContainerContext {
	offset, ok := skipFenceIndent(line, 0)
	if !ok {
		return fenceContainerContext{}
	}
	context := fenceContainerContext{contentOffset: offset}
	for offset < len(line) {
		marker := line[offset]
		markerEnd, found := fenceContainerMarkerEnd(line, offset)
		if !found {
			break
		}
		if marker == '>' {
			context.quoteDepth++
		} else {
			context.hasList = true
		}
		offset = markerEnd
		offset, ok = skipFenceIndent(line, offset)
		if !ok {
			break
		}
		context.contentOffset = offset
	}
	if context.quoteDepth == 0 && !context.hasList {
		context.contentOffset = offset
	}
	return context
}

func fenceDelimiter(line string) (byte, int, string, bool) {
	offset, ok := fenceContentOffset(line)
	if !ok || offset == len(line) {
		return 0, 0, "", false
	}
	marker := line[offset]
	if marker != '`' && marker != '~' {
		return 0, 0, "", false
	}
	end := offset
	for end < len(line) && line[end] == marker {
		end++
	}
	length := end - offset
	if length < 3 {
		return 0, 0, "", false
	}
	trailing := line[end:]
	if marker == '`' && strings.Contains(trailing, "`") {
		return 0, 0, "", false
	}
	return marker, length, trailing, true
}

// fenceContentOffset skips the indentation and block/list container markers
// that Markdown permits before a fenced code delimiter.
func fenceContentOffset(line string) (int, bool) {
	offset, ok := skipFenceIndent(line, 0)
	if !ok {
		return 0, false
	}
	for offset < len(line) {
		markerEnd, found := fenceContainerMarkerEnd(line, offset)
		if !found {
			return offset, true
		}
		offset = markerEnd
		var indentOK bool
		offset, indentOK = skipFenceIndent(line, offset)
		if !indentOK {
			return 0, false
		}
	}
	return offset, true
}

func fenceContainerMarkerEnd(line string, offset int) (int, bool) {
	if line[offset] == '>' {
		offset++
		if offset < len(line) && (line[offset] == ' ' || line[offset] == '\t') {
			offset++
		}
		return offset, true
	}
	if strings.ContainsRune("-+*", rune(line[offset])) {
		return terminatedListMarkerEnd(line, offset+1)
	}
	return orderedListMarkerEnd(line, offset)
}

func orderedListMarkerEnd(line string, offset int) (int, bool) {
	markerEnd := offset
	for markerEnd < len(line) && markerEnd-offset < 9 && line[markerEnd] >= '0' && line[markerEnd] <= '9' {
		markerEnd++
	}
	if markerEnd == offset || markerEnd >= len(line) || (line[markerEnd] != '.' && line[markerEnd] != ')') {
		return 0, false
	}
	return terminatedListMarkerEnd(line, markerEnd+1)
}

func terminatedListMarkerEnd(line string, markerEnd int) (int, bool) {
	if markerEnd >= len(line) || (line[markerEnd] != ' ' && line[markerEnd] != '\t') {
		return 0, false
	}
	return markerEnd + 1, true
}

func skipFenceIndent(line string, offset int) (int, bool) {
	spaces := 0
	for offset < len(line) && line[offset] == ' ' {
		offset++
		spaces++
	}
	return offset, spaces <= 3
}

func sortViolations(violations []Violation) {
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Path != violations[j].Path {
			return violations[i].Path < violations[j].Path
		}
		return violations[i].Line < violations[j].Line
	})
}
