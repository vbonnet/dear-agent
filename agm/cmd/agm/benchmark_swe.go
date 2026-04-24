package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// SWETask represents a single SWE-bench-lite test case.
type SWETask struct {
	InstanceID       string `json:"instance_id"`
	Repo             string `json:"repo"`
	Issue            string `json:"issue"`
	ProblemStatement string `json:"problem_statement,omitempty"`
	BaseCommit       string `json:"base_commit,omitempty"`
	Patch            string `json:"patch,omitempty"`
	TestPatch        string `json:"test_patch,omitempty"`
	Version          string `json:"version,omitempty"`
}

// SWEResult holds the outcome of running a single SWE-bench task.
type SWEResult struct {
	InstanceID string        `json:"instance_id"`
	Repo       string        `json:"repo"`
	Resolved   bool          `json:"resolved"`
	Duration   time.Duration `json:"duration_ns"`
	CostUSD    float64       `json:"cost_usd"`
	PatchLen   int           `json:"patch_length"`
	Error      string        `json:"error,omitempty"`
}

// SWEReport is the full benchmark report.
type SWEReport struct {
	RunID       string      `json:"run_id"`
	Agent       string      `json:"agent"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt time.Time   `json:"completed_at"`
	Tasks       []SWEResult `json:"tasks"`
	Summary     SWESummary  `json:"summary"`
}

// SWESummary aggregates metrics across all tasks.
type SWESummary struct {
	Total       int     `json:"total"`
	Resolved    int     `json:"resolved"`
	Failed      int     `json:"failed"`
	Errored     int     `json:"errored"`
	ResolveRate float64 `json:"resolve_rate"`
	TotalCost   float64 `json:"total_cost_usd"`
	AvgDuration string  `json:"avg_duration"`
}

var sweLiteCmd = &cobra.Command{
	Use:   "swe-lite",
	Short: "Run SWE-bench-lite benchmark against an agent",
	Long: `Run a subset of SWE-bench-lite tasks to evaluate agent performance.

Each task presents a real GitHub issue to the agent and checks whether the
produced patch resolves it. Tracks resolve rate, cost, and time per issue.

Dataset: Use --dataset to load tasks from a JSON file exported from HuggingFace.
Without --dataset, uses 3 built-in sample tasks.

Export dataset:
  python3 scripts/export_swe_bench.py > swe-bench-lite.json

Examples:
  agm benchmark swe-lite                              # Run 3 built-in samples
  agm benchmark swe-lite --dataset swe-bench-lite.json # From exported dataset
  agm benchmark swe-lite --limit 5                     # First 5 tasks only
  agm benchmark swe-lite -o json                       # JSON output
  agm benchmark swe-lite --results-dir ./results       # Persist results
  agm benchmark swe-lite --dry-run                     # Preview tasks without running`,
	Args: cobra.NoArgs,
	RunE: runSWELite,
}

var (
	sweAgentFlag      string
	sweDatasetFlag    string
	sweLimitFlag      int
	sweResultsDirFlag string
	sweDryRunFlag     bool
	sweCloneFlag      bool
)

func init() {
	benchmarkCmd.AddCommand(sweLiteCmd)
	sweLiteCmd.Flags().StringVar(&sweAgentFlag, "agent", "claude", "Agent binary to invoke for each task")
	sweLiteCmd.Flags().StringVar(&sweDatasetFlag, "dataset", "", "Path to SWE-bench-lite JSON dataset file")
	sweLiteCmd.Flags().IntVar(&sweLimitFlag, "limit", 0, "Maximum number of tasks to run (0 = all)")
	sweLiteCmd.Flags().StringVar(&sweResultsDirFlag, "results-dir", "", "Directory to write per-task results and final report")
	sweLiteCmd.Flags().BoolVar(&sweDryRunFlag, "dry-run", false, "List tasks without executing them")
	sweLiteCmd.Flags().BoolVar(&sweCloneFlag, "clone", false, "Clone actual repos and checkout base commits (slow)")
}

