package main

import (
	"context"
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
