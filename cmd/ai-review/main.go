package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// defaultMaxDiffBytes is the size above which the reviewer refuses to run
// rather than truncate. A partial review that looks complete is more dangerous
// than an honest "too big to auto-review" (SPEC R10). Override with
// AI_REVIEW_MAX_DIFF_BYTES.
const defaultMaxDiffBytes = 1_500_000

const maxAllowedReviewDiffBytes = 8 * 1024 * 1024

const reviewAbsoluteDeadlineEnv = "AI_REVIEW_DEADLINE_UNIX"

// config is the parsed environment.
type config struct {
	baseSHA          string
	headSHA          string
	pr               string
	repo             string
	eventName        string
	isFork           bool
	override         bool
	prBody           string
	apiKey           string
	apiBaseURL       string
	keyFromEnv       bool
	githubActions    bool
	absoluteDeadline string
	reviewContext    context.Context
	model            anthropic.Model
	effort           anthropic.OutputConfigEffort
	maxDiff          int
}

// loadConfig reads the non-secret workflow environment. It deliberately
// defers reading the API key until run() has built the authenticated plan and
// completed deterministic prechecks.
func loadConfig() config {
	c := config{
		baseSHA:          os.Getenv("BASE_SHA"),
		headSHA:          os.Getenv("HEAD_SHA"),
		pr:               os.Getenv("PR"),
		repo:             os.Getenv("REPO"),
		eventName:        os.Getenv("EVENT_NAME"),
		isFork:           os.Getenv("IS_FORK") == "true",
		override:         os.Getenv("OVERRIDE") == "true",
		prBody:           os.Getenv("PR_BODY"),
		keyFromEnv:       true,
		githubActions:    os.Getenv("GITHUB_ACTIONS") == "true",
		absoluteDeadline: os.Getenv(reviewAbsoluteDeadlineEnv),
		model:            anthropic.ModelClaudeOpus4_8,
		effort:           anthropic.OutputConfigEffortHigh,
		maxDiff:          defaultMaxDiffBytes,
	}
	if m := os.Getenv("AI_REVIEW_MODEL"); m != "" {
		c.model = anthropic.Model(m)
	}
	if e := os.Getenv("AI_REVIEW_EFFORT"); e != "" {
		c.effort = anthropic.OutputConfigEffort(e)
	}
	if n := os.Getenv("AI_REVIEW_MAX_DIFF_BYTES"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 && v <= maxAllowedReviewDiffBytes {
			c.maxDiff = v
		}
	}
	return c
}

// effectiveReviewDeadline caps one command invocation at the earlier of its
// local pipeline budget and the trusted workflow cutoff. Local invocations do
// not require workflow metadata; GitHub Actions invocations fail closed when
// the protected workflow did not export a canonical, still-future deadline.
func effectiveReviewDeadline(now time.Time, githubActions bool, rawAbsolute string) (time.Time, error) {
	localDeadline := now.Add(reviewPipelineTimeout)
	if !githubActions {
		return localDeadline, nil
	}
	if rawAbsolute == "" {
		return time.Time{}, fmt.Errorf("%s is required in GitHub Actions", reviewAbsoluteDeadlineEnv)
	}
	seconds, err := strconv.ParseInt(rawAbsolute, 10, 64)
	if err != nil || seconds <= 0 {
		return time.Time{}, fmt.Errorf("%s is malformed", reviewAbsoluteDeadlineEnv)
	}
	absolute := time.Unix(seconds, 0).UTC()
	if !absolute.After(now) {
		return time.Time{}, fmt.Errorf("%s has expired", reviewAbsoluteDeadlineEnv)
	}
	if absolute.Before(localDeadline) {
		return absolute, nil
	}
	return localDeadline, nil
}

func newReviewContext(parent context.Context, now time.Time, githubActions bool, rawAbsolute string) (context.Context, context.CancelFunc, error) {
	deadline, err := effectiveReviewDeadline(now, githubActions, rawAbsolute)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	return ctx, cancel, nil
}