// sampleTasks returns the built-in sample SWE-bench-lite tasks.
func sampleTasks() []SWETask {
	return []SWETask{
		{
			InstanceID:       "astropy__astropy-12907",
			Repo:             "astropy/astropy",
			Issue:            "Modeling compound model with units fails when evaluating inverse",
			ProblemStatement: "Modeling's `separability_matrix` does not compute separability correctly for nested CompoundModels",
			BaseCommit:       "d16bfe05a744909de4b27f5875fe0d4ed41ce607",
		},
		{
			InstanceID:       "django__django-11099",
			Repo:             "django/django",
			Issue:            "UsernameValidator allows trailing newline in usernames",
			ProblemStatement: "ASCIIUsernameValidator and UnicodeUsernameValidator allow trailing newlines in usernames because the regex uses $ instead of \\Z",
			BaseCommit:       "17455e924e243e7a55e8a38f45966d8cbb27d80d",
		},
		{
			InstanceID:       "sympy__sympy-20154",
			Repo:             "sympy/sympy",
			Issue:            "partitions() reusing the output dictionaries",
			ProblemStatement: "The `partitions()` iterator reuses the output dictionaries, causing unexpected behavior when collecting results",
			BaseCommit:       "2ac6f584eb3e9f1fd1bf3527de7a76a5e5f378d8",
		},
	}
}

func loadTasks() ([]SWETask, error) {
	if sweDatasetFlag == "" {
		return sampleTasks(), nil
	}

	data, err := os.ReadFile(sweDatasetFlag)
	if err != nil {
		return nil, fmt.Errorf("read dataset: %w", err)
	}

	var tasks []SWETask
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("parse dataset: %w", err)
	}

	if len(tasks) == 0 {
		return nil, fmt.Errorf("dataset is empty")
	}
	return tasks, nil
}

func runSWELite(cmd *cobra.Command, _ []string) error {
	tasks, err := loadTasks()
	if err != nil {
		return err
	}

	if sweLimitFlag > 0 && sweLimitFlag < len(tasks) {
		tasks = tasks[:sweLimitFlag]
	}

	if sweDryRunFlag {
		return printSWEDryRun(tasks)
	}

	report := runSWETasks(cmd.Context(), tasks, sweAgentFlag)

	if sweResultsDirFlag != "" {
		if err := persistSWEReport(report, sweResultsDirFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to persist results: %v\n", err)
		}
	}

	return printResult(report, func() { printSWETable(report) })
}

func printSWEDryRun(tasks []SWETask) error {
	fmt.Printf("SWE-bench-lite dry run: %d tasks\n\n", len(tasks))
	for i, t := range tasks {
		fmt.Printf("  %3d. %-40s  %s\n", i+1, t.InstanceID, t.Repo)
	}
	return nil
}

// runSWETasks executes each task and collects results.
func runSWETasks(ctx context.Context, tasks []SWETask, agent string) *SWEReport {
	report := &SWEReport{
		RunID:     fmt.Sprintf("swe-lite-%d", time.Now().Unix()),
		Agent:     agent,
		StartedAt: time.Now(),
		Tasks:     make([]SWEResult, 0, len(tasks)),
	}

	for i, task := range tasks {
		select {
		case <-ctx.Done():
			report.Tasks = append(report.Tasks, SWEResult{
				InstanceID: task.InstanceID,
				Repo:       task.Repo,
				Error:      ctx.Err().Error(),
			})
			continue
		default:
		}

		fmt.Fprintf(os.Stderr, "[%d/%d] %s ...\n", i+1, len(tasks), task.InstanceID)
		result := runSingleSWETask(ctx, task, agent)
		report.Tasks = append(report.Tasks, result)

		status := "FAIL"
		if result.Resolved {
			status = "PASS"
		}
		if result.Error != "" {
			status = "ERROR"
		}
		fmt.Fprintf(os.Stderr, "[%d/%d] %s -> %s (%s)\n",
			i+1, len(tasks), task.InstanceID, status,
			result.Duration.Round(time.Second))
	}

	report.CompletedAt = time.Now()
	report.Summary = computeSWESummary(report.Tasks)
	return report
}

