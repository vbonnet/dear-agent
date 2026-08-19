package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCapturesDotfileRuntimeAuthorityAcrossHomeDrift(t *testing.T) {
	root := t.TempDir()
	physicalHome := makeAuthorityDir(t, filepath.Join(root, "physical-home"))
	driftHome := makeAuthorityDir(t, filepath.Join(root, "drift-home"))
	logicalHome := filepath.Join(root, "logical-home")
	symlinkOrSkip(t, physicalHome, logicalHome)
	t.Setenv("HOME", logicalHome)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	wantStorage := filepath.Join(physicalHome, ".agm")
	wantSandboxes := filepath.Join(wantStorage, "sandboxes")
	assertRuntimeAuthorityPaths(t, authority, physicalHome, wantStorage, wantSandboxes)

	retargetSymlink(t, logicalHome, driftHome)
	t.Setenv("HOME", driftHome)

	assertRuntimeAuthorityPaths(t, authority, physicalHome, wantStorage, wantSandboxes)
	storagePath, err := GetStoragePath(cfg)
	if err != nil {
		t.Fatalf("GetStoragePath() error = %v", err)
	}
	if storagePath != wantStorage {
		t.Fatalf("GetStoragePath() = %q, want retained %q", storagePath, wantStorage)
	}
}

func TestLoadCapturesCentralizedRuntimeAuthorityAcrossDiscoveryDrift(t *testing.T) {
	for _, selection := range []string{"environment", "cwd", "tilde"} {
		t.Run(selection, func(t *testing.T) {
			clearWorkspaceDiscoveryEnv(t)
			root := t.TempDir()
			physicalHome := makeAuthorityDir(t, filepath.Join(root, "physical-home"))
			driftHome := makeAuthorityDir(t, filepath.Join(root, "drift-home"))
			logicalHome := filepath.Join(root, "logical-home")
			symlinkOrSkip(t, physicalHome, logicalHome)
			t.Setenv("HOME", logicalHome)

			selectedWorkspacePath := filepath.Join(root, "selected")
			workspaceSetting := "selected"
			if selection == "tilde" {
				selectedWorkspacePath = filepath.Join(physicalHome, "workspaces", "selected")
				workspaceSetting = "~/workspaces/selected"
			}
			selectedWorkspace := makeAuthorityDir(t, selectedWorkspacePath)
			driftWorkspace := makeAuthorityDir(t, filepath.Join(root, "drift"))

			switch selection {
			case "environment":
				t.Setenv("ENGRAM_WORKSPACE", selectedWorkspace)
			case "cwd":
				if err := os.Mkdir(filepath.Join(selectedWorkspace, ".git"), 0o700); err != nil {
					t.Fatal(err)
				}
				child := filepath.Join(selectedWorkspace, "nested")
				if err := os.Mkdir(child, 0o700); err != nil {
					t.Fatal(err)
				}
				t.Chdir(child)
			}

			cfgPath := writeRuntimeAuthorityConfig(t, root, workspaceSetting, ".agm-work")
			cfg, err := Load(cfgPath)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			authority, err := cfg.RuntimeAuthority()
			if err != nil {
				t.Fatalf("RuntimeAuthority() error = %v", err)
			}
			wantStorage := filepath.Join(selectedWorkspace, ".agm-work")
			wantSandboxes := filepath.Join(wantStorage, "sandboxes")
			assertRuntimeAuthorityPaths(t, authority, physicalHome, wantStorage, wantSandboxes)

			retargetSymlink(t, logicalHome, driftHome)
			t.Setenv("HOME", driftHome)
			t.Setenv("ENGRAM_WORKSPACE", driftWorkspace)
			t.Chdir(driftWorkspace)
			cfg.Storage = StorageConfig{
				Mode:         "centralized",
				Workspace:    driftWorkspace,
				RelativePath: ".drift-storage",
			}

			assertRuntimeAuthorityPaths(t, authority, physicalHome, wantStorage, wantSandboxes)
			storagePath, err := GetStoragePath(cfg)
			if err != nil {
				t.Fatalf("GetStoragePath() error = %v", err)
			}
			if storagePath != wantStorage {
				t.Fatalf("GetStoragePath() = %q, want retained %q", storagePath, wantStorage)
			}
		})
	}
}

