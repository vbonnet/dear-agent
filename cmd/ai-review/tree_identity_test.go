package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestNormalizedPathRelated_UsesSlashBoundaries(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  bool
	}{
		{left: "REVIEW.md/payload", right: "REVIEW.md", want: true},
		{left: ".GITHUB/RULESETſ", right: ".github/rulesets", want: true},
		{left: "vendor", right: "vendor", want: true},
		{left: "Vendor/module", right: "vendor", want: true},
		{left: "vendors/module", right: "vendor", want: false},
		{left: "nested/vendor/module", right: "vendor", want: false},
		{left: "NOT-A-SPEC.md", right: "SPEC.md", want: false},
	}
	for _, test := range tests {
		if got := normalizedPathRelated(test.left, test.right); got != test.want {
			t.Errorf("normalizedPathRelated(%q, %q) = %t, want %t", test.left, test.right, got, test.want)
		}
	}
}

func TestBuildReviewPlan_AIReviewTestOnlyTreeAliasesRemainAutonomous(t *testing.T) {
	repo := newReviewRepo(t)
	writeReviewFile(t, repo, "cmd/ai-review/review.go", "package main\n")
	gittest.Run(t, repo, "add", "cmd/ai-review/review.go")
	gittest.Run(t, repo, "commit", "-m", "add reviewer implementation")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

	addRawTreeBlob(t, repo, "CMD/AI-REVIEW/new_test.go", "package main\n")
	gittest.Run(t, repo, "commit", "-m", "add test through directory aliases")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	trees, err := loadTreeIdentityEvidence(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !trees.safeAIReviewTestPath("CMD/AI-REVIEW/new_test.go") {
		t.Fatal("tree-only directory aliases must preserve the test-only carveout")
	}
	identityTriggers, err := checkoutIdentityTriggers(context.Background(), trees, []string{"CMD/AI-REVIEW/new_test.go"})
	if err != nil || len(identityTriggers) != 0 {
		t.Fatalf("tree-only directory aliases triggered: %v, err=%v", identityTriggers, err)
	}
	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReviewNeeded || plan.ReviewRelevant || plan.needsHuman() {
		t.Fatalf("test-only directory alias plan = %#v", plan)
	}
}

func TestBuildReviewPlan_AIReviewTestOnlyFileDirectoryAliasesFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		production string
		alias      string
	}{
		{name: "ASCII case fold", production: "cmd/ai-review/review.go", alias: "cmd/ai-review/REVIEW.GO/escape_test.go"},
		{name: "Unicode fold", production: "cmd/ai-review/guardss.go", alias: "cmd/ai-review/guardſſ.go/escape_test.go"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			writeReviewFile(t, repo, test.production, "package main\n")
			gittest.Run(t, repo, "add", test.production)
			gittest.Run(t, repo, "commit", "-m", "add reviewer implementation")
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

			addRawTreeBlob(t, repo, test.alias, "package main\n")
			gittest.Run(t, repo, "commit", "-m", "add file-directory alias")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.ReviewNeeded || !plan.ReviewRelevant || !plan.needsHuman() ||
				!strings.Contains(strings.Join(append(plan.HumanReasons, plan.EscalationTriggers...), "\n"), "identity") {
				t.Fatalf("file-directory alias did not fail closed: %#v", plan)
			}
		})
	}
}

