package gittest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// hookNames are the hooks a normal create/commit/merge sequence would fire.
// post-merge is the one that reaped two live worktrees in audit finding F-01.
var hookNames = []string{
	"pre-commit", "commit-msg", "post-commit",
	"pre-merge-commit", "post-merge", "post-checkout", "pre-push",
}

// poisonHost points the process at a fake "host" whose global Git
// configuration installs a canary hook for every name in hookNames. It
// returns the canary path. Nothing outside the test's temporary directories
// is touched: HOME and the Git config variables are redirected first, so the
// developer's real hooks are unreachable for the rest of the test.
func poisonHost(t *testing.T) (canary string) {
	t.Helper()

	root := t.TempDir()
	canary = filepath.Join(root, "canary")
	hooks := filepath.Join(root, "hooks")
	home := filepath.Join(root, "home")
	for _, dir := range []string{hooks, home} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create %s: %v", dir, err)
		}
	}

	script := "#!/bin/sh\necho fired >> " + canary + "\nexit 0\n"
	for _, name := range hookNames {
		if err := os.WriteFile(filepath.Join(hooks, name), []byte(script), 0o700); err != nil {
			t.Fatalf("write hook %s: %v", name, err)
		}
	}

	config := filepath.Join(home, ".gitconfig")
	contents := "[core]\n\thooksPath = " + hooks + "\n" +
		"[user]\n\tname = poisoned host\n\temail = host@example.invalid\n" +
		"[init]\n\tdefaultBranch = main\n"
	if err := os.WriteFile(config, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", config, err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	return canary
}

func canaryFired(t *testing.T, canary string) bool {
	t.Helper()
	_, err := os.Stat(canary)
	if err == nil {
		return true
	}
	if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", canary, err)
	}
	return false
}

// rawGit runs Git the way an unprotected test does: no explicit environment,
// so the process environment (now poisoned) is inherited.
func rawGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("raw git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// exercise drives the create/commit/branch/merge sequence that fired the host
// hooks in audit finding F-01, using the supplied Git runner.
func exercise(t *testing.T, dir string, run func(dir string, args ...string)) {
	t.Helper()
	run(dir, "init", "-b", "main")
	write(t, filepath.Join(dir, "README.md"), "# repo\n")
	run(dir, "add", "README.md")
	run(dir, "commit", "-m", "initial commit")
	run(dir, "checkout", "-b", "topic")
	write(t, filepath.Join(dir, "topic.txt"), "topic\n")
	run(dir, "add", "topic.txt")
	run(dir, "commit", "-m", "topic commit")
	run(dir, "checkout", "main")
	run(dir, "merge", "--no-ff", "-m", "merge topic", "topic")
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestHostHooksFireWithoutIsolation is the positive control for
// TestSandboxRepositoriesCannotRunHostHooks. Without it, a canary that never
// fires proves nothing: the hooks could be misinstalled and the isolation
// assertion would pass vacuously.
func TestHostHooksFireWithoutIsolation(t *testing.T) {
	canary := poisonHost(t)
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}

	exercise(t, dir, func(dir string, args ...string) { rawGit(t, dir, args...) })

	if !canaryFired(t, canary) {
		t.Fatal("control failed: host hooks did not fire for an unisolated repository, " +
			"so the isolation assertion below would be vacuous")
	}
}

// TestSandboxRepositoriesCannotRunHostHooks is the F-01 regression: the same
// sequence, run through gittest, must not reach a single host hook.
func TestSandboxRepositoriesCannotRunHostHooks(t *testing.T) {
	canary := poisonHost(t)
	sandbox := gittest.New(t)
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}

	exercise(t, dir, func(dir string, args ...string) { sandbox.Run(t, dir, args...) })

	if canaryFired(t, canary) {
		contents, _ := os.ReadFile(canary)
		t.Fatalf("sandboxed repository executed a host hook (%d invocation(s)):\n%s",
			strings.Count(string(contents), "fired"), contents)
	}
}

// TestSandboxIgnoresHostGlobalConfiguration proves the sandbox does not merely
// blank the hooks path: no host global setting reaches the subprocess.
func TestSandboxIgnoresHostGlobalConfiguration(t *testing.T) {
	poisonHost(t)
	sandbox := gittest.New(t)
	repo := sandbox.NewRepo(t)

	got := strings.TrimSpace(sandbox.Run(t, repo, "config", "--get", "core.hooksPath"))
	if got != sandbox.HooksDir {
		t.Fatalf("core.hooksPath = %q, want the sandbox hooks dir %q", got, sandbox.HooksDir)
	}
	if entries, err := os.ReadDir(sandbox.HooksDir); err != nil || len(entries) != 0 {
		t.Fatalf("sandbox hooks dir must stay empty: %d entries, err %v", len(entries), err)
	}

	name := strings.TrimSpace(sandbox.Run(t, repo, "config", "--get", "user.name"))
	if name == "poisoned host" {
		t.Fatal("sandbox inherited the host global user.name")
	}
}

// TestSandboxEnvironmentDropsHostGitVariables proves an inherited GIT_*
// variable cannot re-enable a host hook path through the environment.
func TestSandboxEnvironmentDropsHostGitVariables(t *testing.T) {
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.hooksPath=/host/hooks'")
	t.Setenv("GIT_DIR", "/host/.git")
	t.Setenv("GIT_WORK_TREE", "/host")

	env := gittest.New(t).Env()
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "GIT_CONFIG_PARAMETERS="),
			strings.HasPrefix(kv, "GIT_DIR="),
			strings.HasPrefix(kv, "GIT_WORK_TREE="):
			t.Errorf("sandbox environment leaked host variable %q", kv)
		}
	}
}

