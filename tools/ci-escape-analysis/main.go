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
	escape.PRKnown = true
	if escape.PRNumber < 0 {
		escape.PRNumber, escape.PRKnown = lookupPR(*repo, *sha, stderr)
	}
	escape.PRChecksKnown = true
	if escape.PRNumber > 0 {
		escape.PRChecks, escape.PRChecksKnown = lookupPRChecks(*repo, escape.PRNumber, stderr)
	}
	escape.RequiredContexts, escape.RequiredKnown = lookupRequiredContexts(*repo, stderr)

	escapes := *escapesFlag
	escapesTruncated := false
	escapesMeasured := true
	escapeScope := fmt.Sprintf("escapes supplied on the command line for %d days", *windowDays)
	if escapes < 0 {
		escapes, escapesTruncated, escapesMeasured = countEscapes(*repo, *workflow, *windowDays, stderr)
		escapeScope = escapesScope(*workflow, *windowDays)
	}

	// An explicitly supplied POSITIVE prevention cost is a measurement by
	// definition. Zero is the documented sentinel for "measure it from run
	// history", so honour it as such: treating a supplied zero as measured
	// makes the ratio unbounded and returns ALWAYS PREVENT, which is the
	// opposite of what the flag help promises.
	prevention, preventionMeasured, truncated := *preventMins, set["prevention-minutes"] && *preventMins > 0, false
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
			EscapesTruncated:    escapesTruncated,
			EscapesMeasured:     escapesMeasured,
			EscapesScope:        escapeScope,
			PreventionMinutes:   prevention,
			PreventionMeasured:  preventionMeasured,
			PreventionTruncated: truncated,
			PreventionScope:     fmt.Sprintf("wall-clock of the whole %q workflow on pull requests, not of this job alone", *workflow),
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

// lookupPR finds the pull request that actually introduced this commit on main.
//
// GitHub associates a commit with every pull request that contains it, so the
// first result is not necessarily the one that merged it: an unrelated open
// pull request, or one targeting another branch, would otherwise supply the
// head checks and the merge timestamp for the whole classification — and a
// direct push could be dressed up as a merge. Only a pull request that merged
// into main with this exact merge commit qualifies; anything else is no pull
// request at all, which is the honest answer for a direct push.
// The second return value reports whether the association was actually read.
// A denied or timed-out request otherwise collapses to PR zero, which Classify
// reads as a direct push and reports as an administrative bypass — a serious
// accusation manufactured from an API error.
func lookupPR(repo, sha string, stderr io.Writer) (int, bool) {
	out, ok := gh(stderr, "api", fmt.Sprintf("repos/%s/commits/%s/pulls", repo, sha),
		"--jq", fmt.Sprintf(`[.[] | select(.merged_at != null and .base.ref == "main" and .merge_commit_sha == %q)] | .[0].number // 0`, sha))
	if !ok {
		return 0, false
	}
	var number int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &number); err != nil {
		return 0, false
	}
	return number, true
}

// lookupPRChecks returns the checks as they stood when the pull request merged.
//
// "As they stood" is the whole point. Re-running a workflow after a merge
// replaces the check run attached to the head, so reading the present state can
// turn a green merge into a fabricated bypass, or hide a real gating bypass
// behind a success that did not exist at merge time. Attempts that completed
// after the merge are therefore discarded, and the latest attempt that had
// completed before it wins.
func lookupPRChecks(repo string, pr int, stderr io.Writer) ([]cihealth.CheckRun, bool) {
	out, ok := gh(stderr, "api", fmt.Sprintf("repos/%s/pulls/%d", repo, pr),
		"--jq", `"\(.head.sha) \(.merged_at // "")"`)
	if !ok {
		return nil, false
	}
	head, mergedAtRaw, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	if head == "" {
		return nil, false
	}
	// An open pull request has no merge time; everything currently attached is
	// the current truth for it.
	mergedAt, mergedKnown := time.Time{}, false
	if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(mergedAtRaw)); err == nil {
		mergedAt, mergedKnown = parsed, true
	}

	// filter=all, because the endpoint defaults to `latest` — one run per check
	// suite. A check rerun after the merge would replace its own pre-merge
	// attempt in the response, and the merge-time reducer would then report the
	// check as never having run at all.
	out, ok = gh(stderr, "api", "--paginate",
		fmt.Sprintf("repos/%s/commits/%s/check-runs?filter=all&per_page=100", repo, head),
		"--jq", ".check_runs[] | {name: .name, conclusion: .conclusion, appId: .app.id, startedAt: .started_at, completedAt: .completed_at}")
	if !ok {
		return nil, false
	}

	return checksAtMerge(out, mergedAt, mergedKnown), true
}

