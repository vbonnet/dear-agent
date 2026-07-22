package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopedWorktreeInventoryIgnoresExternalConcurrency(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "current")
	siblingOne := filepath.Join(base, "sibling-one")
	siblingTwo := filepath.Join(base, "sibling-two")
	for _, path := range []string{root, siblingOne, siblingTwo} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	before := worktreeRecord(root, "aaaa") + "\n\n" + worktreeRecord(siblingOne, "bbbb")
	after := worktreeRecord(root, "aaaa") + "\n\n" + worktreeRecord(siblingTwo, "cccc")

	gotBefore, err := scopedWorktreeInventory(root, before)
	if err != nil {
		t.Fatal(err)
	}
	gotAfter, err := scopedWorktreeInventory(root, after)
	if err != nil {
		t.Fatal(err)
	}
	if gotBefore != gotAfter {
		t.Fatalf("external worktree churn changed scoped inventory:\nbefore:\n%s\nafter:\n%s", gotBefore, gotAfter)
	}
}

func TestScopedWorktreeInventoryDetectsInvokingRepositoryLeak(t *testing.T) {
	root := filepath.Join(t.TempDir(), "current")
	leaked := filepath.Join(root, ".worktrees", "leaked-sandbox")
	if err := os.MkdirAll(leaked, 0o755); err != nil {
		t.Fatal(err)
	}
	before := worktreeRecord(root, "aaaa")
	after := before + "\n\n" + worktreeRecord(leaked, "dddd")

	gotBefore, err := scopedWorktreeInventory(root, before)
	if err != nil {
		t.Fatal(err)
	}
	gotAfter, err := scopedWorktreeInventory(root, after)
	if err != nil {
		t.Fatal(err)
	}
	if gotBefore == gotAfter {
		t.Fatalf("in-repository leak was ignored:\n%s", gotAfter)
	}
	if !strings.Contains(gotAfter, "leaked-sandbox") {
		t.Fatalf("scoped inventory omitted leaked sandbox:\n%s", gotAfter)
	}
}

func TestScopedWorktreeInventoryRejectsMalformedRecord(t *testing.T) {
	if _, err := scopedWorktreeInventory("/repo/current", "HEAD aaaa"); err == nil {
		t.Fatal("expected malformed worktree record error")
	}
}

func TestScopedWorktreeInventoryCanonicalizesSymlinkedCheckout(t *testing.T) {
	physicalRoot := filepath.Join(t.TempDir(), "physical-checkout")
	leaked := filepath.Join(physicalRoot, ".worktrees", "leaked-sandbox")
	if err := os.MkdirAll(leaked, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkRoot := filepath.Join(t.TempDir(), "checkout-link")
	if err := os.Symlink(physicalRoot, symlinkRoot); err != nil {
		t.Fatal(err)
	}

	porcelain := worktreeRecord(physicalRoot, "aaaa") + "\n\n" + worktreeRecord(leaked, "bbbb")
	got, err := scopedWorktreeInventory(symlinkRoot, porcelain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "leaked-sandbox") {
		t.Fatalf("symlinked checkout omitted in-repository leak:\n%s", got)
	}
}

func TestScopedWorktreeInventoryIgnoresMissingExternalWorktree(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "current")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	missingSibling := filepath.Join(base, "removed-sibling")
	porcelain := worktreeRecord(root, "aaaa") + "\n\n" + worktreeRecord(missingSibling, "bbbb") +
		"\nprunable gitdir file points to non-existent location"

	got, err := scopedWorktreeInventory(root, porcelain)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "removed-sibling") {
		t.Fatalf("missing external worktree changed scoped inventory:\n%s", got)
	}
}

func worktreeRecord(path, head string) string {
	return "worktree " + path + "\nHEAD " + head + "\ndetached"
}
