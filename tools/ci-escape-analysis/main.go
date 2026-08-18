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
// a partial picture is worth more than no retro. Where "unknown" and a real
// answer would lead to different advice — required contexts, prevention cost —
// the unknown is carried into the retro as unknown rather than silently
// becoming an empty list or a zero.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/cihealth"
)

// ghTimeout bounds every subprocess call. The GitHub CLI can block on network
// stalls and, if it ever loses its token, on an interactive prompt; an unbounded
// wait would hang the whole workflow behind a single query.
const ghTimeout = 60 * time.Second

// runPageLimit is the cap `gh run list` is asked for. It is a fetch cap, not
// pagination — hitting it means the window was only partially observed, which
// the callers must report rather than quietly treat as the whole picture.
const runPageLimit = 100

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
		preMerge     = flags.Bool("pre-merge-capable", true, "the failing check could have run on a pull request; set false for a schedule-only workflow or a job guarded to non-pull-request events")
		scheduled    = flags.Bool("scheduled-detection", false, "the failure was observed on a scheduled or dispatched run, so the head of main is not evidence of its cause")
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

	// Whether a flag was supplied at all is the only way to tell a measured
	// value from the default. A cure cost nobody stated is an assumption, and
	// the retro has to say so.
	set := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { set[f.Name] = true })

	if *sweepMode {
		return sweep(sweepOptions{
			Repo:         *repo,
			WindowDays:   *windowDays,
			CureMinutes:  *cureMinutes,
			CureMeasured: set["cure-minutes"],
			DryRun:       *dryRun,
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
		// Standalone mode has no workflow file to consult, so capability is an
		// input. Defaulting it to true rather than to Go's zero value matters:
		// a false here short-circuits Classify into post-merge-only before it
		// looks at the pull request at all, which would make every hand-run
		// analysis return the same answer.
		PreMergeCapable:    *preMerge,
		ScheduledDetection: *scheduled,
	}

	escape.PRNumber = *prNumberFlag
	if escape.PRNumber < 0 {
		escape.PRNumber = lookupPR(*repo, *sha, stderr)
	}
	if escape.PRNumber > 0 {
		escape.PRChecks = lookupPRChecks(*repo, escape.PRNumber, stderr)
	}
	escape.RequiredContexts, escape.RequiredKnown = lookupRequiredContexts(*repo, stderr)

	escapes := *escapesFlag
	escapeScope := fmt.Sprintf("escapes supplied on the command line for %d days", *windowDays)
	if escapes < 0 {
		escapes = countEscapes(*repo, *workflow, *windowDays, stderr)
		escapeScope = fmt.Sprintf("distinct failing commits for workflow %q over %d days", *workflow, *windowDays)
	}

	// An explicitly supplied prevention cost is a measurement by definition;
	// otherwise fall back to run history, which reports for itself whether it
	// observed anything.
	prevention, preventionMeasured, truncated := *preventMins, set["prevention-minutes"], false
	if !preventionMeasured {
		prevention, preventionMeasured, truncated = estimatePrevention(*repo, *workflow, *windowDays, stderr)
	}

	retro := cihealth.Retro{
		Repo:         *repo,
		FailingCheck: *check,
		WorkflowName: *workflow,
		MainSHA:      *sha,
		RunURL:       *runURL,
		Finding:      cihealth.Classify(escape),
		ROI: cihealth.ROI{
			CureMinutes:         *cureMinutes,
			CureAssumed:         !set["cure-minutes"],
			Escapes:             escapes,
			EscapesScope:        escapeScope,
			PreventionMinutes:   prevention,
			PreventionMeasured:  preventionMeasured,
			PreventionTruncated: truncated,
		},
		Required:      escape.RequiredContexts,
		RequiredKnown: escape.RequiredKnown,
		WindowDays:    *windowDays,
	}

	fmt.Fprint(stdout, retro.Body())
	return 0
}

// ghOutput runs the GitHub CLI and returns stdout, bounded by a timeout so a
// stalled network call or an interactive prompt cannot hang the workflow.
func ghOutput(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), ctxErr)
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// gh is ghOutput for the fact-gathering calls, where a failure is reported and
// degraded to "unknown": see the package comment.
func gh(stderr io.Writer, args ...string) ([]byte, bool) {
	out, err := ghOutput(args...)
	if err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: %v\n", err)
		return nil, false
	}
	return out, true
}

