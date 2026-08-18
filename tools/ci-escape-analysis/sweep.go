package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/cihealth"
)

// retroLabel marks every issue this command owns. Deliberately NOT `ci-red`:
// .github/workflows/ci-health-monitor.yml already claims that label, comments
// on the first issue carrying it, and closes every one of them once its own
// streak check goes green. Sharing the label would let that monitor close a
// watchdog retro for a workflow that is still red, after which the fix agent
// reads no brief at all.
const retroLabel = "main-health-watchdog"

// redConclusions are the terminal conclusions that mean main is broken.
// `failure` alone is too narrow: a required workflow that times out or fails to
// start leaves main just as broken, and treating those as not-red makes the
// watchdog silently skip the incident it exists to catch.
//
// `cancelled` is deliberately absent, and it is the one that needs justifying.
// GitHub reports concurrency cancellation, manual cancellation, and a job
// hitting its timeout with the same conclusion. Most workflows here set
// `cancel-in-progress: true`, so on a busy main the newest run is routinely a
// superseded one — counting that as red would file a retro for every rapid
// second push. Cancelled runs are therefore treated as no evidence at all and
// the search falls through to the last run that did conclude.
var redConclusions = map[string]bool{
	"failure":         true,
	"timed_out":       true,
	"startup_failure": true,
	"action_required": true,
	"stale":           true,
}

// inconclusiveRunConclusions carry no information about health, so the run
// search skips past them exactly as it skips a run still in flight.
var inconclusiveRunConclusions = map[string]bool{
	"":          true, // still running
	"cancelled": true,
	"skipped":   true,
	"neutral":   true,
}

// failedJobConclusions is the wider set used to name the job inside an
// already-red run. Here `cancelled` belongs: once the run as a whole is red,
// a job cancelled by its own `timeout-minutes` is a real diagnosis and is
// often the only job that says anything.
func failedJobConclusions() map[string]bool {
	out := map[string]bool{"cancelled": true}
	for conclusion := range redConclusions {
		out[conclusion] = true
	}
	return out
}

// mainRun is the latest run of one workflow on main.
type mainRun struct {
	DatabaseID   int64  `json:"databaseId"`
	WorkflowName string `json:"workflowName"`
	Conclusion   string `json:"conclusion"`
	HeadSHA      string `json:"headSha"`
	URL          string `json:"url"`
	CreatedAt    string `json:"createdAt"`
	Event        string `json:"event"`
}

// scheduledDetection reports whether the run was started by a clock or a human
// rather than by the commit. Those runs compare the repo against a world that
// moves on its own, so the head of main is not evidence of what caused them.
func (r mainRun) scheduledDetection() bool {
	switch r.Event {
	case "schedule", "workflow_dispatch", "repository_dispatch":
		return true
	default:
		return false
	}
}

