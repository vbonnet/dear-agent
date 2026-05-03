package codeintel

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CheckResult holds the outcome of a single Tier 0 check.
type CheckResult struct {
	Check    string   `json:"check"`
	Passed   bool     `json:"passed"`
	Severity string   `json:"severity"` // "error", "warning", "info"
	Message  string   `json:"message"`
	Details  []string `json:"details,omitempty"`
}

// Tier0Result holds results from all Tier 0 checks.
type Tier0Result struct {
	Languages []string      `json:"languages"`
	Checks    []CheckResult `json:"checks"`
}

// entryPoints are function names that should never be flagged as dead code.
var entryPoints = map[string]bool{
	"main":     true,
	"init":     true,
	"Main":     true,
	"TestMain": true,
}

// CheckDeadCode extracts function definitions from changed files using
// FunctionPattern regex and greps the rest of the project for references.
// Only functions defined in changedFiles are checked.
//
// NOTE: This check has a high false-positive rate for:
//   - Interface implementations
//   - Callback functions passed by reference
//   - Functions called via reflection
//   - Exported functions used by external packages
func CheckDeadCode(cwd string, languages []LanguageSpec, changedFiles []string) CheckResult {
	result := CheckResult{
		Check:    "dead_code",
		Passed:   true,
		Severity: "warning",
		Message:  "No potentially dead functions found in changed files",
	}

	if len(changedFiles) == 0 {
		return result
	}

	// Build a map of language spec by source extension for quick lookup.
	specByExt := buildExtMap(languages)

	var suspects []string
	for _, cf := range changedFiles {
		absPath := cf
		if !filepath.IsAbs(cf) {
			absPath = filepath.Join(cwd, cf)
		}

		ext := filepath.Ext(absPath)
		spec, ok := specByExt[ext]
		if !ok || spec.FunctionPattern == "" {
			continue
		}

		funcs, err := extractFunctions(absPath, spec.FunctionPattern)
		if err != nil {
			continue
		}

		for _, fn := range funcs {
			if entryPoints[fn] {
				continue
			}
			// Skip test helper functions (Test*, Benchmark*, Example* in Go)
			if spec.Name == "go" && (strings.HasPrefix(fn, "Test") || strings.HasPrefix(fn, "Benchmark") || strings.HasPrefix(fn, "Example")) {
				continue
			}
			if !hasReferencesElsewhere(cwd, fn, absPath, spec) {
				suspects = append(suspects, fmt.Sprintf("%s: %s()", relPath(cwd, absPath), fn))
			}
		}
	}

	if len(suspects) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("%d potentially dead function(s) found (high false-positive rate: interface impls, callbacks, reflection)", len(suspects))
		result.Details = suspects
	}
	return result
}

// extractFunctions reads a file and returns all function names matching the pattern.
func extractFunctions(path, pattern string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	var funcs []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		matches := re.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}
		// Use the first non-empty submatch (handles alternation groups).
		for _, m := range matches[1:] {
			if m != "" {
				funcs = append(funcs, m)
				break
			}
		}
	}
	return funcs, nil
}

// hasReferencesElsewhere greps the project for the function name, excluding
// the file where it's defined. Returns true if found anywhere else.
//
// Known limitation: references within the definition file itself (e.g.,
// recursive calls or calls from other functions in the same file) are not
// counted. This may produce false positives for functions only used within
// their own file.
func hasReferencesElsewhere(cwd, funcName, defFile string, spec LanguageSpec) bool {
	for _, glob := range spec.SourceGlobs {
		ext := filepath.Ext(glob)
		if ext == "" {
			continue
		}
		// Walk and search for references.
		found := false
		_ = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
			}
			if filepath.Ext(path) != ext {
				return nil
			}
			// Skip the definition file itself.
			if path == defFile {
				return nil
			}
			// Skip vendor/node_modules.
			rel, _ := filepath.Rel(cwd, path)
			if strings.HasPrefix(rel, "vendor/") || strings.HasPrefix(rel, "node_modules/") {
				return filepath.SkipDir
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
			}
			if bytes.Contains(data, []byte(funcName)) {
				found = true
				return filepath.SkipAll
			}
			return nil
		})
		if found {
			return true
		}
	}
	return false
}

// CheckDanglingRefsScoped runs build commands only for languages that have
// changed files, avoiding unnecessary builds for unrelated languages.
func CheckDanglingRefsScoped(cwd string, languages []LanguageSpec, changedFiles []string) CheckResult {
	if len(changedFiles) == 0 {
		return CheckDanglingRefs(cwd, languages)
	}

	specByExt := buildExtMap(languages)
	seen := make(map[string]bool)
	var scoped []LanguageSpec
	for _, cf := range changedFiles {
		ext := filepath.Ext(cf)
		if spec, ok := specByExt[ext]; ok && !seen[spec.Name] {
			seen[spec.Name] = true
			scoped = append(scoped, spec)
		}
	}
	if len(scoped) == 0 {
		return CheckResult{
			Check:    "dangling_refs",
			Passed:   true,
			Severity: "info",
			Message:  "No changed files match detected languages; skipped",
		}
	}
	return CheckDanglingRefs(cwd, scoped)
}