// runGH is the mutating counterpart: it returns the error so the caller can
// decide, because a retro that failed to file is not a degraded fact, it is a
// missing alert.
func runGH(what string, stderr io.Writer, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ghTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%s: %w", what, ctxErr)
		}
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
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

// lookupRequiredContexts reads the branch ruleset. The second return value says
// whether the read actually succeeded: this endpoint needs Administration
// (read), which the workflow token does not have, and a denied request returns
// nothing — indistinguishable from a repository that requires no checks. Every
// gating verdict turns on that distinction, so it is carried rather than
// flattened.
func lookupRequiredContexts(repo string, stderr io.Writer) ([]string, bool) {
	out, ok := gh(stderr, "api", fmt.Sprintf("repos/%s/rules/branches/main", repo),
		"--jq", ".[] | select(.type == \"required_status_checks\") | .parameters.required_status_checks[].context")
	if !ok {
		return nil, false
	}
	var contexts []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			contexts = append(contexts, line)
		}
	}
	return contexts, true
}

// countEscapes is the Frequency term: how many distinct commits this workflow
// went red on inside the window.
//
// Distinct commits, not failed runs. A scheduled workflow or a manual re-run
// fails repeatedly against the same unchanged SHA, and charging each of those
// as a separate escape lets one unresolved incident inflate the numerator until
// it crosses a threshold and argues for a pre-merge gate that no additional
// change ever escaped.
func countEscapes(repo, workflow string, windowDays int, stderr io.Writer) float64 {
	if workflow == "" {
		return 0
	}
	out, ok := gh(stderr, "run", "list", "--repo", repo, "--workflow", workflow,
		"--branch", "main", "--status", "failure", "--limit", fmt.Sprint(runPageLimit),
		"--json", "createdAt,headSha",
		"--jq", fmt.Sprintf("[.[] | select(.createdAt > (now - %d*86400 | todate)) | .headSha] | unique | length", windowDays))
	if !ok {
		return 0
	}
	var count float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &count); err != nil {
		return 0
	}
	return count
}

// estimatePrevention is the Prevention Cost term: the wall-clock this workflow
// spent on pull requests inside the window. Wall-clock rather than billable
// minutes on purpose — the cost that matters is the engineer waiting, not the
// runner.
//
// Returns whether anything was actually observed, and whether the query hit the
// page limit. Both matter: an unmeasured denominator must not read as "free",
// and a truncated one is a lower bound that biases the ratio toward adding a
// gate.
func estimatePrevention(repo, workflow string, windowDays int, stderr io.Writer) (minutes float64, measured, truncated bool) {
	if workflow == "" {
		return 0, false, false
	}
	out, ok := gh(stderr, "run", "list", "--repo", repo, "--workflow", workflow,
		"--event", "pull_request", "--limit", fmt.Sprint(runPageLimit),
		"--json", "createdAt,updatedAt",
		"--jq", fmt.Sprintf(
			`[.[] | select(.createdAt > (now - %d*86400 | todate))
			   | ((.updatedAt | fromdate) - (.createdAt | fromdate)) / 60]
			 | "\(length) \(add // 0)"`, windowDays))
	if !ok {
		return 0, false, false
	}

	var (
		observed int
		total    float64
	)
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %f", &observed, &total); err != nil {
		return 0, false, false
	}
	if observed == 0 {
		// The workflow declares a pull_request trigger but nothing ran on a
		// pull request in the window — an over-narrow path filter does exactly
		// this. There is no prevention cost to price, and claiming one of zero
		// would make the ratio unbounded.
		return 0, false, false
	}
	return total, true, observed >= runPageLimit
}

func sortStrings(s []string) { sort.Strings(s) }