// sweep finds every workflow currently red on main, files or updates a DEAR
// retro for each, and closes the retros of workflows that have recovered.
//
// Idempotent by design: it runs on a schedule and on every workflow_run
// completion, so it must be safe to run against an unchanged world. Repeat
// failures comment on the existing issue rather than opening a new one — the
// retro policy wants recurrence to read as a frequency signal, not as a pile
// of duplicates.
//
// Returns non-zero if any retro could not be filed. A sweep that could not
// deliver its alert must not report success: the workflow would go on to
// dispatch a fix agent whose entire brief is an issue that does not exist.
func sweep(opts sweepOptions, stdout, stderr io.Writer) int {
	runs, err := latestRunPerWorkflowOnMain(opts.Repo, opts.WindowDays, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: %v\n", err)
		return 1
	}

	red := map[string]mainRun{}
	for name, run := range runs {
		if redConclusions[run.Conclusion] {
			red[name] = run
		}
	}

	fmt.Fprintf(stdout, "Workflows on main: %d checked, %d red\n", len(runs), len(red))

	if len(red) > 0 && !opts.DryRun {
		ensureLabel(opts.Repo, stderr)
	}

	required, requiredKnown := lookupRequiredContexts(opts.Repo, stderr)
	if !requiredKnown {
		fmt.Fprintf(stderr, "ci-escape-analysis: could not read the branch ruleset (needs Administration: read); required-context evidence will be reported as unknown\n")
	}

	var errs []error
	// The set of retro titles this sweep believes should be open. Anything else
	// carrying our label is stale and gets closed below.
	live := map[string]bool{}

	for name, run := range red {
		fmt.Fprintf(stdout, "\n=== RED: %s @ %s (%s, %s)\n", name, shortSHA(run.HeadSHA), run.Conclusion, run.Event)
		retro := buildRetro(opts, run, required, requiredKnown, stderr)
		live[retro.Title()] = true
		if opts.DryRun {
			fmt.Fprintf(stdout, "--- would file: %s\n%s\n", retro.Title(), retro.Body())
			continue
		}
		created, err := upsertRetro(opts.Repo, retro, run, stderr)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		if created {
			fmt.Fprintf(stdout, "NEW INCIDENT: %s\n", retro.Title())
		}
	}

	if !opts.DryRun {
		// Close every retro we own that this sweep did not re-file. That
		// covers both a workflow going green and a workflow whose failing
		// check changed — the old check's retro is just as stale as a
		// recovered one, and leaving it open puts a solved failure in the fix
		// agent's brief.
		if err := closeStaleRetros(opts.Repo, live, stdout, stderr); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		fmt.Fprintf(stderr, "ci-escape-analysis: %d retro mutation(s) failed: %v\n", len(errs), errors.Join(errs...))
		return 1
	}
	return 0
}

type sweepOptions struct {
	Repo        string
	WindowDays  int
	CureMinutes float64
	// CureMeasured is false when CureMinutes came from the flag default rather
	// than from an observation of this incident.
	CureMeasured bool
	// DryRun renders what would be filed without touching the repository, so
	// the analysis can be exercised by hand without spraying issues.
	DryRun bool
}

// ensureLabel creates the retro label if the repository does not have it.
// Without this the very first run fails on "label not found" and files
// nothing — which is exactly how ci-health-monitor.yml has been failing
// silently, since it also hardcodes a label it never creates.
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
//
// Enumerates the workflows first and queries each one's own history, rather
// than taking a single bounded slice of repository-wide runs. `gh run list
// --limit` is a fetch cap, not pagination: an infrequent workflow whose last
// run sits behind a few hundred newer runs from busier workflows would simply
// not appear, and a workflow that is not observed is silently treated as
// healthy.
func latestRunPerWorkflowOnMain(repo string, windowDays int, stderr io.Writer) (map[string]mainRun, error) {
	workflows, err := activeWorkflows(repo)
	if err != nil {
		return nil, err
	}

	// A run older than the lookback window is not evidence about main today.
	// Routing Enforcement is the case that forced this: it dropped its push
	// trigger months ago, so its newest run on main is a failure from a tree
	// that no longer exists — and without a cutoff the watchdog would re-file
	// a retro for it on every sweep, forever, for a workflow that cannot go
	// green because it no longer runs.
	cutoff := time.Now().AddDate(0, 0, -windowDays)

	latest := map[string]mainRun{}
	for _, workflow := range workflows {
		run, ok := latestRunOnMain(repo, workflow, stderr)
		if !ok {
			continue
		}
		created, err := time.Parse(time.RFC3339, run.CreatedAt)
		if err != nil || created.Before(cutoff) {
			continue
		}
		latest[run.WorkflowName] = run
	}
	return latest, nil
}

// activeWorkflows lists the workflow file names GitHub currently considers
// active. File names rather than display names: `gh run list --workflow`
// accepts either, and the file name is stable across renames.
func activeWorkflows(repo string) ([]string, error) {
	out, err := ghOutput("api", "--paginate",
		fmt.Sprintf("repos/%s/actions/workflows", repo),
		"--jq", `.workflows[] | select(.state == "active") | .path`)
	if err != nil {
		return nil, fmt.Errorf("listing workflows: %w", err)
	}
	var paths []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		// GitHub reports managed integrations (Dependabot, Copilot review) as
		// active workflows under a synthetic `dynamic/` path. They have no
		// workflow file, `gh run list --workflow` cannot resolve them, and
		// every sweep would log one lookup failure per integration forever.
		if line == "" || !strings.HasPrefix(line, ".github/workflows/") {
			continue
		}
		paths = append(paths, line)
	}
	if len(paths) == 0 {
		return nil, errors.New("listing workflows: no active workflows returned")
	}
	return paths, nil
}