// gitMergeBase finds the common ancestor of the current base and PR head.
// Reviewing from that point excludes unrelated commits that landed on the base
// branch after the PR forked, and makes every review input describe the PR.
func gitMergeBase(base, head string) (string, error) {
	return resolveMergeBase(context.Background(), base, head)
}

// gitDiff returns either the complete diff or an overflow signal. A bounded
// prefix is never returned as though it were a reviewable complete diff.
func gitDiff(mergeBase, head string, limit int) (string, bool, error) {
	return gitDiffContext(context.Background(), mergeBase, head, limit)
}

func gitDiffContext(ctx context.Context, mergeBase, head string, limit int) (string, bool, error) {
	out, err := gitOutputBounded(ctx, limit, "diff", "--no-ext-diff", "--no-textconv", mergeBase, head)
	if errors.Is(err, errGitOutputLimit) {
		return "", true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("git diff %s %s: %w", mergeBase, head, err)
	}
	return string(out), false, nil
}

// gitChangedPaths lists the files changed between base and head. Used for the
// deterministic REVIEW.md §3 escalation triggers.
//
// --no-renames is load-bearing: with rename detection on, `git mv
// .claude/settings.json safe/config.json` reports only the *destination* path,
// so moving a protected file to an ordinary path would slip past escalation.
// Disabling detection reports the rename as a delete of the old path plus an
// add of the new one, so the protected source is always scanned.
func gitChangedPaths(base, head string) ([]string, error) {
	return gitChangedPathsContext(context.Background(), base, head)
}

func gitChangedPathsContext(ctx context.Context, base, head string) ([]string, error) {
	out, err := gitOutputBounded(ctx, maxGitMetadataBytes, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-only", "-z", base, head)
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s %s: %w", base, head, err)
	}
	fields := strings.Split(string(out), "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}
	if len(fields) > 10000 {
		return nil, errors.New("changed-path count exceeds the review limit")
	}
	for _, path := range fields {
		if !safeGitPath(path) {
			return nil, errors.New("changed-path list contains an unsafe Git path")
		}
	}
	return fields, nil
}

// gitBinaryPaths lists files whose change git could not express as a text
// diff. `git diff` renders these as a one-line "Binary files ... differ"
// marker, so the payload never reaches the reviewers and its real size never
// counts toward the size limit — an unreviewed executable or asset could
// otherwise be approved. These are escalated to a human instead.
func gitBinaryPaths(base, head string) ([]string, error) {
	return gitBinaryPathsContext(context.Background(), base, head)
}

func gitBinaryPathsContext(ctx context.Context, base, head string) ([]string, error) {
	out, err := gitOutputBounded(ctx, maxGitMetadataBytes, "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--numstat", "-z", base, head)
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat %s %s: %w", base, head, err)
	}
	var bins []string
	for _, entry := range bytesSplitNUL(out) {
		if len(entry) == 0 {
			continue
		}
		// numstat marks binary entries with "-\t-\t<path>".
		fields := strings.SplitN(string(entry), "\t", 3)
		if len(fields) != 3 {
			return nil, errors.New("malformed git --numstat output")
		}
		if fields[0] == "-" && fields[1] == "-" {
			if !safeGitPath(fields[2]) {
				return nil, errors.New("git --numstat returned an unsafe binary path")
			}
			bins = append(bins, fields[2])
			if len(bins) > 10000 {
				return nil, errors.New("binary-path count exceeds the review limit")
			}
		}
	}
	return bins, nil
}

// gitlinkMode is git's file mode for a submodule entry (a "gitlink").
const gitlinkMode = "160000"

