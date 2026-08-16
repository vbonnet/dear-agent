package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/cihealth"
)

// retroLabel marks every issue this command owns. Nothing else should use it:
// the sweep closes issues carrying it once the corresponding workflow recovers.
const retroLabel = "ci-red"

// mainRun is the latest run of one workflow on main.
type mainRun struct {
	WorkflowName string `json:"workflowName"`
	Conclusion   string `json:"conclusion"`
	HeadSHA      string `json:"headSha"`
	URL          string `json:"url"`
	CreatedAt    string `json:"createdAt"`
}

// sweep finds every workflow currently red on main, files or updates a DEAR
// retro for each, and closes the retros of workflows that have recovered.
//
// Idempotent by design: it runs on a schedule and on every workflow_run
// completion, so it must be safe to run against an unchanged world. Repeat
// failures comment on the existing issue rather than opening a new one — the
// retro policy wants recurrence to read as a frequency signal, not as a pile
// of duplicates.
func sweep(opts sweepOptions, stdout, stderr io.Writer) int {
	runs, err := latestRunPerWorkflowOnMain(opts.Repo)
	if err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: %v\n", err)
		return 1
	}

	red := map[string]mainRun{}
	for name, run := range runs {
		if run.Conclusion == "failure" {
			red[name] = run
		}
	}

	fmt.Fprintf(stdout, "Workflows on main: %d checked, %d red\n", len(runs), len(red))

	if len(red) > 0 && !opts.DryRun {
		ensureLabel(opts.Repo, stderr)
	}

	required := lookupRequiredContexts(opts.Repo, stderr)

	for name, run := range red {
		fmt.Fprintf(stdout, "\n=== RED: %s @ %s\n", name, shortSHA(run.HeadSHA))
		retro := buildRetro(opts, run, required, stderr)
		if opts.DryRun {
			fmt.Fprintf(stdout, "--- would file: %s\n%s\n", retro.Title(), retro.Body())
			continue
		}
		if err := upsertRetro(opts.Repo, retro, run, stderr); err != nil {
			fmt.Fprintf(stderr, "ci-escape-analysis: %s: %v\n", name, err)
		}
	}

	// Close retros whose workflow is green again. Scoped to workflows we
	// actually observed this sweep — a workflow that did not report at all is
	// not evidence of recovery.
	for name, run := range runs {
		if run.Conclusion != "success" || opts.DryRun {
			continue
		}
		closeRetro(opts.Repo, name, stdout, stderr)
	}

	return 0
}

type sweepOptions struct {
	Repo        string
	WindowDays  int
	CureMinutes float64
	// DryRun renders what would be filed without touching the repository, so
	// the analysis can be exercised by hand without spraying issues.
	DryRun bool
}

// ensureLabel creates the retro label if the repository does not have it.
// Without this the very first run fails on "label not found" and files
// nothing — which is exactly how ci-health-monitor.yml has been failing
// silently, since it also hardcodes --label ci-red.
func ensureLabel(repo string, stderr io.Writer) {
	if err := runGH("creating retro label", io.Discard, "label", "create", retroLabel,
		"--repo", repo,
		"--color", "B60205",
		"--description", "main is red; DEAR retro filed by main-health-watchdog",
		"--force"); err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: could not ensure label %q: %v\n", retroLabel, err)
	}
}

// latestRunPerWorkflowOnMain collapses the run history to the current state of
// each workflow: only the most recent run per workflow tells you whether main
// is red right now.
func latestRunPerWorkflowOnMain(repo string) (map[string]mainRun, error) {
	out, err := exec.Command("gh", "run", "list",
		"--repo", repo, "--branch", "main", "--limit", "150",
		"--json", "workflowName,conclusion,headSha,url,createdAt").Output()
	if err != nil {
		return nil, fmt.Errorf("listing runs on main: %w", err)
	}

	var all []mainRun
	if err := json.Unmarshal(out, &all); err != nil {
		return nil, fmt.Errorf("decoding run list: %w", err)
	}

	// gh returns newest first, so the first sighting of a workflow is its
	// latest run.
	latest := map[string]mainRun{}
	for _, run := range all {
		if run.Conclusion == "" {
			continue // still in flight; not evidence either way
		}
		if _, seen := latest[run.WorkflowName]; !seen {
			latest[run.WorkflowName] = run
		}
	}
	return latest, nil
}

