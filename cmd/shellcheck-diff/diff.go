package main

import (
	"fmt"
	"strconv"
	"strings"
)

// TouchedLines records, per file path, the destination line numbers a diff
// added or rewrote. Deleted lines are deliberately absent: a finding cannot sit
// on a line that no longer exists.
type TouchedLines map[string]map[int]bool

// Contains reports whether the change touched file at line. Paths are compared
// after stripping git's "b/" destination prefix so the keys match the paths
// ShellCheck reports.
func (t TouchedLines) Contains(file string, line int) bool {
	return t[normalizePath(file)][line]
}

func normalizePath(path string) string {
	path = strings.TrimPrefix(path, "./")
	return path
}

// parseTouchedLines reads a unified diff and returns the destination lines each
// hunk introduces.
//
// It is written against `git diff -U0` output, where every hunk header's line
// count is exactly the number of added lines, so no context filtering is
// needed. A larger -U value still parses correctly but attributes surrounding
// context lines to the change, which would over-report; the caller is expected
// to pass -U0.
func parseTouchedLines(diff string) (TouchedLines, error) {
	touched := TouchedLines{}
	current := ""
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			// /dev/null is the destination of a pure deletion; it owns no
			// lines a finding could land on.
			if path == "/dev/null" {
				current = ""
				continue
			}
			current = normalizePath(strings.TrimPrefix(path, "b/"))
		case strings.HasPrefix(line, "@@"):
			if current == "" {
				continue
			}
			start, count, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			if touched[current] == nil {
				touched[current] = map[int]bool{}
			}
			for i := range count {
				touched[current][start+i] = true
			}
		}
	}
	return touched, nil
}

// parseHunkHeader extracts the destination start line and line count from a
// unified-diff hunk header such as "@@ -12,0 +13,4 @@ func main() {".
func parseHunkHeader(header string) (start, count int, err error) {
	for field := range strings.FieldsSeq(header) {
		if !strings.HasPrefix(field, "+") {
			continue
		}
		spec := strings.TrimPrefix(field, "+")
		startText, countText, hasCount := strings.Cut(spec, ",")
		start, err = strconv.Atoi(startText)
		if err != nil {
			return 0, 0, fmt.Errorf("malformed hunk header %q: %w", header, err)
		}
		count = 1
		if hasCount {
			count, err = strconv.Atoi(countText)
			if err != nil {
				return 0, 0, fmt.Errorf("malformed hunk header %q: %w", header, err)
			}
		}
		return start, count, nil
	}
	return 0, 0, fmt.Errorf("hunk header %q has no destination range", header)
}