// gitGitlinkPaths lists submodule (gitlink) entries changed between base and
// head. A submodule bump appears in the diff as a single "Subproject commit
// <sha>" line — the external tree it points at is never in the payload, and
// --numstat counts it as ordinary text rather than binary — so an unreviewed
// dependency could otherwise ride an "approved" outcome (AIREV-06).
func gitGitlinkPaths(base, head string) ([]string, error) {
	return gitGitlinkPathsContext(context.Background(), base, head)
}

func gitGitlinkPathsContext(ctx context.Context, base, head string) ([]string, error) {
	out, err := gitOutputBounded(ctx, maxGitMetadataBytes, "diff", "--no-ext-diff", "--no-textconv", "--raw", "--no-renames", "-z", base, head)
	if err != nil {
		return nil, fmt.Errorf("git diff --raw %s %s: %w", base, head, err)
	}
	var links []string
	entries := bytesSplitNUL(out)
	// With -z, raw diff emits metadata and pathname as separate NUL-delimited
	// fields. --no-renames keeps that framing exactly two fields per change.
	if len(entries)%2 != 0 {
		return nil, errors.New("malformed git --raw NUL framing")
	}
	for i := 0; i < len(entries); i += 2 {
		meta, path := string(entries[i]), string(entries[i+1])
		// :<srcmode> <dstmode> <srcsha> <dstsha> <status>
		if !strings.HasPrefix(meta, ":") {
			return nil, errors.New("malformed git --raw output")
		}
		fields := strings.Fields(strings.TrimPrefix(meta, ":"))
		if len(fields) != 5 {
			return nil, errors.New("malformed git --raw metadata")
		}
		if !safeGitPath(path) {
			return nil, errors.New("git --raw returned an unsafe path")
		}
		if fields[0] == gitlinkMode || fields[1] == gitlinkMode {
			links = append(links, path)
			if len(links) > 10000 {
				return nil, errors.New("gitlink-path count exceeds the review limit")
			}
		}
	}
	return links, nil
}

// gitCommitMessages returns the commit messages in base..head so the explicit
// "HUMAN REVIEW REQUIRED" marker can be detected (REVIEW.md §3).
func gitCommitMessages(base, head string) (string, error) {
	return gitCommitMessagesContext(context.Background(), base, head)
}

func gitCommitMessagesContext(ctx context.Context, base, head string) (string, error) {
	out, err := gitOutputBounded(ctx, maxGitMetadataBytes, "log", "--format=%B", base+".."+head)
	if err != nil {
		return "", fmt.Errorf("git log %s..%s: %w", base, head, err)
	}
	return string(out), nil
}

// deterministicEscalationTriggers collects every Git-derived REVIEW.md §3
// input under the plan's context before any credential decision. It is shared
// by the optional-model path and the credential-free workflow decision: any
// unreadable, malformed, or bounded-out evidence must fail closed rather than
// be mistaken for an irrelevant plan.
func deterministicEscalationTriggers(ctx context.Context, base, head, prBody string) ([]string, error) {
	changed, err := gitChangedPathsContext(ctx, base, head)
	if err != nil {
		return nil, err
	}
	commitMessages, err := gitCommitMessagesContext(ctx, base, head)
	if err != nil {
		return nil, err
	}
	binaries, err := gitBinaryPathsContext(ctx, base, head)
	if err != nil {
		return nil, err
	}
	gitlinks, err := gitGitlinkPathsContext(ctx, base, head)
	if err != nil {
		return nil, err
	}
	triggers := EscalationTriggers(changed, prBody, commitMessages)
	triggers = append(triggers, BinaryEscalationTriggers(binaries)...)
	triggers = append(triggers, GitlinkEscalationTriggers(gitlinks)...)
	return triggers, nil
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--review-plan" {
		plan, err := buildReviewPlanWithPRBody(context.Background(), os.Getenv("BASE_SHA"), os.Getenv("HEAD_SHA"), os.Getenv("PR_BODY"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "ai-review: build review plan: %v\n", err)
			os.Exit(1)
		}
		if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil {
			fmt.Fprintf(os.Stderr, "ai-review: write review plan: %v\n", err)
			os.Exit(1)
		}
		return
	}
	os.Exit(run(loadConfig()))
}