// CheckDanglingRefs runs the language's build command (if available and installed)
// to detect compilation errors / dangling references.
func CheckDanglingRefs(cwd string, languages []LanguageSpec) CheckResult {
	result := CheckResult{
		Check:    "dangling_refs",
		Passed:   true,
		Severity: "error",
		Message:  "No build command available; skipped",
	}

	for _, spec := range languages {
		if len(spec.BuildCmd) == 0 {
			continue
		}
		if !commandExists(spec.BuildCmd[0]) {
			result.Message = fmt.Sprintf("%s build tool (%s) not installed; skipped", spec.Name, spec.BuildCmd[0])
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, spec.BuildCmd[0], spec.BuildCmd[1:]...)
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(), "GOWORK=off")
		out, err := cmd.CombinedOutput()
		if err != nil {
			result.Passed = false
			result.Severity = "error"
			errOutput := string(out)
			if len(errOutput) > 500 {
				errOutput = errOutput[:500]
			}
			result.Message = fmt.Sprintf("%s build failed", spec.Name)
			result.Details = []string{errOutput}
			return result
		}
		result.Message = fmt.Sprintf("%s build passed", spec.Name)
	}
	return result
}

// CheckDebugPrints scans changed files for debug print patterns defined in
// each language spec. Severity is always "warning" (not blocking).
func CheckDebugPrints(cwd string, languages []LanguageSpec, changedFiles []string) CheckResult {
	result := CheckResult{
		Check:    "debug_prints",
		Passed:   true,
		Severity: "warning",
		Message:  "No debug prints found in changed files",
	}

	if len(changedFiles) == 0 {
		return result
	}

	specByExt := buildExtMap(languages)
	var findings []string

	for _, cf := range changedFiles {
		absPath := cf
		if !filepath.IsAbs(cf) {
			absPath = filepath.Join(cwd, cf)
		}

		ext := filepath.Ext(absPath)
		spec, ok := specByExt[ext]
		if !ok || len(spec.DebugPatterns) == 0 {
			continue
		}

		matches := scanFileForPatterns(absPath, spec.DebugPatterns)
		for _, m := range matches {
			findings = append(findings, fmt.Sprintf("%s:%d: %s", relPath(cwd, absPath), m.line, m.text))
		}
	}

	if len(findings) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("%d debug print(s) found in changed files", len(findings))
		result.Details = findings
	}
	return result
}

type patternMatch struct {
	line int
	text string
}

func scanFileForPatterns(path string, patterns []string) []patternMatch {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		compiled = append(compiled, re)
	}

	var matches []patternMatch
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		for _, re := range compiled {
			if re.MatchString(line) {
				matches = append(matches, patternMatch{line: lineNum, text: strings.TrimSpace(line)})
				break // one match per line is enough
			}
		}
	}
	return matches
}

// RunTier0Checks detects languages and runs all Tier 0 checks.
func RunTier0Checks(cwd string, changedFiles []string) (*Tier0Result, error) {
	reg, err := NewRegistry(cwd)
	if err != nil {
		return nil, fmt.Errorf("loading registry: %w", err)
	}

	languages := reg.DetectLanguages(cwd)
	if len(languages) == 0 {
		return &Tier0Result{
			Checks: []CheckResult{{
				Check:    "detect_languages",
				Passed:   true,
				Severity: "info",
				Message:  "No languages detected",
			}},
		}, nil
	}

	// If no changed files provided, try git diff.
	if len(changedFiles) == 0 {
		changedFiles = detectChangedFiles(cwd)
	}

	var names []string
	for _, l := range languages {
		names = append(names, l.Name)
	}

	result := &Tier0Result{
		Languages: names,
		Checks: []CheckResult{
			CheckDeadCode(cwd, languages, changedFiles),
			CheckDanglingRefs(cwd, languages),
			CheckDebugPrints(cwd, languages, changedFiles),
		},
	}
	return result, nil
}

// detectChangedFiles uses git diff to find changed files relative to HEAD.
func detectChangedFiles(cwd string) []string {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		// Also try for untracked/staged files.
		cmd2 := exec.Command("git", "diff", "--name-only", "--cached")
		cmd2.Dir = cwd
		out, err = cmd2.Output()
		if err != nil {
			return nil
		}
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// buildExtMap creates a map from file extension to LanguageSpec.
func buildExtMap(languages []LanguageSpec) map[string]LanguageSpec {
	m := make(map[string]LanguageSpec)
	for _, spec := range languages {
		for _, glob := range spec.SourceGlobs {
			ext := filepath.Ext(glob)
			if ext != "" {
				m[ext] = spec
			}
		}
	}
	return m
}

func relPath(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
