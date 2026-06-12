// ci-drift-guard diffs workflow job names against branch-protection required-status-checks.
//
// Usage:
//
//	ci-drift-guard [--workflows-dir .github/workflows] [--repo owner/repo] [--branch main]
//
// Exit codes: 0=clean, 1=drift found, 2=usage/setup error.
//
// Reads GITHUB_TOKEN from the environment for API access. When the env var is
// absent the tool still parses workflows and prints what it found, but skips
// the API call and reports that branch-protection checks are unknown.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+$`)
	branchRe = regexp.MustCompile(`^[A-Za-z0-9_./\-]+$`)
)

func main() {
	os.Exit(run())
}

func run() int {
	workflowsDir := flag.String("workflows-dir", ".github/workflows", "directory containing *.yml workflow files")
	repoFlag := flag.String("repo", "", "GitHub repo as owner/repo (default: read from GITHUB_REPOSITORY env var)")
	branch := flag.String("branch", "main", "branch to check protection on")
	flag.Parse()

	repo := *repoFlag
	if repo == "" {
		repo = os.Getenv("GITHUB_REPOSITORY")
	}
	if repo == "" {
		fmt.Fprintln(os.Stderr, "error: --repo or GITHUB_REPOSITORY must be set")
		return 2
	}

	jobNames, err := collectJobNames(*workflowsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading workflows: %v\n", err)
		return 2
	}
	if len(jobNames) == 0 {
		fmt.Fprintf(os.Stderr, "warning: no job names found in %s\n", *workflowsDir)
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Fprintf(os.Stderr, "warning: GITHUB_TOKEN not set; cannot fetch branch-protection checks\n")
		fmt.Println("Workflow job names found:")
		for _, j := range sortedKeys(jobNames) {
			fmt.Printf("  %s\n", j)
		}
		return 0
	}

	required, err := fetchRequiredChecks(context.Background(), token, repo, *branch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching branch protection: %v\n", err)
		return 2
	}

	drift := computeDrift(jobNames, required)
	if len(drift.missingFromWorkflows) == 0 && len(drift.missingFromProtection) == 0 {
		fmt.Printf("ok: %d required checks all match workflow job names\n", len(required))
		return 0
	}

	if len(drift.missingFromWorkflows) > 0 {
		fmt.Println("DRIFT: required checks with no matching workflow job (phantom checks):")
		for _, c := range drift.missingFromWorkflows {
			fmt.Printf("  - %q (in branch protection but not in any workflow)\n", c)
		}
	}
	if len(drift.missingFromProtection) > 0 {
		fmt.Println("INFO: workflow job names not in required-checks list (may be intentional):")
		for _, c := range drift.missingFromProtection {
			fmt.Printf("  + %q (in workflow but not required by branch protection)\n", c)
		}
	}
	if len(drift.missingFromWorkflows) > 0 {
		return 1
	}
	return 0
}

type workflowFile struct {
	Jobs map[string]any `yaml:"jobs"`
}

// collectJobNames reads all *.yml files in dir and returns a set of job names.
//
// For jobs using a build matrix, the `name:` field often contains template
// expressions like "Build & Test (${{ matrix.os }})". This function expands
// those by substituting each matrix value, so "Build & Test (${{ matrix.os }})"
// with matrix.os = [ubuntu-latest, macos-latest] produces:
//
//	"Build & Test (ubuntu-latest)" and "Build & Test (macos-latest)"
//
// Both the YAML job key and the (expanded) name: field are collected.
func collectJobNames(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	names := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var wf workflowFile
		if err := yaml.Unmarshal(data, &wf); err != nil {
			continue
		}
		for jobID, jobDef := range wf.Jobs {
			collectNamesFromJob(names, jobID, jobDef)
		}
	}
	return names, nil
}

// collectNamesFromJob registers all check names produced by a single job
// definition into the names set.
func collectNamesFromJob(names map[string]bool, jobID string, jobDef any) {
	names[jobID] = true
	m, ok := jobDef.(map[string]any)
	if !ok {
		return
	}
	nameVal, _ := m["name"].(string)
	if nameVal == "" {
		return
	}
	matrixValues := extractMatrix(m)
	for _, expanded := range expandName(nameVal, matrixValues) {
		names[expanded] = true
	}
	if isCodeQLJob(m) {
		collectCodeQLNames(names, jobID, nameVal, matrixValues)
	}
}

