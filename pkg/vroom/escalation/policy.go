package escalation

import (
	"fmt"
	"regexp"
)

// ApprovalPolicy is the declarative auto-approve decision taxonomy: the answer
// to "what may an agent (or an intermediate supervisor) decide on its own, and
// what must reach a human?". It encodes the DEAR-retro principle (2026-06-12)
// that human review is the bottleneck — 90% of approvals are trivial and drown
// the 10% that are not. The posture is therefore AUTO-APPROVE by default; a
// human is required only for the named high-stakes categories: security
// boundaries (hooks, permissions, write-guards), money/spend, product
// decisions, and irreversible infrastructure — plus a few others a non-human
// node must never decide unilaterally (legal, people, external publishing).
//
// The policy is data, not code. It is the single source of truth shared by the
// VROOM escalation classifier (PolicyClassifier, which routes must-reach-human
// escalations) and the safe-merge P4 gate (loaded from .safe-merge.yml's
// approval_policy block). Keeping the boundary declarative makes it reviewable
// and tunable without a code change, while DefaultApprovalPolicy keeps the
// built-in baseline auditable and stable for CI.
type ApprovalPolicy struct {
	// AutoApproveDefault is the posture when no HumanRequired category matches.
	// True ⇒ the decision auto-approves (the DEAR-retro "AUTO-APPROVE by
	// default"). It is consulted by CompiledApprovalPolicy.AutoApproves, used by
	// the approval/merge surface; the escalation classifier is conservative for
	// genuine questions regardless (see classifyWith).
	AutoApproveDefault bool
	// HumanRequired is the ordered list of categories that force a human. First
	// match wins, so order the most-specific category first when patterns may
	// overlap.
	HumanRequired []HumanRequiredCategory
}

// HumanRequiredCategory is one bucket of the taxonomy that only a human may
// decide. Any question, decision, or blocked action whose text matches a
// Pattern is routed must-reach-human and attributed to this category.
type HumanRequiredCategory struct {
	// Name is the coarse topic recorded for audit and frequency analysis, e.g.
	// "security-boundary", "spend", "product", "irreversible-infra".
	Name string
	// Reason is the human-readable explanation recorded on the verdict. When
	// empty, a default is synthesised from Name at compile time.
	Reason string
	// Patterns are case-insensitive regular-expression sources; a match against
	// any one selects this category. At least one is required.
	Patterns []string
}

// DefaultApprovalPolicy returns the built-in taxonomy used when .safe-merge.yml
// carries no approval_policy override. It is the canonical baseline: the
// hardcoded high-stakes markers the classifier shipped with, re-expressed as the
// four DEAR-retro buckets (security-boundary, spend, product, irreversible-infra)
// plus the remaining must-reach-human categories. AutoApproveDefault is true —
// everything not in a category auto-approves.
func DefaultApprovalPolicy() ApprovalPolicy {
	return ApprovalPolicy{
		AutoApproveDefault: true,
		HumanRequired: []HumanRequiredCategory{
			{
				Name:   "security-boundary",
				Reason: "touches a security boundary (hooks, permissions, write-guards, auth); only the human may decide this",
				Patterns: []string{
					`(?i)\b(add|change|modify|edit|remove|delete|disable|bypass|skip|weaken)\b.{0,30}\b(hook|hooks|pretool|posttool)\b`,
					`(?i)\b(add|change|modify|edit|remove|grant|widen|loosen|relax|disable|bypass)\b.{0,30}\b(permission|permissions|allow.?list|deny.?list|allow rule|deny rule|allow\b|deny\b)`,
					`(?i)\bwrite.?guard\b`,
					`(?i)\b(settings\.json|\.claude/settings|claude code settings)\b`,
					`(?i)\b(security policy|disable (the )?(guard|gate|auth|2fa|mfa)|rotate.{0,15}key|exfiltrat)`,
				},
			},
			{
				Name:     "spend",
				Reason:   "commits money or budget; only the human may decide this",
				Patterns: []string{`(?i)\b(spend|budget|purchase|pay|invoice|\$\d)`},
			},
			{
				Name:   "product",
				Reason: "is a product, pricing, or strategy decision; only the human may decide this",
				Patterns: []string{
					`(?i)\bproduct (decision|direction|strategy)\b`,
					`(?i)\b(pricing|price|paywall|monetiz)`,
					`(?i)\b(roadmap|prioriti[sz]e|deprioriti[sz]e|strategy|strategic)\b`,
				},
			},
			{
				Name:   "irreversible-infra",
				Reason: "is irreversible or destroys infrastructure/data; only the human may decide this",
				Patterns: []string{
					`(?i)\b(delete|drop|wipe|destroy|purge)\b.{0,20}\b(prod|production|database|customer|user data)\b`,
					`(?i)\b(irreversible|cannot be undone|permanent(ly)? (delete|remove))\b`,
				},
			},
			{
				Name:   "publishing",
				Reason: "publishes or communicates externally; only the human may decide this",
				Patterns: []string{
					`(?i)\b(publish|release publicly|go public|announce|tweet|post to)\b`,
					`(?i)\b(email|message|contact|reach out to) (the )?(customer|client|user|press|investor)`,
				},
			},
			{
				Name:     "legal",
				Reason:   "has legal or compliance implications; only the human may decide this",
				Patterns: []string{`(?i)\b(legal|compliance|gdpr|license|licensing|contract|terms of service)\b`},
			},
			{
				Name:     "people",
				Reason:   "is a people or headcount decision; only the human may decide this",
				Patterns: []string{`(?i)\b(hire|fire|headcount|terminate (an?|the) (employee|contractor))\b`},
			},
		},
	}
}

