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
		return upsertComment(ctx, repo, pr, in, stderr)
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
	shouldComment, mixedConcern               bool
	crapFlagged, crapUnknown                  bool
}

type action int

const (
	actionNone action = iota
	actionUpsert
	actionDelete
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
	if !in.shouldComment && !in.mixedConcern && in.crapOutcome == "success" && !in.crapFlagged && !in.crapUnknown {
		return actionDelete
	}
	return actionNone
}

// composeBody renders the comment. priorCrapSection is whatever the comment
// being overwritten already said about code health; it is used only when
// this run has nothing fresher to report.
func composeBody(in inputs, priorCrapSection string) string {
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
	}

	fmt.Fprintln(&b, crapSectionMarker)
	switch {
	case in.crapReport != "":
		fmt.Fprint(&b, in.crapReport)
		if !strings.HasSuffix(in.crapReport, "\n") {
			fmt.Fprintln(&b)
		}
	case priorCrapSection != "":
		fmt.Fprint(&b, priorCrapSection)
		if !strings.HasSuffix(priorCrapSection, "\n") {
			fmt.Fprintln(&b)
		}
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

// upsertComment recovers a prior code-health section if needed, composes the
// body, and edits the existing marker comment in place or posts a new one.
// Best effort throughout: a comment-update failure must never fail the job
// for a signal that promises never to.
func upsertComment(ctx context.Context, repo, pr string, in inputs, stderr io.Writer) int {
	ids, err := markerCommentIDs(ctx, repo, pr)
	if err != nil {
		fmt.Fprintf(stderr, "pr-size-comment: could not list existing comments: %v\n", err)
		return 0
	}

	prior := ""
	if needsCrapRecovery(in) && len(ids) > 0 {
		body, err := commentBody(ctx, repo, ids[0])
		if err != nil {
			fmt.Fprintf(stderr, "pr-size-comment: could not read the existing comment to recover: %v\n", err)
		} else {
			prior = extractCrapSection(body)
		}
	}

	body := composeBody(in, prior)

	if len(ids) > 0 {
		if err := patchComment(ctx, repo, ids[0], body); err != nil {
			fmt.Fprintf(stderr, "pr-size-comment: could not update the comment: %v\n", err)
		}
	} else if err := postComment(ctx, repo, pr, body); err != nil {
		fmt.Fprintf(stderr, "pr-size-comment: could not post the comment: %v\n", err)
	}

	// Collapse any duplicates left by earlier revisions of this workflow.
	// Best effort: a leftover duplicate is cosmetic, not the audit record.
	for _, id := range ids[min(1, len(ids)):] {
		if err := deleteComment(ctx, repo, id); err != nil {
			fmt.Fprintf(stderr, "pr-size-comment: could not delete duplicate comment %s: %v\n", id, err)
		}
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
		return 0
	}
	for _, id := range ids {
		if err := deleteComment(ctx, repo, id); err != nil {
			fmt.Fprintf(stderr, "pr-size-comment: could not delete stale comment %s: %v\n", id, err)
		}
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
