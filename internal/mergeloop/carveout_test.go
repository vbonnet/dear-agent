package mergeloop

import (
	"fmt"
	"strings"
	"testing"
)

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
		{"money — budget", "agm/internal/budget/tracker.go"},
		{"money — quota parity", "agm/internal/quotaparity/compare.go"},
		{"money — rate limiter", "agm/internal/gateway/ratelimiter.go"},
		{"money — budget breaker", "pkg/llm/provider/budget_breaker.go"},

		{"security — auth", "internal/auth/token.go"},
		{"security — secrets", "internal/secrets/store.go"},

		{"infrastructure — terraform", "infra/main.tf"},
		{"infrastructure — rulesets", ".github/rulesets/main.json"},

		{"governance — root agent contract", "AGENTS.md"},
		{"governance — nested agent contract", "agm/AGENTS.md"},
		{"governance — claude contract", "CLAUDE.md"},
		{"governance — codex contract", "CODEX.md"},
		{"governance — gemini contract", "GEMINI.md"},
		{"governance — harness config", ".claude/settings.json"},
		{"governance — agent hooks", ".agents/hooks.json"},
		{"governance — policy documents", "docs/policies/autonomous-merge.ai.md"},

		{"control surface — merge classification", "internal/mergeloop/classify.go"},
		{"control surface — git guard", "internal/safegit/push.go"},
		{"control surface — filesystem guard", "internal/fsguard/bash.go"},
		{"control surface — safe-merge", "cmd/safe-merge/main.go"},
		{"control surface — safe-pr", "cmd/safe-pr/main.go"},
		{"control surface — safe-push", "cmd/safe-push/main.go"},
		{"control surface — alert router", "agm/internal/ops/alert_router.go"},
		{"control surface — completion watcher", "agm/internal/ops/completion_watcher.go"},
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

// gh emits `files(first: 100)` for a pull request's file connection and never
// paginates it, so a governance or control-surface path beyond the cap is
// simply absent from ChangedFiles. Matching globs against a partial list would
// clear a PR nobody fully inspected, so classification fails closed instead.
func TestClassify_TruncatedChangedFilesFailsClosed(t *testing.T) {
	policy := NewPolicy()
	pr := PR{
		Number:                1,
		Mergeable:             "MERGEABLE",
		MergeStateStatus:      "CLEAN",
		ChangedFiles:          []string{"README.md"},
		ChangedFilesTruncated: true,
	}
	got := policy.Classify(pr, 0, false)
	if got.State != StateBlockedPolicy {
		t.Fatalf("Classify() = %v, want StateBlockedPolicy for a truncated file list", got.State)
	}
	if !strings.Contains(got.Reason, "truncated") {
		t.Errorf("Reason = %q, want it to name truncation as the cause", got.Reason)
	}
}

// A complete list of routine files must still classify green, or every large
// ordinary PR becomes human-only.
func TestClassify_CompleteRoutineFileListStaysGreen(t *testing.T) {
	policy := NewPolicy()
	files := make([]string, 0, 120)
	for i := range 120 {
		files = append(files, fmt.Sprintf("internal/thing/file%d.go", i))
	}
	pr := PR{
		Number:           2,
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "CLEAN",
		ChangedFiles:     files,
	}
	if got := policy.Classify(pr, 0, false); got.State == StateBlockedPolicy {
		t.Errorf("Classify() = StateBlockedPolicy (%s), want a complete routine list to stay mergeable", got.Reason)
	}
}