// CompiledApprovalPolicy is an ApprovalPolicy with its patterns compiled once,
// ready to match. Build it with ApprovalPolicy.Compile.
type CompiledApprovalPolicy struct {
	autoApproveDefault bool
	cats               []compiledCategory
}

type compiledCategory struct {
	name   string
	reason string
	pats   []*regexp.Regexp
}

// Compile validates the policy and compiles its patterns. Every category must
// have a name and at least one pattern, and every pattern must be a valid
// regexp; otherwise Compile returns an error naming the offending entry. An
// empty Reason is filled from the category name.
func (p ApprovalPolicy) Compile() (*CompiledApprovalPolicy, error) {
	cp := &CompiledApprovalPolicy{autoApproveDefault: p.AutoApproveDefault}
	for i, c := range p.HumanRequired {
		if c.Name == "" {
			return nil, fmt.Errorf("approval_policy.human_required[%d] has no name", i)
		}
		if len(c.Patterns) == 0 {
			return nil, fmt.Errorf("approval_policy.human_required[%q] has no patterns", c.Name)
		}
		cc := compiledCategory{name: c.Name, reason: c.Reason}
		if cc.reason == "" {
			cc.reason = "matched high-stakes category (" + c.Name + "); only the human may decide this"
		}
		for j, src := range c.Patterns {
			re, err := regexp.Compile(src)
			if err != nil {
				return nil, fmt.Errorf("approval_policy.human_required[%q].patterns[%d]: %w", c.Name, j, err)
			}
			cc.pats = append(cc.pats, re)
		}
		cp.cats = append(cp.cats, cc)
	}
	return cp, nil
}

// RequiresHuman reports whether text falls into a human-required category,
// returning the matching category name and reason. It is the shared primitive
// behind the escalation classifier's must-reach-human check and the safe-merge
// P4 approval gate. First match wins.
func (cp *CompiledApprovalPolicy) RequiresHuman(text string) (category, reason string, required bool) {
	for _, c := range cp.cats {
		for _, re := range c.pats {
			if re.MatchString(text) {
				return c.name, c.reason, true
			}
		}
	}
	return "", "", false
}

// AutoApproves reports whether text may be auto-approved: it matches no
// human-required category and the policy's default posture is to auto-approve.
// This is the approval/merge surface's reading of the taxonomy.
func (cp *CompiledApprovalPolicy) AutoApproves(text string) bool {
	if _, _, required := cp.RequiresHuman(text); required {
		return false
	}
	return cp.autoApproveDefault
}

// defaultCompiledPolicy is the built-in taxonomy compiled once at init. It backs
// DefaultClassifier so the common path allocates no regexps per call.
var defaultCompiledPolicy = mustCompilePolicy(DefaultApprovalPolicy())

func mustCompilePolicy(p ApprovalPolicy) *CompiledApprovalPolicy {
	cp, err := p.Compile()
	if err != nil {
		panic("escalation: built-in DefaultApprovalPolicy failed to compile: " + err.Error())
	}
	return cp
}