func TestCheckoutIdentityTriggers_NonTreePeersAndTypeTransitions(t *testing.T) {
	repo := newReviewRepo(t)
	writeReviewFile(t, repo, "ordinary.txt", "base\n")
	gittest.Run(t, repo, "add", "ordinary.txt")
	gittest.Run(t, repo, "commit", "-m", "add ordinary file")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

	addRawTreeBlob(t, repo, "Ordinary.txt", "shadow\n")
	gittest.Run(t, repo, "commit", "-m", "add colliding ordinary file")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	trees, err := loadTreeIdentityEvidence(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	triggers, err := checkoutIdentityTriggers(context.Background(), trees, []string{"Ordinary.txt"})
	if err != nil || len(triggers) == 0 {
		t.Fatalf("non-tree peers were not rejected: triggers=%v err=%v", triggers, err)
	}
}

func TestCheckoutIdentityTriggers_CrossRevisionFileTreeTransitions(t *testing.T) {
	t.Run("file to tree", func(t *testing.T) {
		repo := newReviewRepo(t)
		writeReviewFile(t, repo, "node", "file\n")
		gittest.Run(t, repo, "add", "node")
		gittest.Run(t, repo, "commit", "-m", "add file node")
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		gittest.Run(t, repo, "update-index", "--force-remove", "node")
		addRawTreeBlob(t, repo, "node/child", "child\n")
		gittest.Run(t, repo, "commit", "-m", "replace file with tree")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)
		assertCheckoutIdentityTrigger(t, base, head)
	})

	t.Run("tree to file", func(t *testing.T) {
		repo := newReviewRepo(t)
		writeReviewFile(t, repo, "node/child", "child\n")
		gittest.Run(t, repo, "add", "node/child")
		gittest.Run(t, repo, "commit", "-m", "add tree node")
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		gittest.Run(t, repo, "rm", "node/child")
		writeReviewFile(t, repo, "node", "file\n")
		gittest.Run(t, repo, "add", "node")
		gittest.Run(t, repo, "commit", "-m", "replace tree with file")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)
		assertCheckoutIdentityTrigger(t, base, head)
	})
}

func TestCheckoutIdentityTriggers_OrdinarySpellingAndModeChangesRemainAutonomous(t *testing.T) {
	t.Run("case-only rename", func(t *testing.T) {
		base, head := caseRenameReviewRepo(t, "ordinary.txt", "Ordinary.txt", "ordinary\n")
		assertNoCheckoutIdentityTrigger(t, base, head)
	})

	t.Run("mode change", func(t *testing.T) {
		base, head := modeChangeReviewRepo(t, "ordinary.txt", "ordinary\n")
		assertNoCheckoutIdentityTrigger(t, base, head)
	})
}

func TestBuildReviewPlan_AIReviewTestOnlySpellingAndModeChangesRemainAutonomous(t *testing.T) {
	t.Run("case-only test rename", func(t *testing.T) {
		base, head := caseRenameReviewRepo(t, "cmd/ai-review/foo_test.go", "cmd/ai-review/FOO_test.go", "package main\n")
		assertAutonomousReviewPlan(t, base, head)
	})

	t.Run("test mode change", func(t *testing.T) {
		base, head := modeChangeReviewRepo(t, "cmd/ai-review/foo_test.go", "package main\n")
		assertAutonomousReviewPlan(t, base, head)
	})
}

func TestBuildReviewPlan_HookGoPackageTestsRemainAutonomous(t *testing.T) {
	paths := []string{
		"agm/hooks/cmd/posttool-cost-guard/main_test.go",
		"agm/hooks/cmd/sessionstart-chezmoi-drift/main_test.go",
		"agm/hooks/cmd/stop-session-guard/main_test.go",
		"agm/internal/hooks/exit_gate_test.go",
		"agm/internal/codexhooks/verify_test.go",
	}
	for _, owner := range toolHookOwners {
		covered := false
		for _, testPath := range paths {
			covered = covered || strings.HasPrefix(testPath, owner.path+"/")
		}
		if owner.goPackage && !covered {
			t.Fatalf("Go-package hook owner lacks an authenticated test regression: %s", owner.path)
		}
		if !owner.goPackage && isHookGoTestPath(owner.path+"/backdoor_test.go") {
			t.Fatalf("script-only hook owner acquired a Go-test carveout: %s", owner.path)
		}
	}
	for _, testPath := range paths {
		t.Run(testPath, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			writeReviewFile(t, repo, testPath, "package hooks\n")
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add hook package test")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			assertAutonomousReviewPlan(t, base, head)
		})
	}
}

func TestBuildReviewPlan_HookGoPackageCaseOnlyTestRenameRemainsAutonomous(t *testing.T) {
	tests := []struct {
		name string
		base string
		head string
	}{
		{"ASCII case fold", "verify_test.go", "Verify_test.go"},
		{"Unicode full fold", "safe_test.go", "ſafe_test.go"},
		{"Unicode normalization", "café_test.go", "cafe\u0301_test.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, head := caseRenameReviewRepo(t,
				"agm/internal/codexhooks/"+tt.base,
				"agm/internal/codexhooks/"+tt.head,
				"package codexhooks\n",
			)
			assertAutonomousReviewPlan(t, base, head)
		})
	}
}