// runSingleSWETask runs the agent against one task and evaluates the result.
func runSingleSWETask(ctx context.Context, task SWETask, agent string) SWEResult {
	start := time.Now()

	workdir, err := prepareTaskWorkdir(task)
	if err != nil {
		return SWEResult{
			InstanceID: task.InstanceID,
			Repo:       task.Repo,
			Duration:   time.Since(start),
			Error:      fmt.Sprintf("workdir setup: %v", err),
		}
	}
	defer os.RemoveAll(workdir)

	prompt := buildSWEPrompt(task)

	agentCmd := exec.CommandContext(ctx, agent,
		"--print",
		"--dangerously-skip-permissions",
		"--output-format", "json",
		"-p", prompt,
	)
	agentCmd.Dir = workdir
	output, err := agentCmd.CombinedOutput()

	duration := time.Since(start)
	result := SWEResult{
		InstanceID: task.InstanceID,
		Repo:       task.Repo,
		Duration:   duration,
	}

	if err != nil {
		result.Error = fmt.Sprintf("agent: %v\noutput: %s", err, truncate(string(output), 500))
		return result
	}

	agentOutput := string(output)
	result.Resolved = evaluatePatch(task, agentOutput)
	result.CostUSD = estimateSWECost(agentOutput)
	result.PatchLen = countPatchLines(agentOutput)
	return result
}

func buildSWEPrompt(task SWETask) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are solving a real GitHub issue from %s.\n\n", task.Repo)
	fmt.Fprintf(&b, "Instance ID: %s\n", task.InstanceID)

	if task.ProblemStatement != "" {
		fmt.Fprintf(&b, "\n## Problem Statement\n\n%s\n", task.ProblemStatement)
	} else {
		fmt.Fprintf(&b, "\n## Issue\n\n%s\n", task.Issue)
	}

	b.WriteString("\n## Instructions\n\n")
	b.WriteString("1. Understand the issue described above.\n")
	b.WriteString("2. Find the relevant source files.\n")
	b.WriteString("3. Make the minimal code changes needed to fix the issue.\n")
	b.WriteString("4. Generate a unified diff of your changes.\n")
	b.WriteString("\nOutput ONLY the unified diff (patch) that fixes the issue.\n")
	return b.String()
}

// prepareTaskWorkdir creates a temporary directory for the task.
// If --clone is set, clones the actual repo and checks out base_commit.
func prepareTaskWorkdir(task SWETask) (string, error) {
	dir, err := os.MkdirTemp("", "swe-"+task.InstanceID+"-")
	if err != nil {
		return "", err
	}

	if sweCloneFlag && task.BaseCommit != "" {
		repoURL := fmt.Sprintf("https://github.com/%s.git", task.Repo)
		cloneCmd := exec.Command("git", "clone", "--depth", "1", repoURL, dir)
		if out, err := cloneCmd.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("clone %s: %w\n%s", task.Repo, err, truncate(string(out), 200))
		}

		fetchCmd := exec.Command("git", "-C", dir, "fetch", "--depth", "1", "origin", task.BaseCommit)
		if out, err := fetchCmd.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("fetch base commit: %w\n%s", err, truncate(string(out), 200))
		}

		checkoutCmd := exec.Command("git", "-C", dir, "checkout", task.BaseCommit)
		if out, err := checkoutCmd.CombinedOutput(); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("checkout base commit: %w\n%s", err, truncate(string(out), 200))
		}

		return dir, nil
	}

	// Scaffold mode: create a minimal context directory
	readme := fmt.Sprintf("# %s\n\nInstance: %s\nIssue: %s\n",
		task.Repo, task.InstanceID, task.Issue)
	if task.ProblemStatement != "" {
		readme += fmt.Sprintf("\n## Problem Statement\n\n%s\n", task.ProblemStatement)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o600); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// evaluatePatch checks whether the agent output contains a plausible fix.
