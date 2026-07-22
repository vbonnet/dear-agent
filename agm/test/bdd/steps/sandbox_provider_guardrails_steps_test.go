package steps

import (
	"strings"
	"testing"
)

func TestScopedWorktreeInventoryIgnoresExternalConcurrency(t *testing.T) {
	const root = "/repo/current"
	before := worktreeRecord(root, "aaaa") + "\n\n" + worktreeRecord("/repo/sibling-one", "bbbb")
	after := worktreeRecord(root, "aaaa") + "\n\n" + worktreeRecord("/repo/sibling-two", "cccc")

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
	const root = "/repo/current"
	before := worktreeRecord(root, "aaaa")
	after := before + "\n\n" + worktreeRecord(root+"/.worktrees/leaked-sandbox", "dddd")

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

func worktreeRecord(path, head string) string {
	return "worktree " + path + "\nHEAD " + head + "\ndetached"
}