func TestBuildReviewPlan_HookGoPackageTestModeChangeRemainsAutonomous(t *testing.T) {
	base, head := modeChangeReviewRepo(t, "agm/internal/codexhooks/verify_test.go", "package codexhooks\n")
	assertAutonomousReviewPlan(t, base, head)
}

func TestBuildReviewPlan_HookOwnerSpecsRemainAutomated(t *testing.T) {
	paths := []string{
		"agm/.githooks/SPEC.md",
		"agm/internal/hooks/SPEC.md",
		"agm/internal/codexhooks/SPEC.md",
	}
	for _, specPath := range paths {
		t.Run(specPath, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			const featurePath = "agm/test/bdd/features/hook.feature"
			writeReviewFile(t, repo, specPath, specDocument("HOOK-01", "When checked, the hook shall report its state.", featurePath))
			writeReviewFile(t, repo, featurePath, featureDocument("# SPEC: "+specPath+"\n", "hook contract"))
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add hook behavioral contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.ReviewNeeded || !plan.ReviewRelevant || plan.needsHuman() || len(plan.EscalationTriggers) != 0 {
				t.Fatalf("hook SPEC did not stay in automated semantic review: %#v", plan)
			}
		})
	}
}

func TestHookOwnerCanonicalSpecsHaveSafeAuthenticatedIdentity(t *testing.T) {
	paths := []string{
		".opencode/hooks/SPEC.md",
		".pi/guardrails/SPEC.md",
		"agm/.githooks/SPEC.md",
		"agm/internal/hooks/SPEC.md",
		"agm/internal/codexhooks/SPEC.md",
	}
	for _, specPath := range paths {
		t.Run(specPath, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			writeReviewFile(t, repo, specPath, "# Hook contract\n")
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add canonical hook contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			evidence, err := loadTreeIdentityEvidence(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if got := unsafeHookOwnerAutomatedPathTriggers(evidence, []string{specPath}); len(got) != 0 {
				t.Fatalf("canonical hook SPEC had unsafe identity: %v", got)
			}
			if got := EscalationTriggers([]string{specPath}, "", ""); len(got) != 0 {
				t.Fatalf("canonical hook SPEC forced deterministic hook escalation: %v", got)
			}
		})
	}
}

func TestBuildReviewPlan_HarnessLocalHookSpecsRetainIndependentOwnershipGate(t *testing.T) {
	paths := []string{
		".opencode/hooks/SPEC.md",
		".pi/guardrails/SPEC.md",
	}
	for _, specPath := range paths {
		t.Run(specPath, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			const featurePath = "features/hook.feature"
			writeReviewFile(t, repo, specPath, specDocument("HOOK-01", "When checked, the hook shall report its state.", featurePath))
			writeReviewFile(t, repo, featurePath, featureDocument("# SPEC: "+specPath+"\n", "hook contract"))
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add harness-local hook contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.ReviewNeeded || !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "harness-local normative SPEC ownership") {
				t.Fatalf("harness-local ownership gate was lost: %#v", plan)
			}
			if len(plan.EscalationTriggers) != 0 {
				t.Fatalf("canonical hook SPEC was separately escalated by hook ownership: %v", plan.EscalationTriggers)
			}
		})
	}
}

func TestBuildReviewPlan_PreservesProtectedGitPathWhitespace(t *testing.T) {
	paths := []string{
		"agm/internal/hooks/SPEC.md ",
		"agm/internal/hooks/ SPEC.md",
		"agm/internal/hooks/exit_gate_test.go ",
		"cmd/ai-review/review_test.go ",
		"agm/internal/hooks/ exit_gate.go",
	}
	for _, changedPath := range paths {
		t.Run(changedPath, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			writeReviewFile(t, repo, changedPath, "protected path bytes\n")
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add protected whitespace path")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.ReviewRelevant || len(plan.EscalationTriggers) == 0 {
				t.Fatalf("protected path whitespace was trimmed or accepted: %#v", plan)
			}
		})
	}
}

func TestBuildReviewPlan_HookOwnerProductionAndAliasesRemainProtected(t *testing.T) {
	paths := []string{
		"agm/internal/hooks/exit_gate.go",
		".opencode/hooks/stop-guardrail-feedback",
		"agm/internal/hooks/backdoor_TEST.go",
		"agm/internal/codexhooks/backdoor_teſt.go",
		"agm/.githooks/spec.md",
		"agm/internal/hooks/SPEC.md/payload",
	}
	for _, changedPath := range paths {
		t.Run(changedPath, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			writeReviewFile(t, repo, changedPath, "protected hook authority\n")
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add protected hook surface")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.ReviewRelevant || len(plan.EscalationTriggers) == 0 {
				t.Fatalf("protected hook surface did not fail closed: %#v", plan)
			}
		})
	}
}

