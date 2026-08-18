package mergeloop

import "testing"

// Every carve-out category named by docs/policies/autonomous-merge.ai.md must
// actually block. A category declared in the policy but missing from
// DefaultSensitiveGlobs is not enforced at all: Classify returns StateGreen and
// hands the PR to safe-merge, so the human-only boundary exists only on paper.
func TestClassify_PolicyCarveOutsAreEnforced(t *testing.T) {
	tests := []struct {
		category string
		path     string
	}{
		{"money — billing", "internal/billing/invoice.go"},
		{"money — payments", "pkg/payments/charge.go"},
		{"money — quota", "pkg/llm/quota/spawngate.go"},

		{"security — auth", "internal/auth/token.go"},
		{"security — secrets", "internal/secrets/store.go"},

		{"infrastructure — terraform", "infra/main.tf"},
		{"infrastructure — rulesets", ".github/rulesets/main.json"},

		{"governance — root agent contract", "AGENTS.md"},
		{"governance — nested agent contract", "agm/AGENTS.md"},
		{"governance — claude contract", "CLAUDE.md"},
		{"governance — policy documents", "docs/policies/autonomous-merge.ai.md"},

		{"control surface — merge classification", "internal/mergeloop/classify.go"},
		{"control surface — git guard", "internal/safegit/push.go"},
		{"control surface — filesystem guard", "internal/fsguard/bash.go"},
		{"control surface — safe-merge", "cmd/safe-merge/main.go"},
		{"control surface — safe-pr", "cmd/safe-pr/main.go"},
		{"control surface — safe-push", "cmd/safe-push/main.go"},
		{"control surface — notifications", "agm/internal/notification/router.go"},
	}

	policy := NewPolicy()
	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			pr := PR{
				Number:           1,
				Mergeable:        "MERGEABLE",
				MergeStateStatus: "CLEAN",
				ChangedFiles:     []string{tt.path},
			}
			got := policy.Classify(pr, 0, false)
			if got.State != StateBlockedPolicy {
				t.Errorf("Classify(%s) = %v, want StateBlockedPolicy — this carve-out is declared in the policy but not enforced", tt.path, got.State)
			}
		})
	}
}

// The carve-outs must not swallow ordinary work, or agents lose the autonomy
// the policy is explicitly granting them.
func TestClassify_RoutinePathsAreNotCarvedOut(t *testing.T) {
	routine := []string{
		"README.md",
		"docs/adr/ADR-034-squash-only-merge-contract.md",
		"internal/speccoverage/speccoverage.go",
		"cmd/cleanup-worktrees/main.go",
		"agm/internal/tmux/tmux.go",
		"pkg/cihealth/escape.go",
	}

	policy := NewPolicy()
	for _, path := range routine {
		t.Run(path, func(t *testing.T) {
			pr := PR{
				Number:           2,
				Mergeable:        "MERGEABLE",
				MergeStateStatus: "CLEAN",
				ChangedFiles:     []string{path},
			}
			if got := policy.Classify(pr, 0, false); got.State == StateBlockedPolicy {
				t.Errorf("Classify(%s) = StateBlockedPolicy (%s) — routine work must stay agent-mergeable", path, got.Reason)
			}
		})
	}
}
