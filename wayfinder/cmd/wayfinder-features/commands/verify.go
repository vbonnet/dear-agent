package commands

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/s7"
)

var testCommand string

// VerifyCmd is the cobra command that verifies a feature by running its tests.
var VerifyCmd = &cobra.Command{
	Use:   "verify <feature-id>",
	Short: "Verify a feature by running tests",
	Long: `Verify a feature is complete by running its tests.

This command:
1. Runs the test command (from --test-command or S7 plan Verification column)
2. If tests pass (exit code 0):
   - Updates status to 'passing'
   - Sets verified_at timestamp
3. If tests fail:
   - Keeps status as is
   - Shows test output

Example:
  wayfinder-features verify auth-login
  wayfinder-features verify auth-login --test-command "npm test test/auth.test.ts"`,
	Args: cobra.ExactArgs(1),
	RunE: runVerify,
}

func init() {
	VerifyCmd.Flags().StringVar(&testCommand, "test-command", "", "Test command to run (overrides S7 plan)")
}

func runVerify(cmd *cobra.Command, args []string) error {
	featureID := args[0]
	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())

	// Load progress and find feature
	progressPath, prog, feature, err := loadProgressAndFeature(featureID, writer)
	if err != nil {
		return err
	}

	// Determine test command
	command, err := determineTestCommand(featureID, writer)
	if err != nil {
		return err
	}

	// Execute tests
	testOutput, err := executeTests(cmd, command, featureID, feature, writer)
	if err != nil {
		return err
	}

	// Update feature status
	if err := updateFeatureStatus(progressPath, prog, featureID, writer); err != nil {
		return err
	}

	// Display results and suggest next
	displayResults(cmd, featureID, testOutput, prog, writer)

	return nil
}

// loadProgressAndFeature loads progress file and finds the feature.
func loadProgressAndFeature(featureID string, _ *cliframe.Writer) (string, *progress.Progress, *progress.Feature, error) {
	progressPath, err := progress.FindProgressFile()
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to find progress file: %w", err)
	}

	prog, err := progress.ReadProgress(progressPath)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to read progress: %w", err)
	}

	feature, err := progress.FindFeature(prog, featureID)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to find feature: %w", err)
	}

	return progressPath, prog, feature, nil
}

// determineTestCommand determines the test command from flag or S7 plan.
func determineTestCommand(featureID string, writer *cliframe.Writer) (string, error) {
	if testCommand != "" {
		return testCommand, nil
	}

	// Get from S7 plan
	planPath, err := s7.FindS7Plan()
	if err != nil {
		writer.Error("Could not find S7 plan to get test command")
		writer.Info("Use --test-command to specify test command explicitly")
		return "", err
	}

	_, features, err := s7.ParseS7Plan(planPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse S7 plan: %w", err)
	}

	// Find verification command for this feature
	for _, f := range features {
		if f.ID == featureID {
			cmd := inferTestCommand(f.Verification)
			if cmd != "" {
				return cmd, nil
			}
		}
	}

	writer.Error(fmt.Sprintf("No test command found for feature '%s'", featureID))
	writer.Info("Specify test command with: --test-command \"<command>\"")
	return "", fmt.Errorf("no test command specified")
}

// executeTests runs the test command and handles failures.
//
// Security: `command` reaches us either from `--test-command` (which the
// invoking user typed explicitly) or from inferTestCommand parsing an S7
// plan markdown file under cwd. The latter is project-scoped and so the
// project's threat model treats it as hostile. Before we hand argv[0] to
// exec.Command, validateTestProgram enforces an allowlist of recognized
// test runners so a row like `| feat-x | desc | 5d | sh -c uname |` cannot
// reach exec.Command("sh", "-c", "uname").
func executeTests(cmd *cobra.Command, command, featureID string, feature *progress.Feature, writer *cliframe.Writer) ([]byte, error) {
	writer.Info(fmt.Sprintf("Running tests: %s", command))

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty test command")
	}
	if err := validateTestProgram(parts[0]); err != nil {
		return nil, err
	}
	testCmd := exec.Command(parts[0], parts[1:]...)
	testOutput, err := testCmd.CombinedOutput()

	if err != nil {
		writer.Error(fmt.Sprintf("Verification failed: %s", featureID))
		fmt.Fprintf(cmd.OutOrStdout(), "\nTest output:\n%s\n", string(testOutput))
		fmt.Fprintf(cmd.OutOrStdout(), "\nStatus remains: %s\n", feature.Status)
		fmt.Fprintf(cmd.OutOrStdout(), "Fix errors and retry: wayfinder-features verify %s\n", featureID)
		return nil, fmt.Errorf("tests failed")
	}

	return testOutput, nil
}

