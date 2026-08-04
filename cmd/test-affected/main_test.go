package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// pkgFixture returns a small synthetic module: three packages
// (a, b, c), where b imports a and c is independent. b has tests,
// c has tests, a has none.
func pkgFixture() []*goListPackage {
	mainMod := &struct {
		Path string
		Main bool
	}{Path: "example.com/m", Main: true}
	return []*goListPackage{
		{
			ImportPath: "example.com/m/a",
			Dir:        "/repo/a",
			GoFiles:    []string{"a.go"},
			Module:     mainMod,
		},
		{
			ImportPath:  "example.com/m/b",
			Dir:         "/repo/b",
			GoFiles:     []string{"b.go"},
			TestGoFiles: []string{"b_test.go"},
			Imports:     []string{"example.com/m/a"},
			Deps:        []string{"example.com/m/a"},
			Module:      mainMod,
		},
		{
			ImportPath:  "example.com/m/c",
			Dir:         "/repo/c",
			GoFiles:     []string{"c.go"},
			TestGoFiles: []string{"c_test.go"},
			Module:      mainMod,
		},
	}
}

func TestDecide_NoChangesReturnsEmpty(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, nil, pkgs)
	if d.forceFull {
		t.Fatal("empty diff should not force full run")
	}
	if len(d.changedPkgs) != 0 {
		t.Fatalf("expected no changed packages, got %v", d.changedPkgs)
	}
	if got := selectPackages(pkgs, d, false); len(got) != 0 {
		t.Fatalf("expected no selected packages, got %v", got)
	}
}

func TestDecide_LeafChangeAffectsDependent(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"a/a.go"}, pkgs)
	if d.forceFull {
		t.Fatal("a/a.go change should not force full run")
	}
	if _, ok := d.changedPkgs["example.com/m/a"]; !ok {
		t.Fatalf("expected a to be in changed packages, got %v", d.changedPkgs)
	}

	got := selectPackages(pkgs, d, false)
	sort.Strings(got)
	want := []string{"example.com/m/b"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_IndependentChangeDoesNotAffectOthers(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"c/c.go"}, pkgs)
	got := selectPackages(pkgs, d, false)
	want := []string{"example.com/m/c"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_GoModForcesFullRun(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"go.mod"}, pkgs)
	if !d.forceFull {
		t.Fatal("go.mod change must force full run")
	}
	got := selectPackages(pkgs, d, false)
	// Both test-bearing packages should be returned; the non-test
	// package a should be excluded.
	want := []string{"example.com/m/b", "example.com/m/c"}
	sort.Strings(got)
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_GoWorkSumForcesFullRun(t *testing.T) {
	d := decide(options{root: "/repo"}, []string{"go.work.sum"}, pkgFixture())
	if !d.forceFull {
		t.Fatal("go.work.sum change must force full run")
	}
}

func TestDecide_CIWorkflowChangeForcesFullRun(t *testing.T) {
	d := decide(options{root: "/repo"}, []string{".github/workflows/ci.yml"}, pkgFixture())
	if !d.forceFull {
		t.Fatal("CI workflow change must force full run")
	}
}

func TestDecide_TestAffectedSelfChangeForcesFullRun(t *testing.T) {
	d := decide(options{root: "/repo"}, []string{"cmd/test-affected/main.go"}, pkgFixture())
	if !d.forceFull {
		t.Fatal("changing the selector itself must force full run (we can't trust its own output)")
	}
}

func TestDecide_TestdataChangeMapsToParentPackage(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"b/testdata/golden/case1.json"}, pkgs)
	if _, ok := d.changedPkgs["example.com/m/b"]; !ok {
		t.Fatalf("expected b to be picked up via testdata walk-up, got %v", d.changedPkgs)
	}
	got := selectPackages(pkgs, d, false)
	want := []string{"example.com/m/b"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages: got %v, want %v", got, want)
	}
}

