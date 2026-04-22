// Package hooks provides PreToolUse hook checks for Claude Code.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckCommitMessage inspects a commit message for docs-only changes that
// reference unbuilt features. It returns a warning string and whether the
// commit should be blocked (currently always false — warn only).
func CheckCommitMessage(msg string, repoRoot string) (warning string, block bool) {
	if !isAspirationDocCommit(msg) {
		return "", false
	}

	feature := extractFeatureName(msg)
	if feature == "" {
		return "", false
	}

	if codeExists(feature, repoRoot) {
		return "", false
	}

	return fmt.Sprintf(
		"This documents an unbuilt feature. Implement before documenting. "+
			"(no Go code found matching %q)", feature), false
}

// isAspirationDocCommit returns true when the commit message starts with
// "docs:" and contains aspirational keywords like "add", "design", or "plan".
func isAspirationDocCommit(msg string) bool {
	lower := strings.ToLower(strings.TrimSpace(msg))
	if !strings.HasPrefix(lower, "docs:") {
		return false
	}
	body := lower[len("docs:"):]
	for _, kw := range []string{"add", "design", "plan"} {
		if strings.Contains(body, kw) {
			return true
		}
	}
	return false
}

// extractFeatureName pulls a likely feature/package name from the commit
// message by taking the last significant word after the "docs:" prefix,
// skipping common verbs and articles.
func extractFeatureName(msg string) string {
	trimmed := strings.TrimSpace(msg)
	// Strip the "docs:" prefix.
	idx := strings.Index(strings.ToLower(trimmed), "docs:")
	if idx < 0 {
		return ""
	}
	body := strings.TrimSpace(trimmed[idx+len("docs:"):])

	skip := map[string]bool{
		"add": true, "design": true, "plan": true,
		"for": true, "the": true, "a": true, "an": true,
		"new": true, "initial": true, "draft": true,
	}

	words := strings.Fields(strings.ToLower(body))
	for _, w := range words {
		// Strip trailing punctuation.
		w = strings.TrimRight(w, ".,;:!?")
		if w == "" || skip[w] {
			continue
		}
		return w
	}
	return ""
}

// codeExists checks whether any .go file (non-test) exists under repoRoot
// whose path contains the feature name.
func codeExists(feature string, repoRoot string) bool {
	if repoRoot == "" {
		return false
	}
	found := false
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable dirs
		}
		if d.IsDir() {
			name := d.Name()
			// Skip hidden dirs and vendor.
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") &&
			!strings.HasSuffix(path, "_test.go") &&
			strings.Contains(strings.ToLower(path), feature) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
