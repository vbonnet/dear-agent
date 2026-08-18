package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/cihealth"
)

// retroLabel marks every issue this command owns. Deliberately NOT `ci-red`:
// .github/workflows/ci-health-monitor.yml already claims that label, comments
// on the first issue carrying it, and closes every one of them once its own
// streak check goes green. Sharing the label would let that monitor close a
// watchdog retro for a workflow that is still red, after which the fix agent
// reads no brief at all.
const retroLabel = "main-health-watchdog"

// queuedLabel marks an incident that has been filed but not yet handed to the
// fix agent. It is what makes "one incident per dispatch" a queue rather than a
// drop: when a sweep opens several incidents at once, all of them are queued,
// one is dequeued and dispatched, and the rest wait for later sweeps. Gating on
// "newly created" alone silently abandoned every incident after the first,
// because a later sweep only comments on an issue that already exists and so
// never reports it as new again.
const queuedLabel = "main-health-queued"

// redConclusions are the terminal conclusions that mean main is broken.
// `failure` alone is too narrow: a required workflow that times out or fails to
// start leaves main just as broken, and treating those as not-red makes the
// watchdog silently skip the incident it exists to catch.
//
// `cancelled` is included, but only when nothing superseded the run — see
// latestRunOnMain. GitHub reports concurrency cancellation, manual
// cancellation, and a job hitting its `timeout-minutes` with the same
// conclusion. Most workflows here set `cancel-in-progress: true`, so a
// cancelled run with a newer run behind it is routine supersession; a
// cancelled run that is the newest run of all was stopped on its own account
// and left that commit without a successful build.
var redConclusions = map[string]bool{
	"failure":         true,
	"timed_out":       true,
	"startup_failure": true,
	"action_required": true,
	"stale":           true,
	"cancelled":       true,
}

// inconclusiveRunConclusions carry no information about health, so the run
// search skips past them exactly as it skips a run still in flight.
var inconclusiveRunConclusions = map[string]bool{
	"":        true, // still running
	"skipped": true,
	"neutral": true,
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
	runs, complete, err := latestRunPerWorkflowOnMain(opts, stderr)
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
		if err := upsertRetro(opts.Repo, retro, run, stderr); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	if !opts.DryRun {
		errs = append(errs, reconcile(opts.Repo, live, complete, stdout, stderr)...)
	}

	if len(errs) > 0 {
		fmt.Fprintf(stderr, "ci-escape-analysis: %d retro mutation(s) failed: %v\n", len(errs), errors.Join(errs...))
		return 1
	}

	// Dequeue LAST, and only on a clean sweep. The workflow runs this under
	// `set -euo pipefail`, so a nonzero exit skips the step that writes the
	// incident to $GITHUB_OUTPUT — dequeuing before the error check would drop
	// the queue label, strand the incident with nothing dispatched, and no
	// later sweep would ever restore it.
	if !opts.DryRun {
		if err := dequeueIncident(opts.Repo, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "ci-escape-analysis: %v\n", err)
			return 1
		}
	}
	return 0
}

