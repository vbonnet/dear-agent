package safepr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The helper is a second race-instrumented test process. Allow enough startup
// time for it to reach the fake Git binary while the full repository suite is
// running in parallel.
const worktreeTestHelperStartTimeout = 30 * time.Second

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

func TestCleanupWorktreesScriptPreservesBranchWhenProtectedWorktreeRemovalFails(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	repo := filepath.Join(base, "repo")
	worktree := filepath.Join(base, "protected-worktree")
	gitTestRun(t, "init", "-q", "--bare", remote)
	gitTestRun(t, "init", "-q", "-b", "main", repo)
	gitTestRun(t, "-C", repo, "config", "user.name", "Safe PR Test")
	gitTestRun(t, "-C", repo, "config", "user.email", "safe-pr@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("safe-pr cleanup test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, "-C", repo, "add", "README.md")
	gitTestRun(t, "-C", repo, "commit", "-q", "-m", "initial")
	gitTestRun(t, "-C", repo, "remote", "add", "origin", remote)
	gitTestRun(t, "-C", repo, "push", "-q", "-u", "origin", "main")
	gitTestRun(t, "-C", repo, "worktree", "add", "-q", "-b", "protected", worktree)
	gitTestRun(t, "-C", repo, "push", "-q", "-u", "origin", "protected")
	gitTestRun(t, "-C", repo, "worktree", "lock", "--reason", "safe-pr transaction", worktree)

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve cleanup-worktrees.sh: runtime.Caller failed")
	}
	script := filepath.Join(filepath.Dir(sourceFile), "..", "..", "scripts", "cleanup-worktrees.sh")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script, repo, "--fix", "--max-age", "0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("cleanup protected worktree error = %v, want exit 3; output:\n%s", err, out)
	}
	if !strings.Contains(string(out), "skipped branch cleanup for protected") {
		t.Fatalf("cleanup output did not explain preserved branches:\n%s", out)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("protected worktree was removed: %v", err)
	}
	gitTestRun(t, "-C", repo, "show-ref", "--verify", "refs/heads/protected")
	gitTestRun(t, "-C", repo, "ls-remote", "--exit-code", "origin", "refs/heads/protected")
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

