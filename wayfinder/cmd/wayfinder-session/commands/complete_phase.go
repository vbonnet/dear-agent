package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/git"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/tracker"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/validator"
)

var (
	phaseOutcome       string
	phaseContext       string
	hashMismatchReason string
)

// CompletePhaseCmd is the cobra command that marks a phase as completed.
var CompletePhaseCmd = &cobra.Command{
	Use:   "complete-phase <phase-name>",
	Short: "Mark a phase as completed",
	Long: `Update WAYFINDER-STATUS.md, publish phase.completed event, and optionally commit to git.

Example:
  wayfinder-session complete-phase PROBLEM --outcome success
  wayfinder-session complete-phase PROBLEM --outcome success --context "Completed user research interviews"`,
	Args: cobra.ExactArgs(1),
	RunE: runCompletePhase,
}

func init() {
	CompletePhaseCmd.Flags().StringVar(&phaseOutcome, "outcome", "success", "Phase outcome (success|partial|skipped)")
	CompletePhaseCmd.Flags().StringVar(&phaseContext, "context", "", "Context for git commit message (optional)")
	CompletePhaseCmd.Flags().StringVar(&hashMismatchReason, "reason", "", "Reason for overriding hash mismatch validation (optional)")
}

func runCompletePhase(cmd *cobra.Command, args []string) error {
	phaseName := args[0]

	// Get project directory
	projectDir := GetProjectDirectory()

	// Read existing STATUS from project directory (version-aware)
	st, err := status.LoadAnyVersion(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read STATUS file: %w", err)
	}

	// Initialize history logger
	hist := history.New(projectDir)

	// Validate phase can be completed
	v := validator.NewValidator(st)
	if err := v.CanCompletePhase(phaseName, projectDir, hashMismatchReason); err != nil {
		// Log validation failure
		failureData := map[string]interface{}{
			"error": err.Error(),
		}
		if logErr := hist.AppendEvent(history.EventTypeValidationFailed, phaseName, failureData); logErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to log validation failure: %v\n", logErr)
		}
		return fmt.Errorf("validation failed: %w", err)
	}

	// Update phase status
	st.UpdatePhase(phaseName, status.PhaseStatusCompleted, phaseOutcome)

	// Initialize tracker
	tr, err := tracker.New(st.GetSessionID())
	if err != nil {
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer tr.Close(context.Background())

	// Collect phase-specific metadata
	metadata := map[string]interface{}{
		"outcome": phaseOutcome,
	}

	// For BUILD (Implementation), collect code and test file metrics
	if phaseName == "BUILD" {
		metadata["code_files_created"] = countCodeFiles(projectDir)
		metadata["tests_written"] = countTestFiles(projectDir)
	}

	// Publish phase.completed event
	if err := tr.CompletePhase(phaseName, phaseOutcome, metadata); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish phase.completed event: %v\n", err)
	}

	// Log phase completed to history
	completedData := map[string]interface{}{
		"outcome": phaseOutcome,
	}
	if err := hist.AppendEvent(history.EventTypePhaseCompleted, phaseName, completedData); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log phase.completed to history: %v\n", err)
	}

	// Write updated STATUS to project directory
	if err := st.WriteTo(projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	// Create git commit if in a git repository
	gitIntegrator := git.New(projectDir)
	if gitIntegrator.IsGitRepo() {
		if err := gitIntegrator.CommitPhaseCompletion(phaseName, phaseOutcome, phaseContext); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create git commit: %v\n", err)
		} else {
			fmt.Println("📝 Git commit created")
		}
	}

	fmt.Printf("✅ Phase %s completed (%s)\n", phaseName, phaseOutcome)
	return nil
}

// countCodeFiles recursively counts source code files in the project directory.
// Code files are identified by extension: .go, .py, .ts, .js, .rs, .java, .c, .cpp, .rb
// Excludes: node_modules, .venv, venv, vendor, .git, __pycache__, .next, dist, build
func countCodeFiles(root string) int {
	return countFiles(root, isCodeFile)
}

// countTestFiles recursively counts test files in the project directory.
// Test files are identified by patterns:
//   - Go: *_test.go
//   - Python: test_*.py or *_test.py
//   - TypeScript/JavaScript: *.test.ts, *.spec.js, etc.
func countTestFiles(root string) int {
	return countFiles(root, isTestFile)
}

// countFiles walks the directory tree and counts files matching the filter predicate.
func countFiles(root string, filter func(string) bool) int {
	count := 0
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}
		// Skip excluded directories
		if shouldExcludeDir(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if filter(path) {
			count++
		}
		return nil
	})
	return count
}

// isCodeFile returns true if the file is a source code file based on extension.
func isCodeFile(path string) bool {
	ext := filepath.Ext(path)
	codeExtensions := []string{".go", ".py", ".ts", ".js", ".rs", ".java", ".c", ".cpp", ".rb"}
	for _, codeExt := range codeExtensions {
		if ext == codeExt {
			return true
		}
	}
	return false
}

// isTestFile returns true if the file is a test file based on naming patterns.
func isTestFile(path string) bool {
	base := filepath.Base(path)

	// Go: *_test.go
	if strings.HasSuffix(base, "_test.go") {
		return true
	}

	// Python: test_*.py or *_test.py
	if strings.HasSuffix(base, ".py") {
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
			return true
		}
	}

	// TypeScript/JavaScript: *.test.ts, *.spec.js, etc.
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}

	return false
}

// shouldExcludeDir returns true if the path contains an excluded directory.
func shouldExcludeDir(path string) bool {
	excludeDirs := []string{"node_modules", ".venv", "venv", "vendor", ".git", "__pycache__", ".next", "dist", "build"}
	for _, dir := range excludeDirs {
		// Check if path contains the excluded directory
		if strings.Contains(path, string(os.PathSeparator)+dir+string(os.PathSeparator)) ||
			strings.HasSuffix(path, string(os.PathSeparator)+dir) {
			return true
		}
	}
	return false
}
