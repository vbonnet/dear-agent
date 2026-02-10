package oligo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitOperation represents a git operation result
type GitOperation struct {
	Success     bool
	HasConflict bool
	ConflictFiles []string
	Output      string
	Error       error
}

// OligoLoop handles automated git integration tasks
type OligoLoop struct {
	repoPath string
	engramBin string
}

// NewOligoLoop creates a new Oligo git automation loop
func NewOligoLoop(repoPath string, engramBin string) (*OligoLoop, error) {
	// Validate repo path
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", repoPath)
	}

	// Use default engram binary if not specified
	if engramBin == "" {
		engramBin = "engram" // Assume in PATH
	}

	return &OligoLoop{
		repoPath:  repoPath,
		engramBin: engramBin,
	}, nil
}

// ExecuteIntegration performs checkout -> merge -> conflict check cycle
func (o *OligoLoop) ExecuteIntegration(sourceBranch, targetBranch string) (*GitOperation, error) {
	// Step 1: Checkout target branch
	if err := o.checkout(targetBranch); err != nil {
		return &GitOperation{
			Success: false,
			Error:   fmt.Errorf("checkout failed: %w", err),
		}, nil
	}

	// Step 2: Attempt merge
	mergeResult := o.merge(sourceBranch)

	// Step 3: Check for conflicts
	if mergeResult.HasConflict {
		// Delegate conflict resolution to Python engram (the "Brain")
		resolveResult, err := o.resolveConflicts(mergeResult.ConflictFiles)
		if err != nil {
			return &GitOperation{
				Success:       false,
				HasConflict:   true,
				ConflictFiles: mergeResult.ConflictFiles,
				Error:         fmt.Errorf("conflict resolution failed: %w", err),
			}, nil
		}

		return resolveResult, nil
	}

	return mergeResult, nil
}

// checkout switches to specified branch
func (o *OligoLoop) checkout(branch string) error {
	cmd := exec.Command("git", "-C", o.repoPath, "checkout", branch)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s: %w (%s)", branch, err, stderr.String())
	}

	return nil
}

// merge attempts to merge source branch into current branch
func (o *OligoLoop) merge(sourceBranch string) *GitOperation {
	cmd := exec.Command("git", "-C", o.repoPath, "merge", sourceBranch)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String() + stderr.String()

	// Check for merge conflicts
	if err != nil && strings.Contains(output, "CONFLICT") {
		conflictFiles := o.detectConflictFiles()
		return &GitOperation{
			Success:       false,
			HasConflict:   true,
			ConflictFiles: conflictFiles,
			Output:        output,
			Error:         err,
		}
	}

	if err != nil {
		return &GitOperation{
			Success: false,
			Output:  output,
			Error:   err,
		}
	}

	return &GitOperation{
		Success:     true,
		HasConflict: false,
		Output:      output,
	}
}

// detectConflictFiles identifies files with merge conflicts
func (o *OligoLoop) detectConflictFiles() []string {
	cmd := exec.Command("git", "-C", o.repoPath, "diff", "--name-only", "--diff-filter=U")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return []string{}
	}

	files := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	// Filter out empty strings
	var nonEmpty []string
	for _, f := range files {
		if f != "" {
			nonEmpty = append(nonEmpty, f)
		}
	}

	return nonEmpty
}

// resolveConflicts delegates conflict resolution to Python engram
func (o *OligoLoop) resolveConflicts(conflictFiles []string) (*GitOperation, error) {
	if len(conflictFiles) == 0 {
		return &GitOperation{
			Success:     true,
			HasConflict: false,
			Output:      "No conflicts to resolve",
		}, nil
	}

	// Build engram invocation prompt
	prompt := o.buildConflictResolutionPrompt(conflictFiles)

	// Invoke Python engram (the "Brain") for semantic resolution
	cmd := exec.Command(o.engramBin, "resolve-conflict", "--prompt", prompt, "--repo", o.repoPath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	errorOutput := stderr.String()

	if err != nil {
		return &GitOperation{
			Success:       false,
			HasConflict:   true,
			ConflictFiles: conflictFiles,
			Output:        output + "\n" + errorOutput,
			Error:         fmt.Errorf("engram resolution failed: %w", err),
		}, err
	}

	// Verify conflicts resolved
	remainingConflicts := o.detectConflictFiles()

	if len(remainingConflicts) > 0 {
		return &GitOperation{
			Success:       false,
			HasConflict:   true,
			ConflictFiles: remainingConflicts,
			Output:        "Some conflicts remain unresolved",
			Error:         fmt.Errorf("%d conflicts still present", len(remainingConflicts)),
		}, nil
	}

	// Success - conflicts resolved
	return &GitOperation{
		Success:     true,
		HasConflict: false,
		Output:      output,
	}, nil
}

// buildConflictResolutionPrompt creates prompt for engram AI resolution
func (o *OligoLoop) buildConflictResolutionPrompt(conflictFiles []string) string {
	var b strings.Builder

	b.WriteString("Resolve merge conflicts in the following files:\n\n")

	for i, file := range conflictFiles {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, file))
	}

	b.WriteString("\nFor each file:\n")
	b.WriteString("1. Analyze both versions (HEAD and incoming)\n")
	b.WriteString("2. Determine semantic intent of each change\n")
	b.WriteString("3. Merge changes preserving both intents where possible\n")
	b.WriteString("4. Remove conflict markers (<<<<<<<, =======, >>>>>>>)\n")
	b.WriteString("5. Ensure code compiles and tests pass\n")
	b.WriteString("6. Stage resolved files with 'git add'\n")

	return b.String()
}

// AbortMerge aborts current merge operation
func (o *OligoLoop) AbortMerge() error {
	cmd := exec.Command("git", "-C", o.repoPath, "merge", "--abort")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git merge --abort: %w (%s)", err, stderr.String())
	}

	return nil
}

// GetCurrentBranch returns current git branch
func (o *OligoLoop) GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "-C", o.repoPath, "branch", "--show-current")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}
