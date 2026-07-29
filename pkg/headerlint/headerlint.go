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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
	atxHeading    = regexp.MustCompile(`^ {0,3}#{1,6}([ \t]|$)`)
	setextHeading = regexp.MustCompile(`^ {0,3}(?:=+|-+)[ \t]*$`)
	thematicBreak = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:_[ \t]*){3,}|(?:-[ \t]*){3,})$`)
	htmlBlockTag  = regexp.MustCompile(`(?i)^</?(?:address|article|aside|base|basefont|blockquote|body|caption|center|col|colgroup|dd|details|dialog|dir|div|dl|dt|fieldset|figcaption|figure|footer|form|frame|frameset|h[1-6]|head|header|hr|html|iframe|legend|li|link|main|menu|menuitem|nav|noframes|ol|optgroup|option|p|param|search|section|summary|table|tbody|td|tfoot|th|thead|title|tr|track|ul)(?:[ \t/>]|$)`)
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
	var violations []Violation
	lines := strings.Split(string(data), "\n")
	for i := range lines {
		// A CRLF checkout leaves '\r' behind when splitting on '\n'. Markdown
		// block parsing is line-ending agnostic, so normalize before matching
		// headings, fences, or inline-code boundaries.
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	var fences fenceState
	var htmlBlocks htmlBlockState
	var inlineCode inlineCodeSpanState
	paragraphOpen := false
	var paragraphContainer fenceContainerContext
	for i, line := range lines {
		lineNo := i + 1
		if fences.consume(line, paragraphOpen) {
			// A fenced block is a Markdown block boundary. An inline-code
			// opener in earlier prose cannot consume a closer from this block
			// or retain span state across it.
			inlineCode = inlineCodeSpanState{}
			paragraphOpen = false
			paragraphContainer = fenceContainerContext{}
			continue
		}
		if htmlBlocks.consume(line) {
			inlineCode = inlineCodeSpanState{}
			paragraphOpen = false
			paragraphContainer = fenceContainerContext{}
			continue
		}
		openerContainer := inlineOpenerContainer(line, paragraphOpen, paragraphContainer)
		scannable := stripInlineCodeSpans(line, lines[i+1:], &inlineCode, openerContainer)
		if matchesContainerATXHeading(scannable, headingH2Plus, paragraphOpen) ||
			matchesListContinuationATXHeading(scannable, headingH2Plus, fences.listContinuation) {
			break
		}
		if lineNo > headerZoneMaxLines {
			break
		}
		if unescapedBoldFieldCount(scannable) >= 2 {
			violations = append(violations, Violation{Path: path, Line: lineNo, Text: strings.TrimSpace(line)})
		}
		paragraphOpen = lineLeavesParagraphOpen(scannable)
		if paragraphOpen {
			paragraphContainer = openerContainer
		} else {
			paragraphContainer = fenceContainerContext{}
		}
	}
	return violations
}

func inlineOpenerContainer(
	line string,
	paragraphOpen bool,
	paragraphContainer fenceContainerContext,
) fenceContainerContext {
	lineContainer := parseFenceContainerContext(line)
	if paragraphOpen &&
		lineContainer.quoteDepth == 0 && !lineContainer.hasList &&
		(paragraphContainer.quoteDepth != 0 || paragraphContainer.hasList) &&
		!inlineCodeBlockBoundary(line, paragraphContainer) {
		// A container paragraph may continue lazily without repeating its
		// quote/list prefix. An inline-code opener on that lazy line still
		// belongs to the paragraph's original container.
		return paragraphContainer
	}
	return lineContainer
}

type fenceState struct {
	marker           byte
	length           int
	container        fenceContainerContext
	listContinuation fenceContainerContext
}

func (s *fenceState) consume(line string, paragraphOpen bool) bool {
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
	marker, length, container, isFence := s.openingDelimiter(line, paragraphOpen)
	if !isFence {
		return false
	}
	s.marker = marker
	s.length = length
	s.container = container
	return true
}

func (s *fenceState) openingDelimiter(line string, paragraphOpen bool) (byte, int, fenceContainerContext, bool) {
	marker, length, isFence := fenceDelimiterWithParagraph(line, paragraphOpen)
	if isFence {
		container := parseFenceContainerContext(line)
		if !container.hasList && s.listContinuation.hasList {
			if _, ok := s.listContinuation.contentStart(line); ok {
				container = s.listContinuation
			}
		}
		return marker, length, container, true
	}
	if paragraphOpen && lineStartsNonInterruptingOrderedList(line) {
		return 0, 0, fenceContainerContext{}, false
	}
	if !s.listContinuation.hasList {
		return 0, 0, fenceContainerContext{}, false
	}
	offset, ok := s.listContinuation.contentStart(line)
	if !ok {
		return 0, 0, fenceContainerContext{}, false
	}
	marker, length, _, isFence = fenceDelimiterAt(line, offset)
	return marker, length, s.listContinuation, isFence
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
	for cursor < len(line) && indent < c.listIndent {
		switch line[cursor] {
		case ' ':
			indent++
			cursor++
		case '\t':
			if indent+4 > c.listIndent {
				return cursor, true
			}
			indent += 4
			cursor++
		default:
			return 0, false
		}
	}
	if indent < c.listIndent {
		return 0, false
	}
	return cursor, true
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

func fenceDelimiter(line string) (byte, int, bool) {
	return fenceDelimiterWithParagraph(line, false)
}

func fenceDelimiterWithParagraph(line string, paragraphOpen bool) (byte, int, bool) {
	offset, ok := containerContentOffset(line, paragraphOpen)
	if !ok || offset == len(line) {
		return 0, 0, false
	}
	marker, length, _, isFence := parseFenceDelimiter(line, offset)
	return marker, length, isFence
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
	container       fenceContainerContext
}

func stripInlineCodeSpans(
	line string,
	followingLines []string,
	state *inlineCodeSpanState,
	openerContainer fenceContainerContext,
) string {
	masked := []byte(line)
	cursor, fullyMasked := maskCarriedInlineCodeSpan(line, masked, state)
	if fullyMasked {
		return string(masked)
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
			if !hasMatchingBacktickRun(followingLines, runLength, openerContainer) {
				// Without a closer before the inline block ends, CommonMark
				// treats the opener as literal text.
				opener = runEnd
				continue
			}
			for index := opener; index < len(masked); index++ {
				masked[index] = ' '
			}
			state.delimiterLength = runLength
			state.container = openerContainer
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

func maskCarriedInlineCodeSpan(line string, masked []byte, state *inlineCodeSpanState) (int, bool) {
	if state.delimiterLength > 0 && inlineCodeBlockBoundary(line, state.container) {
		state.delimiterLength = 0
		state.container = fenceContainerContext{}
	}
	if state.delimiterLength == 0 {
		return 0, false
	}
	closer := matchingBacktickRun(line, 0, state.delimiterLength)
	if closer < 0 {
		for index := range masked {
			masked[index] = ' '
		}
		return 0, true
	}
	spanEnd := closer + state.delimiterLength
	for index := range spanEnd {
		masked[index] = ' '
	}
	state.delimiterLength = 0
	state.container = fenceContainerContext{}
	return spanEnd, false
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

func hasMatchingBacktickRun(lines []string, length int, container fenceContainerContext) bool {
	for _, line := range lines {
		if inlineCodeBlockBoundary(line, container) {
			return false
		}
		if matchingBacktickRun(line, 0, length) >= 0 {
			return true
		}
	}
	return false
}

func inlineCodeBlockBoundary(line string, container fenceContainerContext) bool {
	if containerLineIsBlank(line, container) {
		return true
	}
	if !sameInlineCodeContainer(container, line) {
		return true
	}
	if matchesContainerATXHeading(line, atxHeading, true) {
		return true
	}
	if matchesContainerBlockPattern(line, container, setextHeading) {
		return true
	}
	if matchesContainerBlockPattern(line, container, thematicBreak) {
		return true
	}
	if isInterruptingHTMLBlockStart(line, container) {
		return true
	}
	if _, _, isFence := fenceDelimiterWithParagraph(line, true); isFence {
		return true
	}
	trimmed := strings.TrimLeft(line, " \t")
	_, _, _, isFence := parseFenceDelimiter(trimmed, 0)
	return isFence
}

func containerLineIsBlank(line string, container fenceContainerContext) bool {
	offset, ok := container.contentStart(line)
	if !ok {
		return strings.TrimSpace(line) == ""
	}
	return strings.TrimSpace(line[offset:]) == ""
}

func matchesContainerBlockPattern(line string, container fenceContainerContext, pattern *regexp.Regexp) bool {
	offset, ok := inlineCodeContentStart(container, line)
	return ok && pattern.MatchString(line[offset:])
}

func unescapedBoldFieldCount(line string) int {
	count := 0
	for _, match := range boldField.FindAllStringIndex(line, -1) {
		if !escapedAt(line, match[0]) {
			count++
		}
	}
	return count
}

func sameInlineCodeContainer(container fenceContainerContext, line string) bool {
	if container.quoteDepth == 0 && !container.hasList {
		if lineStartsNonInterruptingOrderedList(line) {
			return true
		}
		lineContainer := parseFenceContainerContext(line)
		return lineContainer.quoteDepth == 0 && !lineContainer.hasList
	}
	contentStart, ok := inlineCodeContentStart(container, line)
	if !ok {
		return false
	}
	nested := parseFenceContainerContext(line[contentStart:])
	return nested.quoteDepth == 0 && !nested.hasList
}

// inlineCodeContentStart recognizes both an explicitly repeated container
// prefix and CommonMark's lazy continuation of a container paragraph. A lazy
// line may omit the quote marker or list indentation, but it may not introduce
// a different container. Block starters are rejected separately by
// inlineCodeBlockBoundary after this returns their content offset.
func inlineCodeContentStart(container fenceContainerContext, line string) (int, bool) {
	if offset, ok := container.contentStart(line); ok {
		return offset, true
	}
	if (container.quoteDepth == 0 && !container.hasList) || strings.TrimSpace(line) == "" {
		return 0, false
	}
	lineContainer := parseFenceContainerContext(line)
	if lineContainer.quoteDepth != 0 || lineContainer.hasList {
		return 0, false
	}
	return 0, true
}

func matchesContainerATXHeading(line string, pattern *regexp.Regexp, paragraphOpen bool) bool {
	offset, ok := headingContentOffset(line, paragraphOpen)
	if !ok {
		return false
	}
	return pattern.MatchString(line[offset:])
}

func matchesListContinuationATXHeading(line string, pattern *regexp.Regexp, container fenceContainerContext) bool {
	if !container.hasList {
		return false
	}
	offset, ok := container.contentStart(line)
	return ok && pattern.MatchString(line[offset:])
}

func isInterruptingHTMLBlockStart(line string, container fenceContainerContext) bool {
	offset, ok := inlineCodeContentStart(container, line)
	if !ok {
		return false
	}
	content, ok := commonMarkBlockContent(line[offset:])
	if !ok {
		return false
	}
	return isHTMLBlockStart(content)
}

type htmlBlockState struct {
	endMarker  string
	untilBlank bool
	container  fenceContainerContext
}

func (s *htmlBlockState) consume(line string) bool {
	if (s.endMarker != "" || s.untilBlank) && !s.container.contains(line) {
		// Raw HTML nested in a quote or list cannot outlive that Markdown
		// container. Reprocess the first line outside it as ordinary Markdown.
		*s = htmlBlockState{}
	}
	if s.endMarker != "" {
		if strings.Contains(strings.ToLower(line), s.endMarker) {
			*s = htmlBlockState{}
		}
		return true
	}
	if s.untilBlank {
		if containerLineIsBlank(line, s.container) {
			*s = htmlBlockState{}
		}
		return true
	}

	offset, ok := containerContentOffset(line, false)
	if !ok {
		return false
	}
	content, ok := commonMarkBlockContent(line[offset:])
	if !ok {
		return false
	}
	lower := strings.ToLower(content)
	endMarker := htmlBlockEndMarker(content, lower)
	if endMarker != "" {
		if !strings.Contains(lower, endMarker) {
			s.endMarker = endMarker
			s.container = parseFenceContainerContext(line)
		}
		return true
	}
	if htmlBlockTag.MatchString(content) {
		s.untilBlank = true
		s.container = parseFenceContainerContext(line)
		return true
	}
	return false
}

func htmlBlockEndMarker(content, lower string) string {
	for _, tag := range []string{"script", "pre", "style", "textarea"} {
		prefix := "<" + tag
		if strings.HasPrefix(lower, prefix) && htmlTagBoundary(lower, len(prefix)) {
			return "</" + tag + ">"
		}
	}
	switch {
	case strings.HasPrefix(content, "<!--"):
		return "-->"
	case strings.HasPrefix(content, "<?"):
		return "?>"
	case strings.HasPrefix(lower, "<![cdata["):
		return "]]>"
	case len(content) > 2 && strings.HasPrefix(content, "<!") &&
		content[2] >= 'A' && content[2] <= 'Z':
		return ">"
	default:
		return ""
	}
}

func commonMarkBlockContent(content string) (string, bool) {
	indent := 0
	for indent < len(content) && indent < 4 && content[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return "", false
	}
	return content[indent:], true
}

func isHTMLBlockStart(content string) bool {
	lower := strings.ToLower(content)
	for _, tag := range []string{"script", "pre", "style", "textarea"} {
		prefix := "<" + tag
		if strings.HasPrefix(lower, prefix) && htmlTagBoundary(lower, len(prefix)) {
			return true
		}
	}
	if strings.HasPrefix(content, "<!--") || strings.HasPrefix(content, "<?") ||
		strings.HasPrefix(lower, "<![cdata[") {
		return true
	}
	if len(content) > 2 && strings.HasPrefix(content, "<!") &&
		content[2] >= 'A' && content[2] <= 'Z' {
		return true
	}
	return htmlBlockTag.MatchString(content)
}

func htmlTagBoundary(value string, offset int) bool {
	if offset == len(value) {
		return true
	}
	switch value[offset] {
	case ' ', '\t', '>', '/':
		return true
	default:
		return false
	}
}

func lineLeavesParagraphOpen(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	if matchesContainerATXHeading(line, atxHeading, false) {
		return false
	}
	container := parseFenceContainerContext(line)
	if matchesContainerBlockPattern(line, container, setextHeading) ||
		matchesContainerBlockPattern(line, container, thematicBreak) {
		return false
	}
	if _, _, isFence := fenceDelimiter(line); isFence {
		return false
	}
	return true
}

func escapedAt(line string, offset int) bool {
	backslashes := 0
	for offset > 0 && line[offset-1] == '\\' {
		backslashes++
		offset--
	}
	return backslashes%2 == 1
}

// headingContentOffset applies CommonMark's paragraph-interruption rule while
// stripping block/list containers before an ATX heading. An ordered list whose
// start is not 1 cannot interrupt a paragraph, so text such as "2. ## literal"
// remains paragraph content instead of becoming a nested heading.
func headingContentOffset(line string, paragraphOpen bool) (int, bool) {
	return containerContentOffset(line, paragraphOpen)
}

func containerContentOffset(line string, paragraphOpen bool) (int, bool) {
	offset, ok := skipFenceIndent(line, 0)
	if !ok {
		return 0, false
	}
	for offset < len(line) {
		if paragraphOpen && nonInterruptingOrderedListMarker(line, offset) {
			return offset, true
		}
		markerEnd, found := fenceContainerMarkerEnd(line, offset)
		if !found {
			return offset, true
		}
		// A valid block quote, unordered-list marker, or start-1 ordered marker
		// interrupts the old paragraph. Nested containers are therefore parsed
		// in fresh block context.
		paragraphOpen = false
		offset = markerEnd
		var indentOK bool
		offset, indentOK = skipFenceIndent(line, offset)
		if !indentOK {
			return 0, false
		}
	}
	return offset, true
}

func nonInterruptingOrderedListMarker(line string, offset int) bool {
	if offset >= len(line) || line[offset] < '0' || line[offset] > '9' {
		return false
	}
	markerEnd := offset
	for markerEnd < len(line) && markerEnd-offset < 9 && line[markerEnd] >= '0' && line[markerEnd] <= '9' {
		markerEnd++
	}
	if markerEnd == offset || markerEnd >= len(line) || (line[markerEnd] != '.' && line[markerEnd] != ')') {
		return false
	}
	if _, ok := terminatedListMarkerEnd(line, markerEnd+1); !ok {
		return false
	}
	start, err := strconv.Atoi(line[offset:markerEnd])
	return err == nil && start != 1
}

func lineStartsNonInterruptingOrderedList(line string) bool {
	offset, ok := skipFenceIndent(line, 0)
	return ok && nonInterruptingOrderedListMarker(line, offset)
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