func TestBuildReviewPlan_HookOwnerCaseAliasRemainsProtected(t *testing.T) {
	repo := newReviewRepo(t)
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	const aliasPath = "AGM/internal/hooks/exit_gate_test.go"
	addRawTreeBlob(t, repo, aliasPath, "package hooks\n")
	gittest.Run(t, repo, "commit", "-m", "add hook owner alias")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ReviewRelevant || len(plan.EscalationTriggers) == 0 {
		t.Fatalf("hook owner alias did not fail closed: %#v", plan)
	}
}

func TestBuildReviewPlan_AGMInternalHookCarveoutRejectsUnsafeEvidence(t *testing.T) {
	t.Run("test symlink", func(t *testing.T) {
		repo := newReviewRepo(t)
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		object := rawBlobObject(t, repo, "target")
		gittest.Run(t, repo, "update-index", "--add", "--cacheinfo", "120000,"+object+",agm/internal/hooks/exit_gate_test.go")
		gittest.Run(t, repo, "commit", "-m", "add hook test symlink")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)

		plan, err := buildReviewPlan(context.Background(), base, head)
		if err != nil {
			t.Fatal(err)
		}
		if !plan.ReviewRelevant || !containsEscalation(plan.EscalationTriggers, "unsafe checkout identity") {
			t.Fatalf("hook test symlink did not fail closed: %#v", plan)
		}
	})

	t.Run("case-fold test leaf peer", func(t *testing.T) {
		repo := newReviewRepo(t)
		writeReviewFile(t, repo, "agm/internal/hooks/exit_gate_test.go", "package hooks\n")
		gittest.Run(t, repo, "add", "-A")
		gittest.Run(t, repo, "commit", "-m", "add canonical hook test")
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		addRawTreeBlob(t, repo, "agm/internal/hooks/Exit_Gate_test.go", "package hooks\n")
		gittest.Run(t, repo, "commit", "-m", "add hook test alias")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)

		plan, err := buildReviewPlan(context.Background(), base, head)
		if err != nil {
			t.Fatal(err)
		}
		if !plan.ReviewRelevant || !containsEscalation(plan.EscalationTriggers, "unsafe checkout identity") {
			t.Fatalf("hook test alias did not fail closed: %#v", plan)
		}
	})

	t.Run("executable SPEC", func(t *testing.T) {
		repo := newReviewRepo(t)
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		writeReviewFile(t, repo, "agm/internal/hooks/SPEC.md", specWithoutTrace("HOOK-01", "When checked, the hook shall report its state."))
		gittest.Run(t, repo, "add", "-A")
		gittest.Run(t, repo, "update-index", "--chmod=+x", "agm/internal/hooks/SPEC.md")
		gittest.Run(t, repo, "commit", "-m", "make hook contract executable")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)

		plan, err := buildReviewPlan(context.Background(), base, head)
		if err != nil {
			t.Fatal(err)
		}
		if !plan.ReviewRelevant || !containsEscalation(plan.EscalationTriggers, "unsafe checkout identity") {
			t.Fatalf("executable hook SPEC evidence did not fail closed: %#v", plan)
		}
	})
}

func TestCheckoutIdentityTriggers_DeduplicatesSharedAncestorConflict(t *testing.T) {
	identity := normalizedPathIdentity("root")
	evidence := treeIdentityEvidence{
		base: treeIdentityIndex{byIdentity: map[string][]treeIdentityEntry{
			identity: {{Path: "root", Mode: "100644", Type: "blob"}},
		}},
		head: treeIdentityIndex{byIdentity: map[string][]treeIdentityEntry{
			identity: {{Path: "root", Mode: "040000", Type: "tree"}},
		}},
	}
	paths := make([]string, 1000)
	for index := range paths {
		paths[index] = fmt.Sprintf("root/leaf-%04d", index)
	}
	triggers, err := checkoutIdentityTriggers(context.Background(), evidence, paths)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("shared ancestor conflict triggers=%v err=%v, want one", triggers, err)
	}
}