// preflightGates runs the checks that decide the outcome without any model
// call: fork access and secret presence. It returns the exit code and whether
// it handled the run.
func preflightGates(c config) (int, bool) {
	// Fork PRs cannot read repo secrets, so the review cannot run. A required
	// check that silently skips would leave the PR pending forever; instead we
	// fail closed with an explanation (SPEC R8). A human maintainer reviews and
	// applies the override label to merge.
	if c.isFork && !c.override {
		fmt.Println("::error::This PR originates from a fork; the automated AI review cannot run (no secret access). A maintainer must review and apply the 'ai-review:override' label to merge.")
		logCommentErr(postComment(c, forkComment()))
		return 1, true
	}

	// Missing secret fails closed (SPEC R4) — no silent skip.
	if strings.TrimSpace(c.apiKey) == "" {
		if c.override {
			fmt.Println("::warning::ANTHROPIC_API_KEY missing but 'ai-review:override' label present; passing on human authority.")
			// AIREV-03: the gate always posts its result before passing on
			// human authority, so the override leaves a sticky audit trail.
			if !requireComment(c, overrideComment("the ANTHROPIC_API_KEY secret is not configured, so no automated review could run")) {
				return 1, true
			}
			return 0, true
		}
		fmt.Println("::error::ANTHROPIC_API_KEY is not set; the AI review gate cannot run and fails closed. Set the secret, or apply the 'ai-review:override' label after a human review.")
		// AIREV-26 is evidence-first even here: the cannot-run status may
		// only be reported once the disposition is recorded on the PR (or no
		// PR context exists to post to). A failed post keeps the plain
		// blocking exit.
		code := keylessExit(c, 1)
		if code == exitKeylessCannotRun {
			if err := postComment(c, buildComment(NeedsHumanReview, "the ANTHROPIC_API_KEY secret is not configured, so no automated review could run", nil, false, nil, true)); err != nil {
				logCommentErr(err)
				return 1, true
			}
		}
		return code, true
	}
	return 0, false
}

