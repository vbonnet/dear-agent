// Package verify provides verify-related functionality.
package verify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// AssertionType indicates whether something should exist or not.
type AssertionType int

const (
	// Positive means the thing should exist/be present.
	Positive AssertionType = iota
	// Negative means the thing should NOT exist/be absent.
	Negative
)

// Assertion represents a single verifiable claim extracted from a prompt.
type Assertion struct {
	Type        AssertionType
	Description string // Human-readable description of what's being checked
	Pattern     string // Search pattern (grep-compatible)
	GlobPattern string // Optional glob pattern for file filtering (e.g., "*.go")
	PathCheck   string // Optional path that should exist or not exist
}

// Result represents the outcome of checking a single assertion.
type Result struct {
	Assertion Assertion
	Pass      bool
	Evidence  string // File path + line or explanation of why it passed/failed
}

// Report is the aggregate result of all assertions.
type Report struct {
	SessionID string
	Purpose   string
	Results   []Result
}

// Passed returns true if all assertions passed.
func (r *Report) Passed() bool {
	for _, result := range r.Results {
		if !result.Pass {
			return false
		}
	}
	return len(r.Results) > 0
}

// FailCount returns the number of failed assertions.
func (r *Report) FailCount() int {
	count := 0
	for _, result := range r.Results {
		if !result.Pass {
			count++
		}
	}
	return count
}

// PassCount returns the number of passed assertions.
func (r *Report) PassCount() int {
	count := 0
	for _, result := range r.Results {
		if result.Pass {
			count++
		}
	}
	return count
}

// ExtractAssertions parses a prompt/purpose string and extracts verifiable assertions.
func ExtractAssertions(purpose string) []Assertion {
	var assertions []Assertion

	// Pattern: "remove X" / "delete X" / "rip out X"
	assertions = append(assertions, extractRemovalAssertions(purpose)...)

	// Pattern: "add X" / "create X" / "implement X" / "build X"
	assertions = append(assertions, extractCreationAssertions(purpose)...)

	// Pattern: "fix X" / "repair X"
	assertions = append(assertions, extractFixAssertions(purpose)...)

	// Pattern: specific dependency removal (go.mod / package.json patterns)
	assertions = append(assertions, extractDependencyAssertions(purpose)...)

	// Pattern: directory deletion
	assertions = append(assertions, extractDirectoryAssertions(purpose)...)

	return assertions
}

// negativePatterns matches removal-type phrases.
var negativePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:remove|delete|rip\s+out|eliminate|drop|strip)\s+(?:the\s+)?(.+?)(?:\s+(?:from|in|SDK|dependency|package|import|usage).*)?$`),
}

// positivePatterns matches creation-type phrases.
var positivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:add|create|implement|build|introduce|write)\s+(?:a\s+|the\s+|new\s+)?(.+?)(?:\s+(?:to|in|for|at|that|which).*)?$`),
}