func TestDecide_RootLevelPackageIsCaughtByWalkUp(t *testing.T) {
	// Synthetic package at the repo root: Dir == "/repo", which
	// filepath.Rel(opts.root, p.Dir) renders as ".".
	rootPkg := &goListPackage{
		ImportPath:  "example.com/m",
		Dir:         "/repo",
		GoFiles:     []string{"root.go"},
		TestGoFiles: []string{"root_test.go"},
		Module: &struct {
			Path string
			Main bool
		}{Path: "example.com/m", Main: true},
	}
	pkgs := append([]*goListPackage{rootPkg}, pkgFixture()...)
	d := decide(options{root: "/repo"}, []string{"root.go"}, pkgs)
	if d.forceFull {
		t.Fatal("root.go change must not force full")
	}
	if _, ok := d.changedPkgs["example.com/m"]; !ok {
		t.Fatalf("walk-up should have reached the root package; got %v", d.changedPkgs)
	}
}

func TestDecide_UnknownPathIsIgnored(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"docs/some-note.md"}, pkgs)
	if d.forceFull {
		t.Fatal("docs change should not force full")
	}
	if len(d.changedPkgs) != 0 {
		t.Fatalf("expected no changed packages, got %v", d.changedPkgs)
	}
}

func TestSelectPackages_EmitAllIncludesNonTestPackages(t *testing.T) {
	pkgs := pkgFixture()
	d := decide(options{root: "/repo"}, []string{"a/a.go"}, pkgs)
	got := selectPackages(pkgs, d, true)
	sort.Strings(got)
	// emitAll should include a (the changed package itself) and b (its
	// dependent). c is unaffected.
	want := []string{"example.com/m/a", "example.com/m/b"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages emitAll: got %v, want %v", got, want)
	}
}

func TestSelectPackages_ExcludesThirdPartyAndStdlib(t *testing.T) {
	thirdParty := &goListPackage{
		ImportPath:  "github.com/external/dep",
		TestGoFiles: []string{"x_test.go"},
		Module: &struct {
			Path string
			Main bool
		}{Path: "github.com/external/dep", Main: false},
	}
	stdlib := &goListPackage{
		ImportPath:  "fmt",
		TestGoFiles: []string{"y_test.go"},
		Standard:    true,
	}
	pkgs := append(pkgFixture(), thirdParty, stdlib)
	d := decide(options{root: "/repo"}, []string{"go.mod"}, pkgs)
	got := selectPackages(pkgs, d, false)
	sort.Strings(got)
	want := []string{"example.com/m/b", "example.com/m/c"}
	if !sliceEqual(got, want) {
		t.Fatalf("selectPackages must exclude third-party and stdlib: got %v, want %v", got, want)
	}
}

func TestGoTestArgs_BoundsNativeTestBinary(t *testing.T) {
	got := goTestArgs(
		options{tags: "integration"},
		[]string{"example.com/m/a", "example.com/m/b"},
	)
	want := []string{
		"test",
		"-race",
		"-count=1",
		"-timeout=20m0s",
		"-tags=integration",
		"example.com/m/a",
		"example.com/m/b",
	}
	if !sliceEqual(got, want) {
		t.Fatalf("goTestArgs: got %v, want %v", got, want)
	}
}

func TestGoCommandTimeoutPreservesNativeAndBuildBudgets(t *testing.T) {
	if goCommandTimeout != 55*time.Minute {
		t.Fatalf("goCommandTimeout = %v, want 55m", goCommandTimeout)
	}
	if goListCommandTimeout != goTestTimeout {
		t.Fatalf("goListCommandTimeout = %v, want native timeout %v", goListCommandTimeout, goTestTimeout)
	}
	if goCommandTimeout < 2*goTestTimeout {
		t.Fatalf("goCommandTimeout = %v, want at least two native intervals", goCommandTimeout)
	}
}

func TestRunTestsPassesNativeTimeoutToGo(t *testing.T) {
	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "go-args")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_AFFECTED_GO_ARGS_FILE\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_AFFECTED_GO_ARGS_FILE", argsFile)

	var stderr bytes.Buffer
	code := runTests(
		options{base: "origin/main", root: t.TempDir(), tags: "integration"},
		[]string{"example.com/m/a"},
		&bytes.Buffer{},
		&stderr,
	)
	if code != 0 {
		t.Fatalf("runTests returned %d: %s", code, stderr.String())
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Fields(string(raw))
	want := []string{"test", "-race", "-count=1", "-timeout=20m0s", "-tags=integration", "example.com/m/a"}
	if !sliceEqual(got, want) {
		t.Fatalf("captured go args: got %v, want %v", got, want)
	}
}

