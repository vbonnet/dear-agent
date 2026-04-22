// Package main implements the pre-merge-commit Git hook with enhanced CI/CD gate enforcement.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/ci"
	"github.com/vbonnet/dear-agent/internal/ci/act"
)

const (
	// Environment variable to bypass gates in emergencies
	bypassEnvVar = "SKIP_CI_GATES" //nolint:gosec // not a credential, it's a feature flag name

	// Default policy file name
	defaultPolicyFile = ".ci-policy.yaml"
)

func main() {
	exitCode := run()
	os.Exit(exitCode)
}

//nolint:gocyclo // CLI entry point requires sequential flag/error handling
func run() int {
	// Parse flags
	dryRun := false
	verbose := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "-v", "--verbose":
			verbose = true
		case "-h", "--help":
			printHelp()
			return 0
		}
	}

	if verbose {
		fmt.Println("🔍 Pre-merge-commit hook started")
	}

	// Check if this is a merge commit
	if !isMergeCommit() {
		if verbose {
			fmt.Println("ℹ️  Not a merge commit - skipping CI gates")
		}
		return 0
	}

	// Get merge target branch
	targetBranch := getMergeTarget()
	if verbose {
		fmt.Printf("ℹ️  Merge target: %s\n", targetBranch)
	}

	// Load gate policy
	workingDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to get working directory: %v\n", err)
		return 1
	}

	policyPath := findPolicyFile(workingDir)
	if policyPath == "" {
		policyPath = filepath.Join(workingDir, defaultPolicyFile)
	}

	policy, err := ci.LoadPolicyFromConfig(policyPath, targetBranch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to load CI policy: %v\n", err)
		fmt.Fprintf(os.Stderr, "💡 Create %s to configure CI gates\n", defaultPolicyFile)
		return 1
	}

	if verbose {
		fmt.Printf("ℹ️  Loaded policy from: %s\n", policyPath)
		fmt.Printf("ℹ️  Required workflows: %v\n", policy.RequiredWorkflows)
		fmt.Printf("ℹ️  Failure behavior: %s\n", policy.FailureBehavior)
	}

	// Check for bypass
	if shouldBypass := checkBypass(policy, verbose); shouldBypass {
		return 0
	}

	// Check if any workflows are required
	if len(policy.RequiredWorkflows) == 0 {
		if verbose {
			fmt.Println("ℹ️  No required workflows configured - allowing merge")
		}
		return 0
	}

	// Dry-run mode
	if dryRun {
		fmt.Println("🧪 Dry-run mode - would execute workflows:")
		for _, workflow := range policy.RequiredWorkflows {
			fmt.Printf("  - %s\n", workflow)
		}
		if len(policy.OptionalWorkflows) > 0 {
			fmt.Println("  Optional workflows:")
			for _, workflow := range policy.OptionalWorkflows {
				fmt.Printf("  - %s\n", workflow)
			}
		}
		return 0
	}

	// Execute workflows
	fmt.Printf("🔍 Merge to %s detected - running CI gates...\n", targetBranch)
	exitCode := executeWorkflows(policy, workingDir, verbose)

	// Handle failure based on policy
	if exitCode != 0 {
		if policy.ShouldWarn() {
			fmt.Println("⚠️  CI checks failed but policy allows merge (warn mode)")
			return 0
		}
		if !policy.ShouldBlock() {
			if verbose {
				fmt.Println("ℹ️  CI checks failed but policy allows merge")
			}
			return 0
		}

		// Block mode - rollback and fail
		fmt.Fprintln(os.Stderr, "\n❌ CI gates failed - merge blocked")
		printRemediation()
		rollback()
		return 1
	}

	fmt.Println("✅ All CI gates passed - merge allowed")
	return 0
}