// run executes the review and returns the process exit code. It is separated
// from main so tests can exercise the env-driven control flow. Every failure
// path returns a non-zero code (fail closed); only an Approved outcome, an
// empty diff with no escalation marker, or the audited override yields 0.
func run(c config) int {
	ctx, cancel, err := newReviewContext(context.Background(), time.Now(), c.githubActions, c.absoluteDeadline)
	if err != nil {
		fmt.Printf("::error::trusted review deadline is unavailable: %v\n", err)
		return 1
	}
	defer cancel()
	c.reviewContext = ctx

	// This plan is deliberately built before any credential or model call. It
	// is the one deep seam that authenticates changed-SPEC evidence and performs
	// deterministic ownership/traceability checks for both CI and tests.
	plan, err := buildReviewPlanWithPRBody(ctx, c.baseSHA, c.headSHA, c.prBody)
	if err != nil {
		fmt.Printf("::error::could not build authenticated review plan: %v\n", err)
		return failClosed(c, "the authenticated SPEC governance review plan could not be built")
	}
	// Credential material is intentionally read only after the authenticated
	// plan and deterministic prechecks have completed.
	if c.keyFromEnv {
		c.apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	modelUnavailable := c.isFork || strings.TrimSpace(c.apiKey) == ""
	// A deterministic SPEC-governance human-review verdict (ownership edge,
	// reviewer-dependency, traceability, stale base) is conclusive without a
	// model: the missing credential is NOT its blocker, so it keeps the
	// ordinary blocking exit even when keyless (AIREV-26 boundary).
	// An AIREV-22 bound proof is not such a verdict. It records that the
	// reviewer could not fit this diff inside its own prompt and response
	// budgets, so no verdict was ever reachable and no human finding was
	// made; keyless, that is the cannot-run disposition. It falls through to
	// the shared oversize probe below before any neutral translation, so it
	// proves the same diff bound every other cannot-run path proves.
	keylessBoundProof := plan.capacityOnly() && keylessTranslatable(c)
	if plan.needsHuman() && modelUnavailable && !keylessBoundProof {
		code, _ := handleSpecHumanReview(c, plan, strings.Join(plan.HumanReasons, "; "), false)
		return code
	}
	// AIREV-26 never softens an oversize or uncomputable diff: with a key
	// those are hard failures at the review stage below, so EVERY keyless
	// run that could reach a cannot-run translation — escalation, reviewable
	// SPEC, or the credential preflight on an ordinary diff — must probe the
	// same bound first and fail closed identically. This deliberately
	// mirrors the keyed oversize handling rather than sharing its code so
	// each path stays visible at its own trust transition.
	if keylessTranslatable(c) {
		_, tooLarge, diffErr := gitDiffContext(ctx, plan.MergeBaseSHA, c.headSHA, c.maxDiff)
		if diffErr != nil {
			fmt.Printf("::error::could not compute diff: %v\n", diffErr)
			return failClosed(c, "the PR diff could not be computed")
		}
		if tooLarge {
			fmt.Printf("::error::diff is over the %d-byte auto-review limit. Split the PR into smaller reviewable changes, or apply the 'ai-review:override' label after a human review.\n", c.maxDiff)
			logCommentErr(postComment(c, oversizeComment(c.maxDiff)))
			return failClosed(c, "the diff exceeded the auto-review size limit")
		}
	}
	// The bound proof deferred above, now past the shared oversize probe.
	// Publication stays honest: the evidence comment carries the reason, no
	// approval is claimed, and a failed evidence post keeps the plain
	// blocking exit exactly as every other cannot-run path does.
	if keylessBoundProof {
		code, evidencePosted := handleSpecHumanReview(c, plan, strings.Join(plan.HumanReasons, "; "), true)
		if evidencePosted {
			return keylessExit(c, code)
		}
		return code
	}
	// A deterministic escalation still benefits from the complete automated
	// review when credentials are available. Fork and secretless runs cannot
	// make model calls, so they take the visible human-review fallback instead.
	// keylessExit relabels only the same-repository, non-override, no-key case
	// with the distinct cannot-run code — and only when the escalation
	// evidence comment demonstrably reached the PR, because the workflow's
	// neutral-with-warning verdict points readers at that evidence. A failed
	// evidence post keeps the plain blocking exit. It never changes whether
	// this run blocks.
	if len(plan.EscalationTriggers) > 0 && modelUnavailable {
		code, evidencePosted := handleMandatoryEscalation(c, plan.EscalationTriggers, keylessTranslatable(c))
		if evidencePosted {
			return keylessExit(c, code)
		}
		return code
	}
	// An otherwise-reviewable changed SPEC stopped solely by the absent
	// credential is the canonical cannot-run disposition, under the same
	// evidence-first requirement.
	if plan.ReviewNeeded && modelUnavailable {
		reason := "a relevant changed SPEC cannot be reviewed because the reviewer credential is unavailable"
		if c.isFork {
			reason = "a relevant changed SPEC originates from a fork and requires human review"
		}
		code, evidencePosted := handleSpecHumanReview(c, plan, reason, keylessTranslatable(c))
		if evidencePosted {
			return keylessExit(c, code)
		}
		return code
	}

	if code, handled := preflightGates(c); handled {
		return code
	}

	mergeBase := plan.MergeBaseSHA
	diff, tooLarge, err := gitDiffContext(ctx, mergeBase, c.headSHA, c.maxDiff)
	if err != nil {
		fmt.Printf("::error::could not compute diff: %v\n", err)
		return failClosed(c, "the PR diff could not be computed")
	}
	if tooLarge {
		msg := fmt.Sprintf("diff is over the %d-byte auto-review limit. Split the PR into smaller reviewable changes, or apply the 'ai-review:override' label after a human review.", c.maxDiff)
		fmt.Printf("::error::%s\n", msg)
		logCommentErr(postComment(c, oversizeComment(c.maxDiff)))
		return failClosed(c, "the diff exceeded the auto-review size limit")
	}

	// An empty tree diff is neutral only when the authenticated plan found no
	// deterministic escalation. Explicit PR-body and allow-empty commit markers
	// have no patch to send to the model, but must still require human review.
	if strings.TrimSpace(diff) == "" {
		if len(plan.EscalationTriggers) > 0 {
			code, _ := handleMandatoryEscalation(c, plan.EscalationTriggers, false)
			return code
		}
		fmt.Println("::notice::empty diff; nothing to review.")
		return 0
	}

	clientOptions := []option.RequestOption{option.WithAPIKey(c.apiKey)}
	if c.apiBaseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(c.apiBaseURL))
	}
	client := anthropic.NewClient(clientOptions...)
	var specReports []dimensionReport
	specVerdict := Approved // no changed SPEC is neutral, not a blocking zero value
	if plan.needsHuman() {
		specVerdict = NeedsHumanReview
		specReports = append(specReports, specContractReport(specContractVerdict{
			Status:  NeedsHumanReview,
			Summary: "Authenticated deterministic checks require human review: " + strings.Join(plan.HumanReasons, "; "),
		}))
	} else if plan.ReviewNeeded {
		verdict, err := reviewSpecContract(ctx, client, c.model, c.effort, plan)
		if err != nil {
			code, _ := handleSpecHumanReview(c, plan, "the changed-SPEC reviewer failed or returned an ambiguous verdict", false)
			return code
		}
		specVerdict = verdict.Status
		specReports = append(specReports, specContractReport(verdict))
	}

	reports, err := runDimensions(ctx, client, c.model, c.effort, diff)
	if err != nil {
		// Any dimension error fails closed (SPEC R5).
		fmt.Printf("::error::a review dimension failed: %v\n", err)
		return failClosed(c, "a review dimension call failed")
	}

	// The SPEC report is an input to synthesis, not an after-the-fact appendix.
	// This lets the synthesis explain the same authoritative evidence that the
	// deterministic final-outcome reconciliation enforces below.
	reports = append(specReports, reports...)
	synthesizedOutcome, synthesis, err := synthesize(ctx, client, c.model, c.effort, reports)
	if err != nil {
		// Synthesis error fails closed (SPEC R6).
		fmt.Printf("::error::review synthesis failed: %v\n", err)
		return failClosed(c, "the review synthesis call failed")
	}

	// REVIEW.md §3 escalation is mandatory "regardless of finding severity".
	// The plan returns every deterministic trigger before the model runs, so
	// this application is defensive rather than a second, unauthenticated Git read.
	triggers := plan.EscalationTriggers
	outcome := finalReviewOutcome(synthesizedOutcome, specVerdict, triggers)
	synthesis = reconcileSynthesisDisplay(synthesizedOutcome, outcome, synthesis)

	// Reporting the outcome is best-effort: the exit code already carries the
	// verdict, so a comment outage must not change it.
	logCommentErr(postComment(c, buildComment(outcome, synthesis, reports, c.override, triggers, false)))

	// But when an override is what converts a NON-approved outcome into a pass,
	// the audit record is load-bearing (AIREV-03) — the check would otherwise
	// go green with no sticky explanation of what was overridden.
	if outcome != Approved && c.override {
		if !requireComment(c, overrideComment(fmt.Sprintf("the review outcome was %s", outcome))) {
			return 1
		}
	}

	code := ExitFor(outcome, c.override)
	fmt.Printf("::notice::AI review outcome: %s (exit %d, override=%t)\n", outcome, code, c.override)
	return code
}