// fixPatterns matches fix-type phrases.
var fixPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:fix|repair|correct|patch|update)\s+(?:the\s+)?(\w+(?:\.\w+)?)`),
}

func extractRemovalAssertions(original string) []Assertion {
	var assertions []Assertion

	for _, pat := range negativePatterns {
		lines := strings.Split(original, "\n")
		for _, line := range lines {
			matches := pat.FindStringSubmatch(line)
			if len(matches) >= 2 {
				target := strings.TrimSpace(matches[1])
				if target == "" || len(target) < 3 {
					continue
				}
				// Extract searchable tokens from the target
				searchPattern := extractSearchPattern(target)
				if searchPattern == "" {
					continue
				}
				assertions = append(assertions, Assertion{
					Type:        Negative,
					Description: fmt.Sprintf("should be removed: %s", target),
					Pattern:     searchPattern,
				})
			}
		}
	}

	return assertions
}

func extractCreationAssertions(original string) []Assertion {
	var assertions []Assertion

	for _, pat := range positivePatterns {
		lines := strings.Split(original, "\n")
		for _, line := range lines {
			matches := pat.FindStringSubmatch(line)
			if len(matches) >= 2 {
				target := strings.TrimSpace(matches[1])
				if target == "" || len(target) < 3 {
					continue
				}
				searchPattern := extractSearchPattern(target)
				if searchPattern == "" {
					continue
				}
				assertions = append(assertions, Assertion{
					Type:        Positive,
					Description: fmt.Sprintf("should exist: %s", target),
					Pattern:     searchPattern,
				})
			}
		}
	}

	return assertions
}

func extractFixAssertions(original string) []Assertion {
	var assertions []Assertion

	for _, pat := range fixPatterns {
		lines := strings.Split(original, "\n")
		for _, line := range lines {
			matches := pat.FindStringSubmatch(line)
			if len(matches) >= 2 {
				target := strings.TrimSpace(matches[1])
				if target == "" || len(target) < 3 {
					continue
				}
				// For fix assertions, the function/thing should still exist
				assertions = append(assertions, Assertion{
					Type:        Positive,
					Description: fmt.Sprintf("should be fixed (still exists): %s", target),
					Pattern:     target,
				})
			}
		}
	}

	return assertions
}

// depRemovalContext matches lines with removal verbs
var depRemovalContext = regexp.MustCompile(`(?i)\b(?:remove|delete|rip\s+out|eliminate|drop)\b`)

// depModulePath matches module-path-like strings (e.g., go.temporal.io/sdk)
var depModulePath = regexp.MustCompile(`((?:[a-z0-9-]+\.)+(?:io|com|org|net|dev)(?:/[\w.-]+)*)`)

func extractDependencyAssertions(original string) []Assertion {
	var assertions []Assertion

	lines := strings.Split(original, "\n")
	for _, line := range lines {
		// Only process lines that mention removal
		if !depRemovalContext.MatchString(line) {
			continue
		}
		// Find all module paths in that line
		matches := depModulePath.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				dep := match[1]
				assertions = append(assertions, Assertion{
					Type:        Negative,
					Description: fmt.Sprintf("dependency should be removed: %s", dep),
					Pattern:     dep,
					GlobPattern: "go.mod",
				})
				assertions = append(assertions, Assertion{
					Type:        Negative,
					Description: fmt.Sprintf("import should be removed: %s", dep),
					Pattern:     dep,
					GlobPattern: "*.go",
				})
			}
		}
	}

	return assertions
}

// dirPattern matches "delete coordinator/" or "remove the coordinator/ directory"
var dirPattern = regexp.MustCompile(`(?i)\b(?:remove|delete|rip\s+out)\s+(?:the\s+)?(\w+(?:/\w+)*)/?\s*(?:directory|dir|folder)?`)

func extractDirectoryAssertions(original string) []Assertion {
	var assertions []Assertion

	matches := dirPattern.FindAllStringSubmatch(original, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			dir := match[1]
			// Skip common false positives
			if dir == "the" || dir == "all" || dir == "any" || len(dir) < 2 {
				continue
			}
			assertions = append(assertions, Assertion{
				Type:        Negative,
				Description: fmt.Sprintf("directory should not exist: %s/", dir),
				PathCheck:   dir,
			})
		}
	}

	return assertions
}

// extractSearchPattern takes a natural language target and returns a grep-able pattern.
func extractSearchPattern(target string) string {
	// If it looks like a code identifier (camelCase, has dots, etc.), use as-is
	if regexp.MustCompile(`^[a-zA-Z_]\w*(\.\w+)*$`).MatchString(target) {
		return target
	}
	// If it contains a module path, use that
	if strings.Contains(target, ".") && !strings.Contains(target, " ") {
		return target
	}
	// For multi-word phrases, try to extract the most specific term
	words := strings.Fields(target)
	if len(words) == 0 {
		return ""
	}
	// Use the longest word as the pattern (most specific)
	longest := words[0]
	for _, w := range words[1:] {
		if len(w) > len(longest) {
			longest = w
		}
	}
	if len(longest) < 3 {
		return ""
	}
	return longest
}

// CheckAssertion verifies a single assertion against a repository directory.
func CheckAssertion(repoDir string, assertion Assertion) Result {
	// Path existence check
	if assertion.PathCheck != "" {
		return checkPathAssertion(repoDir, assertion)
	}

	// Pattern search
	return checkPatternAssertion(repoDir, assertion)
}

func checkPathAssertion(repoDir string, assertion Assertion) Result {
	fullPath := filepath.Join(repoDir, assertion.PathCheck)
	_, err := os.Stat(fullPath)
	exists := err == nil

	switch assertion.Type {
	case Negative:
		if exists {
			return Result{
				Assertion: assertion,
				Pass:      false,
				Evidence:  fmt.Sprintf("directory still exists: %s", fullPath),
			}
		}
		return Result{
			Assertion: assertion,
			Pass:      true,
			Evidence:  fmt.Sprintf("directory does not exist: %s", fullPath),
		}
	case Positive:
		if !exists {
			return Result{
				Assertion: assertion,
				Pass:      false,
				Evidence:  fmt.Sprintf("directory does not exist: %s", fullPath),
			}
		}
		return Result{
			Assertion: assertion,
			Pass:      true,
			Evidence:  fmt.Sprintf("directory exists: %s", fullPath),
		}
	}

	return Result{Assertion: assertion, Pass: false, Evidence: "unknown assertion type"}
}

func checkPatternAssertion(repoDir string, assertion Assertion) Result {
	matches := searchFiles(repoDir, assertion.Pattern, assertion.GlobPattern)

	switch assertion.Type {
	case Negative:
		if len(matches) > 0 {
			evidence := fmt.Sprintf("found %d occurrence(s):\n", len(matches))
			for i, m := range matches {
				if i >= 5 {
					evidence += fmt.Sprintf("  ... and %d more\n", len(matches)-5)
					break
				}
				evidence += fmt.Sprintf("  %s\n", m)
			}
			return Result{
				Assertion: assertion,
				Pass:      false,
				Evidence:  evidence,
			}
		}
		return Result{
			Assertion: assertion,
			Pass:      true,
			Evidence:  "no occurrences found",
		}
	case Positive:
		if len(matches) == 0 {
			return Result{
				Assertion: assertion,
				Pass:      false,
				Evidence:  "no occurrences found",
			}
		}
		evidence := fmt.Sprintf("found %d occurrence(s):\n", len(matches))
		for i, m := range matches {
			if i >= 3 {
				evidence += fmt.Sprintf("  ... and %d more\n", len(matches)-3)
				break
			}
			evidence += fmt.Sprintf("  %s\n", m)
		}
		return Result{
			Assertion: assertion,
			Pass:      true,
			Evidence:  evidence,
		}
	}

	return Result{Assertion: assertion, Pass: false, Evidence: "unknown assertion type"}
}

// searchFiles searches for a pattern in files under dir, optionally filtered by glob.
func searchFiles(dir, pattern, globFilter string) []string {
	var matches []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip errors
		}
		if info.IsDir() {
			// Skip hidden dirs and common noise
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != dir {
				return filepath.SkipDir
			}
			if base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply glob filter if specified
		if globFilter != "" {
			matched, _ := filepath.Match(globFilter, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		// Skip binary files (simple heuristic: check extension)
		ext := filepath.Ext(path)
		if isBinaryExt(ext) {
			return nil
		}

		// Search file contents
		fileMatches := searchInFile(path, pattern, dir)
		matches = append(matches, fileMatches...)

		return nil
	})
	if err != nil {
		// Best effort
		return matches
	}

	return matches
}

func searchInFile(filePath, pattern, baseDir string) []string {
	var matches []string

	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	relPath, _ := filepath.Rel(baseDir, filePath)
	if relPath == "" {
		relPath = filePath
	}

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if strings.Contains(line, pattern) {
			matches = append(matches, fmt.Sprintf("%s:%d: %s", relPath, lineNum, strings.TrimSpace(line)))
			if len(matches) >= 20 {
				break // Limit matches per file
			}
		}
	}

	return matches
}

func isBinaryExt(ext string) bool {
	switch ext {
	case ".exe", ".bin", ".so", ".dylib", ".dll", ".o", ".a",
		".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg",
		".pdf", ".zip", ".tar", ".gz", ".bz2",
		".wasm", ".pyc", ".class":
		return true
	}
	return false
}

// Verify runs all assertions against a repository and returns a report.
func Verify(sessionID, purpose, repoDir string, assertions []Assertion) *Report {
	report := &Report{
		SessionID: sessionID,
		Purpose:   purpose,
	}

	for _, a := range assertions {
		result := CheckAssertion(repoDir, a)
		report.Results = append(report.Results, result)
	}

	return report
}
