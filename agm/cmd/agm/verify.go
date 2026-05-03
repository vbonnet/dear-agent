package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	verifyBranch      string
	verifyRepoDir2    string
	verifyRecordTrust bool
	verifyAutoArchive bool
	verifyAll         bool
)

var verifyWorkerCmd = &cobra.Command{
	Use:   "verify [session-name]",
	Short: "Verify worker output by checking for git commits on session branch",
	Long: `Check whether a worker session produced actual git commits on its branch.

This command resolves the session's working directory and branch, then checks
git log for commits relative to the main branch. It detects false completions
where a worker claimed to finish but produced no commits.

When --record-trust is enabled (default), the result is automatically recorded
as a trust event: "success" for commits found, "false_completion" for none.

Use --auto-archive to automatically archive sessions that pass verification.
Use --all to verify all DONE/OFFLINE sessions at once.
Use --quality-gate to run holdout quality checks from .agm/quality-gates.yaml.

Exit codes:
  0 - Commits found (COMMITS_FOUND) or all quality gates pass
  1 - No commits found (FALSE_COMPLETION), quality gate failure, or error

Examples:
  agm verify my-worker-session
  agm verify my-worker-session --branch feature/fix
  agm verify my-worker-session --repo-dir /path/to/repo
  agm verify my-worker-session --auto-archive
  agm verify my-worker-session --no-record-trust
  agm verify --all --repo-dir /path/to/repo
  agm verify my-worker-session --quality-gate
  agm verify my-worker-session --quality-gate --quality-config /path/to/gates.yaml`,
	Args: cobra.MaximumNArgs(1),
	RunE: runVerifyWorker,
}

func init() {
	verifyWorkerCmd.Flags().StringVar(&verifyBranch, "branch", "", "Branch to check (default: agm/<session-id> or session name)")
	verifyWorkerCmd.Flags().StringVar(&verifyRepoDir2, "repo-dir", "", "Repository directory (default: session's working directory)")
	verifyWorkerCmd.Flags().BoolVar(&verifyRecordTrust, "record-trust", true, "Record trust event on verify result")
	verifyWorkerCmd.Flags().BoolVar(&verifyAutoArchive, "auto-archive", false, "Automatically archive session on successful verification")
	verifyWorkerCmd.Flags().BoolVar(&verifyAll, "all", false, "Verify all DONE/OFFLINE sessions")
	rootCmd.AddCommand(verifyWorkerCmd)
}

// CommitInfo holds parsed git log output for a single commit.
type CommitInfo struct {
	Hash    string
	Subject string
}

// VerifyResult holds the outcome of verifying a single session.
type VerifyResult struct {
	SessionName string `json:"session_name"`
	Status      string `json:"status"` // COMMITS_FOUND, FALSE_COMPLETION, ERROR
	CommitCount int    `json:"commit_count"`
	Error       string `json:"error,omitempty"`
}

func runVerifyWorker(cmd *cobra.Command, args []string) error {
	if verifyContractDrift {
		return runVerifyContractDrift()
	}

	if verifyAll {
		if len(args) > 0 {
			return fmt.Errorf("cannot specify session name with --all flag")
		}
		return runVerifyAll()
	}

	if len(args) == 0 {
		return fmt.Errorf("session name required (or use --all to verify all DONE/OFFLINE sessions)")
	}

	sessionName := args[0]

	if verifyQualityGate {
		return runVerifyQualityGate(sessionName)
	}

	// Resolve session from storage
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	m, err := adapter.GetSessionByName(sessionName)
	if err != nil {
		// Try by session ID
		m, err = adapter.GetSession(sessionName)
		if err != nil {
			ui.PrintError(err, "Failed to find session",
				"  * Session may not exist: "+sessionName+"\n"+
					"  * Try: agm session list")
			return err
		}
	}

	return verifySingleWorker(adapter, m)
}