func TestProtectGoCommandProcessTreeIsGroupCancelable(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "true")
	protectGoCommandProcessTree(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("bounded Go child must run in an isolated process group")
	}
	if cmd.Cancel == nil {
		t.Fatal("bounded Go child must cancel its process group")
	}
	if cmd.WaitDelay != time.Second {
		t.Fatalf("bounded Go child WaitDelay = %v, want %v", cmd.WaitDelay, time.Second)
	}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("cancel before start = %v", err)
	}
	cmd.Process = &os.Process{Pid: 0}
	if err := cmd.Cancel(); err != nil {
		t.Fatalf("cancel with invalid pid = %v", err)
	}
}

func TestRunGoTestCommandTimeoutKillsProcessGroup(t *testing.T) {
	binDir := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "go-pids")
	stub := "#!/bin/sh\nsleep 30 &\nchild=$!\nprintf '%s\\n%s\\n' \"$$\" \"$child\" > \"$TEST_AFFECTED_GO_PID_FILE\"\nwait \"$child\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_AFFECTED_GO_PID_FILE", pidFile)

	const reportedTimeout = time.Second
	ctx, cancel := context.WithCancelCause(context.Background())
	t.Cleanup(func() { cancel(context.Canceled) })
	root := t.TempDir()
	var stderr bytes.Buffer
	result := make(chan int, 1)
	go func() {
		result <- runGoTestCommand(
			ctx,
			reportedTimeout,
			options{root: root},
			[]string{"example.com/m/a"},
			&bytes.Buffer{},
			&stderr,
		)
	}()

	raw := waitForFileContents(t, pidFile)
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		t.Fatalf("captured process IDs = %q, want leader and descendant", raw)
	}
	leaderPID := mustPID(t, fields[0])
	childPID := mustPID(t, fields[1])
	t.Cleanup(func() {
		_ = syscall.Kill(-leaderPID, syscall.SIGKILL)
	})

	cancel(context.DeadlineExceeded)
	var code int
	select {
	case code = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("runGoTestCommand did not return after deadline cancellation")
	}
	if code != goCommandTimeoutExitCode {
		t.Fatalf("runGoTestCommand returned %d, want %d: %s", code, goCommandTimeoutExitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "timed out after 1s") {
		t.Fatalf("timeout diagnostic missing from stderr: %q", stderr.String())
	}
	waitForProcessGone(t, leaderPID)
	waitForProcessGone(t, childPID)
}

func TestListPackagesContextCancellationKillsProcessGroup(t *testing.T) {
	binDir := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "go-list-pids")
	stub := "#!/bin/sh\nsleep 30 &\nchild=$!\nprintf '%s\\n%s\\n' \"$$\" \"$child\" > \"$TEST_AFFECTED_GO_LIST_PID_FILE\"\nwait \"$child\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_AFFECTED_GO_LIST_PID_FILE", pidFile)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() {
		_, err := listPackagesWithContext(ctx, t.TempDir(), "")
		result <- err
	}()

	raw := waitForFileContents(t, pidFile)
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		t.Fatalf("captured process IDs = %q, want leader and descendant", raw)
	}
	leaderPID := mustPID(t, fields[0])
	childPID := mustPID(t, fields[1])
	t.Cleanup(func() {
		_ = syscall.Kill(-leaderPID, syscall.SIGKILL)
	})

	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("listPackagesWithContext returned nil after cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listPackagesWithContext did not return after cancellation")
	}
	waitForProcessGone(t, leaderPID)
	waitForProcessGone(t, childPID)
}

func waitForFileContents(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && len(raw) > 0 {
			return raw
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for process readiness file %s", path)
	return nil
}

func mustPID(t *testing.T, raw string) int {
	t.Helper()
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		t.Fatalf("invalid process ID %q: %v", raw, err)
	}
	return pid
}

func waitForProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d still exists after process-group cancellation", pid)
}

func TestIsForceFullPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"go.mod", true},
		{"go.sum", true},
		{"Makefile", true},
		{".github/workflows/ci.yml", true},
		{".github/workflows/agm-e2e-install.yml", true},
		{"cmd/test-affected/main.go", true},
		{"agm/cmd/agm/main.go", false},
		{"docs/adr/ADR-001-monorepo-consolidation.md", false},
		{"README.md", false},
	}
	for _, tc := range cases {
		if got := isForceFullPath(tc.path); got != tc.want {
			t.Errorf("isForceFullPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