func buildRetro(opts sweepOptions, run mainRun, required []string, stderr io.Writer) cihealth.Retro {
	failing := firstFailingCheck(opts.Repo, run.HeadSHA, stderr)
	if failing == "" {
		failing = run.WorkflowName
	}

	preMerge := opts.PreMergeCapable(run.WorkflowName)

	escape := cihealth.Escape{
		FailingCheck:     failing,
		MainSHA:          run.HeadSHA,
		RequiredContexts: required,
		DiffScoped:       isDiffScoped(failing),
		PreMergeCapable:  preMerge,
	}
	escape.PRNumber = lookupPR(opts.Repo, run.HeadSHA, stderr)
	if escape.PRNumber > 0 {
		escape.PRChecks = lookupPRChecks(opts.Repo, escape.PRNumber, stderr)
	}

	return cihealth.Retro{
		Repo:         opts.Repo,
		FailingCheck: failing,
		WorkflowName: run.WorkflowName,
		MainSHA:      run.HeadSHA,
		RunURL:       run.URL,
		Finding:      cihealth.Classify(escape),
		ROI: cihealth.ROI{
			CureMinutes: opts.CureMinutes,
			Escapes:     countEscapes(opts.Repo, run.WorkflowName, opts.WindowDays, stderr),
			// Only priced for workflows that can actually run pre-merge.
			// Anything else has no pre-merge runs to measure, and a zero
			// denominator would read as "prevention is free".
			PreventionMinutes:  estimatePrevention(opts.Repo, run.WorkflowName, opts.WindowDays, stderr),
			PreventionMeasured: preMerge,
		},
		Required:   required,
		WindowDays: opts.WindowDays,
	}
}

// isDiffScoped names the checks whose pre-merge run is deliberately narrower
// than their post-merge run (ADR-038). Passing pre-merge and failing on main is
// the intended behaviour for these, not an escape, and the retro must say so —
// otherwise every scheduled full-scope finding reads as a filtering bug and
// someone "fixes" it by widening the pre-merge gate back to where it started.
func isDiffScoped(check string) bool {
	switch check {
	case "Vulnerability Scan", "Forbidden temporal artifact paths":
		return true
	default:
		return false
	}
}

func firstFailingCheck(repo, sha string, stderr io.Writer) string {
	out, ok := gh(stderr, "api", "--paginate",
		fmt.Sprintf("repos/%s/commits/%s/check-runs", repo, sha),
		"--jq", `.check_runs[] | select(.conclusion == "failure") | .name`)
	if !ok {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// upsertRetro files a new retro or comments on the existing one. The title is
// stable per failing check, which is what makes recurrence countable.
func upsertRetro(repo string, retro cihealth.Retro, run mainRun, stderr io.Writer) error {
	existing := findOpenRetro(repo, retro.Title(), stderr)
	if existing > 0 {
		body := fmt.Sprintf("Still red at [`%s`](%s).\n\n%s", shortSHA(run.HeadSHA), run.URL, retro.Body())
		return runGH("commenting on retro", stderr, "issue", "comment", fmt.Sprint(existing),
			"--repo", repo, "--body", body)
	}
	return runGH("creating retro", stderr, "issue", "create", "--repo", repo,
		"--title", retro.Title(), "--body", retro.Body(), "--label", retroLabel)
}

func findOpenRetro(repo, title string, stderr io.Writer) int {
	out, ok := gh(stderr, "issue", "list", "--repo", repo,
		"--label", retroLabel, "--state", "open", "--limit", "100",
		"--json", "number,title",
		"--jq", fmt.Sprintf(`.[] | select(.title == %q) | .number`, title))
	if !ok {
		return 0
	}
	var number int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &number); err != nil {
		return 0
	}
	return number
}

// closeRetro closes any open retro naming a check produced by a workflow that
// is green again. Matching is on the workflow name appearing in the issue body,
// because the title carries the check name rather than the workflow name.
func closeRetro(repo, workflow string, stdout, stderr io.Writer) {
	out, ok := gh(stderr, "issue", "list", "--repo", repo,
		"--label", retroLabel, "--state", "open", "--limit", "100",
		"--json", "number,body",
		"--jq", fmt.Sprintf(`.[] | select(.body | contains("Workflow: **%s**")) | .number`, workflow))
	if !ok {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fmt.Fprintf(stdout, "Closing recovered retro #%s (%s is green)\n", line, workflow)
		_ = runGH("closing retro", stderr, "issue", "close", line, "--repo", repo,
			"--comment", fmt.Sprintf("`%s` is green on `main` again; auto-closing. Reopen if the underlying prevention was never landed.", workflow))
	}
}

func runGH(what string, stderr io.Writer, args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
