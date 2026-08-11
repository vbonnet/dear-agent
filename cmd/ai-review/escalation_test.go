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
		{"case-fold alias of review policy", []string{"Review.md"}, true},
		{"trusted review workflow", []string{".github/workflows/review.yml"}, true},
		{"case-fold alias of review workflow", []string{".GitHub/Workflows/Review.yml"}, true},
		{"review gate implementation", []string{"cmd/ai-review/escalation.go"}, true},
		{"review gate governance", []string{"cmd/ai-review/SPEC.md"}, true},
		{"review test-only hardening", []string{"cmd/ai-review/spec_contract_test.go"}, false},
		{"review test-only deletion path", []string{"cmd/ai-review/spec_contract_git_test.go"}, false},
		{"review test-only owner case-fold alias", []string{"CMD/AI-REVIEW/Spec_Contract_test.go"}, false},
		{"review uppercase test suffix is production", []string{"cmd/ai-review/backdoor_TEST.go"}, true},
		{"review Unicode test suffix is production", []string{"cmd/ai-review/backdoor_teſt.go"}, true},
		{"review test-only rename", []string{"cmd/ai-review/old_test.go", "cmd/ai-review/new_test.go"}, false},
		{"review production renamed to test", []string{"cmd/ai-review/review.go", "cmd/ai-review/review_test.go"}, true},
		{"review test renamed to production", []string{"cmd/ai-review/review_test.go", "cmd/ai-review/review.go"}, true},
		{"review non-Go test-shaped file", []string{"cmd/ai-review/review_test.yaml"}, true},
		{"ruleset edit", []string{".github/rulesets/main.json"}, true},
		{"case-fold alias of ruleset", []string{".GitHub/Rulesets/Main.json"}, true},
		{"Unicode-fold alias of ruleset", []string{".github/ruleſetſ/main.json"}, true},
		{"case-only rename retains canonical deletion", []string{"REVIEW.md", "Review.md"}, true},
		{"settings.json", []string{".claude/settings.json"}, true},
		// Permission/authorization surfaces are matched by concept, so a new
		// owning package does not silently escape escalation.
		{"permission parity pkg", []string{"agm/internal/permissionparity/parity.go"}, true},
		{"RBAC policy owner", []string{"agm/internal/rbac/profiles.go"}, true},
		{"pi authorization ext", []string{"agm/internal/permissionparity/piadapter/pi_authorization_extension.js"}, true},
		{"hook installer", []string{"agm/cmd/agm/install_hooks.go"}, true},
		{"hook script", []string{".config/claude-code/hooks/pretool-bash-write-guard"}, true},
		{"OpenCode hook root", []string{".opencode/hooks"}, true},
		{"OpenCode hook case alias", []string{".OPENCODE/HOOKS/stop-guardrail-feedback"}, true},
		{"Pi guardrail Unicode alias", []string{".pi/guardrailſ/stop-guardrail-feedback"}, true},
		{"AGM command hook root", []string{"agm/cmd/agm/hooks"}, true},
		{"AGM command hook descendant", []string{"agm/cmd/agm/hooks/session-start-agm-state-ready"}, true},
		{"AGM internal hook root", []string{"agm/internal/hooks"}, true},
		{"AGM internal hook case alias", []string{"AGM/INTERNAL/HOOKS/exit_gate.go"}, true},
		{"AGM internal hook Unicode alias", []string{"agm/internal/hookſ/living_docs_check.go"}, true},
		{"AGM internal hook descendant", []string{"agm/internal/hooks/living_docs_check.go"}, true},
		{"AGM internal hook Go test", []string{"agm/internal/hooks/exit_gate_test.go"}, false},
		{"OpenCode hook contract", []string{".opencode/hooks/SPEC.md"}, false},
		{"Pi guardrail contract", []string{".pi/guardrails/SPEC.md"}, false},
		{"AGM Git hook contract", []string{"agm/.githooks/SPEC.md"}, false},
		{"AGM internal hook contract", []string{"agm/internal/hooks/SPEC.md"}, false},
		{"hook permission contract keeps independent trigger", []string{"agm/internal/hooks/permissions/SPEC.md"}, true},
		{"hook migration contract keeps independent trigger", []string{"agm/internal/hooks/migrations/SPEC.md"}, true},
		{"hook security contract keeps independent trigger", []string{"agm/internal/hooks/write-guard/SPEC.md"}, true},
		{"hook launchd contract keeps independent trigger", []string{"agm/internal/hooks/launchd/SPEC.md"}, true},
		{"AGM Codex hook contract", []string{"agm/internal/codexhooks/SPEC.md"}, false},
		{"AGM Codex hook Go test", []string{"agm/internal/codexhooks/verify_test.go"}, false},
		{"script hook pseudo-Go test", []string{".opencode/hooks/backdoor_test.go"}, true},
		{"AGM internal hook uppercase test suffix", []string{"agm/internal/hooks/backdoor_TEST.go"}, true},
		{"AGM internal hook Unicode test suffix", []string{"agm/internal/hooks/backdoor_teſt.go"}, true},
		{"AGM internal hook lowercase contract alias", []string{"agm/internal/hooks/spec.md"}, true},
		{"AGM internal hook Unicode contract alias", []string{"agm/internal/hooks/ſPEC.md"}, true},
		{"AGM internal hook test owner case alias", []string{"AGM/internal/hooks/exit_gate_test.go"}, true},
		{"AGM hook contract trailing space", []string{"agm/internal/hooks/SPEC.md "}, true},
		{"AGM hook contract leading basename space", []string{"agm/internal/hooks/ SPEC.md"}, true},
		{"AGM hook test trailing space", []string{"agm/internal/hooks/exit_gate_test.go "}, true},
		{"review test trailing space", []string{"cmd/ai-review/review_test.go "}, true},
		{"hook production leading basename space", []string{"agm/internal/hooks/ exit_gate.go"}, true},
		{"AGM Git hook root alias", []string{"AGM/.GITHOOKS"}, true},
		{"AGM Git hook descendant", []string{"agm/.githooks/pre-commit"}, true},
		{"Codex hook Unicode root alias", []string{"agm/internal/codexhookſ"}, true},
		{"Codex hook descendant", []string{"agm/internal/codexhooks/verify.go"}, true},
		{"pretool prefix", []string{"cmd/pretool-fs-write-guard/main.go"}, true},
		// Hook *registration* files have no "/hooks/" segment, and hook
		// implementations are often named main.go under a -hooks/ owner dir.
		{"agents hooks.json", []string{".agents/hooks.json"}, true},
		{"codex hooks.json", []string{".codex/hooks.json"}, true},
		{"Codex hook feature config", []string{".codex/config.toml"}, true},
		{"Codex hook feature config case alias", []string{".CODEX/CONFIG.TOML"}, true},
		{"Codex hook feature config Unicode alias", []string{".codex/conﬁg.toml"}, true},
		{"Codex hook feature config descendant", []string{".codex/config.toml/payload"}, true},
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