// checksAtMerge reduces the check-run stream to one attempt per (name, app),
// as each stood when the pull request merged.
func checksAtMerge(out []byte, mergedAt time.Time, mergedKnown bool) []cihealth.CheckRun {
	type attempt struct {
		run       cihealth.CheckRun
		completed time.Time
	}
	// Keyed by name AND producing app. Collapsing on the name alone lets a
	// foreign app's later same-named check displace the GitHub Actions attempt
	// before isRequired ever gets to compare identities.
	type key struct {
		name  string
		appID int64
	}
	latest := map[key]attempt{}

	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var raw struct {
			Name        string `json:"name"`
			Conclusion  string `json:"conclusion"`
			AppID       int64  `json:"appId"`
			StartedAt   string `json:"startedAt"`
			CompletedAt string `json:"completedAt"`
		}
		if err := decoder.Decode(&raw); err != nil {
			break
		}
		started, startErr := time.Parse(time.RFC3339, raw.StartedAt)
		completed, completeErr := time.Parse(time.RFC3339, raw.CompletedAt)

		switch {
		case completeErr == nil && (!mergedKnown || !completed.After(mergedAt)):
			// Concluded at or before the merge: this is what the gate saw.
		case startErr == nil && mergedKnown && !started.After(mergedAt):
			// Started before the merge but finished after it, or never
			// finished. The check existed and was still running when the gate
			// let the merge through — which is a different fact from the check
			// never having reported at all, and discarding it produced
			// `never-ran` with path-filter advice.
			raw.Conclusion = cihealth.ConclusionPending
			completed = mergedAt
		default:
			// A re-run started after the merge. Not what the gate saw.
			continue
		}
		k := key{name: raw.Name, appID: raw.AppID}
		if seen, ok := latest[k]; ok && seen.completed.After(completed) {
			continue
		}
		latest[k] = attempt{
			run:       cihealth.CheckRun{Name: raw.Name, Conclusion: raw.Conclusion, AppID: raw.AppID},
			completed: completed,
		}
	}

	runs := make([]cihealth.CheckRun, 0, len(latest))
	for _, a := range latest {
		runs = append(runs, a.run)
	}
	return runs
}

// lookupRequiredContexts reads the branch ruleset. The second return value says
// whether the read actually succeeded: this endpoint needs Administration
// (read), which the workflow token does not have, and a denied request returns
// nothing — indistinguishable from a repository that requires no checks. Every
// gating verdict turns on that distinction, so it is carried rather than
// flattened.
// Keeps the ruleset's `integration_id` alongside the context name: a ruleset
// pins a context to a producing App, and dropping that lets any App's
// same-named check be mistaken for the required one.
func lookupRequiredContexts(repo string, stderr io.Writer) ([]cihealth.RequiredContext, bool) {
	out, ok := gh(stderr, "api", fmt.Sprintf("repos/%s/rules/branches/main", repo),
		"--jq", `.[] | select(.type == "required_status_checks")
		           | .parameters.required_status_checks[]
		           | {name: .context, integrationId: (.integration_id // 0)}`)
	if !ok {
		return nil, false
	}
	var contexts []cihealth.RequiredContext
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for {
		var raw struct {
			Name          string `json:"name"`
			IntegrationID int64  `json:"integrationId"`
		}
		if err := decoder.Decode(&raw); err != nil {
			break
		}
		if raw.Name != "" {
			contexts = append(contexts, cihealth.RequiredContext{Name: raw.Name, IntegrationID: raw.IntegrationID})
		}
	}
	return contexts, true
}

