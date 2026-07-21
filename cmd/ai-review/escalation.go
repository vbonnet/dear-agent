package main

import (
	"fmt"
	"strings"
)

// humanReviewMarker is the explicit escalation trigger REVIEW.md §3 allows an
// author to place in a PR description or commit message.
const humanReviewMarker = "HUMAN REVIEW REQUIRED"

// escalationRule is one deterministic REVIEW.md §3 escalation trigger matched
// against a changed file path.
type escalationRule struct {
	// match reports whether the path trips this rule.
	match func(path string) bool
	// reason is the human-readable trigger name for the PR comment.
	reason string
}

// escalationRules encodes REVIEW.md §3. These are checked in code rather than
// left to the model: §3 says escalation is mandatory "regardless of finding
// severity", so it must not depend on a nondeterministic synthesis.
var escalationRules = []escalationRule{
	{
		reason: "CI/CD pipeline edit",
		match: func(p string) bool {
			return strings.HasPrefix(p, ".github/workflows/") ||
				strings.HasPrefix(p, ".github/rulesets/") ||
				strings.HasPrefix(p, ".github/actions/")
		},
	},
	{
		reason: "agent permissions or settings change",
		match: func(p string) bool {
			base := basename(p)
			return base == "settings.json" || base == "settings.local.json" ||
				strings.Contains(p, "/permissions/")
		},
	},
	{
		reason: "pre/post-tool hook change",
		match: func(p string) bool {
			base := basename(p)
			return strings.Contains(p, "/hooks/") ||
				strings.HasPrefix(base, "pretool-") ||
				strings.HasPrefix(base, "posttool-") ||
				strings.HasPrefix(base, "sessionstart-") ||
				strings.HasPrefix(base, "sessionend-")
		},
	},
	{
		reason: "security boundary change (write guard, deny rules, PII manifest)",
		match: func(p string) bool {
			lower := strings.ToLower(p)
			return strings.Contains(lower, "write-guard") ||
				strings.Contains(lower, "write_guard") ||
				strings.Contains(lower, "pii-manifest") ||
				strings.Contains(lower, "pii_manifest") ||
				strings.Contains(lower, "codeowners")
		},
	},
	{
		reason: "infrastructure that is expensive to reverse (IaC, launchd, systemd)",
		match: func(p string) bool {
			lower := strings.ToLower(p)
			return strings.HasSuffix(lower, ".tf") ||
				strings.HasSuffix(lower, ".plist") ||
				strings.HasSuffix(lower, ".service") ||
				strings.Contains(lower, "/launchd/") ||
				strings.Contains(lower, "/systemd/")
		},
	},
}

// basename returns the final path element.
func basename(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// EscalationTriggers returns the REVIEW.md §3 triggers fired by the changed
// paths and the PR/commit text. A non-empty result means the outcome must be
// forced to needs-human-review regardless of what the model concluded.
func EscalationTriggers(changedPaths []string, prBody, commitMessages string) []string {
	seen := map[string]bool{}
	var triggers []string

	for _, p := range changedPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		for _, rule := range escalationRules {
			if rule.match(p) && !seen[rule.reason] {
				seen[rule.reason] = true
				triggers = append(triggers, fmt.Sprintf("%s (%s)", rule.reason, p))
			}
		}
	}

	// Explicit author/committer escalation marker (REVIEW.md §3).
	if strings.Contains(strings.ToUpper(prBody), humanReviewMarker) ||
		strings.Contains(strings.ToUpper(commitMessages), humanReviewMarker) {
		triggers = append(triggers, "explicit "+humanReviewMarker+" marker")
	}

	return triggers
}

// ApplyEscalation forces needs-human-review when any REVIEW.md §3 trigger
// fired. Escalation only ever moves the outcome *down* (toward blocking); it
// never upgrades a blocking outcome to approved.
func ApplyEscalation(o Outcome, triggers []string) Outcome {
	if len(triggers) == 0 {
		return o
	}
	return NeedsHumanReview
}
