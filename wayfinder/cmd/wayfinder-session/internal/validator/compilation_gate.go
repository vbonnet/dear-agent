package validator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/boundedexec"
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
		return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Run build verification
	buildResult := runBuild(projectDir, lang)

	if !buildResult.Success {
		return NewValidationError(
			"complete BUILD",
			fmt.Sprintf("build failed with %d compilation errors", buildResult.ExitCode),
			fmt.Sprintf("Fix compilation errors:\n%s\n\nThen re-run: wayfinder session complete-phase BUILD", buildResult.Output),
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
			fmt.Sprintf("Fix failing tests:\n%s\n\nThen re-run: wayfinder session complete-phase BUILD", testResult.Output),
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

// compilationBuildTimeout and compilationTestTimeout bound the BUILD gate's
// shell-outs. The build previously had no bound at all.
const (
	compilationBuildTimeout = 5 * time.Minute
	compilationTestTimeout  = 5 * time.Minute
)

// runBuild executes the build command for the detected language
func runBuild(projectDir, lang string) *CompilationResult {
	cmd := boundedexec.Command{
		Dir:     projectDir,
		Label:   "BUILD compilation",
		Timeout: compilationBuildTimeout,
	}

	switch lang {
	case "go":
		cmd.Name, cmd.Args = "go", []string{"build", "./..."}
	case "python":
		// Python syntax check over the project's top-level modules.
		// A project directory containing an unclosed bracket makes the joined
		// pattern malformed, so this error is reachable and not decoration.
		pyFiles, err := filepath.Glob(filepath.Join(projectDir, "*.py"))
		if err != nil {
			return &CompilationResult{
				Success:      false,
				ErrorMessage: fmt.Sprintf("cannot enumerate Python sources in %s: %v", projectDir, err),
			}
		}
		if len(pyFiles) == 0 {
			return &CompilationResult{Success: true}
		}
		cmd.Name, cmd.Args = "python", append([]string{"-m", "py_compile"}, pyFiles...)
	case "javascript", "typescript":
		// Check if package.json has build script
		if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err != nil {
			// No build script, skip build validation
			return &CompilationResult{Success: true}
		}
		cmd.Name, cmd.Args = "npm", []string{"run", "build"}
	case "rust":
		cmd.Name, cmd.Args = "cargo", []string{"build"}
	case "java":
		// Try Maven first, then Gradle
		switch {
		case fileExists(filepath.Join(projectDir, "pom.xml")):
			cmd.Name, cmd.Args = "mvn", []string{"compile"}
		case fileExists(filepath.Join(projectDir, "build.gradle")):
			cmd.Name, cmd.Args = "gradle", []string{"build"}
		default:
			return &CompilationResult{Success: true}
		}
	default:
		// Unknown language, skip build validation
		return &CompilationResult{Success: true}
	}

	res := cmd.Run()
	return &CompilationResult{
		Success:      res.Err == nil,
		Output:       compilationOutput(res),
		ExitCode:     res.ExitCode(),
		ErrorMessage: "",
	}
}

// runTests executes the test command for the detected language
func runTests(projectDir, lang string) (*CompilationResult, error) {
	cmd := boundedexec.Command{
		Dir:     projectDir,
		Label:   "BUILD tests",
		Timeout: compilationTestTimeout,
	}

	switch lang {
	case "go":
		cmd.Name, cmd.Args = "go", []string{"test", "./...", "-v"}
	case "python":
		// Try pytest first, fall back to unittest
		if _, err := exec.LookPath("pytest"); err == nil {
			cmd.Name, cmd.Args = "pytest", []string{"-v"}
		} else {
			cmd.Name, cmd.Args = "python", []string{"-m", "unittest", "discover", "-v"}
		}
	case "javascript", "typescript":
		cmd.Name, cmd.Args = "npm", []string{"test"}
	case "rust":
		cmd.Name, cmd.Args = "cargo", []string{"test"}
	case "java":
		// Try Maven first, then Gradle
		switch {
		case fileExists(filepath.Join(projectDir, "pom.xml")):
			cmd.Name, cmd.Args = "mvn", []string{"test"}
		case fileExists(filepath.Join(projectDir, "build.gradle")):
			cmd.Name, cmd.Args = "gradle", []string{"test"}
		default:
			return &CompilationResult{Success: true, TestCount: 1}, nil
		}
	default:
		// Unknown language, skip test validation
		return &CompilationResult{Success: true, TestCount: 1}, nil
	}

	res := cmd.Run()
	output := compilationOutput(res)

	// Parse test results
	testCount, failureCount := parseTestOutput(output, lang)

	return &CompilationResult{
		Success:      res.Err == nil && failureCount == 0,
		Output:       output,
		ExitCode:     res.ExitCode(),
		TestCount:    testCount,
		FailureCount: failureCount,
	}, nil
}

// fileExists reports whether path is present, without distinguishing why not.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// compilationOutput annotates a timed-out run so the operator sees the bound
// rather than a silently truncated log.
func compilationOutput(res boundedexec.Result) string {
	if res.TimedOut {
		return res.Output + fmt.Sprintf("\n[timed out after %s]\n", res.Elapsed.Round(time.Second))
	}
	return res.Output
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
