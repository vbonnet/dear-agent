package main

// launchagent_install_test.go — install/uninstall/status coverage for the
// macOS LaunchAgent path. The 2026-05-27 coverage audit flagged this as
// the highest-risk gap in the overnight batch: scanner ships with a
// per-user LaunchAgent, the install code was 0% covered on its only
// target platform (macOS), and the existing launchagent_test.go only
// exercised renderPlist.
//
// Strategy: route every launchctl call through the launchctlRun seam so
// the test does not need a running launchd. Then assert on the real
// filesystem effects — directories created, plist rendered with the
// right substitutions, mode 0600, idempotent reinstall, removal on
// uninstall, missing-target tolerated.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// recordingLaunchctl swaps launchctlRun for a recorder that returns
// canned results per subcommand and remembers every invocation. Returns
// a restore func and a pointer to the recorded call list. Concurrent-
// safe so subtests are free to share it.
func recordingLaunchctl(t *testing.T, results map[string]struct {
	out []byte
	err error
}) (*[][]string, func()) {
	t.Helper()
	var mu sync.Mutex
	var calls [][]string
	prev := launchctlRun
	launchctlRun = func(args ...string) ([]byte, error) {
		mu.Lock()
		calls = append(calls, append([]string(nil), args...))
		mu.Unlock()
		if len(args) == 0 {
			return nil, nil
		}
		if r, ok := results[args[0]]; ok {
			return r.out, r.err
		}
		return nil, nil
	}
	return &calls, func() { launchctlRun = prev }
}

func TestInstallLaunchAgent_WritesPlistAndBootstraps(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	calls, restore := recordingLaunchctl(t, nil)
	defer restore()

	if err := installLaunchAgent(home, "gui/501", target); err != nil {
		t.Fatalf("installLaunchAgent: %v", err)
	}

	// Plist landed at the target path with the right substitutions.
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat installed plist: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("plist mode = %o, want 0o600 (only the session owner should read it)", mode)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read installed plist: %v", err)
	}
	if strings.Contains(string(body), "__USER_HOME__") || strings.Contains(string(body), "__BUMBLEBEE_BINARY__") {
		t.Errorf("plist still has unsubstituted placeholders:\n%s", body)
	}
	if !strings.Contains(string(body), home) {
		t.Errorf("plist missing $HOME substitution %q:\n%s", home, body)
	}

	// Log directory pre-created — launchd will not mkdir the
	// StandardOutPath parent for us.
	if _, err := os.Stat(filepath.Join(home, "Library", "Logs", "dear-agent")); err != nil {
		t.Errorf("log dir not created: %v", err)
	}

	// launchctl call sequence: bootout then bootstrap, both targeting
	// the right domain + label.
	if len(*calls) != 2 {
		t.Fatalf("launchctl calls = %d, want 2 (bootout + bootstrap): %v", len(*calls), *calls)
	}
	if (*calls)[0][0] != "bootout" || (*calls)[1][0] != "bootstrap" {
		t.Errorf("launchctl call order = %v, want bootout then bootstrap", *calls)
	}
	if (*calls)[1][1] != "gui/501" || (*calls)[1][2] != target {
		t.Errorf("bootstrap args = %v, want [bootstrap gui/501 %s]", (*calls)[1], target)
	}
}

func TestInstallLaunchAgent_BootstrapFailureSurfaces(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	_, restore := recordingLaunchctl(t, map[string]struct {
		out []byte
		err error
	}{
		"bootstrap": {out: []byte("Bootstrap failed: 5: Input/output error"), err: errors.New("exit 1")},
	})
	defer restore()

	err := installLaunchAgent(home, "gui/501", target)
	if err == nil {
		t.Fatal("installLaunchAgent succeeded despite bootstrap failure; want error")
	}
	if !strings.Contains(err.Error(), "launchctl bootstrap") {
		t.Errorf("error %q should mention launchctl bootstrap", err)
	}
}