func TestChangedWorkflowIdentities_PreservesGitPathWhitespace(t *testing.T) {
	const canonical = ".github/workflows/ordinary.yml"
	got := changedWorkflowIdentities([]string{
		canonical,
		canonical + " ",
		" " + canonical,
	})
	paths, ok := got[normalizedPathIdentity(canonical)]
	if !ok || len(got) != 1 || len(paths) != 1 || paths[0] != canonical {
		t.Fatalf("workflow identities mutated authenticated paths: %#v", got)
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
	if got := BinaryEscalationTriggers([]string{""}); len(got) != 0 {
		t.Fatalf("empty paths must not trigger, got %v", got)
	}
	if got := BinaryEscalationTriggers([]string{"  "}); len(got) != 1 || got[0] != "binary file not reviewable from a text diff (  )" {
		t.Fatalf("authenticated path whitespace was not preserved, got %v", got)
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
	if got := GitlinkEscalationTriggers([]string{""}); len(got) != 0 {
		t.Fatalf("empty paths must not trigger, got %v", got)
	}
	if got := GitlinkEscalationTriggers([]string{" "}); len(got) != 1 || got[0] != "submodule (gitlink) change whose target tree is not in the diff ( )" {
		t.Fatalf("authenticated path whitespace was not preserved, got %v", got)
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
		"agm/.git-hooks-doc/pre-commit",
		"agm/internal/hooksdocs/helper.go",
		"agm/internal/codexhookdocs/verify.go",
		".codex/config.toml.example",
		"agm/internal/hooks/exit_gate_test.go",
		"agm/internal/hooks/SPEC.md",
		"agm/internal/codexhooks/verify_test.go",
		"agm/internal/codexhooks/SPEC.md",
		".opencode/hooks/SPEC.md",
		".pi/guardrails/SPEC.md",
		"agm/.githooks/SPEC.md",
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
		"agm/internal/hooks/living_docs_check.go",
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
