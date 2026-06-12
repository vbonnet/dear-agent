// src-health checks the 7 canonical ~/src repositories for drift.
//
// For each repo it verifies: branch is the expected default (main/master),
// working tree is clean, not ahead of origin (unpushed commits), and not more
// than 5 behind origin (stale checkout). Any violation is reported on stdout
// and counted; the exit code equals the number of violations (0 = all healthy).
//
// This binary is the Phase-A canary for the host-side scheduler (ce-cd14).
// Wire it as an agm loop via:
//
//	agm loop new src-health --cadence 4h \
//	  --cmd 'agm-job run src-health --verify "src-health --verify-only" -- src-health'
//	agm loop run src-health
//
// Usage:
//
//	src-health [--verify-only] [--json]
//
// Exit codes: 0=all healthy, N=violation count, 99=unexpected error.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// repos is the canonical set of ~/src checkouts to monitor.
var repos = []repoSpec{
	{dir: "dear-agent", defaultBranch: "main"},
	{dir: "brain-v2", defaultBranch: "main"},
	{dir: "engram-research", defaultBranch: "main"},
	{dir: "dotfiles", defaultBranch: "main"},
	{dir: "chezmoi", defaultBranch: "master"},
	{dir: "ai-conversation-logs", defaultBranch: "main"},
	{dir: "ai-tools", defaultBranch: "main"},
}

const maxBehind = 5

type repoSpec struct {
	dir           string
	defaultBranch string
}

type repoResult struct {
	Dir            string    `json:"dir"`
	Branch         string    `json:"branch"`
	ExpectedBranch string    `json:"expected_branch"`
	IsClean        bool      `json:"is_clean"`
	Ahead          int       `json:"ahead"`
	Behind         int       `json:"behind"`
	Violations     []string  `json:"violations,omitempty"`
	Missing        bool      `json:"missing"`
	CheckedAt      time.Time `json:"checked_at"`
}

func main() {
	verifyOnly, jsonOut := parseFlags()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "get home dir:", err)
		os.Exit(99)
	}

	if verifyOnly {
		os.Exit(runVerify(homeDir))
	}

	results, violations := runChecks(homeDir)
	printResults(results, violations, jsonOut)

	if err := writeReport(homeDir, results, violations); err != nil {
		fmt.Fprintln(os.Stderr, "write report:", err)
	}

	if violations > 99 {
		os.Exit(99)
	}
	os.Exit(violations)
}

func parseFlags() (verifyOnly, jsonOut bool) {
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--verify-only":
			verifyOnly = true
		case "--json":
			jsonOut = true
		}
	}
	return
}

func runChecks(homeDir string) ([]repoResult, int) {
	var results []repoResult
	violations := 0
	for _, spec := range repos {
		dir := filepath.Join(homeDir, "src", spec.dir)
		r := checkRepo(dir, spec)
		results = append(results, r)
		violations += len(r.Violations)
	}
	return results, violations
}

func printResults(results []repoResult, violations int, jsonOut bool) {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(results)
		return
	}
	for _, r := range results {
		if r.Missing {
			fmt.Printf("[MISSING] ~/src/%s (not found)\n", r.Dir)
			continue
		}
		if len(r.Violations) == 0 {
			fmt.Printf("[OK]      ~/src/%s  branch=%s  clean=true  ahead=%d  behind=%d\n",
				r.Dir, r.Branch, r.Ahead, r.Behind)
		} else {
			for _, v := range r.Violations {
				fmt.Printf("[VIOLATION] ~/src/%s: %s\n", r.Dir, v)
			}
		}
	}
	fmt.Printf("\n%d violation(s) across %d repos\n", violations, len(repos))
}

// runVerify checks that a fresh report file exists (used as the agm-job verify cmd).
// Returns the exit code: 0=fresh, 1=stale or missing.
func runVerify(homeDir string) int {
	reportPath := reportFilePath(homeDir)
	info, err := os.Stat(reportPath)
	if err != nil || time.Since(info.ModTime()) > 5*time.Hour {
		fmt.Fprintln(os.Stderr, "verify: no fresh report file at", reportPath)
		return 1
	}
	return 0
}

func checkRepo(dir string, spec repoSpec) repoResult {
	r := repoResult{
		Dir:            spec.dir,
		ExpectedBranch: spec.defaultBranch,
		CheckedAt:      time.Now().UTC(),
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		r.Missing = true
		r.Violations = append(r.Violations, "directory missing")
		return r
	}

	r.Branch = gitOutput(dir, "symbolic-ref", "--short", "HEAD")
	if r.Branch == "" {
		r.Branch = "(detached)"
	}

	// Is the working tree clean?
	statusOut := gitOutput(dir, "status", "--porcelain")
	r.IsClean = statusOut == ""

	// Ahead/behind counts (fetch is skipped to keep this fast and offline-safe;
	// behind reflects last-fetched state).
	r.Ahead = countRevs(dir, "@{u}..HEAD")
	r.Behind = countRevs(dir, "HEAD..@{u}")

	if r.Branch != spec.defaultBranch {
		r.Violations = append(r.Violations,
			fmt.Sprintf("branch=%q expected=%q", r.Branch, spec.defaultBranch))
	}
	if !r.IsClean {
		r.Violations = append(r.Violations, "working tree dirty")
	}
	if r.Ahead > 0 {
		r.Violations = append(r.Violations, fmt.Sprintf("ahead=%d (unpushed commits)", r.Ahead))
	}
	if r.Behind > maxBehind {
		r.Violations = append(r.Violations, fmt.Sprintf("behind=%d (stale, max=%d)", r.Behind, maxBehind))
	}

	return r
}

func gitOutput(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output() //#nosec G204 -- fixed git args
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func countRevs(dir, refRange string) int {
	out := gitOutput(dir, "rev-list", "--count", refRange)
	n, _ := strconv.Atoi(out)
	return n
}

func reportFilePath(homeDir string) string {
	return filepath.Join(homeDir, ".agm", "logs", "src-health-last.json")
}

func writeReport(homeDir string, results []repoResult, violations int) error {
	type report struct {
		GeneratedAt time.Time    `json:"generated_at"`
		Violations  int          `json:"violations"`
		Repos       []repoResult `json:"repos"`
	}
	r := report{
		GeneratedAt: time.Now().UTC(),
		Violations:  violations,
		Repos:       results,
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	reportPath := reportFilePath(homeDir)
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(reportPath, data, 0o600)
}
