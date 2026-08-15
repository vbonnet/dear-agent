// Package markdownvisible identifies Markdown source text that is visible as
// prose. It uses the repository's CommonMark parser so policy tools do not
// mistake indented or fenced code, HTML comments, or delimiter text for
// normative requirements and traceability.
package markdownvisible

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Line is a source line with comment and code ranges blanked. Visible is false
// when the line contains only excluded material (and optional indentation).
// Ordinary blank lines remain visible so callers can preserve paragraph
// boundaries.
type Line struct {
	Text    string
	Visible bool
}

// Lines parses one complete Markdown document and returns newline-preserving
// visible source lines. Parsing the complete document is load-bearing:
// CommonMark code spans take precedence over apparent HTML comment markers,
// and nested/indented code blocks cannot be recognized correctly one line at a
// time.
func Lines(source []byte) []Line {
	hidden := make([]bool, len(source))
	document := goldmark.DefaultParser().Parse(text.NewReader(source))
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch current := node.(type) {
		case *ast.CodeBlock:
			hideSegmentLines(hidden, source, current.Lines())
		case *ast.FencedCodeBlock:
			hideSegmentLines(hidden, source, current.Lines())
			hideFenceOpening(hidden, source, current)
			hideFenceClosing(hidden, source, current)
		case *ast.HTMLBlock:
			// CommonMark has seven raw HTML block forms. None is normative
			// Markdown prose, so all must be excluded rather than only comments.
			hideSegmentLines(hidden, source, current.Lines())
			if current.HasClosure() {
				hidePhysicalLine(hidden, source, current.ClosureLine.Start)
			}
		case *ast.RawHTML:
			if strings.HasPrefix(strings.TrimSpace(string(current.Segments.Value(source))), "<!--") {
				hideSegments(hidden, current.Segments)
			}
		}
		return ast.WalkContinue, nil
	})
	return visibleLines(source, hidden)
}

func hideSegmentLines(hidden []bool, source []byte, segments *text.Segments) {
	for i := 0; i < segments.Len(); i++ {
		hidePhysicalLine(hidden, source, segments.At(i).Start)
	}
}

func hideSegments(hidden []bool, segments *text.Segments) {
	for i := 0; i < segments.Len(); i++ {
		segment := segments.At(i)
		hideRange(hidden, segment.Start, segment.Stop)
	}
}

func hideRange(hidden []bool, start, stop int) {
	if start < 0 {
		start = 0
	}
	if stop > len(hidden) {
		stop = len(hidden)
	}
	for i := start; i < stop; i++ {
		hidden[i] = true
	}
}

func hidePhysicalLine(hidden []bool, source []byte, offset int) {
	start := lineStart(source, offset)
	hideRange(hidden, start, physicalLineStop(source, start, true))
}

func hideFenceOpening(hidden []bool, source []byte, fence *ast.FencedCodeBlock) {
	start, stop, _, _, ok := fenceOpening(source, fence)
	if !ok {
		return
	}
	hideRange(hidden, start, stop)
}

// hideFenceClosing masks a closing delimiter, which Goldmark intentionally
// does not retain in FencedCodeBlock.Lines(). The AST has already established
// that a block exists; this scan only identifies its first compatible closing
// delimiter after the opener. It mirrors CommonMark's same-character,
// at-least-opening-length, whitespace-only-tail rule.
func hideFenceClosing(hidden []bool, source []byte, fence *ast.FencedCodeBlock) {
	_, openingStop, marker, markerLength, ok := fenceOpening(source, fence)
	if !ok {
		return
	}
	contentLines := make(map[int]struct{}, fence.Lines().Len())
	for i := 0; i < fence.Lines().Len(); i++ {
		contentLines[lineStart(source, fence.Lines().At(i).Start)] = struct{}{}
	}
	boundary := len(source)
	if next := nextSourcePosition(fence); next >= 0 {
		boundary = lineStart(source, next)
	}
	for searchStart := openingStop; searchStart < boundary; {
		lineStop := physicalLineStop(source, searchStart, false)
		_, isContent := contentLines[searchStart]
		if !isContent && compatibleFenceClosing(source[searchStart:lineStop], marker, markerLength) {
			hideRange(hidden, searchStart, physicalLineStop(source, searchStart, true))
			return
		}
		next := physicalLineStop(source, searchStart, true)
		if next <= searchStart {
			return
		}
		searchStart = next
	}
}

func fenceOpening(source []byte, fence *ast.FencedCodeBlock) (int, int, byte, int, bool) {
	offset := fence.Pos()
	if offset < 0 || offset >= len(source) || (source[offset] != '`' && source[offset] != '~') {
		return 0, 0, 0, 0, false
	}
	start := lineStart(source, offset)
	stop := physicalLineStop(source, start, true)
	marker := source[offset]
	length := 0
	for offset+length < len(source) && source[offset+length] == marker {
		length++
	}
	if length < 3 {
		return 0, 0, 0, 0, false
	}
	return start, stop, marker, length, true
}

func compatibleFenceClosing(line []byte, marker byte, markerLength int) bool {
	markerOffset := 0
	for markerOffset < len(line) {
		character := line[markerOffset]
		if character != ' ' && character != '\t' && character != '>' {
			break
		}
		markerOffset++
	}
	if markerOffset >= len(line) || line[markerOffset] != marker {
		return false
	}
	length := 0
	for markerOffset+length < len(line) && line[markerOffset+length] == marker {
		length++
	}
	if length < markerLength {
		return false
	}
	for _, character := range line[markerOffset+length:] {
		if character != ' ' && character != '\t' && character != '\r' {
			return false
		}
	}
	return true
}

func nextSourcePosition(node ast.Node) int {
	for current := node; current != nil; current = current.Parent() {
		if next := current.NextSibling(); next != nil {
			return firstSourcePosition(next)
		}
	}
	return -1
}

func firstSourcePosition(node ast.Node) int {
	if position := node.Pos(); position >= 0 {
		return position
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if position := firstSourcePosition(child); position >= 0 {
			return position
		}
	}
	return -1
}

func lineStart(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	if offset < 0 {
		return 0
	}
	if previous := bytes.LastIndexByte(source[:offset], '\n'); previous >= 0 {
		return previous + 1
	}
	return 0
}

func physicalLineStop(source []byte, start int, includeNewline bool) int {
	if start < 0 {
		start = 0
	}
	if start >= len(source) {
		return len(source)
	}
	relative := bytes.IndexByte(source[start:], '\n')
	if relative < 0 {
		return len(source)
	}
	stop := start + relative
	if includeNewline {
		stop++
	}
	return stop
}

func visibleLines(source []byte, hidden []bool) []Line {
	rawLines := bytes.Split(source, []byte{'\n'})
	lines := make([]Line, 0, len(rawLines))
	offset := 0
	for _, raw := range rawLines {
		masked := append([]byte(nil), raw...)
		hadHidden := offset+len(raw) < len(hidden) && hidden[offset+len(raw)]
		hasVisibleText := false
		for i := range raw {
			if hidden[offset+i] {
				masked[i] = ' '
				hadHidden = true
				continue
			}
			if raw[i] != ' ' && raw[i] != '\t' && raw[i] != '\r' {
				hasVisibleText = true
			}
		}
		lines = append(lines, Line{Text: string(masked), Visible: !hadHidden || hasVisibleText})
		offset += len(raw) + 1
	}
	return lines
}