func verifySingleWorker(adapter *dolt.Adapter, m *manifest.Manifest) error {
	sessionName := sessionDisplayNameVerify(m)

	// Determine repo directory
	repoDir, err := resolveRepoDir(verifyRepoDir2, m.Context.Project, m.WorkingDirectory, sessionName)
	if err != nil {
		return err
	}

	// Determine and validate branch
	branch, err := resolveAndValidateBranch(verifyBranch, m.SessionID, m.Name, repoDir)
	if err != nil {
		// Branch not found = false completion
		if verifyRecordTrust {
			recordTrustEvent(sessionName, "false_completion", "branch not found")
		}
		return err
	}

	// Get commits on branch relative to main
	commits, err := getCommitsOnBranch(repoDir, branch)
	if err != nil {
		return fmt.Errorf("failed to check git log: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Session:  %s\n", sessionName)
	fmt.Fprintf(os.Stderr, "Branch:   %s\n", branch)
	fmt.Fprintf(os.Stderr, "Repo:     %s\n", repoDir)
	fmt.Fprintln(os.Stderr)

	if len(commits) == 0 {
		fmt.Println("FALSE_COMPLETION")
		fmt.Fprintf(os.Stderr, "No commits found on branch %q relative to main.\n", branch)

		if verifyRecordTrust {
			recordTrustEvent(sessionName, "false_completion", "0 commits on branch")
		}

		return fmt.Errorf("false completion: no commits on branch %s", branch)
	}

	fmt.Printf("COMMITS_FOUND: %d\n", len(commits))
	for _, c := range commits {
		fmt.Printf("  %s %s\n", c.Hash, c.Subject)
	}

	if verifyRecordTrust {
		recordTrustEvent(sessionName, "success", fmt.Sprintf("%d commits found", len(commits)))
	}

	if verifyAutoArchive {
		archiveVerifiedSession(adapter, sessionName)
	}

	return nil
}

// recordTrustEvent records a trust event, logging warnings on failure.
func recordTrustEvent(sessionName, eventType, detail string) {
	opCtx := newOpContext()
	_, err := ops.TrustRecord(opCtx, &ops.TrustRecordRequest{
		SessionName: sessionName,
		EventType:   eventType,
		Detail:      detail,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to record trust event: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "Trust: recorded %s event for %s\n", eventType, sessionName)
}

// archiveVerifiedSession archives a session after successful verification.
func archiveVerifiedSession(adapter *dolt.Adapter, sessionName string) {
	opCtx := &ops.OpContext{
		Storage: adapter,
		Tmux:    tmuxClient,
		Manager: managerBackend,
	}
	_, err := ops.ArchiveSession(opCtx, &ops.ArchiveSessionRequest{
		Identifier: sessionName,
		Force:      true, // Skip pre-archive checks since we just verified
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-archive session %s: %v\n", sessionName, err)
		return
	}
	fmt.Fprintf(os.Stderr, "Auto-archived session: %s\n", sessionName)
}

// runVerifyAll verifies all DONE/OFFLINE sessions.
func runVerifyAll() error {
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	// List all non-archived sessions
	all, err := adapter.ListSessions(&dolt.SessionFilter{
		ExcludeArchived: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Filter to DONE/OFFLINE sessions
	var candidates []*manifest.Manifest
	for _, m := range all {
		if m.State == manifest.StateDone || m.State == manifest.StateOffline {
			candidates = append(candidates, m)
		}
	}

	if len(candidates) == 0 {
		fmt.Println("No DONE/OFFLINE sessions found to verify.")
		return nil
	}

	fmt.Printf("Verifying %d DONE/OFFLINE session(s)...\n\n", len(candidates))

	var results []VerifyResult
	var successCount, falseCount, errorCount int

	for i, m := range candidates {
		name := sessionDisplayNameVerify(m)
		fmt.Printf("[%d/%d] %s\n", i+1, len(candidates), name)

		result := verifyOneForAll(adapter, m)
		results = append(results, result)

		switch result.Status {
		case "COMMITS_FOUND":
			successCount++
		case "FALSE_COMPLETION":
			falseCount++
		default:
			errorCount++
		}
	}

	// Print summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Verify All Summary (%d sessions)\n", len(results))
	fmt.Println(strings.Repeat("=", 60))

	for _, r := range results {
		icon := "PASS"
		switch r.Status {
		case "FALSE_COMPLETION":
			icon = "FAIL"
		case "ERROR":
			icon = "ERR "
		}
		fmt.Printf("  [%s] %s", icon, r.SessionName)
		if r.CommitCount > 0 {
			fmt.Printf(" (%d commits)", r.CommitCount)
		}
		if r.Error != "" {
			fmt.Printf(" — %s", r.Error)
		}
		fmt.Println()
	}

	fmt.Println()
	if successCount > 0 {
		ui.PrintSuccess(fmt.Sprintf("%d session(s) verified", successCount))
	}
	if falseCount > 0 {
		ui.PrintWarning(fmt.Sprintf("%d session(s) false completion", falseCount))
	}
	if errorCount > 0 {
		ui.PrintWarning(fmt.Sprintf("%d session(s) had errors", errorCount))
	}

	if falseCount > 0 {
		return fmt.Errorf("%d session(s) had false completions", falseCount)
	}
	return nil
}

// verifyOneForAll verifies a single session in --all mode, capturing the result.
func verifyOneForAll(adapter *dolt.Adapter, m *manifest.Manifest) VerifyResult {
	name := sessionDisplayNameVerify(m)

	// Determine repo directory
	repoDir, err := resolveRepoDir(verifyRepoDir2, m.Context.Project, m.WorkingDirectory, name)
	if err != nil {
		return VerifyResult{SessionName: name, Status: "ERROR", Error: err.Error()}
	}

	// Determine and validate branch
	branch, err := resolveAndValidateBranch("", m.SessionID, m.Name, repoDir)
	if err != nil {
		if verifyRecordTrust {
			recordTrustEvent(name, "false_completion", "branch not found")
		}
		return VerifyResult{SessionName: name, Status: "FALSE_COMPLETION", Error: "branch not found"}
	}

	// Get commits on branch relative to main
	commits, err := getCommitsOnBranch(repoDir, branch)
	if err != nil {
		return VerifyResult{SessionName: name, Status: "ERROR", Error: err.Error()}
	}

	if len(commits) == 0 {
		if verifyRecordTrust {
			recordTrustEvent(name, "false_completion", "0 commits on branch")
		}
		return VerifyResult{SessionName: name, Status: "FALSE_COMPLETION", CommitCount: 0}
	}

	if verifyRecordTrust {
		recordTrustEvent(name, "success", fmt.Sprintf("%d commits found", len(commits)))
	}

	if verifyAutoArchive {
		archiveVerifiedSession(adapter, name)
	}

	return VerifyResult{SessionName: name, Status: "COMMITS_FOUND", CommitCount: len(commits)}
}

// sessionDisplayNameVerify returns the best display name for a session.
func sessionDisplayNameVerify(m *manifest.Manifest) string {
	if m.Name != "" {
		return m.Name
	}
	if m.Tmux.SessionName != "" {
		return m.Tmux.SessionName
	}
	return m.SessionID
}

// resolveRepoDir determines the repository directory from flags or manifest fields.
func resolveRepoDir(flagDir, project, workingDir, sessionName string) (string, error) {
	repoDir := flagDir
	if repoDir == "" {
		repoDir = project
		if repoDir == "" {
			repoDir = workingDir
		}
	}
	if repoDir == "" {
		return "", fmt.Errorf("no repository directory found for session %q; use --repo-dir to specify", sessionName)
	}
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return "", fmt.Errorf("repository directory does not exist: %s", repoDir)
	}
	return repoDir, nil
}

// resolveAndValidateBranch determines the branch and verifies it exists in the repo.
func resolveAndValidateBranch(flagBranch, sessionID, name, repoDir string) (string, error) {
	branch := flagBranch
	if branch == "" {
		branch = resolveBranch(sessionID, name)
	}

	if branchExists(repoDir, branch) {
		return branch, nil
	}

	// Try alternate branch naming conventions
	alternatives := []string{
		"agm/" + sessionID,
		name,
		sessionID,
	}
	for _, alt := range alternatives {
		if alt != branch && branchExists(repoDir, alt) {
			return alt, nil
		}
	}

	fmt.Println("FALSE_COMPLETION")
	fmt.Fprintf(os.Stderr, "Branch %q not found in %s\n", branch, repoDir)
	fmt.Fprintf(os.Stderr, "Tried: %s\n", strings.Join(alternatives, ", "))
	return "", fmt.Errorf("branch not found: %s", branch)
}

// resolveBranch determines the branch name for a session.
// Convention: agm/<session-id> for sandbox sessions, session name otherwise.
func resolveBranch(sessionID, name string) string {
	if sessionID != "" {
		return "agm/" + sessionID
	}
	return name
}

// branchExists checks if a branch exists in the given repo.
func branchExists(repoDir, branch string) bool {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--verify", "--quiet", branch)
	return cmd.Run() == nil
}

// getCommitsOnBranch returns commits on branch that are not on main.
func getCommitsOnBranch(repoDir, branch string) ([]CommitInfo, error) {
	// Determine main branch name
	mainBranch := detectMainBranch(repoDir)

	// git log main..<branch> --oneline
	cmd := exec.Command("git", "-C", repoDir, "log",
		fmt.Sprintf("%s..%s", mainBranch, branch),
		"--format=%h %s",
		"--no-merges",
	)
	output, err := cmd.Output()
	if err != nil {
		// If the main branch doesn't exist, try without range
		cmd2 := exec.Command("git", "-C", repoDir, "log",
			branch, "--format=%h %s", "--no-merges", "-20",
		)
		output, err = cmd2.Output()
		if err != nil {
			return nil, fmt.Errorf("git log failed: %w", err)
		}
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var commits []CommitInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		ci := CommitInfo{Hash: parts[0]}
		if len(parts) > 1 {
			ci.Subject = parts[1]
		}
		commits = append(commits, ci)
	}
	return commits, nil
}

// detectMainBranch returns the name of the main branch (main or master).
func detectMainBranch(repoDir string) string {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "--verify", "--quiet", "main")
	if cmd.Run() == nil {
		return "main"
	}
	return "master"
}
