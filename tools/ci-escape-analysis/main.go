// Command ci-escape-analysis answers one question about a red check on main:
// how did it get past pre-merge, and can filter selection be refined so the
// next one does not?
//
// Two modes:
//
//	# Render one retrospective to stdout. Pure function of its flags, so it can
//	# be run by hand against any historical failure without touching the repo.
//	ci-escape-analysis -repo owner/name -check "Build & Test (ubuntu-latest)" -sha <sha>
//
//	# Sweep main, file or update a DEAR retro per red workflow, close recovered
//	# ones. This is what main-health-watchdog.yml runs.
//	ci-escape-analysis -repo owner/name -sweep
//
// Go rather than shell on purpose: the classification and the ROI arithmetic
// are the parts that must not silently rot, and .github/workflows has a
// standing 20-line ceiling on bash. pkg/cihealth holds the judgement calls and
// is unit-tested; this command is the plumbing around it.
//
// Facts are read via the `gh` CLI, which the runner already authenticates.
// Every fetch failure degrades to "unknown" rather than aborting: a retro with
// a partial picture is worth more than no retro.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/cihealth"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("ci-escape-analysis", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var (
		repo         = flags.String("repo", "", "owner/name of the repository (required)")
		check        = flags.String("check", "", "name of the failing check context on main (required)")
		sha          = flags.String("sha", "", "commit SHA on main where the failure was observed (required)")
		workflow     = flags.String("workflow", "", "human name of the failing workflow")
		runURL       = flags.String("run-url", "", "URL of the failing run")
		diffScoped   = flags.Bool("diff-scoped", false, "the check's pre-merge run is deliberately narrower than its post-merge run")
		windowDays   = flags.Int("window-days", 30, "lookback window for the escape count")
		cureMinutes  = flags.Float64("cure-minutes", 90, "engineer-minutes one escape costs: main red x people blocked, plus triage")
		preventMins  = flags.Float64("prevention-minutes", 0, "engineer-minutes running this check pre-merge would cost over the window; 0 means measure it from run history")
		escapesFlag  = flags.Float64("escapes", -1, "escapes in the window; negative means count them from run history")
		prNumberFlag = flags.Int("pr", -1, "pull request that introduced the SHA; negative means look it up")
		dryRun       = flags.Bool("dry-run", false, "with -sweep, render what would be filed without creating, commenting on, or closing any issue")
		sweepMode    = flags.Bool("sweep", false, "sweep main for red workflows, file or update a DEAR retro for each, and close retros that recovered")
	)

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *repo == "" {
		flags.Usage()
		return 2
	}

	if *sweepMode {
		return sweep(sweepOptions{
			Repo:        *repo,
			WindowDays:  *windowDays,
			CureMinutes: *cureMinutes,
			DryRun:      *dryRun,
		}, stdout, stderr)
	}

	if *check == "" || *sha == "" {
		flags.Usage()
		return 2
	}

	escape := cihealth.Escape{
		FailingCheck: *check,
		MainSHA:      *sha,
		DiffScoped:   *diffScoped,
	}

	escape.PRNumber = *prNumberFlag
	if escape.PRNumber < 0 {
		escape.PRNumber = lookupPR(*repo, *sha, stderr)
	}
	if escape.PRNumber > 0 {
		escape.PRChecks = lookupPRChecks(*repo, escape.PRNumber, stderr)
	}
	escape.RequiredContexts = lookupRequiredContexts(*repo, stderr)

	escapes := *escapesFlag
	if escapes < 0 {
		escapes = countEscapes(*repo, *workflow, *windowDays, stderr)
	}

	prevention := *preventMins
	if prevention <= 0 {
		prevention = estimatePrevention(*repo, *workflow, *windowDays, stderr)
	}

	retro := cihealth.Retro{
		Repo:         *repo,
		FailingCheck: *check,
		WorkflowName: *workflow,
		MainSHA:      *sha,
		RunURL:       *runURL,
		Finding:      cihealth.Classify(escape),
		ROI: cihealth.ROI{
			CureMinutes:       *cureMinutes,
			Escapes:           escapes,
			PreventionMinutes: prevention,
		},
		Required:   escape.RequiredContexts,
		WindowDays: *windowDays,
	}

	fmt.Fprint(stdout, retro.Body())
	return 0
}

