package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/agm/internal/verify"
)

var (
	verifyRepoDir     string
	verifyShowAll     bool
	verifyExtraAssert []string
)

var verifyCmd = &cobra.Command{
	Use:   "verify <session-id>",
	Short: "Verify session completion against invariants from the original prompt",
	Long: `Verify that a session actually completed its stated goal by checking invariant assertions.

This command:
  1. Reads the session manifest for the original prompt/purpose
  2. Extracts negative assertions (things that should be gone)
  3. Extracts positive assertions (things that should exist)
  4. Runs repo-wide searches for each assertion
  5. Reports PASS/FAIL with evidence

Examples:
  # Verify a session completed correctly
  agm session verify abc-123

  # Verify against a specific repo directory
  agm session verify abc-123 --repo-dir /path/to/repo

  # Add extra assertions manually
  agm session verify abc-123 --assert "neg:go.temporal.io:go.mod"
  agm session verify abc-123 --assert "pos:broadcastFromMPC"
  agm session verify abc-123 --assert "dir-neg:coordinator"

Assert format:
  neg:<pattern>[:<glob>]      Pattern should NOT be found
  pos:<pattern>[:<glob>]      Pattern SHOULD be found
  dir-neg:<path>              Directory should NOT exist
  dir-pos:<path>              Directory SHOULD exist`,
	Args: cobra.ExactArgs(1),
	RunE: runVerifyCompletion,
}

func init() {
	verifyCmd.Flags().StringVar(&verifyRepoDir, "repo-dir", "", "Repository directory to verify against (defaults to session's working directory)")
	verifyCmd.Flags().BoolVar(&verifyShowAll, "show-all", false, "Show all assertions including passing ones")
	verifyCmd.Flags().StringArrayVar(&verifyExtraAssert, "assert", nil, "Extra assertions (format: neg:<pattern>[:<glob>], pos:<pattern>[:<glob>], dir-neg:<path>, dir-pos:<path>)")
	sessionCmd.AddCommand(verifyCmd)
}

func runVerifyCompletion(cmd *cobra.Command, args []string) error {
	sessionID := args[0]

	// Get session manifest from Dolt
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	m, err := adapter.GetSession(sessionID)
	if err != nil {
		ui.PrintError(err, "Failed to read session",
			"  * Session may not exist: "+sessionID+"\n"+
				"  * Try: agm session list --all")
		return err
	}

	purpose := m.Context.Purpose
	if purpose == "" {
		purpose = m.Name
	}
	if purpose == "" {
		return fmt.Errorf("session %s has no purpose or name to extract assertions from", sessionID)
	}

	// Determine repo directory
	repoDir := verifyRepoDir
	if repoDir == "" {
		repoDir = m.Context.Project
		if repoDir == "" {
			repoDir = m.WorkingDirectory
		}
	}
	if repoDir == "" {
		return fmt.Errorf("no repository directory found; use --repo-dir to specify")
	}

	// Check repo directory exists
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return fmt.Errorf("repository directory does not exist: %s", repoDir)
	}

	// Extract assertions from prompt
	assertions := verify.ExtractAssertions(purpose)

	// Parse extra manual assertions
	for _, extra := range verifyExtraAssert {
		a, err := parseExtraAssertion(extra)
		if err != nil {
			return fmt.Errorf("invalid --assert %q: %w", extra, err)
		}
		assertions = append(assertions, a)
	}

	if len(assertions) == 0 {
		fmt.Printf("No verifiable assertions extracted from purpose:\n  %q\n\n", purpose)
		fmt.Printf("Use --assert to add assertions manually.\n")
		return nil
	}

	// Run verification
	fmt.Printf("Verifying session: %s\n", sessionID)
	fmt.Printf("Purpose: %s\n", purpose)
	fmt.Printf("Repo: %s\n", repoDir)
	fmt.Printf("Assertions: %d\n\n", len(assertions))

	report := verify.Verify(sessionID, purpose, repoDir, assertions)

	printVerifyAssertionResults(report.Results)

	fmt.Println()
	if report.Passed() {
		ui.PrintSuccess(fmt.Sprintf("VERIFIED: %d/%d assertions passed", report.PassCount(), len(report.Results)))
		return nil
	}

	fmt.Printf("FAILED: %d/%d assertions failed\n", report.FailCount(), len(report.Results))
	return fmt.Errorf("verification failed: %d assertion(s) did not pass", report.FailCount())
}

// printVerifyAssertionResults prints PASS/FAIL lines for each verification
// result, including any associated evidence (PASS lines only when --show-all).
func printVerifyAssertionResults(results []verify.Result) {
	for _, r := range results {
		if r.Pass {
			if !verifyShowAll {
				continue
			}
			fmt.Printf("  PASS  %s\n", r.Assertion.Description)
		} else {
			fmt.Printf("  FAIL  %s\n", r.Assertion.Description)
		}
		if r.Evidence == "" {
			continue
		}
		for _, line := range strings.Split(r.Evidence, "\n") {
			if line != "" {
				fmt.Printf("        %s\n", line)
			}
		}
	}
}

func parseExtraAssertion(s string) (verify.Assertion, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return verify.Assertion{}, fmt.Errorf("expected format type:value[:glob], got %q", s)
	}

	typ := parts[0]
	value := parts[1]

	switch typ {
	case "neg":
		a := verify.Assertion{
			Type:        verify.Negative,
			Description: fmt.Sprintf("should not contain: %s", value),
			Pattern:     value,
		}
		if len(parts) == 3 {
			a.GlobPattern = parts[2]
		}
		return a, nil
	case "pos":
		a := verify.Assertion{
			Type:        verify.Positive,
			Description: fmt.Sprintf("should contain: %s", value),
			Pattern:     value,
		}
		if len(parts) == 3 {
			a.GlobPattern = parts[2]
		}
		return a, nil
	case "dir-neg":
		return verify.Assertion{
			Type:        verify.Negative,
			Description: fmt.Sprintf("directory should not exist: %s", value),
			PathCheck:   value,
		}, nil
	case "dir-pos":
		return verify.Assertion{
			Type:        verify.Positive,
			Description: fmt.Sprintf("directory should exist: %s", value),
			PathCheck:   value,
		}, nil
	default:
		return verify.Assertion{}, fmt.Errorf("unknown assertion type %q (use neg, pos, dir-neg, dir-pos)", typ)
	}
}