// collectCodeQLNames adds synthetic "{name} ({language})" check names that
// CodeQL registers instead of the plain job name.
func collectCodeQLNames(names map[string]bool, jobID, nameVal string, matrix map[string][]string) {
	langs, ok := matrix["language"]
	if !ok {
		langs = []string{"go"}
	}
	baseName := nameVal
	if baseName == "" {
		baseName = jobID
	}
	for _, lang := range langs {
		names[baseName+" ("+lang+")"] = true
	}
}

// extractMatrix returns a map of matrix variable name → slice of string values
// parsed from strategy.matrix in a job definition.
func extractMatrix(job map[string]any) map[string][]string {
	result := make(map[string][]string)
	strategy, ok := job["strategy"].(map[string]any)
	if !ok {
		return result
	}
	matrix, ok := strategy["matrix"].(map[string]any)
	if !ok {
		return result
	}
	for key, val := range matrix {
		switch v := val.(type) {
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					result[key] = append(result[key], s)
				}
			}
		case string:
			result[key] = []string{v}
		}
	}
	return result
}

// isCodeQLJob returns true if the job definition includes the
// github/codeql-action/analyze step, which registers checks as
// "{name} ({language})" rather than just "{name}".
func isCodeQLJob(job map[string]any) bool {
	steps, ok := job["steps"].([]any)
	if !ok {
		return false
	}
	for _, step := range steps {
		s, ok := step.(map[string]any)
		if !ok {
			continue
		}
		if uses, ok := s["uses"].(string); ok {
			if strings.HasPrefix(uses, "github/codeql-action/analyze") {
				return true
			}
		}
	}
	return false
}

// expandName substitutes ${{ matrix.KEY }} placeholders with every
// combination of values. Returns the original string if no placeholders.
func expandName(template string, matrix map[string][]string) []string {
	if !strings.Contains(template, "${{") {
		return []string{template}
	}
	results := []string{template}
	for key, vals := range matrix {
		placeholder := "${{ matrix." + key + " }}"
		var next []string
		for _, r := range results {
			if strings.Contains(r, placeholder) {
				for _, v := range vals {
					next = append(next, strings.ReplaceAll(r, placeholder, v))
				}
			} else {
				next = append(next, r)
			}
		}
		results = next
	}
	// Keep any remaining unexpanded templates as-is (they won't match).
	return results
}

type branchProtectionResponse struct {
	RequiredStatusChecks *struct {
		Contexts []string `json:"contexts"`
		Checks   []struct {
			Context string `json:"context"`
		} `json:"checks"`
	} `json:"required_status_checks"`
}

func fetchRequiredChecks(ctx context.Context, token, repo, branch string) ([]string, error) {
	if !repoRe.MatchString(repo) {
		return nil, fmt.Errorf("invalid repo %q: must be owner/repo with alphanumeric, hyphens, dots, underscores", repo)
	}
	if !branchRe.MatchString(branch) {
		return nil, fmt.Errorf("invalid branch %q: must contain only alphanumeric, hyphens, dots, underscores, slashes", branch)
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/branches/%s/protection", repo, branch)
	// The host is a hardcoded constant; the only interpolated segments are repo
	// and branch, both allowlist-validated above by repoRe/branchRe (no scheme,
	// host, "@", or ".." can appear). gosec's G704 taint analysis cannot see the
	// regex guards as sanitizers, so it flags apiURL as a false positive here.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil) //nolint:gosec // G704 false positive: repo/branch allowlist-validated above; host is constant
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("branch protection not configured (404)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, body)
	}

	var prot branchProtectionResponse
	if err := json.Unmarshal(body, &prot); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	if prot.RequiredStatusChecks == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var out []string
	for _, c := range prot.RequiredStatusChecks.Checks {
		if !seen[c.Context] {
			out = append(out, c.Context)
			seen[c.Context] = true
		}
	}
	for _, c := range prot.RequiredStatusChecks.Contexts {
		if !seen[c] {
			out = append(out, c)
			seen[c] = true
		}
	}
	return out, nil
}

type driftResult struct {
	missingFromWorkflows  []string
	missingFromProtection []string
}

func computeDrift(jobNames map[string]bool, required []string) driftResult {
	var d driftResult
	requiredSet := make(map[string]bool, len(required))
	for _, r := range required {
		requiredSet[r] = true
		if !jobNames[r] {
			d.missingFromWorkflows = append(d.missingFromWorkflows, r)
		}
	}
	for _, j := range sortedKeys(jobNames) {
		if !requiredSet[j] {
			d.missingFromProtection = append(d.missingFromProtection, j)
		}
	}
	sort.Strings(d.missingFromWorkflows)
	sort.Strings(d.missingFromProtection)
	return d
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