// latestRunOnMain returns the most recent concluded run of one workflow on main.
func latestRunOnMain(repo, workflow string, stderr io.Writer) (mainRun, bool) {
	out, ok := gh(stderr, "run", "list",
		"--repo", repo, "--branch", "main", "--workflow", workflow, "--limit", "20",
		"--json", "databaseId,workflowName,conclusion,headSha,url,createdAt,event")
	if !ok {
		return mainRun{}, false
	}

	var all []mainRun
	if err := json.Unmarshal(out, &all); err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: decoding runs for %s: %v\n", workflow, err)
		return mainRun{}, false
	}

	// gh returns newest first, so the first concluded sighting is the latest.
	for _, run := range all {
		if inconclusiveRunConclusions[run.Conclusion] || run.WorkflowName == "" {
			continue
		}
		return run, true
	}
	return mainRun{}, false
}

func buildRetro(opts sweepOptions, run mainRun, required []string, requiredKnown bool, stderr io.Writer) cihealth.Retro {
	failing := failingCheckForRun(opts.Repo, run, stderr)
	if failing == "" {
		failing = run.WorkflowName
	}

	preMerge := opts.PreMergeCapable(run.WorkflowName, failing)

	escape := cihealth.Escape{
		FailingCheck:       failing,
		MainSHA:            run.HeadSHA,
		RequiredContexts:   required,
		RequiredKnown:      requiredKnown,
		DiffScoped:         isDiffScoped(failing),
		PreMergeCapable:    preMerge,
		ScheduledDetection: run.scheduledDetection(),
	}
	// A scheduled detection is not attributable to the head commit, so do not
	// go looking for "the pull request that caused it" — there isn't one.
	if !escape.ScheduledDetection {
		escape.PRNumber = lookupPR(opts.Repo, run.HeadSHA, stderr)
		if escape.PRNumber > 0 {
			escape.PRChecks = lookupPRChecks(opts.Repo, escape.PRNumber, stderr)
		}
	}

	prevention, preventionMeasured, truncated := estimatePrevention(opts.Repo, run.WorkflowName, opts.WindowDays, stderr)

	return cihealth.Retro{
		Repo:         opts.Repo,
		FailingCheck: failing,
		WorkflowName: run.WorkflowName,
		MainSHA:      run.HeadSHA,
		RunURL:       run.URL,
		Finding:      cihealth.Classify(escape),
		ROI: cihealth.ROI{
			CureMinutes: opts.CureMinutes,
			CureAssumed: !opts.CureMeasured,
			Escapes:     countEscapes(opts.Repo, run.WorkflowName, opts.WindowDays, stderr),
			// Counted for the workflow, not the individual check: a
			// multi-job workflow reports one run whichever job failed, so
			// attributing every one of those runs to the check that happens
			// to be failing now would inflate the numerator.
			EscapesScope: fmt.Sprintf("distinct failing commits for workflow %q over %d days", run.WorkflowName, opts.WindowDays),
			// Measured from observed pre-merge runs, never inferred from the
			// trigger alone: a workflow that declares pull_request but has no
			// pull-request runs in the window would otherwise divide by zero
			// and read as "prevention is free".
			PreventionMinutes:   prevention,
			PreventionMeasured:  preventionMeasured,
			PreventionTruncated: truncated,
		},
		Required:      required,
		RequiredKnown: requiredKnown,
		WindowDays:    opts.WindowDays,
	}
}

