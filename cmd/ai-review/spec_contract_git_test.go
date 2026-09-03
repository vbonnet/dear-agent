package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestLoadHeadSpecCorpus_IgnoresUntrackedWorktreesAndArchives(t *testing.T) {
	repo := gittest.NewRepo(t)
	committed := specWithoutTrace("COMMITTED-01", "When the corpus is loaded, the system shall use committed bytes.")
	writeReviewFile(t, repo, "tracked/SPEC.md", committed)
	gittest.Run(t, repo, "add", "tracked/SPEC.md")
	gittest.Run(t, repo, "commit", "-m", "add tracked contract")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "--verify", "HEAD^{commit}"))

	writeReviewFile(t, repo, "tracked/SPEC.md", specWithoutTrace("DIRTY-01", "When the worktree changes, the system shall ignore dirty bytes."))
	writeReviewFile(t, repo, ".worktrees/cache/repo/SPEC.md", specWithoutTrace("CACHE-01", "When a cache exists, the system shall ignore it."))
	writeReviewFile(t, repo, "archives/snapshot/SPEC.md", specWithoutTrace("ARCHIVE-01", "When an archive exists, the system shall ignore it."))
	chdir(t, repo)

	corpus, err := loadHeadSpecCorpus(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus) != 1 || string(corpus["tracked/SPEC.md"]) != committed {
		t.Fatalf("authenticated corpus = %#v, want only exact committed SPEC", corpus)
	}
	for _, path := range []string{".worktrees/cache/repo/SPEC.md", "archives/snapshot/SPEC.md"} {
		if _, ok := corpus[path]; ok {
			t.Errorf("authenticated corpus included mutable checkout path %q", path)
		}
	}
}

func TestGitOutputBounded_KillsProducerOnOverflow(t *testing.T) {
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\nwhile :; do printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := gitOutputBounded(context.Background(), 64, "version"); !errors.Is(err, errGitOutputLimit) {
		t.Fatalf("gitOutputBounded overflow error = %v, want %v", err, errGitOutputLimit)
	}
}

// TestGitMetadataEvidence_FailsClosedOnGitErrorsAndOverflow ensures that the
// mandatory escalation inputs never turn a Git failure into an empty list or
// string. An empty value would let the workflow publish a credential-absent
// neutral verdict for evidence it did not actually inspect.
func TestGitMetadataEvidence_FailsClosedOnGitErrorsAndOverflow(t *testing.T) {
	collectors := []struct {
		name    string
		collect func() error
	}{
		{
			name: "commit log",
			collect: func() error {
				_, err := gitCommitMessages("base", "head")
				return err
			},
		},
		{
			name: "numstat",
			collect: func() error {
				_, err := gitBinaryPaths("base", "head")
				return err
			},
		},
		{
			name: "raw",
			collect: func() error {
				_, err := gitGitlinkPaths("base", "head")
				return err
			},
		},
	}
	for _, tt := range collectors {
		t.Run(tt.name+" error", func(t *testing.T) {
			installReviewGit(t, "exit 17\n")
			if err := tt.collect(); err == nil {
				t.Fatal("accepted unavailable Git metadata as empty evidence")
			}
		})
		t.Run(tt.name+" overflow", func(t *testing.T) {
			// 4,097 KiB is just over maxGitMetadataBytes. Keep the producer a
			// shell builtin so the test still works with the intentionally
			// restricted PATH passed to Git subprocesses.
			chunk := strings.Repeat("x", 1024)
			installReviewGit(t, "i=0\nwhile [ \"$i\" -lt 4097 ]; do\n  printf '%s' '"+chunk+"'\n  i=$((i + 1))\ndone\n")
			if err := tt.collect(); !errors.Is(err, errGitOutputLimit) {
				t.Fatalf("overflow error = %v, want %v", err, errGitOutputLimit)
			}
		})
	}
}

// TestBuildReviewPlan_MarksEveryDeterministicEscalationRelevant proves that a
// credential-free workflow invokes the command for every AIREV-07/08 input,
// not only for changed SPEC ownership. Each case deliberately has no SPEC.md,
// so review_relevant is the only signal preventing an unsafe neutral check.
func TestBuildReviewPlan_MarksEveryDeterministicEscalationRelevant(t *testing.T) {
	pathCases := []struct {
		name string
		path string
	}{
		{"provider rules", ".github/rulesets/secondary.json"},
		{"permission boundary", "agm/internal/permissionparity/parity.go"},
		{"tool hook", ".codex/hooks/pretool-guard"},
		{"security boundary", "internal/fsguard/fsguard.go"},
		{"database migration", "internal/store/migrations/0007_add_column.sql"},
		{"expensive infrastructure", "infra/managed-repo/main.tf"},
	}
	for _, tt := range pathCases {
		t.Run(tt.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			writeReviewFile(t, repo, tt.path, "changed\n")
			gittest.Run(t, repo, "add", tt.path)
			gittest.Run(t, repo, "commit", "-m", "ordinary non-SPEC change")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if plan.ReviewNeeded || !plan.ReviewRelevant || len(plan.EscalationTriggers) == 0 {
				t.Fatalf("non-SPEC %s plan = %#v", tt.name, plan)
			}
		})
	}

	t.Run("PR body marker", func(t *testing.T) {
		repo := newReviewRepo(t)
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		writeReviewFile(t, repo, "docs/ordinary.md", "changed\n")
		gittest.Run(t, repo, "add", "docs/ordinary.md")
		gittest.Run(t, repo, "commit", "-m", "ordinary documentation")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)

		plan, err := buildReviewPlanWithPRBody(context.Background(), base, head, "Please stop: HUMAN REVIEW REQUIRED")
		if err != nil {
			t.Fatal(err)
		}
		if plan.ReviewNeeded || !plan.ReviewRelevant || len(plan.EscalationTriggers) == 0 {
			t.Fatalf("PR-marker plan = %#v", plan)
		}
	})

	t.Run("commit marker", func(t *testing.T) {
		repo := newReviewRepo(t)
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		gittest.Run(t, repo, "commit", "--allow-empty", "-m", "HUMAN REVIEW REQUIRED")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)

		plan, err := buildReviewPlan(context.Background(), base, head)
		if err != nil {
			t.Fatal(err)
		}
		if plan.ReviewNeeded || !plan.ReviewRelevant || len(plan.EscalationTriggers) == 0 {
			t.Fatalf("commit-marker plan = %#v", plan)
		}
	})
}