func TestBuildReviewPlan_EmptyDiffSkipsUnrelatedTreeIdentityEvidence(t *testing.T) {
	repo := newReviewRepo(t)
	for index := range maxTreeIdentityPeers + 1 {
		component := []byte("abcdef")
		for bit := range component {
			if index&(1<<bit) != 0 {
				component[bit] -= 'a' - 'A'
			}
		}
		addRawTreeBlob(t, repo, fmt.Sprintf("%s/leaf-%02d", component, index), "historical alias\n")
	}
	gittest.Run(t, repo, "commit", "-m", "add historical tree-only aliases")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	gittest.Run(t, repo, "commit", "--allow-empty", "-m", "no source change")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReviewNeeded || plan.ReviewRelevant || plan.needsHuman() {
		t.Fatalf("empty diff was not neutral: %#v", plan)
	}
}

func TestBoundedUniqueEscalationTriggers(t *testing.T) {
	triggers := make([]string, maxDeterministicEscalationTriggers)
	for index := range triggers {
		triggers[index] = fmt.Sprintf("trigger-%03d", index)
	}
	if got, err := boundedUniqueEscalationTriggers(append(triggers, triggers[0])); err != nil || len(got) != len(triggers) {
		t.Fatalf("bounded trigger set = %d, %v", len(got), err)
	}
	if _, err := boundedUniqueEscalationTriggers(append(triggers, "overflow")); err == nil {
		t.Fatal("over-bound escalation trigger set was accepted")
	}
}

func TestParseTreeIdentityIndexRejectsMalformedAndBoundedEvidence(t *testing.T) {
	object := strings.Repeat("a", 40)
	entry := func(mode, objectType, path string) string {
		return mode + " " + objectType + " " + object + "\t" + path + "\x00"
	}
	defaults := treeIdentityLimits{entries: 16, peers: 4, components: 8}
	tests := []struct {
		name   string
		input  string
		limits treeIdentityLimits
	}{
		{name: "malformed framing", input: "malformed\x00", limits: defaults},
		{name: "unterminated record", input: strings.TrimSuffix(entry("100644", "blob", "file"), "\x00"), limits: defaults},
		{name: "unsupported mode", input: entry("100664", "blob", "file"), limits: defaults},
		{name: "mode type mismatch", input: entry("040000", "blob", "dir"), limits: defaults},
		{name: "missing tree ancestry", input: entry("100644", "blob", "dir/file"), limits: defaults},
		{name: "entry bound", input: entry("100644", "blob", "one") + entry("100644", "blob", "two"), limits: treeIdentityLimits{entries: 1, peers: 4, components: 8}},
		{name: "peer bound", input: entry("100644", "blob", "alias") + entry("100644", "blob", "ALIAS"), limits: treeIdentityLimits{entries: 4, peers: 1, components: 8}},
		{name: "component bound", input: entry("040000", "tree", "dir") + entry("100644", "blob", "dir/file"), limits: treeIdentityLimits{entries: 4, peers: 4, components: 1}},
		{name: "record bound", input: entry("100644", "blob", strings.Repeat("p", maxTreeIdentityRecordBytes)), limits: defaults},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseTreeIdentityIndex(context.Background(), []byte(test.input), test.limits); err == nil {
				t.Fatal("malformed or over-bound tree identity evidence was accepted")
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parseTreeIdentityIndex(ctx, []byte(entry("100644", "blob", "file")), defaults); err == nil {
		t.Fatal("canceled tree identity parsing was accepted")
	}
}

// TestTreeIdentityInventoryBoundIsDerivedFromEntryLimit keeps the byte bound a
// consequence of the declared entry limit. A whole-repository `ls-tree -r -t`
// inventory is not a diff, so reusing the diff-sized maxGitMetadataBytes made
// an undeclared byte cap bind first and would have failed every review once
// the repository outgrew it.
func TestTreeIdentityInventoryBoundIsDerivedFromEntryLimit(t *testing.T) {
	if maxTreeIdentityBytes <= maxGitMetadataBytes {
		t.Errorf("inventory bound %d is not larger than the diff-sized metadata bound %d",
			maxTreeIdentityBytes, maxGitMetadataBytes)
	}
	// Every record the parser accepts fits in maxTreeIdentityRecordBytes, so
	// the byte bound admits at least this many entries no matter how the
	// repository is shaped.
	guaranteedEntries := maxTreeIdentityBytes / maxTreeIdentityRecordBytes
	if guaranteedEntries < 64*1024 {
		t.Errorf("inventory bound guarantees only %d entries at the worst permitted record size", guaranteedEntries)
	}
	// A 256-component path at one byte per component must still fit one
	// record, or the two declared limits would contradict each other.
	const worstCasePathBytes = maxTreeIdentityPathComponents*2 - 1
	const recordMetadataBytes = len("100644 blob ") + 40 + len("\t") + len("\x00")
	if worstCasePathBytes+recordMetadataBytes > maxTreeIdentityRecordBytes {
		t.Errorf("record bound %d cannot hold a %d-component path", maxTreeIdentityRecordBytes, maxTreeIdentityPathComponents)
	}
}

func TestBuildReviewPlan_ProtectedFileDirectoryRelationsFailClosed(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "review policy descendant", path: "review.md/payload"},
		{name: "SPEC authoring owner descendant", path: "DOCS/SPEC-AUTHORING.MD/payload"},
		{name: "review ruleset root", path: ".GITHUB/RULESETſ"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			addRawTreeBlob(t, repo, test.path, "protected alias\n")
			gittest.Run(t, repo, "commit", "-m", "add protected path alias")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.ReviewRelevant || (!plan.needsHuman() && len(plan.EscalationTriggers) == 0) {
				t.Fatalf("protected path relation remained autonomous: %#v", plan)
			}
		})
	}
}

