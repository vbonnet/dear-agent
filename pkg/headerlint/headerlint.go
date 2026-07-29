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
	var fences fenceState
	var inlineCode inlineCodeSpanState
	for i, line := range lines {
		lineNo := i + 1
		if fences.consume(line) {
			continue
		}
		scannable := stripInlineCodeSpans(line, lines[i+1:], &inlineCode)
		if headingH2Plus.MatchString(scannable) {
			break
		}
		if lineNo > headerZoneMaxLines {
			break
		}
		if matches := boldField.FindAllString(scannable, -1); len(matches) >= 2 {
			violations = append(violations, Violation{Path: path, Line: lineNo, Text: strings.TrimSpace(line)})
		}
	}
	return violations
}

type fenceState struct {
	marker           byte
	length           int
	container        fenceContainerContext
	listContinuation fenceContainerContext
}

func (s *fenceState) consume(line string) bool {
	if s.marker != 0 {
		if s.container.contains(line) {
			marker, length, trailing, isFence := s.container.delimiter(line)
			if isFence && marker == s.marker && length >= s.length && strings.TrimSpace(trailing) == "" {
				s.marker = 0
				s.length = 0
				s.container = fenceContainerContext{}
			}
			return true
		}
		// A fenced block nested in a quote or list ends when that container
		// ends, even if it never supplied a closing fence.
		s.marker = 0
		s.length = 0
		s.container = fenceContainerContext{}
	}
	s.updateListContinuation(line)
	marker, length, _, isFence := fenceDelimiter(line)
	if !isFence {
		return false
	}
	s.marker = marker
	s.length = length
	s.container = parseFenceContainerContext(line)
	if !s.container.hasList && s.listContinuation.hasList {
		if _, ok := s.listContinuation.contentStart(line); ok {
			s.container = s.listContinuation
		}
	}
	return true
}

func (s *fenceState) updateListContinuation(line string) {
	lineContainer := parseFenceContainerContext(line)
	switch {
	case lineContainer.hasList:
		s.listContinuation = lineContainer
	case strings.TrimSpace(line) == "":
		// Blank lines do not end a list container.
	case s.listContinuation.hasList:
		if _, ok := s.listContinuation.contentStart(line); !ok {
			s.listContinuation = fenceContainerContext{}
		}
	}
}

type fenceContainerContext struct {
	contentOffset int
	quoteDepth    int
	hasList       bool
	listIndent    int
}

func (c fenceContainerContext) contains(line string) bool {
	if c.quoteDepth == 0 && !c.hasList {
		return true
	}
	if strings.TrimSpace(line) == "" {
		// An unmarked blank line ends a blockquote container. Blank lines may
		// remain inside a list container without repeating its indentation.
		return c.quoteDepth == 0 && c.hasList
	}
	_, ok := c.contentStart(line)
	return ok
}

func (c fenceContainerContext) contentStart(line string) (int, bool) {
	if c.quoteDepth == 0 && !c.hasList {
		return 0, true
	}
	offset, ok := consumeQuotePrefix(line, c.quoteDepth)
	if !ok {
		return 0, false
	}
	if !c.hasList {
		return offset, true
	}
	cursor := offset
	indent := 0
	for cursor < len(line) {
		switch line[cursor] {
		case ' ':
			indent++
			cursor++
		case '\t':
			indent += 4
			cursor++
		default:
			if indent < c.listIndent {
				return 0, false
			}
			return cursor, true
		}
	}
	return len(line), true
}

func (c fenceContainerContext) delimiter(line string) (byte, int, string, bool) {
	offset, ok := c.contentStart(line)
	if !ok {
		return 0, 0, "", false
	}
	return fenceDelimiterAt(line, offset)
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
	if context.hasList {
		quoteOffset, quoteOK := consumeQuotePrefix(line, context.quoteDepth)
		if quoteOK {
			context.listIndent = context.contentOffset - quoteOffset
		}
	}
	return context
}

func fenceDelimiter(line string) (byte, int, string, bool) {
	offset, ok := fenceContentOffset(line)
	if !ok || offset == len(line) {
		return 0, 0, "", false
	}
	return parseFenceDelimiter(line, offset)
}

func fenceDelimiterAt(line string, offset int) (byte, int, string, bool) {
	offset, ok := skipFenceIndent(line, offset)
	if !ok || offset == len(line) {
		return 0, 0, "", false
	}
	return parseFenceDelimiter(line, offset)
}

func parseFenceDelimiter(line string, offset int) (byte, int, string, bool) {
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

func consumeQuotePrefix(line string, depth int) (int, bool) {
	offset := 0
	for range depth {
		var ok bool
		offset, ok = skipFenceIndent(line, offset)
		if !ok || offset >= len(line) || line[offset] != '>' {
			return 0, false
		}
		offset++
		if offset < len(line) && (line[offset] == ' ' || line[offset] == '\t') {
			offset++
		}
	}
	return offset, true
}

type inlineCodeSpanState struct {
	delimiterLength int
}

func stripInlineCodeSpans(line string, followingLines []string, state *inlineCodeSpanState) string {
	masked := []byte(line)
	cursor := 0
	if state.delimiterLength > 0 {
		closer := matchingBacktickRun(line, 0, state.delimiterLength)
		if closer < 0 {
			for index := range masked {
				masked[index] = ' '
			}
			return string(masked)
		}
		spanEnd := closer + state.delimiterLength
		for index := range spanEnd {
			masked[index] = ' '
		}
		cursor = spanEnd
		state.delimiterLength = 0
	}
	for opener := cursor; opener < len(line); {
		if line[opener] != '`' || escapedAt(line, opener) {
			opener++
			continue
		}
		runEnd := opener
		for runEnd < len(line) && line[runEnd] == '`' {
			runEnd++
		}
		runLength := runEnd - opener
		closer := matchingBacktickRun(line, runEnd, runLength)
		if closer < 0 {
			if !hasMatchingBacktickRun(followingLines, runLength) {
				// Without a closer before the inline block ends, CommonMark
				// treats the opener as literal text.
				opener = runEnd
				continue
			}
			for index := opener; index < len(masked); index++ {
				masked[index] = ' '
			}
			state.delimiterLength = runLength
			break
		}
		spanEnd := closer + runLength
		for index := opener; index < spanEnd; index++ {
			masked[index] = ' '
		}
		opener = spanEnd
	}
	return string(masked)
}

func matchingBacktickRun(line string, offset, length int) int {
	for offset < len(line) {
		// Backslashes have no escaping meaning inside an already-open code
		// span, so a backslash-prefixed run is still a valid closer.
		if line[offset] != '`' {
			offset++
			continue
		}
		end := offset
		for end < len(line) && line[end] == '`' {
			end++
		}
		if end-offset == length {
			return offset
		}
		offset = end
	}
	return -1
}

func hasMatchingBacktickRun(lines []string, length int) bool {
	for _, line := range lines {
		if matchingBacktickRun(line, 0, length) >= 0 {
			return true
		}
	}
	return false
}

func escapedAt(line string, offset int) bool {
	backslashes := 0
	for offset > 0 && line[offset-1] == '\\' {
		backslashes++
		offset--
	}
	return backslashes%2 == 1
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