// escapesScope names what the escape count actually counted, so the brief does
// not read as "every red run of this workflow". Both the exclusion of
// post-merge detections and the workflow-rather-than-check scope change the
// number, and a reader pricing a gate on it has to be able to see that.
func escapesScope(workflow string, windowDays int) string {
	return fmt.Sprintf("distinct failing commits for workflow %q over %d days, excluding scheduled and dispatched runs", workflow, windowDays)
}

// escapeRun is one run of the failing workflow on main, as `gh run list`
// reports it. `event` is fetched because a run's event decides whether it is
// escape evidence at all — see postMergeDetectionEvents.
type escapeRun struct {
	CreatedAt  time.Time `json:"createdAt"`
	HeadSHA    string    `json:"headSha"`
	Conclusion string    `json:"conclusion"`
	Event      string    `json:"event"`
}

// countEscapes is the Frequency term: how many distinct commits this workflow
// went red on inside the window.
//
// Distinct commits, not failed runs. A scheduled workflow or a manual re-run
// fails repeatedly against the same unchanged SHA, and charging each of those
// as a separate escape lets one unresolved incident inflate the numerator until
// it crosses a threshold and argues for a pre-merge gate that no additional
// change ever escaped.
//
// The selection is done in Go rather than in a jq expression embedded in a Go
// string. That is the whole reason this analysis is not a shell script: the
// numerator decides where a gate gets placed, so its rules have to be readable
// and unit-tested rather than trusted.
func countEscapes(repo, workflow string, windowDays int, stderr io.Writer) (count float64, truncated, measured bool) {
	if workflow == "" {
		return 0, false, false
	}
	out, ok := gh(stderr, "run", "list", "--repo", repo, "--workflow", workflow,
		"--branch", "main", "--limit", fmt.Sprint(runPageLimit),
		"--json", "createdAt,headSha,conclusion,event")
	if !ok {
		// A failed query is not evidence that nothing failed. Returning zero
		// here renders as "no escapes recorded — leave the placement alone".
		return 0, false, false
	}
	var runs []escapeRun
	if err := json.Unmarshal(out, &runs); err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: decoding run list for %q: %v\n", workflow, err)
		return 0, false, false
	}
	// `--limit` is a fetch cap, not pagination: hitting it means older failures
	// inside the window were never seen, so the numerator is a lower bound.
	return escapeCommits(runs, time.Now(), windowDays), len(runs) >= runPageLimit, true
}

// escapeCommits counts the distinct commits that this workflow went red on
// inside the window, from runs that are escape evidence.
//
// Every red conclusion counts, not just `failure`: detection treats a timed-out
// or startup-failed workflow as red, and counting only `failure` here would
// hand such an incident a frequency of zero and a NO SIGNAL verdict, so the
// numerator would disagree with the thing that raised the alarm.
//
// Scheduled, dispatched, and repository-dispatched runs are excluded. Classify
// calls those `post-merge-only` — not escapes — because they compare the
// repository against a world that moves on its own. A workflow with both push
// and schedule triggers (CI, CodeQL, Language Policy) would otherwise have its
// drift detections counted as escapes, inflating the frequency used to price a
// later push failure and moving its ROI across a placement threshold.
func escapeCommits(runs []escapeRun, now time.Time, windowDays int) float64 {
	cutoff := now.Add(-time.Duration(windowDays) * 24 * time.Hour)
	commits := map[string]bool{}
	for _, r := range runs {
		if !r.CreatedAt.After(cutoff) {
			continue
		}
		if !redConclusions[r.Conclusion] {
			continue
		}
		if postMergeDetectionEvents[r.Event] {
			continue
		}
		if r.HeadSHA == "" {
			// No commit to attribute the failure to, so it cannot be counted
			// as a distinct escaping change without inventing one.
			continue
		}
		commits[r.HeadSHA] = true
	}
	return float64(len(commits))
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