// executeWorkflows runs all required and optional workflows.
func executeWorkflows(policy *ci.GatePolicy, workingDir string, verbose bool) int {
	executor := act.NewActExecutor()
	ctx := context.Background()

	// Apply timeout if configured
	if policy.TimeoutMinutes > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, policy.Timeout())
		defer cancel()

		if verbose {
			fmt.Printf("ℹ️  Timeout set to %d minutes\n", policy.TimeoutMinutes)
		}
	}

	// Collect all workflows to execute
	allWorkflows := policy.AllWorkflows()
	if len(allWorkflows) == 0 {
		return 0
	}

	// Resolve workflow paths
	var workflowPaths []string
	for _, workflow := range allWorkflows {
		// If workflow is not absolute, assume it's in .github/workflows/
		if !filepath.IsAbs(workflow) {
			workflow = filepath.Join(".github", "workflows", workflow)
		}
		workflowPaths = append(workflowPaths, workflow)
	}

	// Create base request
	req := ci.PipelineRequest{
		EventType:  ci.EventPullRequest,
		WorkingDir: workingDir,
		OutputCallback: func(event ci.PipelineEvent) {
			if event.Type == ci.EventKindOutput && verbose {
				fmt.Print(event.Output)
			}
		},
	}

	// Execute workflows
	var result *act.MultiWorkflowResult
	var err error

	if policy.ParallelExecution && len(policy.WorkflowDependencies) > 0 {
		// Execute with dependencies
		if verbose {
			fmt.Println("ℹ️  Executing workflows with dependency order...")
		}
		result, err = executor.ExecuteWorkflowsWithDependencies(
			ctx,
			req,
			workflowPaths,
			policy.WorkflowDependencies,
		)
	} else {
		// Execute sequentially or in parallel
		if verbose {
			if policy.ParallelExecution {
				fmt.Println("ℹ️  Executing workflows in parallel...")
			} else {
				fmt.Println("ℹ️  Executing workflows sequentially...")
			}
		}
		result, err = executor.ExecuteWorkflows(
			ctx,
			req,
			workflowPaths,
			policy.ParallelExecution,
		)
	}

	// Handle infrastructure errors
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ CI execution failed: %v\n", err)
		return 1
	}

	// Report results
	return reportResults(result, policy, verbose)
}

// reportResults displays workflow results and determines exit code.
//
//nolint:gocyclo // result reporting requires iterating multiple result categories
func reportResults(result *act.MultiWorkflowResult, policy *ci.GatePolicy, verbose bool) int {
	fmt.Printf("\n📊 Workflow Results (completed in %s):\n", result.TotalDuration.Round(time.Second))

	requiredFailed := 0
	optionalFailed := 0

	for _, workflow := range policy.RequiredWorkflows {
		// Resolve path
		workflowPath := workflow
		if !filepath.IsAbs(workflowPath) {
			workflowPath = filepath.Join(".github", "workflows", workflowPath)
		}

		wfResult, exists := result.Results[workflowPath]
		if !exists {
			fmt.Printf("  ❓ %s - not executed\n", workflow)
			requiredFailed++
			continue
		}

		if wfResult.Skipped {
			fmt.Printf("  ⏭️  %s - skipped (%s)\n", workflow, wfResult.SkipReason)
			requiredFailed++
			continue
		}

		if wfResult.Error != nil {
			fmt.Printf("  ❌ %s - infrastructure error: %v\n", workflow, wfResult.Error)
			requiredFailed++
			continue
		}

		if wfResult.Result == nil {
			fmt.Printf("  ❓ %s - no result\n", workflow)
			requiredFailed++
			continue
		}

		if wfResult.Result.Success {
			duration := wfResult.Result.Duration.Round(time.Second)
			fmt.Printf("  ✅ %s - passed (%s)\n", workflow, duration)
		} else {
			duration := wfResult.Result.Duration.Round(time.Second)
			fmt.Printf("  ❌ %s - failed (exit %d, %s)\n",
				workflow, wfResult.Result.ExitCode, duration)
			if verbose && wfResult.Result.Output != "" {
				fmt.Println("\nOutput:")
				fmt.Println(wfResult.Result.Output)
			}
			requiredFailed++
		}
	}

	// Report optional workflows
	if len(policy.OptionalWorkflows) > 0 {
		fmt.Println("\nOptional workflows:")
		for _, workflow := range policy.OptionalWorkflows {
			workflowPath := workflow
			if !filepath.IsAbs(workflowPath) {
				workflowPath = filepath.Join(".github", "workflows", workflowPath)
			}

			wfResult, exists := result.Results[workflowPath]
			if !exists {
				fmt.Printf("  ❓ %s - not executed\n", workflow)
				continue
			}

			if wfResult.Skipped {
				fmt.Printf("  ⏭️  %s - skipped\n", workflow)
				continue
			}

			if wfResult.Error != nil {
				fmt.Printf("  ⚠️  %s - error (optional): %v\n", workflow, wfResult.Error)
				optionalFailed++
				continue
			}

			if wfResult.Result != nil && wfResult.Result.Success {
				fmt.Printf("  ✅ %s - passed\n", workflow)
			} else if wfResult.Result != nil {
				fmt.Printf("  ⚠️  %s - failed (optional)\n", workflow)
				optionalFailed++
			}
		}
	}

	// Determine exit code based on policy
	if policy.RequireAllPassing {
		// All required workflows must pass
		if requiredFailed > 0 {
			return 1
		}
	} else {
		// At least one required workflow must pass
		if requiredFailed == len(policy.RequiredWorkflows) {
			return 1
		}
	}

	return 0
}

