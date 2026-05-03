package monitoring

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// Validator validates sub-agent outputs using multiple signals
type Validator struct {
	agentID  string
	workDir  string
	eventLog string
	config   ValidationConfig
}

// NewValidator creates a new validator
func NewValidator(agentID, workDir, eventLog string, config ValidationConfig) *Validator {
	return &Validator{
		agentID:  agentID,
		workDir:  workDir,
		eventLog: eventLog,
		config:   config,
	}
}

// Validate runs all validation signals and returns aggregate result
func (v *Validator) Validate() (*ValidationResult, error) {
	signals := []ValidationSignal{
		v.ValidateGitCommits(),
		v.ValidateFileCount(),
		v.ValidateLineCount(),
		v.ValidateTestExecution(),
		v.ValidateStubKeywords(),
	}

	// Calculate aggregate score
	score := 0.0
	for _, sig := range signals {
		if sig.Passed {
			score += sig.Weight
		}
	}

	passed := score >= v.config.PassThreshold

	return &ValidationResult{
		Passed:  passed,
		Score:   score,
		Signals: signals,
		Summary: v.generateSummary(signals, score, passed),
	}, nil
}

// ValidateGitCommits checks if enough commits were made
func (v *Validator) ValidateGitCommits() ValidationSignal {
	commitCount := v.countEventsByType(EventGitCommit)

	// Fallback: count commits via git log
	if commitCount == 0 {
		if _, err := os.Stat(filepath.Join(v.workDir, ".git")); err == nil {
			cmd := exec.Command("git", "-C", v.workDir, "log", "--oneline")
			output, err := cmd.Output()
			if err == nil {
				commitCount = len(strings.Split(strings.TrimSpace(string(output)), "\n"))
			}
		}
	}

	passed := commitCount >= v.config.MinCommitCount

	return ValidationSignal{
		Name:     "git_commits",
		Value:    commitCount,
		Expected: v.config.MinCommitCount,
		Weight:   0.2,
		Passed:   passed,
		Message:  fmt.Sprintf("%d commits (expected >= %d)", commitCount, v.config.MinCommitCount),
	}
}

// ValidateFileCount checks if enough files were created
func (v *Validator) ValidateFileCount() ValidationSignal {
	fileCount := v.countEventsByType(EventFileCreated)

	// Fallback: count files in directory
	if fileCount == 0 {
		fileCount = v.countFilesInDir(v.workDir)
	}

	passed := fileCount >= v.config.MinFileCount

	return ValidationSignal{
		Name:     "file_count",
		Value:    fileCount,
		Expected: v.config.MinFileCount,
		Weight:   0.2,
		Passed:   passed,
		Message:  fmt.Sprintf("%d files created (expected >= %d)", fileCount, v.config.MinFileCount),
	}
}

// ValidateLineCount checks total lines of code
func (v *Validator) ValidateLineCount() ValidationSignal {
	lineCount := v.countLinesInDir(v.workDir)

	passed := lineCount >= v.config.MinLineCount

	return ValidationSignal{
		Name:     "line_count",
		Value:    lineCount,
		Expected: v.config.MinLineCount,
		Weight:   0.2,
		Passed:   passed,
		Message:  fmt.Sprintf("%d lines of code (expected >= %d)", lineCount, v.config.MinLineCount),
	}
}

// ValidateTestExecution checks if tests were run
func (v *Validator) ValidateTestExecution() ValidationSignal {
	testRuns := v.countEventsByType(EventTestStarted)
	if testRuns == 0 {
		testRuns = v.countEventsByType(EventTestPassed)
	}

	// Fallback: actually run tests
	if testRuns == 0 && v.config.MinTestRuns > 0 {
		if v.canRunTests() {
			if err := v.runTests(); err == nil {
				testRuns = 1 // Tests passed
			}
		}
	}

	passed := testRuns >= v.config.MinTestRuns

	return ValidationSignal{
		Name:     "test_execution",
		Value:    testRuns,
		Expected: v.config.MinTestRuns,
		Weight:   0.3,
		Passed:   passed,
		Message:  fmt.Sprintf("%d test runs (expected >= %d)", testRuns, v.config.MinTestRuns),
	}
}

// ValidateStubKeywords checks for stub indicators
func (v *Validator) ValidateStubKeywords() ValidationSignal {
	stubCount := v.countStubKeywords(v.workDir)

	passed := stubCount <= v.config.MaxStubKeywords

	return ValidationSignal{
		Name:     "stub_keywords",
		Value:    stubCount,
		Expected: v.config.MaxStubKeywords,
		Weight:   0.1,
		Passed:   passed,
		Message:  fmt.Sprintf("%d stub keywords (expected <= %d)", stubCount, v.config.MaxStubKeywords),
	}
}