// isDiffScoped names the checks whose pre-merge run is deliberately narrower
// than their post-merge run. Passing pre-merge and failing on main is the
// intended behaviour for these, not an escape, and the retro must say so —
// otherwise every scheduled full-scope finding reads as a filtering bug and
// someone "fixes" it by widening the pre-merge gate back to where it started.
//
// Membership is a claim about a specific workflow's configuration, so it has to
// be re-checked whenever that workflow changes. `Vulnerability Scan` is
// deliberately NOT here: sbom-scan.yml runs the same whole-filesystem Trivy
// scan (`scan-ref: '.'`) pre- and post-merge, so a pre-merge pass followed by a
// post-merge failure is a real change in the world, not a scope difference.
func isDiffScoped(check string) bool {
	switch check {
	case "Forbidden temporal artifact paths":
		return true
	default:
		return false
	}
}

// failingCheckForRun names the job that actually failed inside this run.
//
// Scoped to the run rather than to the commit: two workflows failing on the
// same main SHA would otherwise both be handed whichever check-run the commit
// query returned first, collapsing their separate retros into one issue and
// pinning the wrong escape classification on both.
func failingCheckForRun(repo string, run mainRun, stderr io.Writer) string {
	if run.DatabaseID == 0 {
		return ""
	}
	out, ok := gh(stderr, "run", "view", fmt.Sprint(run.DatabaseID), "--repo", repo,
		"--json", "jobs",
		"--jq", fmt.Sprintf(`.jobs[] | select(%s) | .name`, jqInSet(".conclusion", failedJobConclusions())))
	if !ok {
		return ""
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// jqInSet builds a jq membership predicate from the same map the Go side
// uses, so the two cannot drift.
//
// Binds the field before the array literal. Written the other way round —
// `[...] | index(.conclusion)` — the pipe rebinds `.` to the array, and jq
// fails with "expected an object but got: array".
func jqInSet(field string, set map[string]bool) string {
	quoted := make([]string, 0, len(set))
	for conclusion := range set {
		quoted = append(quoted, fmt.Sprintf("%q", conclusion))
	}
	sortStrings(quoted)
	return fmt.Sprintf("%s as $c | [%s] | index($c)", field, strings.Join(quoted, ","))
}

// upsertRetro files a new retro or comments on the existing one. The title is
// stable per failing check, which is what makes recurrence countable. Reports
// whether it opened a new issue, so the caller can tell a new incident from a
// recurrence of one already being worked.
func upsertRetro(repo string, retro cihealth.Retro, run mainRun, stderr io.Writer) (bool, error) {
	existing := findOpenRetro(repo, retro.Title(), stderr)
	if existing > 0 {
		body := fmt.Sprintf("Still red at [`%s`](%s).\n\n%s", shortSHA(run.HeadSHA), run.URL, retro.Body())
		return false, runGH("commenting on retro", stderr, "issue", "comment", fmt.Sprint(existing),
			"--repo", repo, "--body", body)
	}
	return true, runGH("creating retro", stderr, "issue", "create", "--repo", repo,
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

// closeStaleRetros closes every issue carrying this command's label whose title
// the current sweep did not re-file.
//
// Reconciling against the live set rather than against "did this workflow go
// green" is what handles the awkward case: a workflow that first fails check A
// and then fails check B is still red, so a recovery test never fires, and the
// obsolete A retro would sit in the fix agent's brief pointing it at a failure
// that already passes.
func closeStaleRetros(repo string, live map[string]bool, stdout, stderr io.Writer) error {
	out, ok := gh(stderr, "issue", "list", "--repo", repo,
		"--label", retroLabel, "--state", "open", "--limit", "100",
		"--json", "number,title", "--jq", `.[] | "\(.number)\t\(.title)"`)
	if !ok {
		return errors.New("listing open retros to reconcile")
	}

	var errs []error
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		number, title, found := strings.Cut(strings.TrimSpace(line), "\t")
		if !found || live[title] {
			continue
		}
		fmt.Fprintf(stdout, "Closing stale retro #%s (%s is no longer the failing check)\n", number, title)
		if err := runGH("closing retro", stderr, "issue", "close", number, "--repo", repo,
			"--comment", "This failure is no longer what `main` is red on; auto-closing. Reopen if the underlying prevention was never landed."); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