func TestLoadRejectsEscapingCentralizedStorageAuthority(t *testing.T) {
	root := t.TempDir()
	home := makeAuthorityDir(t, filepath.Join(root, "home"))
	workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
	external := makeAuthorityDir(t, filepath.Join(root, "external"))
	t.Setenv("HOME", home)
	clearWorkspaceDiscoveryEnv(t)
	if err := os.Symlink(external, filepath.Join(workspace, "linked-outside")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	for _, relativePath := range []string{
		"../escape",
		".",
		"nested/../escape",
		filepath.Join(root, "absolute"),
		"linked-outside",
	} {
		t.Run(relativePath, func(t *testing.T) {
			cfgPath := writeRuntimeAuthorityConfig(t, t.TempDir(), workspace, relativePath)
			if _, err := Load(cfgPath); err == nil {
				t.Fatalf("Load() accepted escaping storage.relative_path %q", relativePath)
			}
		})
	}
}

func TestLoadRejectsDanglingAuthoritySymlinks(t *testing.T) {
	for _, target := range []string{"storage", "sandboxes"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			home := makeAuthorityDir(t, filepath.Join(root, "home"))
			workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
			t.Setenv("HOME", home)
			clearWorkspaceDiscoveryEnv(t)

			relativePath := ".agm-work"
			linkPath := filepath.Join(workspace, relativePath)
			if target == "sandboxes" {
				if err := os.Mkdir(linkPath, 0o700); err != nil {
					t.Fatal(err)
				}
				linkPath = filepath.Join(linkPath, "sandboxes")
			}
			if err := os.Symlink(filepath.Join(root, "missing-target"), linkPath); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			cfgPath := writeRuntimeAuthorityConfig(t, root, workspace, relativePath)
			if _, err := Load(cfgPath); err == nil {
				t.Fatalf("Load() accepted dangling %s authority symlink", target)
			}
		})
	}
}

func TestLoadResolvesContainedSymlinkWithMissingTail(t *testing.T) {
	root := t.TempDir()
	home := makeAuthorityDir(t, filepath.Join(root, "home"))
	workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
	contained := makeAuthorityDir(t, filepath.Join(workspace, "contained"))
	t.Setenv("HOME", home)
	clearWorkspaceDiscoveryEnv(t)
	if err := os.Symlink(contained, filepath.Join(workspace, "linked-inside")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cfg, err := Load(writeRuntimeAuthorityConfig(t, root, workspace, "linked-inside/missing"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatal(err)
	}
	storage := filepath.Join(contained, "missing")
	assertRuntimeAuthorityPaths(t, authority, home, storage, filepath.Join(storage, "sandboxes"))
}

func TestLoadPreservesExistingExternalDotfileSymlink(t *testing.T) {
	root := t.TempDir()
	home := makeAuthorityDir(t, filepath.Join(root, "home"))
	externalStorage := makeAuthorityDir(t, filepath.Join(root, "external-storage"))
	driftStorage := makeAuthorityDir(t, filepath.Join(root, "drift-storage"))
	t.Setenv("HOME", home)
	symlinkOrSkip(t, externalStorage, filepath.Join(home, ".agm"))

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeAuthorityPaths(t, authority, home, externalStorage, filepath.Join(externalStorage, "sandboxes"))
	retargetSymlink(t, filepath.Join(home, ".agm"), driftStorage)
	assertRuntimeAuthorityPaths(t, authority, home, externalStorage, filepath.Join(externalStorage, "sandboxes"))
}

func TestRuntimeAuthorityRejectsPostLoadSymlinkInsertion(t *testing.T) {
	root := t.TempDir()
	home := makeAuthorityDir(t, filepath.Join(root, "home"))
	external := makeAuthorityDir(t, filepath.Join(root, "external"))
	sentinel := filepath.Join(external, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(home, ".agm")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	rootAuthority, err := authority.Sandboxes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rootAuthority.Workspace("session"); err == nil {
		t.Fatal("Workspace() accepted a post-load storage symlink insertion")
	}
	if _, err := GetStoragePath(cfg); err == nil {
		t.Fatal("GetStoragePath() accepted a post-load storage symlink insertion")
	}
	assertFileContents(t, sentinel, "preserve")
}

func TestSandboxRootRejectsExistingSessionSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	home := makeAuthorityDir(t, filepath.Join(root, "home"))
	sandboxes := makeAuthorityDir(t, filepath.Join(home, ".agm", "sandboxes"))
	external := makeAuthorityDir(t, filepath.Join(root, "external"))
	sentinel := filepath.Join(external, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(sandboxes, "session")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatal(err)
	}
	sandboxRoot, err := authority.Sandboxes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sandboxRoot.Workspace("session"); err == nil {
		t.Fatal("Workspace() accepted an existing session symlink escape")
	}
	assertFileContents(t, sentinel, "preserve")
}

func TestRuntimeAuthorityRejectsUnloadedAndZeroValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for name, cfg := range map[string]*Config{
		"nil":     nil,
		"direct":  {},
		"default": Default(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := cfg.RuntimeAuthority()
			if !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
				t.Fatalf("RuntimeAuthority() error = %v, want ErrRuntimeAuthorityUnavailable", err)
			}
		})
	}

	var authority RuntimeAuthority
	if _, err := authority.Home(); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("zero RuntimeAuthority.Home() error = %v, want ErrRuntimeAuthorityUnavailable", err)
	}
	if _, err := authority.Storage(); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("zero RuntimeAuthority.Storage() error = %v, want ErrRuntimeAuthorityUnavailable", err)
	}
	if _, err := authority.Sandboxes(); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("zero RuntimeAuthority.Sandboxes() error = %v, want ErrRuntimeAuthorityUnavailable", err)
	}
	for label, root := range map[string]interface{ Path() (string, error) }{
		"home":      HomeRoot{},
		"storage":   StorageRoot{},
		"sandboxes": SandboxRoot{},
	} {
		if _, err := root.Path(); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
			t.Fatalf("zero %s Path() error = %v, want ErrRuntimeAuthorityUnavailable", label, err)
		}
	}
	if _, err := (SandboxRoot{}).Workspace("session"); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("zero SandboxRoot.Workspace() error = %v, want ErrRuntimeAuthorityUnavailable", err)
	}
}

