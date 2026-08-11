package main

import "testing"

func TestEscalationTriggers_Paths(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{"ordinary workflow", []string{".github/workflows/structural-health.yml"}, false},
		{"ordinary custom action", []string{".github/actions/setup-go/action.yml"}, false},
		{"authoritative review policy", []string{"REVIEW.md"}, true},
		{"mixed-case non-authoritative review policy", []string{"Review.md"}, false},
		{"trusted review workflow", []string{".github/workflows/review.yml"}, true},
		{"mixed-case non-authoritative review workflow", []string{".GitHub/Workflows/Review.yml"}, false},
		{"review gate implementation", []string{"cmd/ai-review/escalation.go"}, true},
		{"ruleset edit", []string{".github/rulesets/main.json"}, true},
		{"mixed-case non-authoritative ruleset", []string{".GitHub/Rulesets/Main.json"}, false},
		{"case-only rename retains canonical deletion", []string{"REVIEW.md", "Review.md"}, true},
		{"settings.json", []string{".claude/settings.json"}, true},
		// Permission/authorization surfaces are matched by concept, so a new
		// owning package does not silently escape escalation.
		{"permission parity pkg", []string{"agm/internal/permissionparity/parity.go"}, true},
		{"RBAC policy owner", []string{"agm/internal/rbac/profiles.go"}, true},
		{"pi authorization ext", []string{"agm/internal/permissionparity/piadapter/pi_authorization_extension.js"}, true},
		{"hook installer", []string{"agm/cmd/agm/install_hooks.go"}, true},
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
		{"Dolt schema owner", []string{"agm/internal/dolt/adapter.go"}, true},
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

// TestBinaryEscalationTriggers guards the binary bypass: git renders a binary
// change as a bare "Binary files differ" marker, so the reviewers never see the
// payload and it must escalate instead of riding an "approved".
func TestBinaryEscalationTriggers(t *testing.T) {
	if got := BinaryEscalationTriggers([]string{"build/tool", "assets/logo.png"}); len(got) != 2 {
		t.Fatalf("expected 2 binary triggers, got %v", got)
	}
	if got := BinaryEscalationTriggers(nil); len(got) != 0 {
		t.Fatalf("expected no triggers, got %v", got)
	}
	if got := BinaryEscalationTriggers([]string{"", "  "}); len(got) != 0 {
		t.Fatalf("blank paths must not trigger, got %v", got)
	}
	// A binary addition must block even when the model said approved.
	if ApplyEscalation(Approved, BinaryEscalationTriggers([]string{"build/tool"})) != NeedsHumanReview {
		t.Fatal("binary change must escalate an approved outcome")
	}
}

// TestGitlinkEscalationTriggers guards the submodule bypass: a gitlink bump
// shows only "Subproject commit <sha>", so the target tree is never reviewed.
func TestGitlinkEscalationTriggers(t *testing.T) {
	if got := GitlinkEscalationTriggers([]string{"vendor/dep"}); len(got) != 1 {
		t.Fatalf("expected a gitlink trigger, got %v", got)
	}
	if got := GitlinkEscalationTriggers([]string{"", " "}); len(got) != 0 {
		t.Fatalf("blank paths must not trigger, got %v", got)
	}
	if ApplyEscalation(Approved, GitlinkEscalationTriggers([]string{"vendor/dep"})) != NeedsHumanReview {
		t.Fatal("submodule change must escalate an approved outcome")
	}
}

// TestHookEscalationIsScoped keeps the hook rule targeted at tool-hook owners:
// over-escalating every directory named "hooks" would force needless human
// review on ordinary application packages.
func TestHookEscalationIsScoped(t *testing.T) {
	shouldNot := []string{
		"engram/hooks/registry_test.go",
		"engram/hooks/registry.go",
		"pkg/hooks/doc.go",
	}
	for _, p := range shouldNot {
		if got := EscalationTriggers([]string{p}, "", ""); len(got) != 0 {
			t.Errorf("%s must not escalate as a tool hook, got %v", p, got)
		}
	}
	shouldEscalate := []string{
		".claude/hooks/pretool-guard",
		".config/claude-code/hooks/pretool-bash-write-guard",
		"agm/cmd/agm-hooks/pretool-bash-blocker/main.go",
		"agm/hooks/cmd/posttool-cost-guard/main.go",
		".agents/hooks.json",
	}
	for _, p := range shouldEscalate {
		if got := EscalationTriggers([]string{p}, "", ""); len(got) == 0 {
			t.Errorf("%s must escalate as a tool hook surface", p)
		}
	}
}

func TestApplyEscalation(t *testing.T) {
	// Escalation forces needs-human-review from any outcome...
	for _, o := range []Outcome{Approved, NeedsWork, Rejected, NeedsHumanReview} {
		if got := ApplyEscalation(o, []string{"critical trust-root change"}); got != NeedsHumanReview {
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

// TestEscalationProtectsReviewTrustRootAndAllowsRoutineCI keeps the human-only
// boundary on review authority rather than on every workflow consumer.
func TestEscalationProtectsReviewTrustRootAndAllowsRoutineCI(t *testing.T) {
	routine := EscalationTriggers([]string{".github/workflows/structural-health.yml"}, "", "")
	if len(routine) != 0 {
		t.Fatalf("routine workflow must stay in automated review, got %v", routine)
	}
	if outcome := ApplyEscalation(Approved, routine); outcome != Approved || ExitFor(outcome, false) != 0 {
		t.Fatalf("routine workflow approved outcome = %v, want mergeable", outcome)
	}

	protected := EscalationTriggers([]string{".github/workflows/review.yml"}, "", "")
	outcome := ApplyEscalation(Approved, protected)
	if outcome != NeedsHumanReview {
		t.Fatalf("trusted review workflow must escalate, got %v", outcome)
	}
	if ExitFor(outcome, false) != 1 {
		t.Fatal("protected review workflow escalation must block the merge")
	}
}