func TestInstallLaunchAgent_IsIdempotent(t *testing.T) {
	// Re-running install must overwrite cleanly: the bootout-then-
	// bootstrap dance is what makes the reinstall safe even though
	// launchctl bootstrap refuses to re-load an existing label.
	home := t.TempDir()
	target := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	calls, restore := recordingLaunchctl(t, nil)
	defer restore()

	if err := installLaunchAgent(home, "gui/501", target); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := installLaunchAgent(home, "gui/501", target); err != nil {
		t.Fatalf("second install: %v", err)
	}

	// 4 calls = (bootout, bootstrap) twice. If install ever forgets
	// the bootout-first step on reinstall this assertion will catch it.
	if len(*calls) != 4 {
		t.Fatalf("calls = %d, want 4 (two bootout+bootstrap pairs): %v", len(*calls), *calls)
	}
	for i, want := range []string{"bootout", "bootstrap", "bootout", "bootstrap"} {
		if (*calls)[i][0] != want {
			t.Errorf("call %d = %s, want %s", i, (*calls)[i][0], want)
		}
	}
}

func TestUninstallLaunchAgent_RemovesPlist(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("<plist/>"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	calls, restore := recordingLaunchctl(t, nil)
	defer restore()

	if err := uninstallLaunchAgent("gui/501", target); err != nil {
		t.Fatalf("uninstallLaunchAgent: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("plist still present after uninstall: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0][0] != "bootout" {
		t.Errorf("uninstall must call bootout exactly once; got %v", *calls)
	}
}

func TestUninstallLaunchAgent_MissingPlistIsNotAnError(t *testing.T) {
	// Bumblebee's uninstall is called from `make uninstall` and shouldn't
	// fail on a host that never installed it. Bootout's error from a
	// not-loaded service must also be swallowed.
	home := t.TempDir()
	target := filepath.Join(home, "Library", "LaunchAgents", "never-installed.plist")
	_, restore := recordingLaunchctl(t, map[string]struct {
		out []byte
		err error
	}{
		"bootout": {out: []byte("Could not find service"), err: errors.New("exit 113")},
	})
	defer restore()

	if err := uninstallLaunchAgent("gui/501", target); err != nil {
		t.Fatalf("uninstall on never-installed host should be a no-op, got %v", err)
	}
}

func TestStatusLaunchAgent_PrintsHeaderWhenLoaded(t *testing.T) {
	// Pad enough lines to exercise the truncate-to-20 branch in
	// statusLaunchAgent. The function returns nil on success — we just
	// check that the launchctl print verb is invoked with the right
	// label and that a "loaded" result does not error.
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, "key = value")
	}
	calls, restore := recordingLaunchctl(t, map[string]struct {
		out []byte
		err error
	}{
		"print": {out: []byte(strings.Join(lines, "\n")), err: nil},
	})
	defer restore()

	if err := statusLaunchAgent("gui/501"); err != nil {
		t.Fatalf("statusLaunchAgent: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0][0] != "print" {
		t.Fatalf("expected one launchctl print call, got %v", *calls)
	}
	if (*calls)[0][1] != "gui/501/"+launchAgentLabel {
		t.Errorf("print target = %q, want gui/501/%s", (*calls)[0][1], launchAgentLabel)
	}
}

func TestStatusLaunchAgent_NotLoadedReturnsError(t *testing.T) {
	_, restore := recordingLaunchctl(t, map[string]struct {
		out []byte
		err error
	}{
		"print": {out: nil, err: errors.New("exit 113")},
	})
	defer restore()

	err := statusLaunchAgent("gui/501")
	if err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("statusLaunchAgent on unloaded label = %v, want 'not loaded' error", err)
	}
}

func TestRunInstallLaunchAgent_RejectsNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("non-darwin guard only triggers off-darwin")
	}
	err := runInstallLaunchAgent(nil)
	if err == nil || !strings.Contains(err.Error(), "macOS-only") {
		t.Fatalf("runInstallLaunchAgent off-darwin = %v, want macOS-only error", err)
	}
}

func TestRunInstallLaunchAgent_FlagParseError(t *testing.T) {
	// Unknown flag must surface from flag.Parse, regardless of OS, so
	// the wrapper never gets to launchd and tests stay hermetic.
	err := runInstallLaunchAgent([]string{"--no-such-flag"})
	if err == nil {
		t.Fatal("runInstallLaunchAgent accepted an unknown flag")
	}
}
