package main

import "testing"

func TestEscalationTriggers_Paths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{"workflow edit", []string{".github/workflows/review.yml"}, true},
		{"ruleset edit", []string{".github/rulesets/main.json"}, true},
		{"settings.json", []string{".claude/settings.json"}, true},
		{"hook script", []string{".config/claude-code/hooks/pretool-bash-write-guard"}, true},
		{"pretool prefix", []string{"cmd/pretool-fs-write-guard/main.go"}, true},
		// Hook *registration* files have no "/hooks/" segment, and hook
		// implementations are often named main.go under a -hooks/ owner dir.
		{"agents hooks.json", []string{".agents/hooks.json"}, true},
		{"codex hooks.json", []string{".codex/hooks.json"}, true},
		{"agm-hooks impl", []string{"agm/cmd/agm-hooks/pretool-bash-blocker/main.go"}, true},
		{"write guard", []string{"internal/writeguard/write-guard.go"}, true},
		// The package that owns the boundary must escalate even though its
		// filename contains none of the guard spellings.
		{"fsguard package", []string{"internal/fsguard/fsguard.go"}, true},
		{"safegit package", []string{"internal/safegit/merge.go"}, true},
		{"codeowners", []string{"CODEOWNERS"}, true},
		{"pii manifest", []string{".config/dear-agent/pii-manifest.yaml"}, true},
		{"terraform", []string{"infra/modules/managed-repo/main.tf"}, true},
		// REVIEW.md §3 lists database schema changes explicitly.
		{"sql migration", []string{"agm/internal/dolt/migrations/0007_add_col.sql"}, true},
		{"schema.sql", []string{"agm/internal/db/schema.sql"}, true},
		{"migrations dir go file", []string{"internal/store/migrations/002_users.go"}, true},
		{"launchd plist", []string{"deploy/launchd/com.example.plist"}, true},
		{"systemd service", []string{"agm/systemd/git-auto-sync.service"}, true},
		{"ordinary go file", []string{"pkg/foo/bar.go"}, false},
		{"ordinary doc", []string{"docs/README.md"}, false},
		{"empty", []string{}, false},
		{"blank entry", []string{""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscalationTriggers(tt.paths, "", "")
			if (len(got) > 0) != tt.want {
				t.Errorf("EscalationTriggers(%v) = %v, want triggered=%v", tt.paths, got, tt.want)
			}
		})
	}
}

func TestEscalationTriggers_ExplicitMarker(t *testing.T) {
	if got := EscalationTriggers(nil, "This is fine\nHUMAN REVIEW REQUIRED\n", ""); len(got) == 0 {
		t.Error("expected marker in PR body to trigger escalation")
	}
	if got := EscalationTriggers(nil, "", "feat: thing\n\nHUMAN REVIEW REQUIRED"); len(got) == 0 {
		t.Error("expected marker in commit message to trigger escalation")
	}
	if got := EscalationTriggers(nil, "nothing special", "ordinary commit"); len(got) != 0 {
		t.Errorf("unexpected escalation: %v", got)
	}
}

func TestApplyEscalation(t *testing.T) {
	// Escalation forces needs-human-review from any outcome...
	for _, o := range []Outcome{Approved, NeedsWork, Rejected, NeedsHumanReview} {
		if got := ApplyEscalation(o, []string{"CI/CD pipeline edit"}); got != NeedsHumanReview {
			t.Errorf("ApplyEscalation(%v, triggered) = %v, want NeedsHumanReview", o, got)
		}
	}
	// ...and is a no-op when nothing fired.
	for _, o := range []Outcome{Approved, NeedsWork, Rejected, NeedsHumanReview} {
		if got := ApplyEscalation(o, nil); got != o {
			t.Errorf("ApplyEscalation(%v, none) = %v, want unchanged", o, got)
		}
	}
}

// TestEscalationBlocksApprovedCIChange is the regression guard for the Codex
// P1 finding: a CI/CD edit must never merge on a model-produced "approved".
func TestEscalationBlocksApprovedCIChange(t *testing.T) {
	triggers := EscalationTriggers([]string{".github/workflows/review.yml"}, "", "")
	outcome := ApplyEscalation(Approved, triggers)
	if outcome != NeedsHumanReview {
		t.Fatalf("CI/CD edit synthesized as approved must escalate, got %v", outcome)
	}
	if ExitFor(outcome, false) != 1 {
		t.Fatal("escalated outcome must block the merge")
	}
}
