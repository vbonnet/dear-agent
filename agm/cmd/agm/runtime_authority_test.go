package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
)

func TestLoadConfigWithFlagsFailsClosedWhenCentralizedBootstrapFails(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(home, ".agm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(home, ".agm", "dangling")
	if err := os.Symlink(filepath.Join(root, "missing"), dangling); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", home)
	for _, key := range []string{"ENGRAM_TEST_MODE", "ENGRAM_TEST_WORKSPACE", "ENGRAM_WORKSPACE"} {
		t.Setenv(key, "")
	}

	configPath := filepath.Join(root, "config.yaml")
	contents := "storage:\n  mode: centralized\n  workspace: " + workspace + "\n  relative_path: .agm-work\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	originalCfgFile, originalCfg := cfgFile, cfg
	originalSessionsDir, originalProvider := sessionsDir, sandboxProvider
	providerFlag := newCmd.Flags().Lookup("sandbox-provider")
	originalProviderChanged := providerFlag.Changed
	t.Cleanup(func() {
		cfgFile = originalCfgFile
		cfg = originalCfg
		sessionsDir = originalSessionsDir
		sandboxProvider = originalProvider
		providerFlag.Changed = originalProviderChanged
	})
	cfgFile = configPath
	sessionsDir = filepath.Join(home, "sessions")
	sandboxProvider = "auto"
	providerFlag.Changed = false

	loaded, err := loadConfigWithFlags()
	if err == nil {
		t.Fatal("loadConfigWithFlags() error = nil, want centralized bootstrap failure")
	}
	if loaded != nil {
		t.Fatalf("loadConfigWithFlags() config = %#v, want nil", loaded)
	}
	if !strings.Contains(err.Error(), "setup centralized storage") {
		t.Fatalf("loadConfigWithFlags() error = %v, want centralized bootstrap context", err)
	}
	if info, statErr := os.Lstat(dangling); statErr != nil {
		t.Fatalf("bootstrap rollback did not restore source: %v", statErr)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("bootstrap rollback replaced dangling source with mode %v", info.Mode())
	}
	if _, statErr := os.Lstat(filepath.Join(workspace, ".agm-work")); !os.IsNotExist(statErr) {
		t.Fatalf("bootstrap failure left centralized target behind: %v", statErr)
	}
}

// Test-environment activation relocates HOME after PersistentPreRunE loaded the
// configuration, so preflight has to recapture the runtime authority. Without
// that recapture an isolated run provisions its sandbox into the host's real
// AGM storage and scans the host HOME for lower dirs.
func TestPreflightRebindsRuntimeAuthorityToIsolatedHome(t *testing.T) {
	hostHome := filepath.Join(t.TempDir(), "host-home")
	if err := os.MkdirAll(hostHome, 0o700); err != nil {
		t.Fatal(err)
	}
	physicalHostHome, err := filepath.EvalSymlinks(hostHome)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", physicalHostHome)
	for _, key := range []string{
		"AGM_TEST_RUN_ID", "AGM_TEST_ENV", "AGM_TMUX_SOCKET", "AGM_SESSIONS_DIR",
		"AGM_DB_PATH", "AGM_STATE_DIR", "AGM_LOCK_PATH", "AGM_TEST_SANDBOX",
		"ENGRAM_TEST_MODE", "ENGRAM_TEST_WORKSPACE", "ENGRAM_WORKSPACE",
	} {
		t.Setenv(key, "")
	}

	loaded, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got := authorityHomePath(t, loaded); got != physicalHostHome {
		t.Fatalf("loaded authority HOME = %q, want %q", got, physicalHostHome)
	}

	originalCfg, originalTestMode := cfg, testMode
	t.Cleanup(func() {
		cfg = originalCfg
		testMode = originalTestMode
	})
	cfg = loaded
	testMode = true

	if _, err := preflight("authority-rebind-probe"); err != nil {
		t.Fatalf("preflight() error = %v", err)
	}

	isolatedHome := os.Getenv("HOME")
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(isolatedHome)) })
	if isolatedHome == physicalHostHome {
		t.Fatal("preflight() left HOME on the host: test environment did not activate")
	}
	physicalIsolatedHome, err := filepath.EvalSymlinks(isolatedHome)
	if err != nil {
		t.Fatal(err)
	}
	if got := authorityHomePath(t, cfg); got != physicalIsolatedHome {
		t.Fatalf("authority HOME after preflight() = %q, want isolated %q", got, physicalIsolatedHome)
	}

	sandboxRoot, err := mustAuthority(t, cfg).Sandboxes()
	if err != nil {
		t.Fatalf("Sandboxes() error = %v", err)
	}
	sandboxPath, err := sandboxRoot.Path()
	if err != nil {
		t.Fatalf("sandbox root Path() error = %v", err)
	}
	want := filepath.Join(physicalIsolatedHome, ".agm", "sandboxes")
	if sandboxPath != want {
		t.Fatalf("sandbox root = %q, want %q", sandboxPath, want)
	}
	if strings.HasPrefix(sandboxPath, physicalHostHome+string(filepath.Separator)) {
		t.Fatalf("sandbox root %q still lives under the host HOME %q", sandboxPath, physicalHostHome)
	}
}

func mustAuthority(t *testing.T, c *config.Config) config.RuntimeAuthority {
	t.Helper()
	authority, err := c.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	return authority
}

func authorityHomePath(t *testing.T, c *config.Config) string {
	t.Helper()
	home, err := mustAuthority(t, c).Home()
	if err != nil {
		t.Fatalf("Home() error = %v", err)
	}
	path, err := home.Path()
	if err != nil {
		t.Fatalf("home Path() error = %v", err)
	}
	return path
}
