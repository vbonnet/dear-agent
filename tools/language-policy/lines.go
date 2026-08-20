package main

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strings"
)

// CountLines returns the number of "countable" lines in a shell script: lines
// that are neither blank nor comment-only. A shebang is a comment by this
// definition and so is excluded too.
//
// This reproduces the semantics of the shell pipeline this checker replaced
// (`grep -v '^[[:space:]]*$' | grep -v '^[[:space:]]*#' | grep -v '^#!/'`) so
// the migration does not silently reclassify any script.
func CountLines(r io.Reader) (int, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	n := 0
	for sc.Scan() {
		t := strings.TrimLeft(sc.Text(), " \t")
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		n++
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("scanning lines: %w", err)
	}
	return n, nil
}

// excludedSegments are path segments that take a script out of policy scope at
// any depth. Matching on segments (not a prefix) keeps
// third_party/vendor/tool.sh excluded, which an anchored "vendor/" check would
// wrongly pull into scope.
var excludedSegments = []string{".archived", "node_modules", "vendor", ".worktrees"}

// InScope reports whether a path is subject to the policy.
func InScope(path string) bool {
	for seg := range strings.SplitSeq(normalizePath(path), "/") {
		if slices.Contains(excludedSegments, seg) {
			return false
		}
	}
	return true
}
