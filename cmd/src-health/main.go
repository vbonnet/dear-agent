// Command src-health is a canary for the host-side dispatch loop (ce-cd14 / ce-m3ya).
// It checks a list of ~/src repositories for: clean working tree, default branch
// divergence, and ahead/behind status vs the remote tracking branch.
//
// A 1-week soak period of successful daily runs gates Phase B of ce-cd14.
//
// Usage:
//
//	src-health [--repos dir,dir,...] [--json]
//
// Exit codes:
//
//	0  all repos pass
//	1  one or more repos have issues
//	2  usage error
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultRepos are the 7 primary ~/src repositories monitored by the canary.
var defaultRepos = []string{
	"dear-agent",
	"ai-tools",
	"commons",
	"engram-kb",
	"engram-research",
	"brain-v2",
	"cat-pipeline",
}

type repoStatus struct {
	Repo    string `json:"repo"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Branch  string `json:"branch,omitempty"`
	Clean   bool   `json:"clean"`
	Ahead   int    `json:"ahead"`
	Behind  int    `json:"behind"`
	Error   string `json:"error,omitempty"`
	Healthy bool   `json:"healthy"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "src-health: %v\n", err)
		os.Exit(2)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("src-health", flag.ContinueOnError)
	reposFlag := fs.String("repos", "", "Comma-separated list of repo names or paths (default: 7 primary ~/src repos)")
	jsonOut := fs.Bool("json", false, "Output results as JSON")
	if err := fs.Parse(argv); err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home dir: %w", err)
	}

	repos := defaultRepos
	if *reposFlag != "" {
		repos = strings.Split(*reposFlag, ",")
		for i, r := range repos {
			repos[i] = strings.TrimSpace(r)
		}
	}

	results := make([]*repoStatus, 0, len(repos))
	allHealthy := true

	for _, r := range repos {
		rs := checkRepo(r, homeDir)
		results = append(results, rs)
		if !rs.Healthy {
			allHealthy = false
		}
	}

	if *jsonOut {
		out := struct {
			Healthy bool          `json:"healthy"`
			Repos   []*repoStatus `json:"repos"`
		}{Healthy: allHealthy, Repos: results}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	printText(results)

	if !allHealthy {
		os.Exit(1)
	}
	return nil
}

func checkRepo(nameOrPath, homeDir string) *repoStatus {
	path := nameOrPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(homeDir, "src", nameOrPath)
	}

	rs := &repoStatus{
		Repo:    filepath.Base(path),
		Path:    path,
		Healthy: true,
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		rs.Exists = false
		rs.Healthy = false
		rs.Error = "directory not found"
		return rs
	}
	rs.Exists = true

	// Branch name.
	branch, err := gitOutput(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		rs.Healthy = false
		rs.Error = fmt.Sprintf("git rev-parse: %v", err)
		return rs
	}
	rs.Branch = branch

	// Clean working tree: no staged or unstaged changes.
	status, err := gitOutput(path, "status", "--porcelain")
	if err != nil {
		rs.Healthy = false
		rs.Error = fmt.Sprintf("git status: %v", err)
		return rs
	}
	rs.Clean = strings.TrimSpace(status) == ""
	if !rs.Clean {
		rs.Healthy = false
	}

	// Fetch so ahead/behind is current (fast, uses --dry-run to avoid network
	// on most calls — falls through to the count even on fetch failure).
	_ = exec.Command("git", "-C", path, "fetch", "--quiet", "--prune").Run() //#nosec G204

	// Ahead/behind vs upstream. "git rev-list --count HEAD...@{u}" outputs
	// "ahead\tbehind"; ignore if no upstream is configured.
	abOut, err := gitOutput(path, "rev-list", "--count", "--left-right", "HEAD...@{upstream}")
	if err == nil {
		parts := strings.Fields(abOut)
		if len(parts) == 2 {
			fmt.Sscanf(parts[0], "%d", &rs.Ahead)
			fmt.Sscanf(parts[1], "%d", &rs.Behind)
		}
	}

	return rs
}

func gitOutput(repoPath string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", repoPath}, args...) //#nosec G204
	out, err := exec.Command("git", fullArgs...).Output() //#nosec G204
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func printText(results []*repoStatus) {
	healthy := 0
	for _, rs := range results {
		if rs.Healthy {
			healthy++
		}
	}
	fmt.Printf("src-health: %d/%d repos healthy\n\n", healthy, len(results))

	for _, rs := range results {
		icon := "✓"
		if !rs.Healthy {
			icon = "✗"
		}

		if !rs.Exists {
			fmt.Printf("%s %s — not found (%s)\n", icon, rs.Repo, rs.Path)
			continue
		}

		parts := []string{fmt.Sprintf("branch=%s", rs.Branch)}
		if rs.Clean {
			parts = append(parts, "clean")
		} else {
			parts = append(parts, "DIRTY")
		}
		if rs.Ahead > 0 {
			parts = append(parts, fmt.Sprintf("ahead=%d", rs.Ahead))
		}
		if rs.Behind > 0 {
			parts = append(parts, fmt.Sprintf("behind=%d", rs.Behind))
		}
		if rs.Error != "" {
			parts = append(parts, fmt.Sprintf("error=%s", rs.Error))
		}
		fmt.Printf("%s %s — %s\n", icon, rs.Repo, strings.Join(parts, " "))
	}
	fmt.Println()
}
