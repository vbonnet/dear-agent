// Package main implements a git pre-push hook that validates CI workflows
// locally using nektos/act before allowing a push.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	// Git pre-push hook receives lines on stdin: <local ref> <local sha> <remote ref> <remote sha>
	// We just need to know a push is happening - the changed files are determined by git diff
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		// No input means nothing to push
		os.Exit(0)
	}

	// Check if act is installed
	actPath, err := exec.LookPath("act")
	if err != nil {
		fmt.Fprintln(os.Stderr, "[act-validator] act not installed, skipping local CI validation")
		fmt.Fprintln(os.Stderr, "[act-validator] Install: https://github.com/nektos/act")
		os.Exit(0)
	}

	// Find repo root
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[act-validator] cannot find repo root: %v\n", err)
		os.Exit(0)
	}

	// Check for .actrc
	actrcPath := filepath.Join(repoRoot, ".actrc")
	if _, err := os.Stat(actrcPath); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "[act-validator] no .actrc found, skipping local CI validation")
		os.Exit(0)
	}

	// Determine which jobs to run based on changed files
	jobs := determineJobs(repoRoot)
	if len(jobs) == 0 {
		fmt.Fprintln(os.Stderr, "[act-validator] no relevant workflow jobs for changed files")
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "[act-validator] running %d job(s) via %s\n", len(jobs), actPath)

	// Find event file
	eventFile := filepath.Join(repoRoot, ".github", "act", "event-push.json")

	failed := 0
	for _, job := range jobs {
		fmt.Fprintf(os.Stderr, "[act-validator] running job: %s\n", job)

		args := []string{"-j", job}
		if _, err := os.Stat(eventFile); err == nil {
			args = append(args, "-e", eventFile)
		}

		cmd := exec.CommandContext(context.Background(), "act", args...)
		cmd.Dir = repoRoot
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[act-validator] FAIL: job %s failed: %v\n", job, err)
			failed++
		} else {
			fmt.Fprintf(os.Stderr, "[act-validator] PASS: job %s\n", job)
		}
	}

	if failed > 0 {
		fmt.Fprintf(os.Stderr, "\n[act-validator] %d/%d job(s) failed\n", failed, len(jobs))
		fmt.Fprintln(os.Stderr, "[act-validator] use --no-verify to bypass this check")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\n[act-validator] all %d job(s) passed\n", len(jobs))
}

func findRepoRoot() (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func determineJobs(repoRoot string) []string {
	// Get changed files relative to the remote tracking branch
	cmd := exec.CommandContext(context.Background(), "git", "diff", "--name-only", "@{push}..HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		// Fallback: run lint job if we can't determine changes
		return []string{"lint"}
	}

	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(files) == 0 || (len(files) == 1 && files[0] == "") {
		return nil
	}

	jobSet := make(map[string]bool)

	for _, f := range files {
		switch {
		case strings.HasPrefix(f, "core/"):
			jobSet["test"] = true
			jobSet["lint"] = true
		case strings.HasPrefix(f, "hooks/"):
			jobSet["lint"] = true
		case strings.HasPrefix(f, "agm/"):
			jobSet["unit-tests"] = true
			jobSet["lint"] = true
		case strings.HasPrefix(f, "corpus-callosum/"):
			jobSet["lint"] = true
		case strings.HasSuffix(f, ".go"):
			jobSet["lint"] = true
		case strings.HasPrefix(f, ".github/"):
			jobSet["lint"] = true
		case f == ".golangci.yml":
			jobSet["lint"] = true
		}
	}

	var jobs []string
	for j := range jobSet {
		jobs = append(jobs, j)
	}
	return jobs
}