// gh runs the GitHub CLI and returns stdout. Errors are reported and swallowed:
// see the package comment on degrading rather than aborting.
func gh(stderr io.Writer, args ...string) ([]byte, bool) {
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: gh %s: %v\n", strings.Join(args, " "), err)
		return nil, false
	}
	return out, true
}

func lookupPR(repo, sha string, stderr io.Writer) int {
	out, ok := gh(stderr, "api", fmt.Sprintf("repos/%s/commits/%s/pulls", repo, sha), "--jq", ".[0].number // 0")
	if !ok {
		return 0
	}
	var number int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &number); err != nil {
		return 0
	}
	return number
}

func lookupPRChecks(repo string, pr int, stderr io.Writer) []cihealth.CheckRun {
	out, ok := gh(stderr, "api", fmt.Sprintf("repos/%s/pulls/%d", repo, pr), "--jq", ".head.sha")
	if !ok {
		return nil
	}
	head := strings.TrimSpace(string(out))
	if head == "" {
		return nil
	}

	out, ok = gh(stderr, "api", "--paginate",
		fmt.Sprintf("repos/%s/commits/%s/check-runs", repo, head),
		"--jq", ".check_runs[] | {name: .name, conclusion: .conclusion}")
	if !ok {
		return nil
	}

	var runs []cihealth.CheckRun
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var raw struct {
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		}
		if err := decoder.Decode(&raw); err != nil {
			break
		}
		runs = append(runs, cihealth.CheckRun{Name: raw.Name, Conclusion: raw.Conclusion})
	}
	return runs
}

func lookupRequiredContexts(repo string, stderr io.Writer) []string {
	out, ok := gh(stderr, "api", fmt.Sprintf("repos/%s/rules/branches/main", repo),
		"--jq", ".[] | select(.type == \"required_status_checks\") | .parameters.required_status_checks[].context")
	if !ok {
		return nil
	}
	var contexts []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			contexts = append(contexts, line)
		}
	}
	return contexts
}

// countEscapes is the Frequency term: how many times this workflow went red on
// main inside the window.
func countEscapes(repo, workflow string, windowDays int, stderr io.Writer) float64 {
	if workflow == "" {
		return 0
	}
	out, ok := gh(stderr, "run", "list", "--repo", repo, "--workflow", workflow,
		"--branch", "main", "--status", "failure", "--limit", "100",
		"--json", "createdAt", "--jq", fmt.Sprintf("[.[] | select(.createdAt > (now - %d*86400 | todate))] | length", windowDays))
	if !ok {
		return 0
	}
	var count float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &count); err != nil {
		return 0
	}
	return count
}

// estimatePrevention is the Prevention Cost term: median wall-clock of this
// workflow on pull requests, multiplied by how many pull requests ran it in the
// window. Wall-clock rather than billable minutes on purpose — the cost that
// matters is the engineer waiting, not the runner.
func estimatePrevention(repo, workflow string, windowDays int, stderr io.Writer) float64 {
	if workflow == "" {
		return 0
	}
	out, ok := gh(stderr, "run", "list", "--repo", repo, "--workflow", workflow,
		"--event", "pull_request", "--limit", "100",
		"--json", "createdAt,updatedAt",
		"--jq", fmt.Sprintf(
			`[.[] | select(.createdAt > (now - %d*86400 | todate))
			   | ((.updatedAt | fromdate) - (.createdAt | fromdate)) / 60]
			 | if length == 0 then 0 else add end`, windowDays))
	if !ok {
		return 0
	}
	var minutes float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &minutes); err != nil {
		return 0
	}
	return minutes
}