func TestWithWorktreeLockReclaimsStaleSafePROwnerDespiteLivePID(t *testing.T) {
	_, worktree := initLinkedWorktree(t)
	staleReason := fmt.Sprintf("safe-pr-owned:%d:0011223344556677:interrupted transaction", os.Getpid())
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

func TestWithWorktreeLockSerializesConcurrentOwners(t *testing.T) {
	_, worktree := initLinkedWorktree(t)
	identity, err := resolveLinkedWorktree(worktree)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan string, 1)
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- WithWorktreeLock(worktree, "first transaction", func() error {
			state, inspectErr := inspectWorktreeLock(worktree)
			if inspectErr != nil {
				return inspectErr
			}
			entered <- state.reason
			<-release
			return nil
		})
	}()

	var firstReason string
	select {
	case firstReason = <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("first transaction did not acquire worktree protection")
	}
	secondCalled := false
	err = withWorktreeTransactionLock(identity.gitDir, 50*time.Millisecond, func(*WorktreeTransaction) error {
		secondCalled = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "another safe-pr transaction") {
		t.Fatalf("concurrent transaction = %v, want serialization rejection", err)
	}
	if secondCalled {
		t.Fatal("concurrent transaction ran while the first owner was live")
	}
	state, inspectErr := inspectWorktreeLock(worktree)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if !state.locked || state.reason != firstReason {
		t.Fatalf("first transaction lock changed during contention: %+v", state)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	state, inspectErr = inspectWorktreeLock(worktree)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if state.locked {
		t.Fatalf("first transaction lock survived release: %+v", state)
	}
	secondEntered := false
	if err := WithWorktreeLock(worktree, "second transaction", func() error {
		secondEntered = true
		state, inspectErr := inspectWorktreeLock(worktree)
		if inspectErr != nil {
			return inspectErr
		}
		if !state.locked || !strings.Contains(state.reason, "second transaction") {
			t.Fatalf("second owner lock after serialized release = %+v", state)
		}
		return nil
	}); err != nil {
		t.Fatalf("second transaction after release = %v", err)
	}
	if !secondEntered {
		t.Fatal("second transaction did not enter after serialized release")
	}
}

func TestSafePROwnedReasonRequiresGeneratedOwnershipShape(t *testing.T) {
	valid := fmt.Sprintf("safe-pr-owned:%d:0011223344556677:transaction", os.Getpid())
	if !isSafePROwnedLockReason(valid) {
		t.Fatalf("isSafePROwnedLockReason(%q) = false", valid)
	}
	for _, reason := range []string{
		"manual-owner",
		"safe-pr-owned:not-a-pid:0011223344556677:transaction",
		"safe-pr-owned:1:short:transaction",
		"safe-pr-owned:1:0011223344556677",
	} {
		if isSafePROwnedLockReason(reason) {
			t.Errorf("isSafePROwnedLockReason(%q) = true; want malformed ownership rejected", reason)
		}
	}
}

func TestWorktreeParsersAcceptUnixAndWindowsLineEndings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktree")
	directories := root + "\n" + filepath.Join(root, ".git", "worktrees", "feature") + "\n" + filepath.Join(root, ".git") + "\n"
	porcelain := "worktree " + filepath.Join(filepath.Dir(root), "repo") + "\nHEAD abc\n\n" +
		"worktree " + root + "\nHEAD def\nlocked safe-pr-owned:123:0011223344556677:test\n"

	for _, test := range []struct {
		name       string
		lineEnding string
	}{
		{name: "LF", lineEnding: "\n"},
		{name: "CRLF", lineEnding: "\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			identity, err := parseLinkedWorktree(strings.ReplaceAll(directories, "\n", test.lineEnding))
			if err != nil {
				t.Fatal(err)
			}
			if identity.root != root {
				t.Fatalf("linked root = %q, want %q", identity.root, root)
			}
			state, err := parseWorktreeLock(root, strings.ReplaceAll(porcelain, "\n", test.lineEnding))
			if err != nil {
				t.Fatal(err)
			}
			if !state.locked || state.reason != "safe-pr-owned:123:0011223344556677:test" {
				t.Fatalf("worktree lock = %+v", state)
			}
		})
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

func TestWorktreeTransactionHelper(t *testing.T) {
	if os.Getenv("SAFEPR_TRANSACTION_HELPER") != "1" {
		return
	}
	worktree := os.Getenv("SAFEPR_TRANSACTION_WORKTREE")
	marker := os.Getenv("SAFEPR_TRANSACTION_MARKER")
	if err := WithWorktreeTransaction(worktree, "parent-death helper", func(transaction *WorktreeTransaction) error {
		cmd := exec.Command("sh", "-c", `touch "$SAFEPR_TRANSACTION_MARKER"; while [ ! -e "$SAFEPR_TRANSACTION_RELEASE" ]; do sleep 0.01; done`)
		cmd.Env = os.Environ()
		if err := transaction.ProtectCommand(cmd); err != nil {
			return err
		}
		return cmd.Run()
	}); err != nil {
		t.Fatalf("transaction helper: %v (marker %s)", err, marker)
	}
}

func TestWorktreeTransactionLockOutlivesKilledParentForProtectedChild(t *testing.T) {
	_, worktree := initLinkedWorktree(t)
	identity, err := resolveLinkedWorktree(worktree)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "child-started")
	release := filepath.Join(t.TempDir(), "release-child")
	helper := exec.Command(os.Args[0], "-test.run=^TestWorktreeTransactionHelper$")
	helper.Env = append(os.Environ(),
		"SAFEPR_TRANSACTION_HELPER=1",
		"SAFEPR_TRANSACTION_WORKTREE="+worktree,
		"SAFEPR_TRANSACTION_MARKER="+marker,
		"SAFEPR_TRANSACTION_RELEASE="+release,
	)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(release, nil, 0o600)
		if helper.ProcessState == nil {
			_ = helper.Process.Kill()
			_ = helper.Wait()
		}
	})

	deadline := time.Now().Add(worktreeTestHelperStartTimeout)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("protected child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()
	state, inspectErr := inspectWorktreeLock(worktree)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if !state.locked || !strings.Contains(state.reason, "parent-death helper") {
		t.Fatalf("Git lock after parent death = %+v, want inherited transaction protection", state)
	}

	entered := false
	err = withWorktreeTransactionLock(identity.gitDir, 100*time.Millisecond, func(*WorktreeTransaction) error {
		entered = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "another safe-pr transaction") {
		t.Fatalf("replacement while protected child lives = %v, want serialization rejection", err)
	}
	if entered {
		t.Fatal("replacement entered while killed parent child retained transaction")
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	releaseDeadline := time.Now().Add(4 * time.Second)
	for {
		err = withWorktreeTransactionLock(identity.gitDir, 100*time.Millisecond, func(*WorktreeTransaction) error {
			entered = true
			return nil
		})
		if err == nil {
			break
		}
		if time.Now().After(releaseDeadline) {
			t.Fatalf("transaction remained locked after protected child exit: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !entered {
		t.Fatal("replacement did not enter after protected child exit")
	}
	entered = false
	if err := WithWorktreeTransaction(worktree, "replacement after child exit", func(*WorktreeTransaction) error {
		entered = true
		return nil
	}); err != nil {
		t.Fatalf("replacement transaction after child exit = %v", err)
	}
	if !entered {
		t.Fatal("replacement transaction did not reclaim the interrupted Git lock")
	}
	state, inspectErr = inspectWorktreeLock(worktree)
	if inspectErr != nil {
		t.Fatal(inspectErr)
	}
	if state.locked {
		t.Fatalf("replacement owned Git lock survived release: %+v", state)
	}
}

func TestWorktreeGitTransactionHelper(t *testing.T) {
	if os.Getenv("SAFEPR_GIT_TRANSACTION_HELPER") != "1" {
		return
	}
	gitDir := os.Getenv("SAFEPR_GIT_TRANSACTION_DIR")
	if err := withWorktreeTransactionLock(gitDir, time.Second, func(transaction *WorktreeTransaction) error {
		_, err := runWorktreeGitProtected(transaction, gitDir, "worktree", "list", "--porcelain")
		return err
	}); err != nil {
		t.Fatalf("protected Git helper: %v", err)
	}
}

func TestWorktreeTransactionLockOutlivesKilledParentForGitHelper(t *testing.T) {
	gitDir := t.TempDir()
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "git-helper-started")
	release := filepath.Join(t.TempDir(), "release-git-helper")
	fakeGit := filepath.Join(binDir, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\n: > \"$SAFEPR_GIT_HELPER_MARKER\"\nwhile [ ! -e \"$SAFEPR_GIT_HELPER_RELEASE\" ]; do sleep 0.01; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	helper := exec.Command(os.Args[0], "-test.run=^TestWorktreeGitTransactionHelper$")
	helper.Env = append(os.Environ(),
		"SAFEPR_GIT_TRANSACTION_HELPER=1",
		"SAFEPR_GIT_TRANSACTION_DIR="+gitDir,
		"SAFEPR_GIT_HELPER_MARKER="+marker,
		"SAFEPR_GIT_HELPER_RELEASE="+release,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(release, nil, 0o600)
		if helper.ProcessState == nil {
			_ = helper.Process.Kill()
			_ = helper.Wait()
		}
	})

	deadline := time.Now().Add(worktreeTestHelperStartTimeout)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("protected Git helper did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()

	entered := false
	err := withWorktreeTransactionLock(gitDir, 100*time.Millisecond, func(*WorktreeTransaction) error {
		entered = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "another safe-pr transaction") {
		t.Fatalf("replacement while Git helper lives = %v, want serialization rejection", err)
	}
	if entered {
		t.Fatal("replacement entered while killed parent's Git helper retained the transaction")
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	releaseDeadline := time.Now().Add(4 * time.Second)
	for {
		err = withWorktreeTransactionLock(gitDir, 100*time.Millisecond, func(*WorktreeTransaction) error {
			entered = true
			return nil
		})
		if err == nil {
			break
		}
		if time.Now().After(releaseDeadline) {
			t.Fatalf("transaction remained locked after Git helper exit: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !entered {
		t.Fatal("replacement did not enter after protected Git helper exit")
	}
}

func TestWorktreeTransactionGuardUsesNonGitLockName(t *testing.T) {
	if strings.HasSuffix(worktreeTransactionLockFile, ".lock") {
		t.Fatalf("transaction guard %q is eligible for stale Git-lock cleanup", worktreeTransactionLockFile)
	}
}