func specContractReport(verdict specContractVerdict) dimensionReport {
	return dimensionReport{
		key:  "spec-contract",
		text: fmt.Sprintf("Authoritative outcome: %s\n%s", verdict.Status, renderSpecVerdict(verdict)),
	}
}

// finalReviewOutcome applies the authoritative SPEC verdict before mandatory
// escalation, so a REVIEW.md §3 trigger is always the final needs-human-review
// outcome regardless of what either model review concluded.
func finalReviewOutcome(synthesized, specVerdict Outcome, triggers []string) Outcome {
	return ApplyEscalation(applySpecVerdict(synthesized, specVerdict), triggers)
}

// reconcileSynthesisDisplay prevents the sticky comment from presenting a
// model verdict that deterministic policy has already overruled. When policy
// changes the outcome, the model's conflicting first line and summary are
// replaced rather than displayed as though they were still authoritative.
func reconcileSynthesisDisplay(synthesized, final Outcome, synthesis string) string {
	trimmed := strings.TrimSpace(synthesis)
	firstLine, rest, hasRest := strings.Cut(trimmed, "\n")
	if synthesized == final && strings.TrimSpace(firstLine) == final.String() {
		return trimmed
	}
	if synthesized != final {
		return final.String() + "\nFinal outcome reconciled with deterministic policy and authoritative review reports; see the detailed evidence below."
	}
	if hasRest && strings.TrimSpace(rest) != "" {
		return final.String() + "\n" + strings.TrimSpace(rest)
	}
	return final.String()
}