// updateFeatureStatus updates feature to passing status and writes progress.
func updateFeatureStatus(progressPath string, prog *progress.Progress, featureID string, _ *cliframe.Writer) error {
	now := time.Now()
	err := progress.UpdateFeature(prog, featureID, func(f *progress.Feature) {
		f.Status = progress.StatusPassing
		f.VerifiedAt = &now
	})
	if err != nil {
		return fmt.Errorf("failed to update feature: %w", err)
	}

	if err := progress.WriteProgress(progressPath, prog); err != nil {
		return fmt.Errorf("failed to update progress: %w", err)
	}

	return nil
}

// displayResults shows verification results and suggests next feature.
func displayResults(cmd *cobra.Command, featureID string, testOutput []byte, prog *progress.Progress, writer *cliframe.Writer) {
	writer.Success(fmt.Sprintf("Verified: %s", featureID))
	if len(testOutput) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\nTest output:\n%s\n", string(testOutput))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nStatus: passing\n")

	suggestNextFeature(cmd, prog, writer)
}

// suggestNextFeature suggests the next failing feature to work on.
func suggestNextFeature(cmd *cobra.Command, prog *progress.Progress, writer *cliframe.Writer) {
	for _, f := range prog.Features {
		if f.Status == progress.StatusFailing {
			fmt.Fprintf(cmd.OutOrStdout(), "\nNext: wayfinder-features start %s\n", f.ID)
			return
		}
	}

	writer.Success("All features verified! 🎉")
}

// inferTestCommand tries to infer the test command from the S7 plan's
// Verification cell. The previous implementation accepted any cell
// containing a space as a verbatim command — a row like `sh -c uname` then
// flowed straight to exec.Command("sh", "-c", "uname") inside any repo
// containing an S7 plan. We now only emit recognized test-runner shapes
// over a validated path, and we reject the verbatim-command branch.
func inferTestCommand(verification string) string {
	verification = strings.TrimSpace(verification)
	if verification == "" {
		return ""
	}
	if !isSafeTestPath(verification) {
		return ""
	}
	switch {
	case strings.HasSuffix(verification, ".test.ts"),
		strings.HasSuffix(verification, ".spec.ts"),
		strings.HasSuffix(verification, ".test.js"),
		strings.HasSuffix(verification, ".spec.js"):
		return fmt.Sprintf("npm test %s", verification)
	case strings.HasSuffix(verification, "_test.go"):
		return fmt.Sprintf("go test %s", verification)
	case strings.HasSuffix(verification, ".py"):
		return fmt.Sprintf("pytest %s", verification)
	}
	// Without a recognized test extension we refuse to guess. The user
	// can still supply --test-command explicitly.
	return ""
}

// isSafeTestPath restricts S7-plan-derived paths to a shape that cannot
// inject extra argv: no shell metacharacters, no whitespace, no traversal,
// no absolute path. The set is intentionally narrow.
func isSafeTestPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "-") {
		return false
	}
	if strings.Contains(p, "..") {
		return false
	}
	for _, r := range p {
		if !isSafeTestPathRune(r) {
			return false
		}
	}
	return true
}

func isSafeTestPathRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == '-' || r == '/':
		return true
	}
	return false
}

// validateTestProgram restricts the argv[0] passed to exec.Command to a
// fixed set of test runners. Defense in depth — inferTestCommand already
// only emits these shapes for S7-plan-derived input, but --test-command
// is also a vector and we want one chokepoint.
func validateTestProgram(prog string) error {
	switch prog {
	case "npm", "go", "pytest", "jest", "yarn", "pnpm":
		return nil
	}
	return fmt.Errorf("test program %q is not on the allowlist (npm, go, pytest, jest, yarn, pnpm)", prog)
}