func TestBuildReviewPlan_SPECControlDescendantAliasRequiresHuman(t *testing.T) {
	repo := newReviewRepo(t)
	const specPath = "module/SPEC.md"
	const featurePath = "features/module.feature"
	writeReviewFile(t, repo, specPath, specDocument("MOD-01", "When checked, the system shall report its state.", featurePath))
	writeReviewFile(t, repo, featurePath, featureDocument("# SPEC: "+specPath+"\n", "contract"))
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add canonical SPEC")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

	addRawTreeBlob(t, repo, "module/spec.md/payload", "shadow\n")
	gittest.Run(t, repo, "commit", "-m", "add SPEC descendant alias")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "SPEC control ancestor or descendant identity") {
		t.Fatalf("SPEC descendant alias did not fail closed: %#v", plan)
	}
}

func TestBuildReviewPlan_SPECNamedDirectoryWithoutControlRemainsAutonomous(t *testing.T) {
	repo := newReviewRepo(t)
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	writeReviewFile(t, repo, "agm/tests/e2e-install/Dockerfiles/SPEC.md/payload", "ordinary fixture\n")
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add ordinary SPEC-named directory")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReviewNeeded || plan.ReviewRelevant || plan.needsHuman() {
		t.Fatalf("ordinary SPEC-named directory escalated: %#v", plan)
	}
}

func TestBuildReviewPlan_ChangedSpecIgnoresUnrelatedSPECNamedDirectory(t *testing.T) {
	repo := newReviewRepo(t)
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	const specPath = "module/SPEC.md"
	const featurePath = "features/module.feature"
	writeReviewFile(t, repo, specPath, specDocument("MOD-01", "When checked, the system shall report its state.", featurePath))
	writeReviewFile(t, repo, featurePath, featureDocument("# SPEC: "+specPath+"\n", "contract"))
	writeReviewFile(t, repo, "ordinary/SPEC.md/payload", "ordinary fixture\n")
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add contract beside ordinary SPEC-named directory")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ReviewNeeded || plan.needsHuman() || len(plan.Changes) != 1 || plan.Changes[0].Path != specPath {
		t.Fatalf("changed SPEC plan was polluted by ordinary directory: %#v", plan)
	}
}

func TestBuildReviewPlan_VendorRootSymlinkRequiresHuman(t *testing.T) {
	repo := newReviewRepo(t)
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	object := rawBlobObject(t, repo, "external-vendor")
	gittest.Run(t, repo, "update-index", "--add", "--cacheinfo", "120000,"+object+",vendor")
	gittest.Run(t, repo, "commit", "-m", "redirect vendor root")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "dependency graph") {
		t.Fatalf("vendor root symlink did not require human review: %#v", plan)
	}
}

