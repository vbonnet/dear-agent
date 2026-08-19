package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A dotfile snapshot rebound to an isolated HOME must move every root below
// that HOME. Test-environment activation relocates HOME after Load, so an
// authority that keeps pointing at the host roots provisions isolated runs into
// host storage.
func TestRebindRuntimeAuthorityToIsolatedHomeMovesEveryRoot(t *testing.T) {
	clearWorkspaceDiscoveryEnv(t)
	root := t.TempDir()
	hostHome := makeAuthorityDir(t, filepath.Join(root, "host-home"))
	isolatedHome := makeAuthorityDir(t, filepath.Join(root, "isolated-home"))
	t.Setenv("HOME", hostHome)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := cfg.RebindRuntimeAuthorityToIsolatedHome(isolatedHome); err != nil {
		t.Fatalf("RebindRuntimeAuthorityToIsolatedHome() error = %v", err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	wantStorage := filepath.Join(isolatedHome, ".agm")
	assertRuntimeAuthorityPaths(t, authority, isolatedHome, wantStorage, filepath.Join(wantStorage, "sandboxes"))
}

// Isolation must not be routed back into centralized production storage by the
// configuration the run inherited: the isolated HOME always projects the
// dotfile layout.
func TestRebindRuntimeAuthorityToIsolatedHomeIgnoresCentralizedStorage(t *testing.T) {
	clearWorkspaceDiscoveryEnv(t)
	root := t.TempDir()
	hostHome := makeAuthorityDir(t, filepath.Join(root, "host-home"))
	isolatedHome := makeAuthorityDir(t, filepath.Join(root, "isolated-home"))
	workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
	t.Setenv("HOME", hostHome)

	cfgPath := writeRuntimeAuthorityConfig(t, root, workspace, ".agm-work")
	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := cfg.RebindRuntimeAuthorityToIsolatedHome(isolatedHome); err != nil {
		t.Fatalf("RebindRuntimeAuthorityToIsolatedHome() error = %v", err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	wantStorage := filepath.Join(isolatedHome, ".agm")
	assertRuntimeAuthorityPaths(t, authority, isolatedHome, wantStorage, filepath.Join(wantStorage, "sandboxes"))

	if _, err := os.Lstat(filepath.Join(workspace, ".agm-work", "sandboxes")); !os.IsNotExist(err) {
		t.Fatalf("isolated rebind touched centralized storage: %v", err)
	}
	if cfg.Storage.Mode != "centralized" {
		t.Fatalf("cfg.Storage.Mode = %q, want the loaded snapshot left unmodified", cfg.Storage.Mode)
	}
}

// Rebinding is a relocation of existing authority, never a way to manufacture
// it: a Config that never survived Load stays unusable.
func TestRebindRuntimeAuthorityToIsolatedHomeRefusesUnloadedConfig(t *testing.T) {
	clearWorkspaceDiscoveryEnv(t)
	isolatedHome := makeAuthorityDir(t, filepath.Join(t.TempDir(), "isolated-home"))

	for name, cfg := range map[string]*Config{
		"nil":         nil,
		"default":     Default(),
		"constructed": {},
	} {
		t.Run(name, func(t *testing.T) {
			err := cfg.RebindRuntimeAuthorityToIsolatedHome(isolatedHome)
			if !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
				t.Fatalf("RebindRuntimeAuthorityToIsolatedHome() error = %v, want %v",
					err, ErrRuntimeAuthorityUnavailable)
			}
			if _, err := cfg.RuntimeAuthority(); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
				t.Fatalf("RuntimeAuthority() error = %v, want the snapshot to stay unusable", err)
			}
		})
	}
}

// A rebind that cannot resolve its target is a failure, not a silent fallback
// to the previously retained roots.
func TestRebindRuntimeAuthorityToIsolatedHomeFailsOnUnresolvableHome(t *testing.T) {
	clearWorkspaceDiscoveryEnv(t)
	root := t.TempDir()
	hostHome := makeAuthorityDir(t, filepath.Join(root, "host-home"))
	t.Setenv("HOME", hostHome)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	notADirectory := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(notADirectory, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := cfg.RebindRuntimeAuthorityToIsolatedHome(notADirectory); err == nil {
		t.Fatal("RebindRuntimeAuthorityToIsolatedHome() error = nil, want a resolution failure")
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	wantStorage := filepath.Join(hostHome, ".agm")
	assertRuntimeAuthorityPaths(t, authority, hostHome, wantStorage, filepath.Join(wantStorage, "sandboxes"))
}