// handleSpecHumanReview keeps an unresolved SPEC contract visibly in the
// existing review lifecycle. A revision-bound override may pass the check on
// human authority, but it never turns this verdict into approved. The second
// result reports whether the human-review evidence comment demonstrably
// reached the PR (or no PR context exists to post to): the AIREV-26 cannot-run
// translation is only honest when the evidence it advertises actually exists.
func handleSpecHumanReview(c config, plan reviewPlan, reason string, keylessNeutral bool) (int, bool) {
	trigger := "SPEC governance change requires human review: " + reason
	if len(plan.Changes) > 0 {
		trigger += " (" + plan.Changes[0].Path + ")"
	}
	return handleHumanReview(c, reason, []string{trigger}, keylessNeutral)
}

// handleMandatoryEscalation makes a deterministic REVIEW.md §3 trigger
// visible even when no SPEC changed and the reviewer credential is absent.
func handleMandatoryEscalation(c config, triggers []string, keylessNeutral bool) (int, bool) {
	reason := "REVIEW.md §3 requires human review: " + strings.Join(triggers, "; ")
	return handleHumanReview(c, reason, triggers, keylessNeutral)
}

// handleHumanReview posts the evidence comment and returns the blocking exit.
// keylessNeutral marks the AIREV-26 cannot-run disposition, whose banner must
// describe the neutral-with-warning publication the trusted workflow will
// actually make; every other disposition keeps the fail-closed banner.
func handleHumanReview(c config, reason string, triggers []string, keylessNeutral bool) (int, bool) {
	commentErr := postComment(c, buildComment(NeedsHumanReview, reason, nil, c.override, triggers, keylessNeutral))
	logCommentErr(commentErr)
	if c.override && !requireComment(c, overrideComment(reason)) {
		return 1, commentErr == nil
	}
	return ExitFor(NeedsHumanReview, c.override), commentErr == nil
}

// failClosed returns the blocking exit code (1) for an intended failure, unless
// the audited override label is present, in which case an operator has
// consciously unblocked the merge and it returns 0.
//
// Every override path posts a sticky comment (AIREV-03) so a check that passed
// on human authority always carries a visible, auditable explanation of what
// failed — a silent green check is exactly what this gate exists to prevent.
func failClosed(c config, reason string) int {
	if c.override {
		fmt.Println("::warning::failure overridden by 'ai-review:override' label.")
		if !requireComment(c, overrideComment(reason)) {
			return 1
		}
		return 0
	}
	return 1
}
