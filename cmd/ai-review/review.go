package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"golang.org/x/sync/errgroup"
)

// dimension is one of the five independent REVIEW.md §2 review lenses. The
// system prompts are copied verbatim from REVIEW.md §6 so that the enforced
// personas stay version-controlled alongside the protocol.
type dimension struct {
	key    string
	system string
}

// dimensions returns the five review lenses in a stable order. The order does
// not create anchoring: each lens is an independent model call that never sees
// another lens's output (REVIEW.md §2).
func dimensions() []dimension {
	return []dimension{
		{
			key: "bugs",
			system: "You are a meticulous Go engineer reviewing a diff for logic errors. Your " +
				"default assumption is that the code is wrong and you are trying to find " +
				"the evidence. Focus on: incorrect error handling (swallowed errors, " +
				"missing checks), nil pointer dereferences, off-by-one errors, incorrect " +
				"use of goroutines and channels, and any code path that could panic or " +
				"silently corrupt state. Report every finding with: file, line range, " +
				"severity (blocking | advisory), and a one-sentence fix suggestion. " +
				"If you find no issues, say so explicitly — do not invent findings. " +
				"Keep your response under 800 tokens.",
		},
		{
			key: "security",
			system: "You are a paranoid security engineer. Assume the diff will be deployed to " +
				"production and that an attacker will read this review. Focus on: injection " +
				"vectors (command, SQL, template), path traversal, privilege escalation, " +
				"secret or credential exposure (including in logs), insecure defaults, and " +
				"anything that weakens an existing security boundary. For each finding: " +
				"state the attack scenario, the vulnerable line(s), the severity " +
				"(blocking | advisory), and the minimal fix. Your default is to flag and " +
				"escalate, not to approve quietly. Keep your response under 800 tokens.",
		},
		{
			key: "perf",
			system: "You are a performance-focused Go engineer. Focus on: algorithmic " +
				"regressions (e.g. O(n^2) in a previously O(n) path), unnecessary heap " +
				"allocations in hot paths, goroutine leaks (launched without a cancel path), " +
				"missing context propagation, and unbounded slices or maps. For each " +
				"finding: name the hot path, describe the regression, estimate the impact " +
				"(blocking | advisory), and suggest the fix. Keep your response under 800 tokens.",
		},
		{
			key: "style",
			system: "You are enforcing the project Go style conventions. Check: exported " +
				"symbols have doc comments; error strings are lowercase and do not end with " +
				"punctuation; packages follow standard naming; golangci-lint rules are " +
				"satisfied; and the diff does not introduce dead code or unused imports. " +
				"Advisory-only: you cannot block a merge on style alone, but you must list " +
				"all violations. Suggest the lint command or exact rewrite for each. " +
				"Keep your response under 800 tokens.",
		},
		{
			key: "regression",
			system: "You are checking whether this diff is consistent with the existing test " +
				"suite. Look for: tests deleted without replacement, behaviour changes with " +
				"no corresponding test update, and any changed exported symbol whose " +
				"callers in the test suite now receive different semantics. If a changed " +
				"function is untested, flag it. Report each gap with file, affected symbol, " +
				"and whether it is blocking (deleted test coverage) or advisory (missing " +
				"new coverage). Keep your response under 800 tokens.",
		},
	}
}

// dimensionReport holds one lens's rendered findings.
type dimensionReport struct {
	key  string
	text string
}

// callClaude issues a single Messages API request with adaptive thinking and
// the configured effort. Any error is returned to the caller so the run can
// fail closed (SPEC R5/R6); it never substitutes a placeholder success.
func callClaude(ctx context.Context, client anthropic.Client, model anthropic.Model, effort anthropic.OutputConfigEffort, system, user string) (string, error) {
	adaptive := anthropic.ThinkingConfigAdaptiveParam{}
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 2048,
		System:    []anthropic.TextBlockParam{{Text: system}},
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: effort,
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(t.Text)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("empty model response")
	}
	return out, nil
}

// runDimensions runs all five lenses concurrently. If any lens errors, the
// whole batch errors (errgroup first-error semantics) so the caller fails
// closed — a partial review is never treated as complete (SPEC R5, R12).
func runDimensions(ctx context.Context, client anthropic.Client, model anthropic.Model, effort anthropic.OutputConfigEffort, diff string) ([]dimensionReport, error) {
	dims := dimensions()
	reports := make([]dimensionReport, len(dims))
	g, ctx := errgroup.WithContext(ctx)
	user := "Review this PR diff and report your findings per your persona instructions:\n\n```diff\n" + diff + "\n```"
	for i, d := range dims {
		g.Go(func() error {
			text, err := callClaude(ctx, client, model, effort, d.system, user)
			if err != nil {
				return fmt.Errorf("dimension %s: %w", d.key, err)
			}
			reports[i] = dimensionReport{key: d.key, text: text}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return reports, nil
}

// synthesize collects the dimension reports and asks the model for a single
// outcome word. A synthesis error is returned (fail closed, SPEC R6); an
// unparseable word is handled by ParseOutcome (fail closed, SPEC R7). Returns
// the raw synthesis text (for the PR comment) alongside the parsed outcome.
func synthesize(ctx context.Context, client anthropic.Client, model anthropic.Model, effort anthropic.OutputConfigEffort, reports []dimensionReport) (Outcome, string, error) {
	var sb strings.Builder
	sb.WriteString("You are a senior engineer synthesizing five independent code review dimensions.\n")
	sb.WriteString("Based on the dimension reports below, determine the overall outcome per this taxonomy:\n")
	sb.WriteString("- approved: no blocking findings\n")
	sb.WriteString("- needs-work: fixable blocking findings exist\n")
	sb.WriteString("- rejected: fundamental design problem\n")
	sb.WriteString("- needs-human-review: security auto-fail trigger, escalation trigger, or novel irreversible decision\n\n")
	sb.WriteString("Rules: any security blocking finding -> needs-human-review. Any other blocking finding -> needs-work. ")
	sb.WriteString("Data-loss finding -> rejected. Ambiguous findings resolve down (needs-work beats approved; needs-human-review beats needs-work).\n")
	sb.WriteString("Escalate to needs-human-review regardless of severity when the diff touches any of: agent permissions; pre/post-tool hooks or hook registration; ")
	sb.WriteString("security boundaries (write guards, deny rules, CODEOWNERS, PII manifests); infrastructure that is expensive to reverse (IaC, schema changes, CI/CD pipeline edits); ")
	sb.WriteString("or an explicit HUMAN REVIEW REQUIRED marker.\n")
	sb.WriteString("FORMAT (strict): the FIRST LINE must contain exactly one outcome word (approved | needs-work | rejected | needs-human-review) and NOTHING ELSE — no prefix, no punctuation, no explanation. ")
	sb.WriteString("Begin the brief summary on the SECOND line. A first line that is not a bare outcome word is treated as a failure and blocks the merge.\n\n")
	for _, r := range reports {
		sb.WriteString(strings.ToUpper(r.key))
		sb.WriteString(":\n")
		sb.WriteString(r.text)
		sb.WriteString("\n\n")
	}

	const synthSystem = "You are a synthesis agent. The first line of your reply must be exactly one " +
		"outcome word (approved/needs-work/rejected/needs-human-review) and nothing else; " +
		"start the brief summary on the second line."

	text, err := callClaude(ctx, client, model, effort, synthSystem, sb.String())
	if err != nil {
		return NeedsHumanReview, "", err
	}
	firstLine, _, _ := strings.Cut(text, "\n")
	return ParseOutcome(firstLine), text, nil
}
