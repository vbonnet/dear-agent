package main

import (
	"fmt"
	"slices"
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
	// exempt reports whether a path that matches this rule is deliberately
	// routed elsewhere. Exemptions are rule-local so one carveout cannot erase
	// an independent permission, migration, or infrastructure trigger.
	exempt func(path string) bool
	// reason is the human-readable trigger name for the PR comment.
	reason string
}

// escalationRules encodes REVIEW.md §3. These are checked in code rather than
// left to the model: §3 says escalation is mandatory "regardless of finding
// severity", so it must not depend on a nondeterministic synthesis.
var escalationRules = []escalationRule{
	{
		reason: "protected review policy, enforcement, or provider rules change",
		match: func(p string) bool {
			// Git tree paths are case-sensitive, but supported case-insensitive
			// checkouts can make an added case variant shadow the canonical
			// trust-root bytes. Match case-fold aliases as protected too.
			return normalizedPathRelated(p, "REVIEW.md") ||
				normalizedPathRelated(p, ".github/workflows/review.yml") ||
				normalizedPathRelated(p, ".github/rulesets")
		},
	},
	{
		reason: "agent permissions or authorization change",
		match: func(p string) bool {
			lower := normalizedPathIdentity(p)
			base := basename(lower)
			// Matched by concept rather than by an enumerated path list: any
			// permission/authorization surface is a §3 trigger, and an
			// allowlist of specific packages is structurally incomplete (a new
			// owner can always be added). Over-escalation here is the safe
			// direction — REVIEW.md §3 calls escalation "a correct outcome".
			return normalizedPathRelated(p, "agm/internal/rbac") ||
				base == "settings.json" || base == "settings.local.json" ||
				strings.Contains(lower, "permission") ||
				strings.Contains(lower, "authorization") ||
				strings.Contains(lower, "authz")
		},
	},
	{
		reason: "pre/post-tool hook change or hook registration",
		exempt: isCanonicalHookOwnerSpec,
		match: func(p string) bool {
			lower := normalizedPathIdentity(p)
			base := basename(lower)
			if normalizedPathRelated(p, ".codex/config.toml") {
				return true
			}
			// Registration files (hooks.json) and the packages that own hook
			// implementations must match too — a hooks.json has no "/hooks/"
			// segment, and a hook implementation's basename is often main.go.
			// Scoped to the directories that own *tool* hooks and to hook
			// registration files. A bare "/hooks/" substring also matches
			// unrelated application packages (e.g. engram/hooks/), which would
			// force needless human review on routine maintenance.
			for _, owner := range toolHookOwners {
				if normalizedPathRelated(p, owner.path) {
					return true
				}
			}
			if pathHasComponentSuffix(lower, "-hooks") {
				return true
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
			lower := normalizedPathIdentity(p)
			// Match the packages that *own* the boundary, not just filename
			// spellings — internal/fsguard owns the pre-tool write guards and
			// ~/src enforcement, and its path contains none of the spellings.
			for _, owner := range securityBoundaryDirs {
				if normalizedPathRelated(p, owner) {
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
			lower := normalizedPathIdentity(p)
			base := basename(lower)
			return normalizedPathRelated(p, "agm/internal/dolt") ||
				strings.HasSuffix(lower, ".sql") ||
				pathHasComponent(lower, "migrations") ||
				pathHasComponent(lower, "migration") ||
				base == "schema.sql" || base == "schema.go" ||
				strings.HasSuffix(lower, "_schema.sql")
		},
	},
	{
		reason: "infrastructure that is expensive to reverse (IaC, launchd, systemd)",
		match: func(p string) bool {
			lower := normalizedPathIdentity(p)
			return strings.HasSuffix(lower, ".tf") ||
				strings.HasSuffix(lower, ".plist") ||
				strings.HasSuffix(lower, ".service") ||
				pathHasComponent(lower, "launchd") ||
				pathHasComponent(lower, "systemd")
		},
	},
}

type toolHookOwner struct {
	path      string
	goPackage bool
}

// toolHookOwners are the directories that own pre/post-tool hooks — the
// surface REVIEW.md §3 means by "pre/post-tool hooks". This is deliberately
// narrower than any directory named "hooks". goPackage is the single source
// of truth for the exact _test.go carveout; script-only roots remain false.
var toolHookOwners = []toolHookOwner{
	{path: ".claude/hooks"},
	{path: ".config/claude-code/hooks"},
	{path: ".config/git/hooks"},
	{path: ".agents/hooks"},
	{path: ".codex/hooks"},
	{path: ".opencode/hooks"},
	{path: ".pi/guardrails"},
	{path: "agm/.githooks"},
	{path: "agm/hooks"},
	{path: "agm/hooks/cmd/posttool-cost-guard", goPackage: true},
	{path: "agm/hooks/cmd/sessionstart-chezmoi-drift", goPackage: true},
	{path: "agm/hooks/cmd/stop-session-guard", goPackage: true},
	{path: "agm/cmd/agm/hooks"},
	{path: "agm/internal/hooks", goPackage: true},
	{path: "agm/internal/codexhooks", goPackage: true},
}

// securityBoundaryDirs are the packages that own a security boundary. A
// production or governance change anywhere inside one escalates, regardless
// of the individual filename. EscalationTriggers separately keeps Go test-only
// changes under cmd/ai-review in the automated review loop.
var securityBoundaryDirs = []string{
	"internal/fsguard",
	"internal/safegit",
	"internal/writeguard",
	"pkg/fsguard",
	"cmd/ai-review",
}

func isAIReviewGoTestPath(path string) bool {
	lower := normalizedPathIdentity(path)
	// Go excludes only the exact lowercase ASCII _test.go suffix from a normal
	// package build. Fold the owner directory for checkout-alias safety, but
	// never fold the suffix: *_TEST.go and *_teſt.go can be production inputs.
	return strings.HasPrefix(lower, "cmd/ai-review/") && strings.HasSuffix(path, "_test.go")
}

func isCanonicalHookOwnerSpec(path string) bool {
	// The raw spellings are intentional. Only a canonical SPEC.md under a
	// canonical protected hook root is contract evidence rather than hook
	// implementation authority. Case/normalization aliases stay protected.
	if basename(path) != "SPEC.md" {
		return false
	}
	for _, owner := range toolHookOwners {
		if strings.HasPrefix(path, owner.path+"/") {
			return true
		}
	}
	return false
}

func hookGoPackageOwner(path string) (string, bool) {
	for _, owner := range toolHookOwners {
		if owner.goPackage && strings.HasPrefix(path, owner.path+"/") {
			return owner.path, true
		}
	}
	return "", false
}

func isHookGoTestPath(path string) bool {
	// Go excludes only the exact lowercase ASCII _test.go suffix. Keep this
	// carveout limited to known Go package owners, never arbitrary script roots.
	if !strings.HasSuffix(path, "_test.go") {
		return false
	}
	_, ok := hookGoPackageOwner(path)
	return ok
}

func isHookOwnerAutomatedPath(path string) bool {
	return isCanonicalHookOwnerSpec(path) || isHookGoTestPath(path)
}

// basename returns the final path element.
func basename(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

func pathHasComponent(path, component string) bool {
	return slices.Contains(strings.Split(path, "/"), component)
}

func pathHasComponentSuffix(path, suffix string) bool {
	for part := range strings.SplitSeq(path, "/") {
		if strings.HasSuffix(part, suffix) {
			return true
		}
	}
	return false
}

// BinaryEscalationTriggers returns a trigger per binary path. A binary change
// cannot be reviewed from a text diff — git renders it as a bare "Binary files
// differ" marker — so the five dimensions never see the payload. Rather than
// let an unreviewed executable or asset ride an "approved" outcome, escalate.
func BinaryEscalationTriggers(binaryPaths []string) []string {
	var triggers []string
	for _, p := range binaryPaths {
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
		if p == "" {
			continue
		}
		// Go test files cannot enter the production reviewer binary. Keep a
		// pure test-hardening change autonomous while --no-renames ensures that
		// either production side of a boundary-crossing rename is still seen.
		if isAIReviewGoTestPath(p) || isHookGoTestPath(p) {
			continue
		}
		for _, rule := range escalationRules {
			if rule.exempt != nil && rule.exempt(p) {
				continue
			}
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
