package main

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestSpecReviewOwnerPath_CoversCanonicalSpecAuthoringSurfaceAndAliases(t *testing.T) {
	expected := []string{
		specAuthoringPolicyPath,
		"docs/templates/SPEC.md.tmpl",
		"spec-governance/skills/write-spec/SKILL.md",
		"spec-governance/skills/write-spec/references/contract-model.md",
		"spec-governance/skills/write-spec/references/ears-and-bdd.md",
		"spec-governance/skills/audit-specs/SKILL.md",
		"spec-governance/skills/audit-specs/references/audit-verdicts.md",
		"spec-governance/skills/audit-specs/references/report-schema.md",
	}
	if !slices.Equal(canonicalSpecAuthoringOwnerPaths[:], expected) {
		t.Fatalf("canonical SPEC authoring owners = %v, want %v", canonicalSpecAuthoringOwnerPaths, expected)
	}
	for _, path := range expected {
		if !specReviewOwnerPath(path) {
			t.Errorf("canonical SPEC authoring owner %q is not protected", path)
		}
	}
	for _, path := range []string{
		"DOCS/SPEC-AUTHORING.MD",
		"docſ/spec-authoring.md",
		"AGM/INTERNAL/AGENT/HARNESSES.GO",
		".GitHub/Workflows/Review.yml",
		".GitHub/Rulesets/Main.json",
		"INTERNAL/EARSLINT/LINT.GO",
		"internal/earſlint/lint.go",
		"INTERNAL/MARKDOWNVISIBLE/MARKDOWN.GO",
	} {
		if !specReviewOwnerPath(path) {
			t.Errorf("case-fold alias of protected owner %q is not protected", path)
		}
	}
	for _, path := range []string{
		"docs/templates/README.md",
		"spec-governance/skills/write-spec/references/README.md",
		"spec-governance/skills/audit-specs/examples/report-schema.md",
		"spec-governance/skills/other/SKILL.md",
		"cmd/ai-review/spec_contract_test.go",
		"CMD/AI-REVIEW/spec_contract_test.go",
	} {
		if specReviewOwnerPath(path) {
			t.Errorf("non-owner path %q entered the protected authoring surface", path)
		}
	}
}

func TestBuildReviewPlan_SPECControlAliasesFailClosed(t *testing.T) {
	tests := []struct {
		name          string
		canonicalPath string
		aliasPath     string
		rename        bool
	}{
		{name: "addition-only case alias", canonicalPath: "module/SPEC.md", aliasPath: "module/spec.md"},
		{name: "case-only rename", canonicalPath: "module/SPEC.md", aliasPath: "module/spec.md", rename: true},
		{name: "addition-only Unicode fold alias", canonicalPath: "module/SPEC.md", aliasPath: "module/ſPEC.md"},
		{name: "addition-only ownership alias", canonicalPath: "module/SPEC.owner", aliasPath: "module/spec.owner"},
		{name: "addition-only ASCII directory alias", canonicalPath: "module/SPEC.md", aliasPath: "MODULE/SPEC.md"},
		{name: "addition-only ownership directory alias", canonicalPath: "owners/SPEC.owner", aliasPath: "OWNERS/SPEC.owner"},
		{name: "case-only directory rename", canonicalPath: "module/SPEC.md", aliasPath: "MODULE/SPEC.md", rename: true},
		{name: "addition-only Unicode fold directory alias", canonicalPath: "specs/SPEC.md", aliasPath: "ſpecs/SPEC.md"},
		{name: "addition-only Unicode normalization directory alias", canonicalPath: "café/SPEC.md", aliasPath: "café/SPEC.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			content := "../SPEC.md\n"
			paths := []string{tt.canonicalPath}
			if filepath.Base(tt.canonicalPath) == "SPEC.md" {
				content = specDocument("MOD-01", "When checked, the system shall report it.", "features/module.feature")
				writeReviewFile(t, repo, "features/module.feature", featureDocument("# SPEC: module/SPEC.md\n", "contract"))
				paths = append(paths, "features/module.feature")
			}
			writeReviewFile(t, repo, tt.canonicalPath, content)
			gittest.Run(t, repo, append([]string{"add"}, paths...)...)
			gittest.Run(t, repo, "commit", "-m", "add canonical contract")
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

			blob := strings.TrimSpace(gittest.Run(t, repo, "hash-object", tt.canonicalPath))
			if tt.rename {
				gittest.Run(t, repo, "update-index", "--force-remove", tt.canonicalPath)
			}
			index := gittest.Command(t, repo, "update-index", "-z", "--index-info")
			index.Stdin = strings.NewReader("100644 " + blob + "\t" + tt.aliasPath + "\x00")
			if out, err := index.CombinedOutput(); err != nil {
				t.Fatalf("add raw SPEC control alias to index: %v\n%s", err, out)
			}
			gittest.Run(t, repo, "commit", "-m", "add SPEC control alias")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			changed, err := gitChangedPaths(base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(changed, tt.aliasPath) {
				t.Fatalf("changed paths = %v, want alias %q", changed, tt.aliasPath)
			}
			if tt.rename && !slices.Contains(changed, tt.canonicalPath) {
				t.Fatalf("case-only rename paths = %v, want canonical deletion", changed)
			}

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.ReviewNeeded || !plan.needsHuman() ||
				!strings.Contains(strings.Join(plan.HumanReasons, "\n"), "SPEC control") {
				t.Fatalf("SPEC alias plan did not fail closed: %#v", plan)
			}
		})
	}
}

