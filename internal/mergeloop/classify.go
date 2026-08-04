package mergeloop

import "strings"

// State is the classification of a PR within one tick of the merge loop.
type State string

const (
	// StateDraft — not ready; skipped.
	StateDraft State = "DRAFT"
	// StateAbandoned — explicitly abandoned, or exhausted its agent-fix
	// attempts against the same failure; skipped permanently and escalated
	// with a concrete diagnosis.
	StateAbandoned State = "ABANDONED"
	// StateBlockedPolicy — a genuine human-only block (security/product/money
	// label, sensitive changed paths, or a human "changes requested"). The
	// ONLY legitimate human escalation. The loop never touches its mechanics.
	StateBlockedPolicy State = "BLOCKED_POLICY"
	// StateAgentInFlight — an agent session is already working this PR; the
	// loop skips it to avoid double-spawning.
	StateAgentInFlight State = "AGENT_IN_FLIGHT"
	// StateConflicted — merge conflicts; an agent resolves them in a worktree.
	StateConflicted State = "CONFLICTED"
	// StateBehind — base out of date; rebase mechanically. Checked before
	// CI_FAILING because a rebase re-runs CI, so the current verdict is stale.
	StateBehind State = "BEHIND"
	// StateCIFailing — a required check failed on an up-to-date head; an agent
	// diagnoses and fixes it.
	StateCIFailing State = "CI_FAILING"
	// StateCIPending — required checks still running; wait (waiting is not
	// stalling). The loop's persistence retries next tick.
	StateCIPending State = "CI_PENDING"
	// StateGreen — mergeable and all required checks pass; squash-merge via
	// safe-merge (which independently enforces soak + bot-review gates).
	StateGreen State = "GREEN"
)

// DefaultBlockLabels mark a PR as a genuine human-only policy block.
var DefaultBlockLabels = []string{
	"needs-security-review",
	"needs-product-decision",
	"needs-human",
	"do-not-merge",
	"blocked",
}

// DefaultAbandonLabels mark a PR as abandoned.
var DefaultAbandonLabels = []string{"abandoned", "wontfix"}

// DefaultSensitiveGlobs are changed-path patterns that force human review even
// without a label — money, auth, secrets, and infra surfaces. Patterns support
// "*" (within a segment) and "**" (any number of segments).
var DefaultSensitiveGlobs = []string{
	"**/billing/**",
	"**/payment/**",
	"**/payments/**",
	"**/auth/**",
	"**/secrets/**",
	"**/secret/**",
	"infra/**",
	"**/*.tf",
}

// DefaultMaxAgentAttempts caps agent code-fix attempts per PR against the same
// failure signature before abandonment (AGENTS.md principle 3: two-retry max).
const DefaultMaxAgentAttempts = 2

// Policy holds the classification thresholds. The zero value is unusable;
// call [NewPolicy] for sane defaults.
type Policy struct {
	BlockLabels      []string
	AbandonLabels    []string
	SensitiveGlobs   []string
	MaxAgentAttempts int
}

// NewPolicy returns a Policy seeded with the package defaults.
func NewPolicy() Policy {
	return Policy{
		BlockLabels:      append([]string(nil), DefaultBlockLabels...),
		AbandonLabels:    append([]string(nil), DefaultAbandonLabels...),
		SensitiveGlobs:   append([]string(nil), DefaultSensitiveGlobs...),
		MaxAgentAttempts: DefaultMaxAgentAttempts,
	}
}

// Classification is the result of inspecting a single PR.
type Classification struct {
	State  State
	Reason string // human-readable; surfaced in audit, comments, and escalations
}