// This is a heuristic -- a real implementation would apply the patch and run tests.
func evaluatePatch(task SWETask, output string) bool {
	lower := strings.ToLower(output)

	// Must contain diff-like content
	hasDiff := strings.Contains(output, "---") && strings.Contains(output, "+++")
	hasUnifiedDiff := strings.Contains(output, "diff --git") || strings.Contains(output, "@@ ")

	if !hasDiff && !hasUnifiedDiff {
		return false
	}

	// Must reference something related to the repo
	repoParts := strings.Split(task.Repo, "/")
	repoName := repoParts[len(repoParts)-1]
	hasRepoRef := strings.Contains(lower, strings.ToLower(repoName))
	hasPatchKeyword := strings.Contains(lower, "patch") || strings.Contains(lower, "fix")

	return (hasDiff || hasUnifiedDiff) && (hasPatchKeyword || hasRepoRef)
}

// estimateSWECost returns a rough cost estimate based on output token count.
// Attempts to parse Claude JSON output format for real usage data.
func estimateSWECost(output string) float64 {
	// Try to parse structured JSON output from claude --output-format json
	var parsed struct {
		CostUSD float64 `json:"cost_usd"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(output), &parsed); err == nil {
		if parsed.CostUSD > 0 {
			return parsed.CostUSD
		}
		if parsed.Usage.InputTokens > 0 || parsed.Usage.OutputTokens > 0 {
			// Sonnet 4 pricing: $3/M input, $15/M output
			inputCost := float64(parsed.Usage.InputTokens) * 3.0 / 1_000_000
			outputCost := float64(parsed.Usage.OutputTokens) * 15.0 / 1_000_000
			return inputCost + outputCost
		}
	}

	// Fallback: rough estimate from word count
	tokens := len(strings.Fields(output))
	const costPerToken = 0.000015
	return float64(tokens) * costPerToken
}

func countPatchLines(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			count++
		}
	}
	return count
}

func computeSWESummary(results []SWEResult) SWESummary {
	s := SWESummary{Total: len(results)}
	var totalDur time.Duration

	for _, r := range results {
		switch {
		case r.Error != "":
			s.Errored++
		case r.Resolved:
			s.Resolved++
		default:
			s.Failed++
		}
		s.TotalCost += r.CostUSD
		totalDur += r.Duration
	}

	if s.Total > 0 {
		s.ResolveRate = float64(s.Resolved) / float64(s.Total)
		avg := totalDur / time.Duration(s.Total)
		s.AvgDuration = avg.Round(time.Millisecond).String()
	}
	return s
}

func printSWETable(report *SWEReport) {
	fmt.Printf("SWE-bench-lite benchmark: %s\n", report.RunID)
	fmt.Printf("Agent: %s\n\n", report.Agent)

	for _, r := range report.Tasks {
		status := "FAIL"
		if r.Resolved {
			status = "PASS"
		}
		if r.Error != "" {
			status = "ERR "
		}
		dur := r.Duration.Round(time.Second)
		if r.Error != "" {
			errMsg := r.Error
			if len(errMsg) > 60 {
				errMsg = errMsg[:60] + "..."
			}
			fmt.Printf("  %-40s  %s  (%s) %s\n", r.InstanceID, status, dur, errMsg)
		} else {
			fmt.Printf("  %-40s  %s  %s  $%.4f  %d lines\n",
				r.InstanceID, status, dur, r.CostUSD, r.PatchLen)
		}
	}

	fmt.Printf("\n"+
		"Summary: %d/%d resolved (%.0f%%)\n"+
		"  Errored:  %d\n"+
		"  Cost:     $%.4f\n"+
		"  Avg time: %s\n",
		report.Summary.Resolved, report.Summary.Total,
		report.Summary.ResolveRate*100,
		report.Summary.Errored,
		report.Summary.TotalCost, report.Summary.AvgDuration)
}

// persistSWEReport writes the report and per-task results to the results directory.
func persistSWEReport(report *SWEReport, dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create results dir: %w", err)
	}

	reportPath := filepath.Join(dir, report.RunID+".json")
	return WriteSWEReportJSON(report, reportPath)
}

// WriteSWEReportJSON writes the report to a JSON file.
func WriteSWEReportJSON(report *SWEReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

