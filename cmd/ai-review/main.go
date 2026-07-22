package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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

// config is the parsed environment.
type config struct {
	baseSHA   string
	headSHA   string
	pr        string
	repo      string
	eventName string
	isFork    bool
	override  bool
	prBody    string
	apiKey    string
	model     anthropic.Model
	effort    anthropic.OutputConfigEffort
	maxDiff   int
}

// loadConfig reads the workflow environment. It does not validate the API key
// here — key handling is a fail-closed decision made in run().
func loadConfig() config {
	c := config{
		baseSHA:   os.Getenv("BASE_SHA"),
		headSHA:   os.Getenv("HEAD_SHA"),
		pr:        os.Getenv("PR"),
		repo:      os.Getenv("REPO"),
		eventName: os.Getenv("EVENT_NAME"),
		isFork:    os.Getenv("IS_FORK") == "true",
		override:  os.Getenv("OVERRIDE") == "true",
		prBody:    os.Getenv("PR_BODY"),
		apiKey:    os.Getenv("ANTHROPIC_API_KEY"),
		model:     anthropic.ModelClaudeOpus4_8,
		effort:    anthropic.OutputConfigEffortHigh,
		maxDiff:   defaultMaxDiffBytes,
	}
	if m := os.Getenv("AI_REVIEW_MODEL"); m != "" {
		c.model = anthropic.Model(m)
	}
	if e := os.Getenv("AI_REVIEW_EFFORT"); e != "" {
		c.effort = anthropic.OutputConfigEffort(e)
	}
	if n := os.Getenv("AI_REVIEW_MAX_DIFF_BYTES"); n != "" {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			c.maxDiff = v
		}
	}
	return c
}

// gitDiff returns the full diff between base and head. No truncation.
func gitDiff(base, head string) (string, error) {
	out, err := exec.Command("git", "diff", base, head).Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s %s: %w", base, head, err)
	}
	return string(out), nil
}

// gitChangedPaths lists the files changed between base and head. Used for the
// deterministic REVIEW.md §3 escalation triggers.
func gitChangedPaths(base, head string) ([]string, error) {
	out, err := exec.Command("git", "diff", "--name-only", base, head).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only %s %s: %w", base, head, err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}

// gitCommitMessages returns the commit messages in base..head so the explicit
// "HUMAN REVIEW REQUIRED" marker can be detected (REVIEW.md §3).
func gitCommitMessages(base, head string) string {
	out, err := exec.Command("git", "log", "--format=%B", base+".."+head).Output()
	if err != nil {
		// Non-fatal: the marker may still appear in the PR body.
		return ""
	}
	return string(out)
}

func main() {
	os.Exit(run(loadConfig()))
}

// run executes the review and returns the process exit code. It is separated
// from main so tests can exercise the env-driven control flow. Every failure
// path returns a non-zero code (fail closed); only an Approved outcome, an
// empty diff, or the audited override yields 0.
func run(c config) int {
	// Fork PRs cannot read repo secrets, so the review cannot run. A required
	// check that silently skips would leave the PR pending forever; instead we
	// fail closed with an explanation (SPEC R8). A human maintainer reviews and
	// applies the override label to merge.
	if c.isFork && !c.override {
		fmt.Println("::error::This PR originates from a fork; the automated AI review cannot run (no secret access). A maintainer must review and apply the 'ai-review:override' label to merge.")
		postComment(c, forkComment())
		return 1
	}

	// Missing secret fails closed (SPEC R4) — no silent skip.
	if strings.TrimSpace(c.apiKey) == "" {
		if c.override {
			fmt.Println("::warning::ANTHROPIC_API_KEY missing but 'ai-review:override' label present; passing on human authority.")
			// AIREV-03: the gate always posts its result before passing on
			// human authority, so the override leaves a sticky audit trail.
			postComment(c, overrideComment("the ANTHROPIC_API_KEY secret is not configured, so no automated review could run"))
			return 0
		}
		fmt.Println("::error::ANTHROPIC_API_KEY is not set; the AI review gate cannot run and fails closed. Set the secret, or apply the 'ai-review:override' label after a human review.")
		return 1
	}

	diff, err := gitDiff(c.baseSHA, c.headSHA)
	if err != nil {
		fmt.Printf("::error::could not compute diff: %v\n", err)
		return failClosed(c)
	}

	// Empty diff: nothing to review (SPEC R11).
	if strings.TrimSpace(diff) == "" {
		fmt.Println("::notice::empty diff; nothing to review.")
		return 0
	}

	// Oversize diff fails closed rather than truncating (SPEC R10).
	if len(diff) > c.maxDiff {
		msg := fmt.Sprintf("diff is %d bytes, over the %d-byte auto-review limit. Split the PR into smaller reviewable changes, or apply the 'ai-review:override' label after a human review.", len(diff), c.maxDiff)
		fmt.Printf("::error::%s\n", msg)
		postComment(c, oversizeComment(len(diff), c.maxDiff))
		return failClosed(c)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	client := anthropic.NewClient(option.WithAPIKey(c.apiKey))

	reports, err := runDimensions(ctx, client, c.model, c.effort, diff)
	if err != nil {
		// Any dimension error fails closed (SPEC R5).
		fmt.Printf("::error::a review dimension failed: %v\n", err)
		return failClosed(c)
	}

	outcome, synthesis, err := synthesize(ctx, client, c.model, c.effort, reports)
	if err != nil {
		// Synthesis error fails closed (SPEC R6).
		fmt.Printf("::error::review synthesis failed: %v\n", err)
		return failClosed(c)
	}

	// REVIEW.md §3 escalation is mandatory "regardless of finding severity", so
	// it is enforced deterministically here rather than trusted to the model.
	changed, err := gitChangedPaths(c.baseSHA, c.headSHA)
	if err != nil {
		fmt.Printf("::error::could not list changed paths: %v\n", err)
		return failClosed(c)
	}
	triggers := EscalationTriggers(changed, c.prBody, gitCommitMessages(c.baseSHA, c.headSHA))
	if len(triggers) > 0 {
		fmt.Printf("::warning::REVIEW.md §3 escalation triggered: %s\n", strings.Join(triggers, "; "))
	}
	outcome = ApplyEscalation(outcome, triggers)

	// Comment is best-effort and never changes the exit code.
	postComment(c, buildComment(outcome, synthesis, reports, c.override, triggers))

	code := ExitFor(outcome, c.override)
	fmt.Printf("::notice::AI review outcome: %s (exit %d, override=%t)\n", outcome, code, c.override)
	return code
}

// failClosed returns the blocking exit code (1) for an intended failure, unless
// the audited override label is present, in which case an operator has
// consciously unblocked the merge and it returns 0.
func failClosed(c config) int {
	if c.override {
		fmt.Println("::warning::failure overridden by 'ai-review:override' label.")
		return 0
	}
	return 1
}
