package validator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CompilationResult holds the result of a compilation attempt
type CompilationResult struct {
	Success         bool
	Output          string
	ExitCode        int
	ErrorMessage    string
	TestCount       int
	FailureCount    int
	SkipCount       int     // Number of skipped/ignored tests
	CoveragePercent float64 // Test coverage percentage (0.0 if not available)
}

// validateCompilation runs language-specific build and test commands
// Returns ValidationError if code doesn't compile or tests fail
func validateCompilation(projectDir, phaseName string) error {
	// Only validate compilation for BUILD (Implementation) phase
	if phaseName != "BUILD" {
		return nil
	}

	// Detect project language
	lang, err := detectProjectLanguage(projectDir)
	if err != nil {
		// If we can't detect language, skip compilation validation
		// (allows for non-code projects like documentation)
		return nil
	}

	// Run build verification
	buildResult, err := runBuild(projectDir, lang)
	if err != nil {
		return NewValidationError(
			"complete BUILD",
			fmt.Sprintf("failed to run build command: %v", err),
			"Check build tool installation and try again",
		)
	}

	if !buildResult.Success {
		return NewValidationError(
			"complete BUILD",
			fmt.Sprintf("build failed with %d compilation errors", buildResult.ExitCode),
			fmt.Sprintf("Fix compilation errors:\n%s\n\nThen re-run: wayfinder-session complete-phase BUILD", buildResult.Output),
		)
	}

	// Run test verification
	testResult, err := runTests(projectDir, lang)
	if err != nil {
		return NewValidationError(
			"complete BUILD",
			fmt.Sprintf("failed to run test command: %v", err),
			"Check test framework installation and try again",
		)
	}

	if !testResult.Success {
		return NewValidationError(
			"complete BUILD",
			fmt.Sprintf("tests failed with %d failures", testResult.FailureCount),
			fmt.Sprintf("Fix failing tests:\n%s\n\nThen re-run: wayfinder-session complete-phase BUILD", testResult.Output),
		)
	}

	// Verify test count > 0
	if testResult.TestCount == 0 {
		return NewValidationError(
			"complete BUILD",
			"no tests found (test count = 0)",
			"Add at least 1 test before completing BUILD. For new features, aim for ≥3 tests (happy path + error cases + edge cases).",
		)
	}

	// Warn if test count < 3 for new features (non-blocking)
	if testResult.TestCount < 3 {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Only %d test(s) found. For new features, aim for ≥3 tests.\n", testResult.TestCount)
	}

	return nil
}

// detectProjectLanguage detects the primary language of the project
func detectProjectLanguage(projectDir string) (string, error) {
	// Check for language-specific marker files in order of preference
	checks := []struct {
		file string
		lang string
	}{
		{"go.mod", "go"},
		{"go.sum", "go"},
		{"package.json", "javascript"},
		{"tsconfig.json", "typescript"},
		{"requirements.txt", "python"},
		{"setup.py", "python"},
		{"Cargo.toml", "rust"},
		{"pom.xml", "java"},
		{"build.gradle", "java"},
	}

	for _, check := range checks {
		path := filepath.Join(projectDir, check.file)
		if _, err := os.Stat(path); err == nil {
			return check.lang, nil
		}
	}

	// Fallback: check for source files
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return "", err
	}

	langCounts := make(map[string]int)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		switch ext {
		case ".go":
			langCounts["go"]++
		case ".py":
			langCounts["python"]++
		case ".ts":
			langCounts["typescript"]++
		case ".js":
			langCounts["javascript"]++
		case ".rs":
			langCounts["rust"]++
		case ".java":
			langCounts["java"]++
		}
	}

	// Return language with most files
	maxCount := 0
	detectedLang := ""
	for lang, count := range langCounts {
		if count > maxCount {
			maxCount = count
			detectedLang = lang
		}
	}

	if detectedLang == "" {
		return "", fmt.Errorf("no recognized language found in %s", projectDir)
	}

	return detectedLang, nil
}

// runBuild executes the build command for the detected language
func runBuild(projectDir, lang string) (*CompilationResult, error) {
	var cmd *exec.Cmd

	switch lang {
	case "go":
		cmd = exec.Command("go", "build", "./...")
	case "python":
		// Python syntax check
		cmd = exec.Command("python", "-m", "py_compile")
		// Add all .py files as arguments
		pyFiles, _ := filepath.Glob(filepath.Join(projectDir, "*.py"))
		if len(pyFiles) > 0 {
			cmd.Args = append(cmd.Args, pyFiles...)
		}
	case "javascript", "typescript":
		// Check if package.json has build script
		pkgPath := filepath.Join(projectDir, "package.json")
		if _, err := os.Stat(pkgPath); err == nil {
			cmd = exec.Command("npm", "run", "build")
		} else {
			// No build script, skip build validation
			return &CompilationResult{Success: true}, nil
		}
	case "rust":
		cmd = exec.Command("cargo", "build")
	case "java":
		// Try Maven first, then Gradle
		if _, err := os.Stat(filepath.Join(projectDir, "pom.xml")); err == nil {
			cmd = exec.Command("mvn", "compile")
		} else if _, err := os.Stat(filepath.Join(projectDir, "build.gradle")); err == nil {
			cmd = exec.Command("gradle", "build")
		}
	default:
		// Unknown language, skip build validation
		return &CompilationResult{Success: true}, nil
	}

	if cmd == nil {
		return &CompilationResult{Success: true}, nil
	}

	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	exitCode := 0
	exitErr := &exec.ExitError{}
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	return &CompilationResult{
		Success:      err == nil,
		Output:       string(output),
		ExitCode:     exitCode,
		ErrorMessage: "",
	}, nil
}