func TestSandboxRootRejectsInvalidWorkspaceComponent(t *testing.T) {
	home := physicalPath(t, t.TempDir())
	t.Setenv("HOME", home)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatal(err)
	}
	sandboxRoot, err := authority.Sandboxes()
	if err != nil {
		t.Fatal(err)
	}

	for _, sessionID := range []string{"", ".", "..", "nested/session", `nested\session`, filepath.Join(home, "absolute")} {
		t.Run(sessionID, func(t *testing.T) {
			if _, err := sandboxRoot.Workspace(sessionID); err == nil {
				t.Fatalf("Workspace(%q) error = nil, want rejection", sessionID)
			}
		})
	}

	got, err := sandboxRoot.Workspace("session-123")
	if err != nil {
		t.Fatalf("Workspace() error = %v", err)
	}
	want := filepath.Join(home, ".agm", "sandboxes", "session-123")
	if got != want {
		t.Fatalf("Workspace() = %q, want %q", got, want)
	}
}

func assertRuntimeAuthorityPaths(
	t *testing.T,
	authority RuntimeAuthority,
	wantHome string,
	wantStorage string,
	wantSandboxes string,
) {
	t.Helper()
	home, err := authority.Home()
	if err != nil {
		t.Fatal(err)
	}
	storage, err := authority.Storage()
	if err != nil {
		t.Fatal(err)
	}
	sandboxes, err := authority.Sandboxes()
	if err != nil {
		t.Fatal(err)
	}
	for label, paths := range map[string]struct {
		root interface{ Path() (string, error) }
		want string
	}{
		"home":      {root: home, want: wantHome},
		"storage":   {root: storage, want: wantStorage},
		"sandboxes": {root: sandboxes, want: wantSandboxes},
	} {
		got, err := paths.root.Path()
		if err != nil {
			t.Fatalf("%s Path() error = %v", label, err)
		}
		if got != paths.want {
			t.Fatalf("%s Path() = %q, want %q", label, got, paths.want)
		}
	}
}

func clearWorkspaceDiscoveryEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"ENGRAM_TEST_MODE", "ENGRAM_TEST_WORKSPACE", "ENGRAM_WORKSPACE"} {
		t.Setenv(key, "")
	}
}

func makeAuthorityDir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return physicalPath(t, path)
}

func physicalPath(t *testing.T, path string) string {
	t.Helper()
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}
	return physical
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func retargetSymlink(t *testing.T, link, target string) {
	t.Helper()
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, link)
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func writeRuntimeAuthorityConfig(t *testing.T, dir, workspace, relativePath string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	contents := fmt.Appendf(nil,
		"storage:\n  mode: centralized\n  workspace: %q\n  relative_path: %q\n",
		workspace,
		relativePath,
	)
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
