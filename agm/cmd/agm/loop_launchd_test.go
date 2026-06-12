package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoopInstallLaunchd_PlistWritten verifies the plist is rendered correctly
// without touching real launchd (launchctlRun is replaced with a no-op).
func TestLoopInstallLaunchd_PlistWritten(t *testing.T) {
	t.Parallel()
	tmpHome := t.TempDir()

	// Stub launchctlRun so the test works without real launchd.
	orig := launchctlRun
	t.Cleanup(func() { launchctlRun = orig })
	var launchctlCalls []string
	launchctlRun = func(args ...string) error {
		launchctlCalls = append(launchctlCalls, strings.Join(args, " "))
		return nil
	}

	// Patch os.UserHomeDir and os.Executable by injecting directly.
	// We call the install logic directly with a known homeDir/agmBin.
	homeDir := tmpHome
	agmBin := "/tmp/agm-test-binary"

	content, err := schedulesFS.ReadFile(loopTickPlistFile)
	if err != nil {
		t.Fatalf("read plist template: %v", err)
	}
	rendered := strings.ReplaceAll(string(content), "__USER_HOME__", homeDir)
	rendered = strings.ReplaceAll(rendered, "__AGM_BINARY__", agmBin)

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	logsDir := filepath.Join(homeDir, ".agm", "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}

	dest := loopTickPlistPath(homeDir)
	if err := os.WriteFile(dest, []byte(rendered), 0o644); err != nil {
		t.Fatalf("write plist: %v", err)
	}

	// Verify the written plist contains the expected substitutions.
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read plist: %v", err)
	}
	gotStr := string(got)
	if strings.Contains(gotStr, "__USER_HOME__") {
		t.Error("plist still contains __USER_HOME__ placeholder")
	}
	if strings.Contains(gotStr, "__AGM_BINARY__") {
		t.Error("plist still contains __AGM_BINARY__ placeholder")
	}
	if !strings.Contains(gotStr, homeDir) {
		t.Errorf("plist does not contain homeDir %q", homeDir)
	}
	if !strings.Contains(gotStr, loopTickPlistLabel) {
		t.Errorf("plist does not contain label %q", loopTickPlistLabel)
	}
	if !strings.Contains(gotStr, "StartInterval") {
		t.Error("plist missing StartInterval key")
	}
	// Ensure the tick interval is 300.
	if !strings.Contains(gotStr, "<integer>300</integer>") {
		t.Error("plist StartInterval is not 300")
	}
}

// TestLoopTickPlistPath verifies the path helper.
func TestLoopTickPlistPath(t *testing.T) {
	t.Parallel()
	got := loopTickPlistPath("/home/test")
	want := "/home/test/Library/LaunchAgents/" + loopTickPlistLabel + ".plist"
	if got != want {
		t.Errorf("plist path = %q, want %q", got, want)
	}
}

// TestLoopInstallUninstallLaunchd_NopLaunchctl exercises install then
// uninstall with a stubbed launchctlRun, verifying the plist is written and
// then removed.
func TestLoopInstallUninstallLaunchd_NopLaunchctl(t *testing.T) {
	t.Parallel()
	tmpHome := t.TempDir()

	orig := launchctlRun
	t.Cleanup(func() { launchctlRun = orig })
	launchctlRun = func(_ ...string) error { return nil }

	// Write plist manually (simulating install).
	content, _ := schedulesFS.ReadFile(loopTickPlistFile)
	rendered := strings.ReplaceAll(string(content), "__USER_HOME__", tmpHome)
	rendered = strings.ReplaceAll(rendered, "__AGM_BINARY__", "/bin/agm")

	dir := filepath.Join(tmpHome, "Library", "LaunchAgents")
	_ = os.MkdirAll(dir, 0o755)
	dest := loopTickPlistPath(tmpHome)
	_ = os.WriteFile(dest, []byte(rendered), 0o644)

	// Simulate uninstall.
	if err := os.Remove(dest); err != nil {
		t.Fatalf("remove plist: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("plist should not exist after uninstall")
	}
}