// reconcile closes the incidents this sweep did not re-file, provided the sweep
// actually saw the whole picture.
func reconcile(repo string, live map[string]bool, complete bool, stdout, stderr io.Writer) []error {
	if !complete {
		// At least one workflow could not be observed, so its absence from the
		// live set is ignorance, not recovery. Closing on that would let one
		// transient API failure delete a live alert.
		fmt.Fprintf(stdout, "Skipping stale-retro reconciliation: not every workflow was observed this sweep.\n")
		return nil
	}
	// Covers both a workflow going green and a workflow whose failing check
	// changed — the old check's incident is just as stale as a recovered one,
	// and leaving it open puts a solved failure in the fix agent's brief.
	if err := closeStaleRetros(repo, live, stdout, stderr); err != nil {
		return []error{err}
	}
	return nil
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
	for _, label := range []struct{ name, colour, description string }{
		{retroLabel, "B60205", "main is red; incident brief filed by main-health-watchdog"},
		{queuedLabel, "FBCA04", "incident brief awaiting a fix agent"},
	} {
		if err := runGH("creating label", io.Discard, "label", "create", label.name,
			"--repo", repo,
			"--color", label.colour,
			"--description", label.description,
			"--force"); err != nil {
			fmt.Fprintf(stderr, "ci-escape-analysis: could not ensure label %q: %v\n", label.name, err)
		}
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
// The second return value reports whether every active workflow was actually
// observed. A workflow whose run lookup failed is unknown, not healthy, and the
// caller must not read its absence as recovery.
func latestRunPerWorkflowOnMain(opts sweepOptions, stderr io.Writer) (map[string]mainRun, bool, error) {
	repo := opts.Repo
	workflows, err := activeWorkflows(repo)
	if err != nil {
		return nil, false, err
	}

	latest := map[string]mainRun{}
	complete := true
	for _, workflow := range workflows {
		run, seen := latestRunOnMain(repo, workflow, stderr)
		switch seen {
		case lookupFailed:
			// Record the gap rather than letting silence pass for health.
			complete = false
			continue
		case observedNoRun:
			continue // looked, nothing qualifying; not a gap in observation
		case observedRun:
		}
		// A workflow that no longer has any trigger reaching main cannot be
		// red on main today, whatever its history says. Routing Enforcement
		// dropped its push trigger, so its newest main run is a failure from a
		// tree that no longer exists; without this it re-files forever.
		//
		// Deliberately NOT an age cutoff. Health state must not expire on the
		// ROI lookback: a genuinely unresolved failure stays red past any
		// window, and monthly-audit.yml alone can leave 31-day gaps.
		if !opts.EvaluatesMain(run.WorkflowName) {
			continue
		}
		latest[run.WorkflowName] = run
	}
	return latest, complete, nil
}

// dequeueIncident hands the oldest queued incident to the fix agent by printing
// it, and drops the queue label so the next sweep moves on to the one behind
// it. At most one per sweep: an agent told to fix several unrelated workflows
// must either combine them into one pull request or abandon some.
func dequeueIncident(repo string, stdout, stderr io.Writer) error {
	out, ok := gh(stderr, "issue", "list", "--repo", repo,
		"--label", retroLabel, "--label", queuedLabel, "--state", "open",
		"--limit", "50", "--json", "number,title,createdAt",
		"--jq", `sort_by(.createdAt) | .[0] | "\(.number)\t\(.title)"`)
	if !ok {
		return errors.New("listing queued incidents")
	}

	number, title, found := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !found || number == "" {
		return nil // nothing waiting
	}

	if err := runGH("dequeuing incident", stderr, "issue", "edit", number,
		"--repo", repo, "--remove-label", queuedLabel); err != nil {
		// Do not announce an incident that is still queued: the next sweep
		// would announce it again and dispatch a second agent onto it.
		return err
	}
	fmt.Fprintf(stdout, "NEW INCIDENT: %s\n", title)
	return nil
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
// observation distinguishes "the workflow has no qualifying run on main" — a
// perfectly good observation, true of every newly added or pull-request-only
// workflow — from "the query failed". Collapsing them made one PR-only workflow
// enough to mark every sweep incomplete, which suppressed stale reconciliation
// permanently and left recovered incidents open forever.
type observation int

const (
	observedRun observation = iota
	observedNoRun
	lookupFailed
)

func latestRunOnMain(repo, workflow string, stderr io.Writer) (mainRun, observation) {
	out, ok := gh(stderr, "run", "list",
		"--repo", repo, "--branch", "main", "--workflow", workflow, "--limit", "20",
		"--json", "databaseId,workflowName,conclusion,headSha,url,createdAt,event")
	if !ok {
		return mainRun{}, lookupFailed
	}

	var all []mainRun
	if err := json.Unmarshal(out, &all); err != nil {
		fmt.Fprintf(stderr, "ci-escape-analysis: decoding runs for %s: %v\n", workflow, err)
		return mainRun{}, lookupFailed
	}

	// gh returns newest first, so the first concluded sighting is the latest.
	for index, run := range all {
		if inconclusiveRunConclusions[run.Conclusion] || run.WorkflowName == "" {
			continue
		}
		// A cancelled run with anything newer behind it — in flight or not —
		// is concurrency supersession, which says nothing about health. A
		// cancelled run that is the newest of all was stopped by a human or by
		// its own timeout, and that commit has no successful build.
		if run.Conclusion == "cancelled" && index > 0 {
			continue
		}
		// Only events that actually evaluate main count. `Claude Code` runs on
		// the default branch for issue and review events, so a failed @claude
		// invocation would otherwise be filed as main being broken and hand a
		// repair agent an incident that never existed.
		if !mainEvaluatingEvents[run.Event] {
			continue
		}
		return run, observedRun
	}
	return mainRun{}, observedNoRun
}

func buildRetro(opts sweepOptions, run mainRun, required []cihealth.RequiredContext, requiredKnown bool, stderr io.Writer) cihealth.Retro {
	failing, jobs := failingCheckForRun(opts.Repo, run, stderr)
	// A run that failed before producing any job has no check to name. Using
	// the workflow's display name instead invents a context no pull request
	// could have reported, which Classify then reads as `never-ran`. But a
	// failed lookup is not the same as a job-less run: calling that a
	// workflow-level failure would send the fixer after workflow syntax when
	// the failing job is merely unknown.
	workflowLevel := jobs == noJobs
	if failing == "" {
		failing = run.WorkflowName
	}

	preMerge := opts.PreMergeCapable(run.WorkflowName, failing)

	escape := cihealth.Escape{
		FailingCheck:         failing,
		MainSHA:              run.HeadSHA,
		RequiredContexts:     required,
		RequiredKnown:        requiredKnown,
		DiffScoped:           isDiffScoped(failing),
		WorkflowLevelFailure: workflowLevel,
		PreMergeCapable:      preMerge,
		ScheduledDetection:   run.scheduledDetection(),
	}

	// A job list that could not be read leaves the classification without its
	// subject, exactly as an unread check list does — so it starts the
	// known-state off, and a later successful check lookup cannot turn it back
	// on.
	escape.PRKnown = true
	escape.PRChecksKnown = jobs != jobLookupFailed

	// A scheduled detection is not attributable to the head commit, so do not
	// go looking for "the pull request that caused it" — there isn't one.
	if !escape.ScheduledDetection {
		escape.PRNumber, escape.PRKnown = lookupPR(opts.Repo, run.HeadSHA, stderr)
		if escape.PRNumber > 0 {
			checks, known := lookupPRChecks(opts.Repo, escape.PRNumber, stderr)
			escape.PRChecks = checks
			escape.PRChecksKnown = escape.PRChecksKnown && known
		}
	}

	prevention, preventionMeasured, truncated := estimatePrevention(opts.Repo, run.WorkflowName, opts.WindowDays, stderr)
	escapes, escapesTruncated, escapesMeasured := countEscapes(opts.Repo, run.WorkflowName, opts.WindowDays, stderr)

	return cihealth.Retro{
		Repo:         opts.Repo,
		FailingCheck: failing,
		WorkflowName: run.WorkflowName,
		MainSHA:      run.HeadSHA,
		RunURL:       run.URL,
		Finding:      cihealth.Classify(escape),
		ROI: cihealth.ROI{
			CureMinutes:      opts.CureMinutes,
			CureAssumed:      !opts.CureMeasured,
			Escapes:          escapes,
			EscapesTruncated: escapesTruncated,
			EscapesMeasured:  escapesMeasured,
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
			// Timed for the whole workflow, not the failing job. In a
			// multi-job workflow an unrelated matrix or integration job can
			// dominate the wall clock and make a cheap check look expensive,
			// so the denominator is labelled for what it is.
			PreventionScope: fmt.Sprintf("wall-clock of the whole %q workflow on pull requests, not of this job alone", run.WorkflowName),
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
// jobLookup distinguishes a run that genuinely produced no job from one whose
// job list could not be read.
type jobLookup int

const (
	foundJob jobLookup = iota
	noJobs
	jobLookupFailed
)

func failingCheckForRun(repo string, run mainRun, stderr io.Writer) (string, jobLookup) {
	if run.DatabaseID == 0 {
		return "", jobLookupFailed
	}
	out, ok := gh(stderr, "run", "view", fmt.Sprint(run.DatabaseID), "--repo", repo,
		"--json", "jobs",
		"--jq", fmt.Sprintf(`.jobs[] | select(%s) | .name`, jqInSet(".conclusion", failedJobConclusions())))
	if !ok {
		return "", jobLookupFailed
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line, foundJob
		}
	}
	return "", noJobs
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

// upsertRetro files a new incident brief or comments on the existing one. The
// title is stable per failing check, which is what makes recurrence countable.
// A new brief is created carrying the queue label; dispatch is decided later by
// dequeueIncident, not here.
func upsertRetro(repo string, retro cihealth.Retro, run mainRun, stderr io.Writer) error {
	existing, open, known := findRetro(repo, retro.Title(), stderr)
	if !known {
		return errors.New("could not read existing incidents; refusing to risk a duplicate")
	}

	switch {
	case existing > 0 && open:
		body := fmt.Sprintf("Still red at [`%s`](%s).\n\n%s", shortSHA(run.HeadSHA), run.URL, retro.Body())
		return runGH("commenting on retro", stderr, "issue", "comment", fmt.Sprint(existing),
			"--repo", repo, "--body", body)

	case existing > 0:
		// Recurrence of a check whose incident was closed. Reopen and requeue
		// rather than opening a duplicate: the point of a stable title is that
		// one issue carries the whole history of this check going red.
		// Label first, THEN reopen. The other order leaves a window where the
		// issue is open but unqueued if the label edit fails: every later
		// sweep takes the already-open branch and only comments, while
		// dequeueIncident cannot see it, so the recurrence is permanently
		// denied a fixer. A closed-but-queued issue is harmless by contrast —
		// dequeueIncident only considers open ones.
		if err := runGH("requeuing retro", stderr, "issue", "edit", fmt.Sprint(existing),
			"--repo", repo, "--add-label", queuedLabel); err != nil {
			return err
		}
		if err := runGH("reopening retro", stderr, "issue", "reopen", fmt.Sprint(existing),
			"--repo", repo); err != nil {
			return err
		}
		body := fmt.Sprintf("Red again at [`%s`](%s) after this incident was closed.\n\n%s", shortSHA(run.HeadSHA), run.URL, retro.Body())
		return runGH("commenting on reopened retro", stderr, "issue", "comment", fmt.Sprint(existing),
			"--repo", repo, "--body", body)

	default:
		return runGH("creating retro", stderr, "issue", "create", "--repo", repo,
			"--title", retro.Title(), "--body", retro.Body(),
			"--label", retroLabel, "--label", queuedLabel)
	}
}

// findRetro locates this command's issue for a check in ANY state.
//
// Open-only would miss a check that recovered, had its incident closed, and
// then failed again — the recurrence would open a second issue with the same
// title, scattering exactly the history the stable title exists to keep in one
// place. Returns the issue number and whether it is currently open.
func findRetro(repo, title string, stderr io.Writer) (number int, open, known bool) {
	out, ok := gh(stderr, "issue", "list", "--repo", repo,
		"--label", retroLabel, "--state", "all", "--limit", "100",
		"--json", "number,title,state,createdAt",
		"--jq", fmt.Sprintf(`[.[] | select(.title == %q)] | sort_by(.createdAt) | .[-1] | "\(.number) \(.state)"`, title))
	if !ok {
		// A failed read is not proof of absence. Treating it as absence has
		// upsertRetro open a second issue with the same title and queue label,
		// so one transient error dispatches a duplicate fix agent.
		return 0, false, false
	}
	numberText, state, found := strings.Cut(strings.TrimSpace(string(out)), " ")
	if !found || strings.HasPrefix(strings.TrimSpace(string(out)), "null") {
		return 0, false, true // read cleanly, nothing there
	}
	if _, err := fmt.Sscanf(numberText, "%d", &number); err != nil {
		return 0, false, false
	}
	return number, strings.EqualFold(state, "OPEN"), true
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
		"--json", "number,title,body", "--jq", `.[] | "\(.number)\t\(.title)\t\(.body | @json)"`)
	if !ok {
		return errors.New("listing open retros to reconcile")
	}

	var errs []error
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 3)
		if len(fields) < 3 || live[fields[1]] {
			continue
		}
		number, title, body := fields[0], fields[1], fields[2]

		// The workflow being green is not enough. A workflow with
		// event-specific jobs can pass on a push run that never executed the
		// job this incident is about — CI's schedule-only `AGM Tagged Sweep`
		// is the standing example — and closing on that would retire an
		// incident no successful run ever addressed.
		// Title is "main red — <workflow> / <check>"; the body carries the
		// workflow too, and is the more reliable of the two.
		check := strings.TrimPrefix(title, "main red — ")
		if _, after, found := strings.Cut(check, " / "); found {
			check = after
		}
		workflow := workflowFromBody(body)
		if !checkRecovered(repo, workflow, check, stderr) {
			fmt.Fprintf(stdout, "Keeping retro #%s open: no successful run of %q observed yet\n", number, check)
			continue
		}

		fmt.Fprintf(stdout, "Closing recovered retro #%s (%s has since passed)\n", number, check)
		if err := runGH("closing retro", stderr, "issue", "close", number, "--repo", repo,
			"--comment", "This check has since passed on `main`; auto-closing. Reopen if the underlying prevention was never landed."); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// workflowBodyMarker is how the retro body names its producing workflow.
var workflowBodyMarker = regexp.MustCompile(`Workflow: \*\*(.+?)\*\*`)

func workflowFromBody(body string) string {
	if m := workflowBodyMarker.FindStringSubmatch(body); len(m) == 2 {
		return m[1]
	}
	return ""
}

// checkRecovered reports whether the named job has actually succeeded in a
// recent run of its workflow on main. Requiring the same job to pass — rather
// than the workflow as a whole — is what keeps an event-specific job's incident
// open until that job itself is green again.
func checkRecovered(repo, workflow, check string, stderr io.Writer) bool {
	if workflow == "" || check == "" {
		return false
	}

	// A workflow-level incident (a startup failure, which produced no job)
	// carries the workflow's own name as its check. No workflow has a job named
	// after itself, so looking for one would keep such an incident open
	// forever; a later successful run of the workflow is what closes it.
	if check == workflow {
		out, ok := gh(stderr, "run", "list", "--repo", repo, "--branch", "main",
			"--workflow", workflow, "--limit", "1", "--status", "success",
			"--json", "databaseId", "--jq", ".[].databaseId")
		return ok && strings.TrimSpace(string(out)) != ""
	}

	// Only a success AFTER the failure counts. Scanning recent runs without a
	// boundary finds the success that preceded the failure and closes the
	// incident on the strength of the very run the failure came after.
	out, ok := gh(stderr, "run", "list", "--repo", repo, "--branch", "main",
		"--workflow", workflow, "--limit", "20",
		"--json", "databaseId,conclusion",
		"--jq", `.[] | "\(.databaseId) \(.conclusion)"`)
	if !ok {
		return false
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		id, conclusion, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found || id == "" {
			continue
		}
		// gh returns newest first. Walking from the newest, the first run that
		// ran this job decides: a success closes the incident, a failure means
		// the newest evidence is still red and the scan stops.
		jobs, ok := gh(stderr, "run", "view", id, "--repo", repo, "--json", "jobs",
			"--jq", fmt.Sprintf(`.jobs[] | select(.name == %q) | .conclusion`, check))
		if !ok {
			return false
		}
		state := strings.TrimSpace(string(jobs))
		if state == "" {
			continue // this run did not execute the job; keep looking back
		}
		_ = conclusion
		return state == "success"
	}
	return false
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