// Classify maps a PR to its state. attempts is the PR's recorded agent-fix
// attempt count; agentActive reports whether an agent session is already
// driving this PR. The order of checks is the priority order documented in
// ADR-029.
func (p Policy) Classify(pr PR, attempts int, agentActive bool) Classification {
	if pr.IsDraft || pr.MergeStateStatus == "DRAFT" {
		return Classification{StateDraft, "draft PR"}
	}

	// Abandonment by label.
	for _, l := range p.AbandonLabels {
		if pr.hasLabel(l) {
			return Classification{StateAbandoned, "labeled " + l}
		}
	}

	// Genuine human-only policy blocks — the only legitimate escalation.
	if cls, blocked := p.policyBlock(pr); blocked {
		return cls
	}

	// An agent is already working this PR: don't double-spawn.
	if agentActive {
		return Classification{StateAgentInFlight, "agent session already active"}
	}

	// Conflicts need code edits.
	if pr.Mergeable == "CONFLICTING" || pr.MergeStateStatus == "DIRTY" {
		return Classification{StateConflicted, "merge conflicts need resolution"}
	}

	// Behind base: rebase first. The current CI verdict is stale because a
	// rebase re-runs CI, so we must not spawn an agent off a stale red here.
	if pr.MergeStateStatus == "BEHIND" {
		return Classification{StateBehind, "behind base; rebase (re-runs CI)"}
	}

	if pr.CheckProjectionError != "" {
		if isPermanentProjectionError(pr.CheckProjectionError) {
			return Classification{StateBlockedPolicy, "required-check projection blocked: " + pr.CheckProjectionError}
		}
		return Classification{StateCIPending, "required-check projection unavailable: " + pr.CheckProjectionError}
	}

	// CI verdict on an up-to-date head.
	switch pr.requiredVerdict() {
	case CheckFail:
		// Exhausted agent attempts → abandon with a concrete diagnosis.
		if attempts >= p.maxAttempts() {
			return Classification{StateAbandoned,
				"required CI failing after " + itoa(attempts) +
					" agent fix attempt(s); needs human"}
		}
		return Classification{StateCIFailing, "required CI failing; agent fix"}
	case CheckPending:
		return Classification{StateCIPending, "required CI still running"}
	case CheckPass:
		// all required checks pass — fall through to the green/merge logic
	}

	// All required checks pass. If GitHub still says non-mergeable for a
	// reason we didn't catch (UNKNOWN mergeability), treat as pending rather
	// than forcing a merge.
	if pr.Mergeable == "UNKNOWN" {
		return Classification{StateCIPending, "mergeability still computing"}
	}
	return Classification{StateGreen, "all required checks pass; merge"}
}

// policyBlock reports a human-only block from labels, sensitive changed paths,
// or a human changes-requested review.
func (p Policy) policyBlock(pr PR) (Classification, bool) {
	for _, l := range p.BlockLabels {
		if pr.hasLabel(l) {
			return Classification{StateBlockedPolicy, "labeled " + l + " (human review required)"}, true
		}
	}
	for _, f := range pr.ChangedFiles {
		for _, g := range p.SensitiveGlobs {
			if matchGlob(g, f) {
				return Classification{StateBlockedPolicy,
					"touches sensitive path " + f + " (matches " + g + ")"}, true
			}
		}
	}
	if pr.ReviewDecision == "CHANGES_REQUESTED" {
		return Classification{StateBlockedPolicy, "human reviewer requested changes"}, true
	}
	return Classification{}, false
}

func (p Policy) maxAttempts() int {
	if p.MaxAgentAttempts <= 0 {
		return DefaultMaxAgentAttempts
	}
	return p.MaxAgentAttempts
}

// matchGlob reports whether path matches a glob supporting "*" (any run of
// non-"/" characters) and "**" (any number of path segments, including zero).
func matchGlob(pattern, path string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func matchSegments(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			// Collapse consecutive "**".
			for len(pat) > 1 && pat[1] == "**" {
				pat = pat[1:]
			}
			if len(pat) == 1 {
				return true // trailing "**" matches everything remaining
			}
			// Try to match the remainder of the pattern at each position.
			for i := 0; i <= len(name); i++ {
				if matchSegments(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if !matchSegment(pat[0], name[0]) {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// matchSegment matches a single path segment against a pattern that may contain
// "*" wildcards (each matching any run of characters within the segment).
func matchSegment(pat, seg string) bool {
	parts := strings.Split(pat, "*")
	if len(parts) == 1 {
		return pat == seg // no wildcard
	}
	// Must start with parts[0] and end with the last part, with the middle
	// parts appearing in order.
	if !strings.HasPrefix(seg, parts[0]) {
		return false
	}
	seg = seg[len(parts[0]):]
	last := parts[len(parts)-1]
	mid := parts[1 : len(parts)-1]
	for _, m := range mid {
		idx := strings.Index(seg, m)
		if idx < 0 {
			return false
		}
		seg = seg[idx+len(m):]
	}
	return strings.HasSuffix(seg, last)
}

// itoa is a tiny non-negative int formatter to keep classify.go's surface tight.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func isPermanentProjectionError(errStr string) bool {
	lower := strings.ToLower(errStr)
	return strings.Contains(lower, "required workflow") ||
		strings.Contains(lower, "unsupported") ||
		strings.Contains(lower, "policy") ||
		strings.Contains(lower, "disagreement") ||
		strings.Contains(lower, "ambiguous") ||
		strings.Contains(lower, "incomplete") ||
		strings.Contains(lower, "ruleset") ||
		strings.Contains(lower, "reconciling") ||
		strings.Contains(lower, "normalizing")
}
