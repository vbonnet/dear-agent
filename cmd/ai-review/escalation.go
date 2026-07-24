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
		reason: "agent permissions or authorization change",
		match: func(p string) bool {
			lower := strings.ToLower(p)
			base := basename(lower)
			// Matched by concept rather than by an enumerated path list: any
			// permission/authorization surface is a §3 trigger, and an
			// allowlist of specific packages is structurally incomplete (a new
			// owner can always be added). Over-escalation here is the safe
			// direction — REVIEW.md §3 calls escalation "a correct outcome".
			return strings.HasPrefix(lower, "agm/internal/rbac/") ||
				base == "settings.json" || base == "settings.local.json" ||
				strings.Contains(lower, "permission") ||
				strings.Contains(lower, "authorization") ||
				strings.Contains(lower, "authz")
		},
	},
	{
		reason: "pre/post-tool hook change or hook registration",
		match: func(p string) bool {
			lower := strings.ToLower(p)
			base := basename(lower)
			// Registration files (hooks.json) and the packages that own hook
			// implementations must match too — a hooks.json has no "/hooks/"
			// segment, and a hook implementation's basename is often main.go.
			// Scoped to the directories that own *tool* hooks and to hook
			// registration files. A bare "/hooks/" substring also matches
			// unrelated application packages (e.g. engram/hooks/), which would
			// force needless human review on routine maintenance.
			for _, owner := range toolHookDirs {
				if strings.Contains(lower, owner) {
					return true
				}
			}
			// Registration/installer surfaces own hook wiring even when they
			// live outside a hooks directory (e.g. agm/cmd/agm/install_hooks.go
			// writes hook registrations into ~/.claude/settings.json).
			return base == "hooks.json" || base == "hooks.yaml" || base == "hooks.yml" ||
				strings.Contains(base, "install_hooks") ||
				strings.Contains(base, "install-hooks") ||
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
			// Match the packages that *own* the boundary, not just filename
			// spellings — internal/fsguard owns the pre-tool write guards and
			// ~/src enforcement, and its path contains none of the spellings.
			for _, owner := range securityBoundaryDirs {
				if strings.Contains(lower, owner) {
					return true
				}
			}
			return strings.Contains(lower, "write-guard") ||
				strings.Contains(lower, "write_guard") ||
				strings.Contains(lower, "writeguard") ||
				strings.Contains(lower, "pii-manifest") ||
				strings.Contains(lower, "pii_manifest") ||
				strings.Contains(lower, "codeowners")
		},
	},
	{
		reason: "database schema or migration change",
		match: func(p string) bool {
			lower := strings.ToLower(p)
			base := basename(lower)
			return strings.HasPrefix(lower, "agm/internal/dolt/") ||
				strings.HasSuffix(lower, ".sql") ||
				strings.Contains(lower, "/migrations/") ||
				strings.Contains(lower, "/migration/") ||
				base == "schema.sql" || base == "schema.go" ||
				strings.HasSuffix(lower, "_schema.sql")
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

// toolHookDirs are the directories that own pre/post-tool hooks — the surface
// REVIEW.md §3 means by "pre-tool hooks". Deliberately narrower than any
// directory named "hooks".
var toolHookDirs = []string{
	".claude/hooks/",
	".config/claude-code/hooks/",
	".config/git/hooks/",
	".agents/hooks",
	".codex/hooks",
	"agm/hooks/",
	"-hooks/",
}

// securityBoundaryDirs are the packages that own a security boundary. A change
// anywhere inside one escalates, regardless of the individual filename.
var securityBoundaryDirs = []string{
	"internal/fsguard/",
	"internal/safegit/",
	"internal/writeguard/",
	"pkg/fsguard/",
	"cmd/ai-review/",
}

// basename returns the final path element.
func basename(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// BinaryEscalationTriggers returns a trigger per binary path. A binary change
// cannot be reviewed from a text diff — git renders it as a bare "Binary files
// differ" marker — so the five dimensions never see the payload. Rather than
// let an unreviewed executable or asset ride an "approved" outcome, escalate.
func BinaryEscalationTriggers(binaryPaths []string) []string {
	var triggers []string
	for _, p := range binaryPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		triggers = append(triggers, fmt.Sprintf("binary file not reviewable from a text diff (%s)", p))
	}
	return triggers
}

// GitlinkEscalationTriggers returns a trigger per changed submodule. A gitlink
// bump shows up as a lone "Subproject commit <sha>" line, so the external tree
// it now points at is never reviewed — escalate rather than approve an
// unreviewed dependency.
func GitlinkEscalationTriggers(gitlinkPaths []string) []string {
	var triggers []string
	for _, p := range gitlinkPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		triggers = append(triggers, fmt.Sprintf("submodule (gitlink) change whose target tree is not in the diff (%s)", p))
	}
	return triggers
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