// runTests executes the test command for the detected language
func runTests(projectDir, lang string) (*CompilationResult, error) {
	var cmd *exec.Cmd

	switch lang {
	case "go":
		cmd = exec.Command("go", "test", "./...", "-v")
	case "python":
		// Try pytest first, fall back to unittest
		if _, err := exec.LookPath("pytest"); err == nil {
			cmd = exec.Command("pytest", "-v")
		} else {
			cmd = exec.Command("python", "-m", "unittest", "discover", "-v")
		}
	case "javascript", "typescript":
		cmd = exec.Command("npm", "test")
	case "rust":
		cmd = exec.Command("cargo", "test")
	case "java":
		// Try Maven first, then Gradle
		if _, err := os.Stat(filepath.Join(projectDir, "pom.xml")); err == nil {
			cmd = exec.Command("mvn", "test")
		} else if _, err := os.Stat(filepath.Join(projectDir, "build.gradle")); err == nil {
			cmd = exec.Command("gradle", "test")
		}
	default:
		// Unknown language, skip test validation
		return &CompilationResult{Success: true, TestCount: 1}, nil
	}

	if cmd == nil {
		return &CompilationResult{Success: true, TestCount: 1}, nil
	}

	cmd.Dir = projectDir

	// Set timeout to 5 minutes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmd.Dir = projectDir

	output, err := cmd.CombinedOutput()
	exitCode := 0
	exitErr := &exec.ExitError{}
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	// Parse test results
	testCount, failureCount := parseTestOutput(string(output), lang)

	return &CompilationResult{
		Success:      err == nil && failureCount == 0,
		Output:       string(output),
		ExitCode:     exitCode,
		ErrorMessage: "",
		TestCount:    testCount,
		FailureCount: failureCount,
	}, nil
}

// parseTestOutput parses test output to extract test count and failure count
func parseTestOutput(output, lang string) (testCount, failureCount int) {
	lines := strings.Split(output, "\n")

	switch lang {
	case "go":
		return parseGoTestOutput(lines)
	case "python":
		return parsePythonTestOutput(lines)
	case "javascript", "typescript":
		return parseJSTestOutput(lines)
	case "rust":
		return parseRustTestOutput(lines)
	case "java":
		return parseJavaTestOutput(lines)
	}

	return 0, 0
}

// parseGoTestOutput parses Go test output
// Looks for "--- PASS: TestName" and "--- FAIL: TestName"
func parseGoTestOutput(lines []string) (testCount, failureCount int) {
	for _, line := range lines {
		if strings.HasPrefix(line, "--- PASS:") {
			testCount++
		} else if strings.HasPrefix(line, "--- FAIL:") {
			testCount++
			failureCount++
		}
	}
	return testCount, failureCount
}

// parsePythonTestOutput parses pytest or unittest output
// pytest: "test_file.py::test_name PASSED"
// unittest: "test_name (module.TestClass) ... ok"
func parsePythonTestOutput(lines []string) (testCount, failureCount int) {
	for _, line := range lines {
		if strings.Contains(line, "PASSED") || strings.Contains(line, "... ok") {
			testCount++
		} else if strings.Contains(line, "FAILED") || strings.Contains(line, "... FAIL") {
			testCount++
			failureCount++
		}
	}
	return testCount, failureCount
}

// parseJSTestOutput parses Jest/Mocha output
// Looks for "✓ test name" or "✗ test name"
func parseJSTestOutput(lines []string) (testCount, failureCount int) {
	for _, line := range lines {
		if strings.Contains(line, "✓") || strings.Contains(line, "PASS") {
			testCount++
		} else if strings.Contains(line, "✗") || strings.Contains(line, "FAIL") {
			testCount++
			failureCount++
		}
	}
	return testCount, failureCount
}

// parseRustTestOutput parses Cargo test output
// Looks for "test result: ok. X passed"
func parseRustTestOutput(lines []string) (testCount, failureCount int) {
	for _, line := range lines {
		if strings.Contains(line, "test result:") {
			// Extract test count from "X passed; Y failed"
			parts := strings.Split(line, ".")
			if len(parts) > 1 {
				stats := parts[1]
				if strings.Contains(stats, "passed") {
					fmt.Sscanf(stats, " %d passed", &testCount)
				}
				if strings.Contains(stats, "failed") {
					fmt.Sscanf(stats, " %d passed; %d failed", &testCount, &failureCount)
				}
			}
		}
	}
	return testCount, failureCount
}

// parseJavaTestOutput parses Maven/Gradle test output
// Looks for "Tests run: X, Failures: Y"
func parseJavaTestOutput(lines []string) (testCount, failureCount int) {
	for _, line := range lines {
		if strings.Contains(line, "Tests run:") {
			fmt.Sscanf(line, "Tests run: %d, Failures: %d", &testCount, &failureCount)
		}
	}
	return testCount, failureCount
}
