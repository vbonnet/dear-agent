package deploy

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubBuild swaps buildBinary for the duration of a test. The stub writes the
// given content to outPath (or returns buildErr without writing).
func stubBuild(t *testing.T, content string, buildErr error) {
	t.Helper()
	orig := buildBinary
	buildBinary = func(_, _, outPath string) error {
		if buildErr != nil {
			return buildErr
		}
		return os.WriteFile(outPath, []byte(content), 0o644)
	}
	t.Cleanup(func() { buildBinary = orig })
}

func TestAtomicInstall_Success(t *testing.T) {
	repo, oldSha, headSha := gitRepo(t)
	stubBuild(t, "fresh-binary", nil)
	stubVersion(t, headSha, false, nil) // built HEAD == origin/main tip

	target := filepath.Join(t.TempDir(), "bin", "agm")
	// pre-existing (old) binary at the target: it must be atomically replaced.
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := AtomicInstall("./cmd/agm", target, headSha, Options{RepoRoot: repo})
	if err != nil {
		t.Fatalf("AtomicInstall: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fresh-binary" {
		t.Errorf("target content = %q, want the freshly built binary", got)
	}
	if res.Revision != shortRev(headSha) {
		t.Errorf("res.Revision = %q, want %q", res.Revision, shortRev(headSha))
	}
	// no temp binary left behind in the target dir.
	assertNoTempLeftover(t, filepath.Dir(target), filepath.Base(target))
	_ = oldSha
}

func TestAtomicInstall_AncestorPasses(t *testing.T) {
	repo, oldSha, headSha := gitRepo(t)
	stubBuild(t, "fresh", nil)
	stubVersion(t, oldSha, false, nil) // built an older on-trunk commit

	target := filepath.Join(t.TempDir(), "agm")
	if _, err := AtomicInstall("./cmd/agm", target, headSha, Options{RepoRoot: repo}); err != nil {
		t.Fatalf("ancestor of origin/main should install: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("target not installed: %v", err)
	}
}

func TestAtomicInstall_RefusesOffHistory(t *testing.T) {
	repo, _, headSha := gitRepo(t)
	stubBuild(t, "fresh", nil)
	// a sha that is not in this repo's history at all (the June-14 scenario).
	stubVersion(t, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", false, nil)

	target := filepath.Join(t.TempDir(), "agm")
	_, err := AtomicInstall("./cmd/agm", target, headSha, Options{RepoRoot: repo})
	if err == nil {
		t.Fatal("expected refusal for off-history build")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Errorf("error = %v, want a refusal", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("target must NOT be written when the gate refuses")
	}
	assertNoTempLeftover(t, filepath.Dir(target), "agm")
}

func TestAtomicInstall_FailsLoudOnBuildError(t *testing.T) {
	repo, _, headSha := gitRepo(t)
	stubBuild(t, "", os.ErrPermission) // build blows up
	stubVersion(t, headSha, false, nil)

	target := filepath.Join(t.TempDir(), "agm")
	_, err := AtomicInstall("./cmd/agm", target, headSha, Options{RepoRoot: repo})
	if err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Fatalf("want loud build failure, got %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("target must not exist after a failed build")
	}
	assertNoTempLeftover(t, filepath.Dir(target), "agm")
}

func TestAtomicInstall_FailsLoudOnUnstamped(t *testing.T) {
	repo, _, headSha := gitRepo(t)
	stubBuild(t, "fresh", nil)
	stubVersion(t, "", false, nil) // no vcs.revision stamp => broken build env

	target := filepath.Join(t.TempDir(), "agm")
	_, err := AtomicInstall("./cmd/agm", target, headSha, Options{RepoRoot: repo})
	if err == nil || !strings.Contains(err.Error(), "no vcs.revision") {
		t.Fatalf("want loud unstamped failure, got %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("target must not exist for an unstamped build")
	}
}

func TestAtomicInstall_LinkedWorktreeRetriesCleanCloneForUnstampedBuild(t *testing.T) {
	repo, _, headSha := gitRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "--detach", linked, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("create linked worktree: %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", linked).Run() })

	origBuild, origFallback, origVersion := buildBinary, buildFromCleanClone, readBinaryVersion
	buildBinary = func(_, _, outPath string) error { return os.WriteFile(outPath, []byte("unstamped"), 0o644) }
	usedFallback := false
	buildFromCleanClone = func(_, _, outPath string) error {
		usedFallback = true
		return os.WriteFile(outPath, []byte("stamped"), 0o644)
	}
	reads := 0
	readBinaryVersion = func(string) (string, bool, error) {
		reads++
		if reads == 1 {
			return "", false, nil
		}
		return headSha, false, nil
	}
	t.Cleanup(func() {
		buildBinary = origBuild
		buildFromCleanClone = origFallback
		readBinaryVersion = origVersion
	})

	target := filepath.Join(t.TempDir(), "agm")
	if _, err := AtomicInstall("./cmd/agm", target, headSha, Options{RepoRoot: linked}); err != nil {
		t.Fatalf("AtomicInstall: %v", err)
	}
	if !usedFallback {
		t.Fatal("expected clean-clone fallback for unstamped linked-worktree build")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "stamped" {
		t.Fatalf("installed content = %q, %v; want stamped", got, err)
	}
}

func TestRunGitCheckout_UsesCloneFallbackDeadline(t *testing.T) {
	orig := gitCheckout
	gitCheckout = func(ctx context.Context, _ string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("checkout context must have a deadline")
		}
		if remaining := time.Until(deadline); remaining < cloneTimeout-time.Second {
			t.Fatalf("checkout deadline leaves %s; want approximately %s", remaining, cloneTimeout)
		}
		return nil, nil
	}
	t.Cleanup(func() { gitCheckout = orig })

	if err := runGitCheckout(t.TempDir()); err != nil {
		t.Fatalf("checkout must receive the standalone-clone deadline: %v", err)
	}
}

func TestGoBuild_UsesColdBuildDeadline(t *testing.T) {
	if buildTimeout != 10*time.Minute {
		t.Fatalf("buildTimeout = %s; want 10m", buildTimeout)
	}

	orig := goBuildCommand
	goBuildCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("build context must have a deadline")
		}
		if remaining := time.Until(deadline); remaining < buildTimeout-time.Second {
			t.Fatalf("build deadline leaves %s; want approximately %s", remaining, buildTimeout)
		}
		if name != "go" || strings.Join(args, " ") != "build -buildvcs=true -o /tmp/agm ./agm/cmd/agm" {
			t.Fatalf("build command = %q %q", name, args)
		}
		return exec.CommandContext(ctx, "true")
	}
	t.Cleanup(func() { goBuildCommand = orig })

	if err := goBuild(t.TempDir(), "./agm/cmd/agm", "/tmp/agm"); err != nil {
		t.Fatalf("goBuild: %v", err)
	}
}

func TestAtomicInstall_FailsLoudOnBadSourceRef(t *testing.T) {
	repo, _, _ := gitRepo(t)
	stubBuild(t, "fresh", nil)

	target := filepath.Join(t.TempDir(), "agm")
	_, err := AtomicInstall("./cmd/agm", target, "origin/does-not-exist", Options{RepoRoot: repo})
	if err == nil || !strings.Contains(err.Error(), "cannot resolve") {
		t.Fatalf("want loud unresolvable-ref failure, got %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Error("target must not exist when the source ref cannot resolve")
	}
}

func TestAtomicInstall_ResolvesRelativeTarget(t *testing.T) {
	repo, _, headSha := gitRepo(t)
	stubBuild(t, "fresh", nil)
	stubVersion(t, headSha, false, nil)

	base := t.TempDir()
	t.Chdir(base) // build runs in repoRoot; a relative target must still resolve here
	res, err := AtomicInstall("./cmd/agm", "bin/agm", headSha, Options{RepoRoot: repo})
	if err != nil {
		t.Fatalf("relative target should install: %v", err)
	}
	if !filepath.IsAbs(res.Target) {
		t.Errorf("res.Target = %q, want absolute", res.Target)
	}
	if _, err := os.Stat(filepath.Join(base, "bin", "agm")); err != nil {
		t.Errorf("installed binary not at the resolved absolute path: %v", err)
	}
}

func TestAtomicInstall_RequiresArgs(t *testing.T) {
	if _, err := AtomicInstall("", "/x", "HEAD", Options{RepoRoot: "/r"}); err == nil {
		t.Error("empty pkg should error")
	}
	if _, err := AtomicInstall("./x", "/x", "HEAD", Options{}); err == nil {
		t.Error("empty RepoRoot should error")
	}
}

// assertNoTempLeftover fails if any temp install file (.<name>.install-*) is
// left in dir — the defer cleanup must remove it on every failure path.
func assertNoTempLeftover(t *testing.T, dir, name string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // dir may not exist (nothing created) — that's fine
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "."+name+".install-") {
			t.Errorf("temp install file left behind: %s", e.Name())
		}
	}
}
