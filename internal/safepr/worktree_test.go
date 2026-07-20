package safepr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func initLinkedWorktree(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	worktree := filepath.Join(base, "worktree")
	gitTestRun(t, "init", "-q", "-b", "main", repo)
	gitTestRun(t, "-C", repo, "config", "user.name", "Safe PR Test")
	gitTestRun(t, "-C", repo, "config", "user.email", "safe-pr@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("safe-pr test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, "-C", repo, "add", "README.md")
	gitTestRun(t, "-C", repo, "commit", "-q", "-m", "initial")
	gitTestRun(t, "-C", repo, "worktree", "add", "-q", "-b", "feature", worktree)
	return repo, worktree
}

func gitTestRun(t *testing.T, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func TestWithWorktreeLockPreservesOrReleasesAcrossOutcomes(t *testing.T) {
	transactionErr := errors.New("transaction failed")
	for _, test := range []struct {
		name        string
		preexisting bool
		fail        bool
	}{
		{name: "acquired success"},
		{name: "acquired failure", fail: true},
		{name: "pre-existing success", preexisting: true},
		{name: "pre-existing failure", preexisting: true, fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, worktree := initLinkedWorktree(t)
			if test.preexisting {
				gitTestRun(t, "-C", worktree, "worktree", "lock", "--reason", "manual-owner", worktree)
			}

			err := WithWorktreeLock(worktree, "test transaction", func() error {
				state, inspectErr := inspectWorktreeLock(worktree)
				if inspectErr != nil {
					return inspectErr
				}
				if !state.locked {
					t.Fatal("worktree was not locked inside the transaction")
				}
				if test.preexisting && state.reason != "manual-owner" {
					t.Fatalf("pre-existing reason = %q, want manual-owner", state.reason)
				}
				if !test.preexisting && !strings.Contains(state.reason, "test transaction") {
					t.Fatalf("owned reason = %q, want transaction attribution", state.reason)
				}
				if test.fail {
					return transactionErr
				}
				return nil
			})
			if test.fail != errors.Is(err, transactionErr) {
				t.Fatalf("WithWorktreeLock() error = %v, transaction failure = %t", err, test.fail)
			}

			state, inspectErr := inspectWorktreeLock(worktree)
			if inspectErr != nil {
				t.Fatal(inspectErr)
			}
			if test.preexisting {
				if !state.locked || state.reason != "manual-owner" {
					t.Fatalf("pre-existing lock after transaction = %+v", state)
				}
			} else if state.locked {
				t.Fatalf("owned lock survived transaction: %+v", state)
			}
		})
	}
}

func TestWithWorktreeLockRejectsPrimaryCheckout(t *testing.T) {
	repo, _ := initLinkedWorktree(t)
	called := false
	err := WithWorktreeLock(repo, "primary", func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "primary checkout") {
		t.Fatalf("WithWorktreeLock(primary) = %v, want primary-checkout rejection", err)
	}
	if called {
		t.Fatal("transaction ran from an unprotectable primary checkout")
	}
}

func TestWithWorktreeLockPreservesReplacementOwner(t *testing.T) {
	_, worktree := initLinkedWorktree(t)
	err := WithWorktreeLock(worktree, "ownership change", func() error {
		gitTestRun(t, "-C", worktree, "worktree", "unlock", worktree)
		gitTestRun(t, "-C", worktree, "worktree", "lock", "--reason", "replacement-owner", worktree)
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "ownership changed") {
		t.Fatalf("WithWorktreeLock(replaced) = %v, want ownership-change error", err)
	}
	state, inspectErr := inspectWorktreeLock(worktree)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if !state.locked || state.reason != "replacement-owner" {
		t.Fatalf("replacement lock was changed: %+v", state)
	}
}

func TestWithWorktreeLockReclaimsStaleSafePROwner(t *testing.T) {
	_, worktree := initLinkedWorktree(t)
	staleReason := "safe-pr-owned:2147483647:0011223344556677:interrupted transaction"
	gitTestRun(t, "-C", worktree, "worktree", "lock", "--reason", staleReason, worktree)

	called := false
	if err := WithWorktreeLock(worktree, "replacement transaction", func() error {
		called = true
		state, inspectErr := inspectWorktreeLock(worktree)
		if inspectErr != nil {
			return inspectErr
		}
		if !state.locked || state.reason == staleReason || !strings.Contains(state.reason, "replacement transaction") {
			t.Fatalf("reclaimed lock during transaction = %+v", state)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("transaction did not run after reclaiming stale safe-pr owner")
	}
	state, err := inspectWorktreeLock(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if state.locked {
		t.Fatalf("replacement owned lock survived transaction: %+v", state)
	}
}

func TestWithWorktreeLockRejectsActiveSafePROwner(t *testing.T) {
	_, worktree := initLinkedWorktree(t)
	activeReason := fmt.Sprintf("safe-pr-owned:%d:0011223344556677:active transaction", os.Getpid())
	gitTestRun(t, "-C", worktree, "worktree", "lock", "--reason", activeReason, worktree)

	called := false
	err := WithWorktreeLock(worktree, "overlapping transaction", func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "active safe-pr transaction") {
		t.Fatalf("WithWorktreeLock(active owner) = %v, want active-owner rejection", err)
	}
	if called {
		t.Fatal("overlapping transaction ran while safe-pr owner was live")
	}
	state, inspectErr := inspectWorktreeLock(worktree)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if !state.locked || state.reason != activeReason {
		t.Fatalf("active safe-pr lock changed: %+v", state)
	}
}

func TestSafePROwnerPIDRequiresGeneratedOwnershipShape(t *testing.T) {
	valid := fmt.Sprintf("safe-pr-owned:%d:0011223344556677:transaction", os.Getpid())
	pid, ok := safePROwnerPID(valid)
	if !ok || pid != os.Getpid() {
		t.Fatalf("safePROwnerPID(%q) = %d, %t", valid, pid, ok)
	}
	for _, reason := range []string{
		"manual-owner",
		"safe-pr-owned:not-a-pid:0011223344556677:transaction",
		"safe-pr-owned:1:short:transaction",
		"safe-pr-owned:1:0011223344556677",
	} {
		if pid, ok := safePROwnerPID(reason); ok {
			t.Errorf("safePROwnerPID(%q) = %d, true; want malformed ownership rejected", reason, pid)
		}
	}
}

func TestWithWorktreeLockReleasesOwnedLockAfterPanic(t *testing.T) {
	_, worktree := initLinkedWorktree(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("WithWorktreeLock did not propagate the transaction panic")
			}
		}()
		_ = WithWorktreeLock(worktree, "panic transaction", func() error {
			panic("simulated panic")
		})
	}()
	state, err := inspectWorktreeLock(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if state.locked {
		t.Fatalf("owned lock survived panic: %+v", state)
	}
}

func TestWorktreeGitCommandIsBoundedAndGroupCancelable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeCommandTimeout)
	defer cancel()
	cmd := newWorktreeGitCommand(ctx, "/tmp", "status")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("worktree git command must run in an isolated process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("worktree git command must cancel its process group")
	}
	if cmd.WaitDelay != time.Second {
		t.Fatalf("worktree git command WaitDelay = %v, want %v", cmd.WaitDelay, time.Second)
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("cancel before start = %v", err)
	}
}
