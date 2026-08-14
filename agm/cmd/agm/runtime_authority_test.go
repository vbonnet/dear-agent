package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
