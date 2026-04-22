package ops

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HygieneIssue represents a single hygiene problem found in the codebase.
type HygieneIssue struct {
	Category string `json:"category"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// HygieneReport is the result of a hygiene check on a Go package.
type HygieneReport struct {
	PackagePath string         `json:"package_path"`
	Score       int            `json:"score"`
	Issues      []HygieneIssue `json:"issues"`
	Summary     map[string]int `json:"summary"`
}

// CheckHygiene runs lightweight dead-code and code-quality checks on a Go package.
// It returns a score (0-100) and a list of issues found.
//
// Checks performed:
//   - TODO/FIXME/HACK/XXX marker count
//   - go vet diagnostics
//   - staticcheck diagnostics (gracefully skipped if unavailable)
func CheckHygiene(packagePath string) (*HygieneReport, error) {
	if packagePath == "" {
		return nil, fmt.Errorf("packagePath must not be empty")
	}

	info, err := os.Stat(packagePath)
	if err != nil {
		return nil, fmt.Errorf("stat packagePath: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("packagePath must be a directory: %s", packagePath)
	}

	report := &HygieneReport{
		PackagePath: packagePath,
		Score:       100,
		Summary:     make(map[string]int),
	}

	todoIssues := countTODOs(packagePath)
	report.Issues = append(report.Issues, todoIssues...)
	report.Summary["todo"] = len(todoIssues)

	vetIssues := runGoVet(packagePath)
	report.Issues = append(report.Issues, vetIssues...)
	report.Summary["govet"] = len(vetIssues)

	staticcheckIssues := runStaticcheck(packagePath)
	report.Issues = append(report.Issues, staticcheckIssues...)
	report.Summary["staticcheck"] = len(staticcheckIssues)

	// Compute score: start at 100, deduct per issue category.
	todoDeduct := len(todoIssues)
	if todoDeduct > 20 {
		todoDeduct = 20
	}
	report.Score -= todoDeduct
	report.Score -= len(vetIssues) * 5
	report.Score -= len(staticcheckIssues) * 3

	if report.Score < 0 {
		report.Score = 0
	}

	return report, nil
}

// countTODOs scans .go files for TODO, FIXME, HACK, and XXX markers.
func countTODOs(dir string) []HygieneIssue {
	markers := []string{"TODO", "FIXME", "HACK", "XXX"}
	var issues []HygieneIssue

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			for _, marker := range markers {
				if strings.Contains(line, marker) {
					issues = append(issues, HygieneIssue{
						Category: "todo",
						File:     entry.Name(),
						Line:     lineNum,
						Message:  fmt.Sprintf("%s marker found", marker),
					})
					break // one issue per line even if multiple markers
				}
			}
		}
		f.Close()
	}

	return issues
}

// runGoVet runs "go vet" on the package and parses diagnostic output.
func runGoVet(dir string) []HygieneIssue {
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil // clean
	}
	return parseToolOutput(string(out), "govet")
}

// runStaticcheck runs "staticcheck" on the package if available.
// Returns nil if staticcheck is not installed.
func runStaticcheck(dir string) []HygieneIssue {
	if _, err := exec.LookPath("staticcheck"); err != nil {
		return nil
	}

	cmd := exec.Command("staticcheck", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	return parseToolOutput(string(out), "staticcheck")
}

// parseToolOutput parses "file.go:line:col: message" formatted output
// from tools like go vet and staticcheck.
func parseToolOutput(output, category string) []HygieneIssue {
	var issues []HygieneIssue
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Format: path/file.go:line:col: message
		// or:     path/file.go:line: message
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 3 {
			continue
		}

		file := parts[0]
		lineNum := 0
		fmt.Sscanf(parts[1], "%d", &lineNum)

		msg := line
		if idx := strings.Index(line, ": "); idx >= 0 {
			// Find the last ": " after the file:line:col prefix to get the message.
			afterFile := strings.SplitN(line, ": ", 2)
			if len(afterFile) == 2 {
				msg = afterFile[1]
			}
		}

		issues = append(issues, HygieneIssue{
			Category: category,
			File:     filepath.Base(file),
			Line:     lineNum,
			Message:  msg,
		})
	}
	return issues
}