func TestLoadTreeIdentityEvidenceRejectsInvalidAndCanceledReads(t *testing.T) {
	if _, err := loadTreeIdentityEvidence(context.Background(), "invalid", strings.Repeat("a", 40)); err == nil {
		t.Fatal("invalid revision was accepted")
	}
	repo := newReviewRepo(t)
	revision := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loadTreeIdentityEvidence(ctx, revision, revision); err == nil {
		t.Fatal("canceled tree identity read was accepted")
	}
}

func TestSpecReviewDependencyPath_IncludesExactVendorRootOnly(t *testing.T) {
	for _, path := range []string{"vendor", "Vendor/module", "go.mod/payload", "GO.WORK"} {
		if !specReviewDependencyPath(path) {
			t.Errorf("specReviewDependencyPath(%q) = false", path)
		}
	}
	for _, path := range []string{"vendors", "vendors/module", "nested/vendor/module", "go.mod.backup"} {
		if specReviewDependencyPath(path) {
			t.Errorf("specReviewDependencyPath(%q) = true", path)
		}
	}
}

func addRawTreeBlob(t *testing.T, repo, path, contents string) {
	t.Helper()
	object := rawBlobObject(t, repo, contents)
	gittest.Run(t, repo, "update-index", "--add", "--cacheinfo", "100644,"+object+","+path)
}

func rawBlobObject(t *testing.T, repo, contents string) string {
	t.Helper()
	command := gittest.Command(t, repo, "hash-object", "-w", "--stdin")
	command.Stdin = strings.NewReader(contents)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("write raw Git blob: %v\n%s", err, out)
	}
	object := strings.TrimSpace(string(out))
	if !validObjectID(object) || slices.Contains([]string{"", strings.Repeat("0", len(object))}, object) {
		t.Fatalf("hash-object returned invalid object ID %q", object)
	}
	return object
}

func assertCheckoutIdentityTrigger(t *testing.T, base, head string) {
	t.Helper()
	trees, err := loadTreeIdentityEvidence(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	triggers, err := checkoutIdentityTriggers(context.Background(), trees, paths)
	if err != nil || len(triggers) == 0 {
		t.Fatalf("file/tree transition was not rejected: paths=%v triggers=%v err=%v", paths, triggers, err)
	}
}

func assertNoCheckoutIdentityTrigger(t *testing.T, base, head string) {
	t.Helper()
	trees, err := loadTreeIdentityEvidence(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := gitChangedPaths(base, head)
	if err != nil {
		t.Fatal(err)
	}
	triggers, err := checkoutIdentityTriggers(context.Background(), trees, paths)
	if err != nil || len(triggers) != 0 {
		t.Fatalf("ordinary spelling/mode change triggered: paths=%v triggers=%v err=%v", paths, triggers, err)
	}
}

func assertAutonomousReviewPlan(t *testing.T, base, head string) {
	t.Helper()
	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ReviewNeeded || plan.ReviewRelevant || plan.needsHuman() {
		t.Fatalf("test-only plan was not autonomous: %#v", plan)
	}
}

func caseRenameReviewRepo(t *testing.T, basePath, headPath, contents string) (string, string) {
	t.Helper()
	repo := newReviewRepo(t)
	writeReviewFile(t, repo, basePath, contents)
	gittest.Run(t, repo, "add", basePath)
	gittest.Run(t, repo, "commit", "-m", "add file before case-only rename")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	object := strings.TrimSpace(gittest.Run(t, repo, "hash-object", basePath))
	// Preserve the exact raw path bytes for normalization-only rename fixtures
	// even on macOS, where Git otherwise precomposes index paths by default.
	gittest.Run(t, repo, "config", "core.precomposeunicode", "false")
	gittest.Run(t, repo, "update-index", "--force-remove", basePath)
	gittest.Run(t, repo, "update-index", "--add", "--cacheinfo", "100644,"+object+","+headPath)
	gittest.Run(t, repo, "commit", "-m", "rename file by case")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)
	return base, head
}

func modeChangeReviewRepo(t *testing.T, path, contents string) (string, string) {
	t.Helper()
	repo := newReviewRepo(t)
	writeReviewFile(t, repo, path, contents)
	gittest.Run(t, repo, "add", path)
	gittest.Run(t, repo, "commit", "-m", "add file before mode change")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	gittest.Run(t, repo, "update-index", "--chmod=+x", path)
	gittest.Run(t, repo, "commit", "-m", "change file mode")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)
	return base, head
}