func TestBuildReviewPlan_MarksOpaqueGitEvidenceRelevant(t *testing.T) {
	t.Run("binary numstat", func(t *testing.T) {
		repo := newReviewRepo(t)
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		if err := os.WriteFile(filepath.Join(repo, "asset.bin"), []byte{0, 1, 2}, 0o600); err != nil {
			t.Fatal(err)
		}
		gittest.Run(t, repo, "add", "asset.bin")
		gittest.Run(t, repo, "commit", "-m", "add opaque asset")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)

		plan, err := buildReviewPlan(context.Background(), base, head)
		if err != nil {
			t.Fatal(err)
		}
		if plan.ReviewNeeded || !plan.ReviewRelevant || !containsEscalation(plan.EscalationTriggers, "binary file") {
			t.Fatalf("binary plan = %#v", plan)
		}
	})

	t.Run("gitlink raw", func(t *testing.T) {
		repo := newReviewRepo(t)
		base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		gittest.Run(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+base+",deps/opaque-dependency")
		gittest.Run(t, repo, "commit", "-m", "update opaque dependency")
		head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
		chdir(t, repo)

		plan, err := buildReviewPlan(context.Background(), base, head)
		if err != nil {
			t.Fatal(err)
		}
		if plan.ReviewNeeded || !plan.ReviewRelevant || !containsEscalation(plan.EscalationTriggers, "submodule (gitlink)") {
			t.Fatalf("gitlink plan = %#v", plan)
		}
	})
}

func containsEscalation(triggers []string, want string) bool {
	for _, trigger := range triggers {
		if strings.Contains(trigger, want) {
			return true
		}
	}
	return false
}

func installReviewGit(t *testing.T, program string) {
	t.Helper()
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\n"+program), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGitOutputBounded_StripsCredentialEnvironment(t *testing.T) {
	bin := t.TempDir()
	git := filepath.Join(bin, "git")
	if err := os.WriteFile(git, []byte("#!/bin/sh\nprintf '%s' \"${ANTHROPIC_API_KEY-unset}\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ANTHROPIC_API_KEY", "must-not-reach-git")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-reach-git")
	out, err := gitOutputBounded(context.Background(), 64, "version")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "unset" {
		t.Fatalf("Git inherited credential environment: %q", out)
	}
	for _, entry := range cleanReviewGitEnv() {
		if strings.Contains(entry, "must-not-reach-git") || strings.HasPrefix(entry, "ANTHROPIC_API_KEY=") || strings.HasPrefix(entry, "AWS_SECRET_ACCESS_KEY=") {
			t.Fatalf("credential leaked into Git environment: %q", entry)
		}
	}
}

func TestLoadHeadSpecCorpus_IgnoresRevisionControlledArchiveAttributes(t *testing.T) {
	repo := gittest.NewRepo(t)
	literal := "# Contract\n\n**RAW-01** When exported, the system shall preserve $Format:%s$ literally.\n"
	writeReviewFile(t, repo, "module/SPEC.md", literal)
	writeReviewFile(t, repo, ".gitattributes", "module/SPEC.md export-subst export-ignore\n")
	gittest.Run(t, repo, "add", "module/SPEC.md", ".gitattributes")
	gittest.Run(t, repo, "commit", "-m", "attacker-controlled substitution text")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	corpus, err := loadHeadSpecCorpus(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(corpus["module/SPEC.md"]); got != literal {
		t.Fatalf("corpus bytes = %q, want exact committed blob %q", got, literal)
	}
}

func TestLoadHeadSpecCorpus_RejectsExecutableSpecObjects(t *testing.T) {
	repo := gittest.NewRepo(t)
	writeReviewFile(t, repo, "module/SPEC.md", specWithoutTrace("RAW-01", "When checked, the system shall report it."))
	gittest.Run(t, repo, "add", "module/SPEC.md")
	gittest.Run(t, repo, "update-index", "--chmod=+x", "module/SPEC.md")
	gittest.Run(t, repo, "commit", "-m", "make contract executable")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	if _, err := loadHeadSpecCorpus(context.Background(), head); err == nil || !strings.Contains(err.Error(), "not a regular non-executable blob") {
		t.Fatalf("executable SPEC error = %v", err)
	}
}

func TestSafeGitPathRejectsUntrustedNames(t *testing.T) {
	for _, path := range []string{"../SPEC.md", "bad\n/SPEC.md", "bad\u202e/SPEC.md", "/SPEC.md", "bad`/SPEC.md", `bad\\SPEC.md`} {
		if safeGitPath(path) {
			t.Errorf("safeGitPath(%q) = true", path)
		}
	}
}