// countEventsByType counts events of a specific type in the log
func (v *Validator) countEventsByType(eventType string) int {
	if v.eventLog == "" {
		return 0
	}

	file, err := os.Open(v.eventLog)
	if err != nil {
		return 0
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event eventbus.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}

		// Check if event matches agent and type
		if agentID, ok := event.Data["agent_id"].(string); ok {
			if agentID == v.agentID && event.Type == eventType {
				count++
			}
		}
	}

	return count
}

// countFilesInDir counts source files in directory
func (v *Validator) countFilesInDir(dir string) int {
	count := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}
		if !info.IsDir() && v.isSourceFile(path) {
			count++
		}
		return nil
	})
	return count
}

// countLinesInDir counts total lines in source files
func (v *Validator) countLinesInDir(dir string) int {
	totalLines := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && v.isSourceFile(path) {
			file, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				totalLines++
			}
		}
		return nil
	})
	return totalLines
}

// countStubKeywords counts TODO, FIXME, NotImplemented patterns
func (v *Validator) countStubKeywords(dir string) int {
	// Use grep for efficiency
	cmd := exec.Command("grep", "-r", "-E", "TODO|FIXME|NotImplemented|panic\\(\"not implemented\"\\)", dir)
	output, err := cmd.Output()
	if err != nil {
		return 0 // No matches or error (both acceptable)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

// canRunTests checks if tests can be run in this directory
func (v *Validator) canRunTests() bool {
	// Check for common test indicators
	if _, err := os.Stat(filepath.Join(v.workDir, "go.mod")); err == nil {
		return true // Go project
	}
	if _, err := os.Stat(filepath.Join(v.workDir, "package.json")); err == nil {
		return true // npm project
	}
	if _, err := os.Stat(filepath.Join(v.workDir, "setup.py")); err == nil {
		return true // Python project
	}
	return false
}

// runTests attempts to run tests (fallback validation)
func (v *Validator) runTests() error {
	// Try Go tests
	if _, err := os.Stat(filepath.Join(v.workDir, "go.mod")); err == nil {
		cmd := exec.Command("go", "test", "./...")
		cmd.Dir = v.workDir
		return cmd.Run()
	}

	// Try npm tests
	if _, err := os.Stat(filepath.Join(v.workDir, "package.json")); err == nil {
		cmd := exec.Command("npm", "test")
		cmd.Dir = v.workDir
		return cmd.Run()
	}

	// Try pytest
	if _, err := os.Stat(filepath.Join(v.workDir, "setup.py")); err == nil {
		cmd := exec.Command("pytest")
		cmd.Dir = v.workDir
		return cmd.Run()
	}

	return fmt.Errorf("no test runner found")
}

// isSourceFile checks if a file is a source code file
func (v *Validator) isSourceFile(path string) bool {
	// Exclude common non-source files
	if strings.Contains(path, "/.git/") ||
		strings.Contains(path, "/node_modules/") ||
		strings.Contains(path, "/__pycache__/") ||
		strings.Contains(path, "/.vscode/") ||
		strings.Contains(path, "/.idea/") {
		return false
	}

	ext := filepath.Ext(path)
	sourceExts := map[string]bool{
		".go":   true,
		".js":   true,
		".ts":   true,
		".py":   true,
		".java": true,
		".c":    true,
		".cpp":  true,
		".h":    true,
		".rb":   true,
		".rs":   true,
	}
	return sourceExts[ext]
}

// generateSummary creates human-readable validation summary
func (v *Validator) generateSummary(signals []ValidationSignal, score float64, passed bool) string {
	if passed {
		failedSignals := []string{}
		for _, sig := range signals {
			if !sig.Passed {
				failedSignals = append(failedSignals, sig.Name)
			}
		}

		if len(failedSignals) == 0 {
			return "All validation signals passed. Implementation appears complete."
		}
		return fmt.Sprintf("Validation passed overall (score: %.2f), but some signals failed: %s. Review recommended.",
			score, strings.Join(failedSignals, ", "))
	}

	// Failed
	failedSignals := []string{}
	for _, sig := range signals {
		if !sig.Passed {
			failedSignals = append(failedSignals, sig.Message)
		}
	}

	return fmt.Sprintf("Validation failed (score: %.2f). Failed signals: %s",
		score, strings.Join(failedSignals, "; "))
}
