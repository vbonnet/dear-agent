package instructionlint

import (
	"bytes"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// SegmentKind distinguishes prose from command-shaped Markdown content.
type SegmentKind string

const (
	// SegmentProse is ordinary Markdown outside executable fences.
	SegmentProse SegmentKind = "prose"
	// SegmentInline is inline code that may contain a command form.
	SegmentInline SegmentKind = "inline"
	// SegmentShell is content in a shell-language fenced block.
	SegmentShell SegmentKind = "shell"
)

// Segment is one normalized, line-addressable policy input.
type Segment struct {
	Kind SegmentKind
	Line int
	Text string
}

func parseSegments(source []byte) []Segment {
	root := goldmark.New().Parser().Parse(text.NewReader(source))
	classified, inline := classifyMarkdown(root, source)
	segments := sourceSegments(source, classified)
	segments = append(segments, inline...)
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].Line != segments[j].Line {
			return segments[i].Line < segments[j].Line
		}
		if segments[i].Kind != segments[j].Kind {
			return segments[i].Kind < segments[j].Kind
		}
		return segments[i].Text < segments[j].Text
	})
	return segments
}

func classifyMarkdown(root ast.Node, source []byte) (map[int]SegmentKind, []Segment) {
	classified := map[int]SegmentKind{}
	var inline []Segment
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.FencedCodeBlock:
			classifyFence(classified, typed, source)
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			if segment, ok := codeSpanSegment(typed, source); ok {
				inline = append(inline, segment)
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return classified, inline
}

func classifyFence(classified map[int]SegmentKind, block *ast.FencedCodeBlock, source []byte) {
	kind := SegmentKind("skip")
	if shellLanguage(string(block.Language(source))) {
		kind = SegmentShell
	}
	for i := 0; i < block.Lines().Len(); i++ {
		segment := block.Lines().At(i)
		startLine := sourceLine(source, segment.Start)
		value := segment.Value(source)
		lineCount := bytes.Count(value, []byte{'\n'})
		if !bytes.HasSuffix(value, []byte{'\n'}) {
			lineCount++
		}
		for offset := 0; offset < lineCount; offset++ {
			classified[startLine+offset] = kind
		}
	}
}

func codeSpanSegment(span *ast.CodeSpan, source []byte) (Segment, bool) {
	var value strings.Builder
	line := 1
	for child := span.FirstChild(); child != nil; child = child.NextSibling() {
		textNode, ok := child.(*ast.Text)
		if !ok {
			continue
		}
		if value.Len() == 0 {
			line = sourceLine(source, textNode.Segment.Start)
		}
		value.Write(textNode.Segment.Value(source))
	}
	normalized := strings.TrimSpace(value.String())
	return Segment{Kind: SegmentInline, Line: line, Text: normalized}, normalized != ""
}

func sourceSegments(source []byte, classified map[int]SegmentKind) []Segment {
	lines := strings.Split(string(source), "\n")
	segments := make([]Segment, 0, len(lines))
	for i, raw := range lines {
		line := i + 1
		normalized := strings.TrimSpace(raw)
		if strings.HasPrefix(normalized, "```") || strings.HasPrefix(normalized, "~~~") {
			continue
		}
		kind, classifiedLine := classified[line]
		if classifiedLine && kind == "skip" {
			continue
		}
		if !classifiedLine {
			kind = SegmentProse
		}
		if normalized != "" {
			segments = append(segments, Segment{Kind: kind, Line: line, Text: normalized})
		}
	}
	return segments
}

func shellLanguage(language string) bool {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "bash", "console", "sh", "shell", "zsh":
		return true
	default:
		return false
	}
}

func sourceLine(source []byte, offset int) int {
	if offset < 0 {
		return 1
	}
	if offset > len(source) {
		offset = len(source)
	}
	return bytes.Count(source[:offset], []byte{'\n'}) + 1
}