func TestBuildReviewPlan_SPECControlUniqueUppercaseDirectoryRemainsReviewable(t *testing.T) {
	repo := newReviewRepo(t)
	const specPath = "Module/SPEC.md"
	const featurePath = "features/module.feature"
	writeReviewFile(t, repo, specPath, specDocument("MOD-01", "When checked, the system shall report its initial state.", featurePath))
	writeReviewFile(t, repo, featurePath, featureDocument("# SPEC: "+specPath+"\n", "contract"))
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add unique uppercase-directory contract")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

	writeReviewFile(t, repo, specPath, specDocument("MOD-01", "When checked, the system shall report its final state.", featurePath))
	gittest.Run(t, repo, "add", specPath)
	gittest.Run(t, repo, "commit", "-m", "change unique uppercase-directory contract")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.needsHuman() || !plan.ReviewNeeded || !slices.Equal(plan.Changes, []specChange{{Path: specPath, Status: "modified"}}) {
		t.Fatalf("unique uppercase-directory SPEC was not ordinarily reviewable: %#v", plan)
	}
}

func TestBuildReviewPlan_SPECControlTypeEvidenceFailsClosed(t *testing.T) {
	repo := newReviewRepo(t)
	const specPath = "module/SPEC.md"
	const featurePath = "features/module.feature"
	writeReviewFile(t, repo, specPath, specDocument("MOD-01", "When checked, the system shall report its state.", featurePath))
	writeReviewFile(t, repo, featurePath, featureDocument("# SPEC: "+specPath+"\n", "contract"))
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add contract")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

	gittest.Run(t, repo, "update-index", "--chmod=+x", specPath)
	gittest.Run(t, repo, "commit", "-m", "make SPEC executable")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "SPEC control path type evidence") {
		t.Fatalf("nonregular SPEC control evidence did not fail closed: %#v", plan)
	}
}

func TestChangedSpecControlIdentityRisks_NearNamesDoNotReadGit(t *testing.T) {
	risks, err := changedSpecControlIdentityRisks(context.Background(), treeIdentityEvidence{}, []string{
		"module/NOT-A-SPEC.md",
		"module/NOT-SPEC.owner",
	})
	if err != nil || len(risks) != 0 {
		t.Fatalf("near-name SPEC controls escalated: risks=%v err=%v", risks, err)
	}
}

func TestSpecReviewDependencyPathDoesNotCallManifestsSpecOwners(t *testing.T) {
	for _, path := range []string{
		"go.mod", "go.sum", "go.work", "go.work.sum", "vendor/example/module.go",
		"GO.MOD", "go.ſum", "VENDOR/example/module.go",
	} {
		if !specReviewDependencyPath(path) {
			t.Errorf("reviewer dependency path %q is not protected", path)
		}
		if specReviewOwnerPath(path) {
			t.Errorf("reviewer dependency path %q is mislabeled as a SPEC owner", path)
		}
	}
	for _, path := range []string{"nested/go.mod", "go.mod.backup", "vendors/example.go", "docs/ordinary.md"} {
		if specReviewDependencyPath(path) {
			t.Errorf("ordinary path %q entered the reviewer dependency boundary", path)
		}
	}
}

func TestBuildReviewPlan_AIReviewTestOnlyRenameBoundary(t *testing.T) {
	tests := []struct {
		name       string
		basePath   string
		headPath   string
		wantReview bool
	}{
		{name: "test deletion", basePath: "cmd/ai-review/old_test.go"},
		{name: "test to test rename", basePath: "cmd/ai-review/old_test.go", headPath: "cmd/ai-review/new_test.go"},
		{name: "production to test rename", basePath: "cmd/ai-review/review.go", headPath: "cmd/ai-review/review_test.go", wantReview: true},
		{name: "test to production rename", basePath: "cmd/ai-review/review_test.go", headPath: "cmd/ai-review/review.go", wantReview: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			writeReviewFile(t, repo, tt.basePath, "package main\n")
			gittest.Run(t, repo, "add", tt.basePath)
			gittest.Run(t, repo, "commit", "-m", "add review source")
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))

			if tt.headPath == "" {
				gittest.Run(t, repo, "rm", tt.basePath)
			} else {
				gittest.Run(t, repo, "mv", tt.basePath, tt.headPath)
			}
			gittest.Run(t, repo, "commit", "-m", "move review source")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if plan.ReviewNeeded != tt.wantReview {
				t.Fatalf("ReviewNeeded = %t, want %t; reasons=%v", plan.ReviewNeeded, tt.wantReview, plan.HumanReasons)
			}
			if tt.wantReview && !plan.needsHuman() {
				t.Fatalf("production-side rename did not require maintainer review: %#v", plan)
			}
		})
	}
}
