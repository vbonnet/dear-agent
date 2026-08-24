// Command pr-size-comment composes and upserts the sticky PR comment that
// reports .github/workflows/pr-size-scope.yml's deterministic size, scope,
// and code-health signals.
//
// It exists because that logic outgrew what a workflow bash step should
// carry: composing the body, recovering a code-health finding across a run
// that could not re-measure it, and upserting or deduplicating the GitHub
// comment are all easier to get right — and to test — in Go than in a
// growing block of embedded shell.
//
// Usage:
//
//	pr-size-comment -repo <owner/repo> -pr <n> [flags...]
//
// It is advisory by construction, matching the signals it reports: every
// failure mode here is a best-effort comment update, never a build failure.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	os.Exit(mainExitCode())
}

func mainExitCode() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout, os.Stderr)
}

// marker identifies the sticky size-scope comment so a later run edits it in
// place rather than posting a new one. An edit notifies nobody, which is what
// stops the signal from training subscribers to mute the thread — the
// previous post-then-delete-the-rest order re-notified everyone on every
// synchronize.
const marker = "<!-- pr-size-scope -->"

// crapSectionMarker delimits the code-health portion of the body so a later
// run can recover it verbatim from the comment it is about to overwrite.
const crapSectionMarker = "<!-- crap-section -->"