// checkBypass checks if CI gates should be bypassed.
func checkBypass(policy *ci.GatePolicy, _ bool) bool { //nolint:unparam // verbose reserved for future logging
	bypassValue := os.Getenv(bypassEnvVar)
	if bypassValue == "" {
		return false
	}

	// Check if bypass is allowed
	if !policy.AllowBypass {
		fmt.Fprintf(os.Stderr, "⚠️  Bypass attempted but policy does not allow bypass\n")
		fmt.Fprintf(os.Stderr, "💡 Update .ci-policy.yaml to set allow_bypass: true\n")
		return false
	}

	// Parse bypass value
	bypassRequested := bypassValue == "true" || bypassValue == "1" || bypassValue == "yes"
	if !bypassRequested {
		return false
	}

	// Log bypass event
	fmt.Println("⚠️  CI GATE BYPASS ACTIVATED")
	fmt.Println("⚠️  This bypass has been logged for audit purposes")
	logBypass()

	return true
}

// logBypass records a bypass event for audit.
func logBypass() {
	// Log to stderr for git hook output capture
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	timestamp := time.Now().Format(time.RFC3339)
	branch := getMergeTarget()

	logEntry := fmt.Sprintf("[%s] User %s bypassed CI gates for merge to %s",
		timestamp, user, branch)

	// Log to a file if possible
	logFile := ".git/ci-gate-bypass.log"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil {
		fmt.Fprintln(f, logEntry)
		f.Close()
		fmt.Printf("📝 Bypass logged to %s\n", logFile)
	} else {
		// Fallback to stderr
		fmt.Fprintln(os.Stderr, "📝 "+logEntry)
	}
}

// printRemediation provides helpful advice for fixing CI failures.
func printRemediation() {
	fmt.Fprintln(os.Stderr, "\n💡 Remediation steps:")
	fmt.Fprintln(os.Stderr, "  1. Review workflow failures above")
	fmt.Fprintln(os.Stderr, "  2. Fix the issues in your branch")
	fmt.Fprintln(os.Stderr, "  3. Retry the merge")
	fmt.Fprintln(os.Stderr, "\n🚨 Emergency bypass (use with caution):")
	fmt.Fprintln(os.Stderr, "  SKIP_CI_GATES=true git merge <branch>")
	fmt.Fprintln(os.Stderr, "  (requires allow_bypass: true in .ci-policy.yaml)")
}

// isMergeCommit checks if a merge is in progress.
func isMergeCommit() bool {
	_, err := os.Stat(".git/MERGE_HEAD")
	return err == nil
}

// getMergeTarget returns the current branch being merged into.
func getMergeTarget() string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// rollback aborts the merge.
func rollback() {
	fmt.Println("🔄 Rolling back merge...")
	cmd := exec.Command("git", "reset", "--merge")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: rollback failed: %v\n", err)
	}
}

// findPolicyFile searches for .ci-policy.yaml in current and parent directories.
func findPolicyFile(startDir string) string {
	return ci.FindPolicyFile(startDir)
}

// printHelp displays usage information.
func printHelp() {
	fmt.Println("pre-merge-commit - Enhanced CI/CD gate enforcement for Git merges")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  This hook runs automatically during git merge")
	fmt.Println("  Manually: git hook run pre-merge-commit [options]")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  --dry-run         Show what would be executed without running")
	fmt.Println("  -v, --verbose     Show detailed execution information")
	fmt.Println("  -h, --help        Show this help message")
	fmt.Println()
	fmt.Println("ENVIRONMENT:")
	fmt.Println("  SKIP_CI_GATES=true   Bypass CI gates (requires allow_bypass in policy)")
	fmt.Println()
	fmt.Println("CONFIGURATION:")
	fmt.Println("  Create .ci-policy.yaml in repository root to configure gates")
	fmt.Println("  See docs/CI_GATES.md for examples and documentation")
}