func TestHardenRepoOverridesCommandScopeHooksForProductionCommands(t *testing.T) {
	sandbox := gittest.New(t)
	repo := sandbox.NewRepo(t)
	hostHooks := filepath.Join(t.TempDir(), "host-hooks")
	if err := os.MkdirAll(hostHooks, 0o700); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(t.TempDir(), "host-hook-fired")
	hook := filepath.Join(hostHooks, "post-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf fired >>"+canary+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hostHooks)
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.hooksPath="+hostHooks+"'")
	sandbox.HardenRepo(t, repo)

	tracked := filepath.Join(repo, "production-path.txt")
	if err := os.WriteFile(tracked, []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", repo, "add", "production-path.txt"},
		{"-C", repo, "commit", "-m", "exercise inherited production environment"},
	} {
		output, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("raw production-style git %v failed: %v\n%s", args, err, output)
		}
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("command-scope host hook executed; stat error = %v", err)
	}
}

// TestSandboxRedirectsGlobalConfigWrites proves `git config --global` in a
// test lands inside the sandbox instead of the developer's ~/.gitconfig.
func TestSandboxRedirectsGlobalConfigWrites(t *testing.T) {
	poisonHost(t)
	hostConfig := os.Getenv("GIT_CONFIG_GLOBAL")
	before, err := os.ReadFile(hostConfig)
	if err != nil {
		t.Fatalf("read %s: %v", hostConfig, err)
	}

	sandbox := gittest.New(t)
	repo := sandbox.NewRepo(t)
	sandbox.Run(t, repo, "config", "--global", "dearagent.canary", "written")

	after, err := os.ReadFile(hostConfig)
	if err != nil {
		t.Fatalf("read %s: %v", hostConfig, err)
	}
	if string(before) != string(after) {
		t.Fatalf("sandboxed `git config --global` mutated the host config %s", hostConfig)
	}
	written, err := os.ReadFile(sandbox.ConfigFile)
	if err != nil {
		t.Fatalf("read %s: %v", sandbox.ConfigFile, err)
	}
	if !strings.Contains(string(written), "canary") {
		t.Fatalf("sandbox config %s did not receive the write:\n%s", sandbox.ConfigFile, written)
	}
}

// TestDefaultSandboxIsStablePerTest proves the package-level helpers reuse one
// sandbox per test, so a repository created by InitRepo stays readable by a
// later Run from the same test.
func TestDefaultSandboxIsStablePerTest(t *testing.T) {
	first := gittest.Default(t)
	if second := gittest.Default(t); first != second {
		t.Fatal("Default returned a different sandbox for the same test")
	}
	repo := gittest.NewRepo(t)
	if out := gittest.Run(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); strings.TrimSpace(out) != "main" {
		t.Fatalf("default sandbox repo is not on main: %q", out)
	}
}

// TestProductionStyleGitCannotRunHostHooksInSandboxRepos covers the hazard one
// layer below the test file. Production Git wrappers build their own
// *exec.Cmd and never set Cmd.Env, so pointing one at a temporary repository
// re-creates F-01 inside the code under test — the migration of the test call
// sites does not reach them.
//
// rawGit here is exactly that shape: inherited environment, poisoned HOME,
// repository created by the sandbox. It must still be unable to reach a host
// hook, because InitRepo plants the empty hooks path in the repository's own
// config and repository configuration outranks global configuration.
func TestProductionStyleGitCannotRunHostHooksInSandboxRepos(t *testing.T) {
	canary := poisonHost(t)
	sandbox := gittest.New(t)
	repo := sandbox.NewRepo(t)

	// Every command below is raw: no sandbox environment, no -c overrides.
	write(t, filepath.Join(repo, "topic.txt"), "topic\n")
	rawGit(t, repo, "add", "topic.txt")
	rawGit(t, repo, "-c", "user.name=t", "-c", "user.email=t@t.invalid", "commit", "-m", "raw commit")
	rawGit(t, repo, "checkout", "-b", "topic")
	rawGit(t, repo, "checkout", "main")
	rawGit(t, repo, "-c", "user.name=t", "-c", "user.email=t@t.invalid",
		"merge", "--no-ff", "-m", "raw merge", "topic")

	if canaryFired(t, canary) {
		contents, _ := os.ReadFile(canary)
		t.Fatalf("an unsandboxed Git command reached a host hook inside a sandbox repository "+
			"(%d invocation(s)):\n%s", strings.Count(string(contents), "fired"), contents)
	}
}

// TestSandboxRepoStillAllowsItsOwnHooks proves the repository-level hardening
// is not a blanket ban: a test that needs its own hook to fire can still say
// so on the command line, which outranks repository configuration.
func TestSandboxRepoStillAllowsItsOwnHooks(t *testing.T) {
	poisonHost(t)
	sandbox := gittest.New(t)
	repo := sandbox.NewRepo(t)

	ownHooks := filepath.Join(t.TempDir(), "own-hooks")
	if err := os.MkdirAll(ownHooks, 0o700); err != nil {
		t.Fatalf("create %s: %v", ownHooks, err)
	}
	fired := filepath.Join(t.TempDir(), "own-fired")
	if err := os.WriteFile(filepath.Join(ownHooks, "post-commit"),
		[]byte("#!/bin/sh\necho fired > "+fired+"\n"), 0o700); err != nil {
		t.Fatalf("write own hook: %v", err)
	}

	write(t, filepath.Join(repo, "own.txt"), "own\n")
	sandbox.Run(t, repo, "add", "own.txt")
	sandbox.Run(t, repo, "-c", "core.hooksPath="+ownHooks, "commit", "-m", "with own hook")

	if _, err := os.Stat(fired); err != nil {
		t.Fatalf("a test's own hook must still be able to fire: %v", err)
	}
}