func run(ctx context.Context, args []string, _, stderr io.Writer) int {
	fs := flag.NewFlagSet("pr-size-comment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var in inputs
	var repo, pr string
	// The four "boolean" flags are read as strings, not fs.BoolVar: a GitHub
	// Actions step output that never got set (a skipped or crashed upstream
	// step) renders as an empty string, and Go's flag package rejects
	// `-flag=` for a bool outright rather than treating it as false. Anything
	// other than exactly "true" is treated as false, matching how the
	// workflow's own `== 'true'` conditions already read these values.
	var shouldComment, mixedConcern, crapFlagged, crapUnknown string
	fs.StringVar(&repo, "repo", "", "GitHub repo as owner/repo")
	fs.StringVar(&pr, "pr", "", "pull request number")
	fs.StringVar(&in.changedLines, "changed-lines", "", "")
	fs.StringVar(&in.changedFiles, "changed-files", "", "")
	fs.StringVar(&in.topLevelAreas, "top-level-areas", "", "")
	fs.StringVar(&in.reasons, "reasons", "", "")
	fs.StringVar(&in.concernReason, "concern-reason", "", "")
	fs.StringVar(&in.crapReport, "crap-report", "", "")
	fs.StringVar(&in.crapSummary, "crap-summary", "", "")
	fs.StringVar(&in.crapOutcome, "crap-outcome", "", "the crap-lint step's own outcome (success, failure, skipped, ...)")
	fs.StringVar(&in.scopeOutcome, "scope-outcome", "", "the scope-detector step's own outcome")
	fs.StringVar(&in.concernOutcome, "concern-outcome", "", "the mixed-concern-detector step's own outcome")
	fs.StringVar(&shouldComment, "should-comment", "", "")
	fs.StringVar(&mixedConcern, "mixed-concern", "", "")
	fs.StringVar(&crapFlagged, "crap-flagged", "", "")
	fs.StringVar(&crapUnknown, "crap-unknown", "", "")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if repo == "" || pr == "" {
		fmt.Fprintln(stderr, "pr-size-comment: both -repo and -pr are required")
		return 2
	}
	in.shouldComment = shouldComment == "true"
	in.mixedConcern = mixedConcern == "true"
	in.crapFlagged = crapFlagged == "true"
	in.crapUnknown = crapUnknown == "true"

	switch decide(in) {
	case actionDelete:
		return deleteStaleComments(ctx, repo, pr, stderr)
	case actionUpsert:
		return upsertComment(ctx, repo, pr, in, false, stderr)
	case actionRefreshIfExists:
		return upsertComment(ctx, repo, pr, in, true, stderr)
	case actionNone:
		return 0
	}
	return 0
}

// inputs mirrors the step outputs the workflow previously wired directly
// into bash env vars.
type inputs struct {
	changedLines, changedFiles, topLevelAreas string
	reasons, concernReason                    string
	crapReport, crapSummary, crapOutcome      string
	scopeOutcome, concernOutcome              string
	shouldComment, mixedConcern               bool
	crapFlagged, crapUnknown                  bool
}

type action int

const (
	actionNone action = iota
	actionUpsert
	actionDelete
	actionRefreshIfExists
)

// decide reproduces the workflow's two `if:` conditions as one testable
// function.
//
// crapUnknown now also triggers an update (it did not before): a run that
// measured nothing still has something to say — that it measured nothing —
// and staying silent left a first-time unknown run completely invisible.
func decide(in inputs) action {
	if in.shouldComment || in.mixedConcern || in.crapFlagged || in.crapUnknown {
		return actionUpsert
	}
	// A clean delete requires every detector to have actually run and
	// confirmed clean, not just the absence of a flag. shouldComment and
	// mixedConcern come from the scope/concern steps' own GITHUB_OUTPUT;
	// if either step failed outright, its output is simply never written,
	// which reads identically to "false" here. Without checking these
	// steps' own outcomes too, a scope-detector crash on an otherwise
	// clean-looking revision would delete a standing "this PR is oversized"
	// comment on the strength of a signal that was never actually computed.
	if in.crapOutcome == "success" && in.scopeOutcome == "success" && in.concernOutcome == "success" {
		return actionDelete
	}
	if in.crapOutcome == "failure" || in.scopeOutcome == "failure" || in.concernOutcome == "failure" {
		// Nothing about this revision currently flags size, mixed concern,
		// or code health, but at least one detector ran and operationally
		// failed (continue-on-error swallows it into the workflow's own
		// exit code, not this one). We cannot safely delete on an
		// unconfirmed result, but doing nothing is wrong too: whichever
		// detectors DID succeed are still fresh, so if an earlier, larger
		// revision left a comment claiming this PR is oversized, that
		// comment may now be stale and must be refreshed to the current,
		// accurate scope check. Refresh only if a marker comment already
		// exists — there is nothing worth posting for the first time over
		// a mere detector hiccup on an otherwise unremarkable PR.
		return actionRefreshIfExists
	}
	// Any other outcome (cancelled, skipped, or genuinely absent — the
	// step never ran) carries no confirmed signal either way; matches the
	// long-standing no-op behavior for that ambiguity.
	return actionNone
}

// recoveredSectionOpen and recoveredSectionClose bracket a recovered
// code-health finding in the rendered comment body. unwrapRecoveredSection
// looks for this exact pair to peel a previously recovered section back to
// its innermost content before composeBody re-wraps it.
const (
	recoveredSectionOpen  = "<details><summary>Last known code-health result (recovered — not from this revision)</summary>\n\n"
	recoveredSectionClose = "</details>\n"
)

// unwrapRecoveredSection returns the innermost content of a previously
// recovered code-health section, discarding both the wrapper and whatever
// "current run" text composeBody printed ahead of it. Without this, two or
// more consecutive runs that each need to recover the prior section (for
// example, back-to-back crapUnknown syncs) would nest the already-recovered
// section inside another recovery wrapper on every sync, growing the sticky
// comment and duplicating the same stale diagnostic each time instead of
// holding steady at one recovered finding. A section that was never
// recovered (no wrapper present) is returned unchanged.
func unwrapRecoveredSection(s string) string {
	_, inner, ok := strings.Cut(s, recoveredSectionOpen)
	if !ok {
		return s
	}
	end := strings.LastIndex(inner, recoveredSectionClose)
	if end < 0 {
		return s
	}
	return inner[:end]
}

// composeBody renders the comment. priorCrapSection is whatever the comment
// being overwritten already said about code health; it is used only when
// this run has nothing fresher to report.
func composeBody(in inputs, priorCrapSection string, priorSizeScope ...string) string { //nolint:gocyclo // renders four independently recoverable signal states
	priorCrapSection = unwrapRecoveredSection(priorCrapSection)

	var b strings.Builder
	fmt.Fprintln(&b, marker)
	fmt.Fprintln(&b, "## PR size, scope, and code health signals")
	fmt.Fprintln(&b)
	if in.reasons != "" || in.concernReason != "" {
		fmt.Fprintln(&b, "This PR tripped a deterministic split-suggestion signal:")
		fmt.Fprintln(&b)
		if in.reasons != "" {
			fmt.Fprintln(&b, in.reasons)
		}
		if in.concernReason != "" {
			fmt.Fprintln(&b, in.concernReason)
		}
		fmt.Fprintln(&b)
		fmt.Fprintf(&b, "Current scope: %s changed lines, %s changed files, %s top-level areas.\n", in.changedLines, in.changedFiles, in.topLevelAreas)
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Please consider splitting this into stacked PRs: mechanical refactors or renames first, then focused behavior changes that can be reviewed and tested independently. See [CONTRIBUTING.md — Small, stacked PRs](../blob/main/CONTRIBUTING.md#small-stacked-prs), which also covers restacking a descendant once its predecessor lands.")
		fmt.Fprintln(&b)
	} else if len(priorSizeScope) > 0 && priorSizeScope[0] != "" {
		fmt.Fprint(&b, priorSizeScope[0])
		if !strings.HasSuffix(priorSizeScope[0], "\n") {
			fmt.Fprintln(&b)
		}
	}

	fmt.Fprintln(&b, crapSectionMarker)
	switch {
	case in.crapReport != "":
		fmt.Fprint(&b, in.crapReport)
		if !strings.HasSuffix(in.crapReport, "\n") {
			fmt.Fprintln(&b)
		}
	case priorCrapSection != "":
		// Recovering a prior section -- whether this run measured nothing
		// (crapUnknown) or the code-health step simply failed operationally
		// -- must always say so. Labeling only the crapUnknown case (the
		// original behavior) let an operational failure republish a stale
		// finding with no indication it was not from this revision.
		if in.crapUnknown && in.crapSummary != "" {
			fmt.Fprintln(&b, in.crapSummary)
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b, "<details><summary>Last known code-health result (recovered — not from this revision)</summary>")
		fmt.Fprintln(&b)
		fmt.Fprint(&b, priorCrapSection)
		if !strings.HasSuffix(priorCrapSection, "\n") {
			fmt.Fprintln(&b)
		}
		fmt.Fprintln(&b, "</details>")
	case in.crapUnknown && in.crapSummary != "":
		fmt.Fprintln(&b, in.crapSummary)
	}
	return b.String()
}

// needsCrapRecovery reports whether this run's own crap result is not fresh
// enough to trust on its own, so a prior comment's section should be
// recovered rather than silently dropped.
//
// A blank CRAP_REPORT is ambiguous by itself: it means "clean" only when the
// crap step actually completed AND found nothing worth reporting. An
// operational failure (continue-on-error swallows it) and a run that
// genuinely could not measure anything (crapUnknown) both leave the same
// blank behind, and both must fall back to whatever the last run said rather
// than let an unrelated update (still oversized, say) silently erase it.
func needsCrapRecovery(in inputs) bool {
	return in.crapReport == "" && (in.crapOutcome != "success" || in.crapUnknown)
}

// extractCrapSection pulls the code-health section out of a previously
// rendered comment body, or "" if the marker is absent (an older comment
// predating this marker, or none at all).
func extractCrapSection(body string) string {
	_, after, found := strings.Cut(body, crapSectionMarker)
	if !found {
		return ""
	}
	return strings.TrimPrefix(after, "\n")
}

func extractSizeScopeSection(body string) string {
	_, after, found := strings.Cut(body, "## PR size, scope, and code health signals\n\n")
	if !found {
		return ""
	}
	section, _, found := strings.Cut(after, crapSectionMarker)
	if !found {
		return ""
	}
	// A run with nothing to say left this section empty; TrimSpace alone
	// would still turn that back into "\n\n" below, which composeBody's
	// non-empty check would treat as real content to restore, adding two
	// stray blank lines above the crap-section marker on the next render.
	trimmed := strings.TrimSpace(section)
	if trimmed == "" {
		return ""
	}
	return trimmed + "\n\n"
}

// upsertComment recovers a prior code-health section if needed, composes the
// body, and edits the existing marker comment in place or posts a new one.
// Best effort throughout: a comment-update failure must never fail the job
// for a signal that promises never to.
//
// onlyIfExists is set for actionRefreshIfExists: when nothing about this
// revision currently flags anything but the CRAP step's own result could not
// be confirmed clean, there is a stale comment to correct if one exists, but
// nothing worth posting for the first time.
// exitFailedOperation is returned when the decided action ran but at least
// one gh operation it depended on failed. It is distinct from the flag/usage
// exit code (2) and from success (0): the workflow step that calls this
// command has no other way to notice that its user-facing comment was not
// actually updated — every gh error was previously logged and then this
// returned 0 regardless, so the run reported success even though the
// signal reviewers rely on silently went stale or missing.
const exitFailedOperation = 1

func upsertComment(ctx context.Context, repo, pr string, in inputs, onlyIfExists bool, stderr io.Writer) int { //nolint:gocyclo // preserves independent detector and cleanup failure states
	ids, err := markerCommentIDs(ctx, repo, pr)
	if err != nil {
		fmt.Fprintf(stderr, "pr-size-comment: could not list existing comments: %v\n", err)
		return exitFailedOperation
	}
	if onlyIfExists && len(ids) == 0 {
		return 0
	}

	prior, priorSize := "", ""
	if (needsCrapRecovery(in) || in.scopeOutcome != "success" || in.concernOutcome != "success") && len(ids) > 0 {
		body, err := commentBody(ctx, repo, ids[0])
		if err != nil {
			// Do not fall through to patching with prior == "": recovery
			// exists precisely because this run has nothing fresh of its
			// own to show, so patching now would overwrite the only stored
			// code-health finding with a blank section — the exact data
			// recovery was meant to preserve — instead of leaving the
			// existing, still-accurate comment alone until a future run can
			// actually read it.
			fmt.Fprintf(stderr, "pr-size-comment: could not read the existing comment to recover: %v\n", err)
			return exitFailedOperation
		}
		prior = extractCrapSection(body)
		if in.scopeOutcome != "success" || in.concernOutcome != "success" {
			priorSize = extractSizeScopeSection(body)
		}
	}

	failed := false
	body := composeBody(in, prior, priorSize)

	if len(ids) > 0 {
		if err := patchComment(ctx, repo, ids[0], body); err != nil {
			fmt.Fprintf(stderr, "pr-size-comment: could not update the comment: %v\n", err)
			failed = true
		}
	} else if err := postComment(ctx, repo, pr, body); err != nil {
		fmt.Fprintf(stderr, "pr-size-comment: could not post the comment: %v\n", err)
		failed = true
	}

	// Collapse any duplicates left by earlier revisions of this workflow.
	// A duplicate deletion is part of the requested update and must be
	// reported if it fails, even when the primary update succeeded.
	if !failed {
		for _, id := range ids[min(1, len(ids)):] {
			if err := deleteComment(ctx, repo, id); err != nil {
				fmt.Fprintf(stderr, "pr-size-comment: could not delete duplicate comment %s: %v\n", id, err)
				failed = true
			}
		}
	}
	if failed {
		return exitFailedOperation
	}
	return 0
}

// deleteStaleComments removes every marker comment. A later synchronize can
// drop below every threshold; this clears the marker an earlier, larger
// revision of the PR left behind.
func deleteStaleComments(ctx context.Context, repo, pr string, stderr io.Writer) int {
	ids, err := markerCommentIDs(ctx, repo, pr)
	if err != nil {
		fmt.Fprintf(stderr, "pr-size-comment: could not list existing comments: %v\n", err)
		return exitFailedOperation
	}
	failed := false
	for _, id := range ids {
		if err := deleteComment(ctx, repo, id); err != nil {
			fmt.Fprintf(stderr, "pr-size-comment: could not delete stale comment %s: %v\n", id, err)
			failed = true
		}
	}
	if failed {
		return exitFailedOperation
	}
	return 0
}

func markerCommentIDs(ctx context.Context, repo, pr string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/issues/%s/comments", repo, pr), "--paginate",
		"--jq", fmt.Sprintf(`.[] | select(.body | startswith(%q)) | .id`, marker)).Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func commentBody(ctx context.Context, repo, id string) (string, error) {
	out, err := exec.CommandContext(ctx, "gh", "api",
		fmt.Sprintf("repos/%s/issues/comments/%s", repo, id), "--jq", ".body").Output()
	return string(out), err
}

func patchComment(ctx context.Context, repo, id, body string) error {
	f, err := os.CreateTemp("", "pr-size-comment-*.md")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "gh", "api", "--method", "PATCH",
		fmt.Sprintf("repos/%s/issues/comments/%s", repo, id), "-F", "body=@"+f.Name())
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func postComment(ctx context.Context, repo, pr, body string) error {
	cmd := exec.CommandContext(ctx, "gh", "pr", "comment", pr, "--repo", repo, "--body-file", "-")
	cmd.Stdin = strings.NewReader(body)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func deleteComment(ctx context.Context, repo, id string) error {
	return exec.CommandContext(ctx, "gh", "api", "--method", "DELETE",
		fmt.Sprintf("repos/%s/issues/comments/%s", repo, id)).Run()
}
