package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetStoragePath(t *testing.T) {
	t.Run("loaded dotfile authority", func(t *testing.T) {
		home := physicalPath(t, t.TempDir())
		t.Setenv("HOME", home)
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		got, err := GetStoragePath(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, ".agm"); got != want {
			t.Fatalf("GetStoragePath() = %q, want %q", got, want)
		}
	})

	t.Run("loaded centralized authority", func(t *testing.T) {
		root := t.TempDir()
		home := makeAuthorityDir(t, filepath.Join(root, "home"))
		workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
		t.Setenv("HOME", home)
		clearWorkspaceDiscoveryEnv(t)
		cfg, err := Load(writeRuntimeAuthorityConfig(t, root, workspace, ".agm-work"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := GetStoragePath(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(workspace, ".agm-work"); got != want {
			t.Fatalf("GetStoragePath() = %q, want %q", got, want)
		}
	})

	for name, cfg := range map[string]*Config{
		"direct":  {},
		"invalid": {Storage: StorageConfig{Mode: "invalid"}},
	} {
		t.Run(name+" config has no authority", func(t *testing.T) {
			if _, err := GetStoragePath(cfg); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
				t.Fatalf("GetStoragePath() error = %v, want ErrRuntimeAuthorityUnavailable", err)
			}
		})
	}
}

func TestDetectWorkspace(t *testing.T) {
	tests := []struct {
		name          string
		nameOrPath    string
		envVars       map[string]string
		expectedError bool
	}{
		{
			name:          "absolute path exists",
			nameOrPath:    "/tmp",
			expectedError: false,
		},
		{
			name:       "test mode env var",
			nameOrPath: "test-workspace",
			envVars: map[string]string{
				"ENGRAM_TEST_MODE":      "1",
				"ENGRAM_TEST_WORKSPACE": "/tmp/test-workspace",
			},
			expectedError: false,
		},
		{
			name:       "ENGRAM_WORKSPACE env var",
			nameOrPath: "ignored",
			envVars: map[string]string{
				"ENGRAM_WORKSPACE": "/tmp/engram-workspace",
			},
			expectedError: false,
		},
		{
			name:          "workspace not found",
			nameOrPath:    "nonexistent-workspace-12345",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			workspace, err := DetectWorkspace(tt.nameOrPath)

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if workspace == "" {
				t.Errorf("workspace path is empty")
			}

			if !filepath.IsAbs(workspace) {
				t.Errorf("workspace path should be absolute, got: %s", workspace)
			}
		})
	}
}

func TestHasWorkspaceMarker(t *testing.T) {
	// Create temp directory with .git marker
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	os.Mkdir(gitDir, 0755)

	tests := []struct {
		name       string
		dir        string
		targetName string
		expected   bool
	}{
		{
			name:       "has .git marker with matching name",
			dir:        tmpDir,
			targetName: filepath.Base(tmpDir),
			expected:   true,
		},
		{
			name:       "has .git marker with empty target name",
			dir:        tmpDir,
			targetName: "",
			expected:   true,
		},
		{
			name:       "has .git marker with non-matching name",
			dir:        tmpDir,
			targetName: "different-name",
			expected:   false,
		},
		{
			name:       "no markers",
			dir:        "/tmp",
			targetName: "test",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasWorkspaceMarker(tt.dir, tt.targetName)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEnsureSymlinkBootstrap(t *testing.T) {
	t.Run("loaded dotfile mode does nothing", func(t *testing.T) {
		home := physicalPath(t, t.TempDir())
		t.Setenv("HOME", home)
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if err := EnsureSymlinkBootstrap(cfg); err != nil {
			t.Fatalf("EnsureSymlinkBootstrap() error = %v", err)
		}
		if _, err := os.Lstat(filepath.Join(home, ".agm")); !os.IsNotExist(err) {
			t.Fatalf("dotfile bootstrap unexpectedly mutated HOME: %v", err)
		}
	})

	t.Run("unloaded config fails closed", func(t *testing.T) {
		cfg := &Config{Storage: StorageConfig{Mode: "centralized"}}
		if err := EnsureSymlinkBootstrap(cfg); !errors.Is(err, ErrRuntimeAuthorityUnavailable) {
			t.Fatalf("EnsureSymlinkBootstrap() error = %v, want ErrRuntimeAuthorityUnavailable", err)
		}
	})
}

func TestCentralizedStorageBootstrapUsesCapturedAuthority(t *testing.T) {
	clearWorkspaceDiscoveryEnv(t)
	root := t.TempDir()
	physicalHome := makeAuthorityDir(t, filepath.Join(root, "physical-home"))
	driftHome := makeAuthorityDir(t, filepath.Join(root, "drift-home"))
	logicalHome := filepath.Join(root, "logical-home")
	symlinkOrSkip(t, physicalHome, logicalHome)
	selectedWorkspace := makeAuthorityDir(t, filepath.Join(root, "selected-workspace"))
	driftWorkspace := makeAuthorityDir(t, filepath.Join(root, "drift-workspace"))
	t.Setenv("HOME", logicalHome)
	t.Setenv("ENGRAM_WORKSPACE", selectedWorkspace)

	cfg, err := Load(writeRuntimeAuthorityConfig(t, root, "selected", ".agm-work"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	retargetSymlink(t, logicalHome, driftHome)
	t.Setenv("HOME", driftHome)
	t.Setenv("ENGRAM_WORKSPACE", driftWorkspace)
	t.Chdir(driftWorkspace)
	cfg.Storage = StorageConfig{Mode: "dotfile", Workspace: driftWorkspace, RelativePath: ".drift"}

	sentinel := filepath.Join(driftHome, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSymlinkBootstrap(cfg); err != nil {
		t.Fatalf("EnsureSymlinkBootstrap() error = %v", err)
	}
	if err := VerifyStorageIntegrity(cfg); err != nil {
		t.Fatalf("VerifyStorageIntegrity() error = %v", err)
	}

	dotfileLink := filepath.Join(physicalHome, ".agm")
	info, err := os.Lstat(dotfileLink)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", dotfileLink)
	}
	if got, err := filepath.EvalSymlinks(dotfileLink); err != nil {
		t.Fatal(err)
	} else if want := filepath.Join(selectedWorkspace, ".agm-work"); got != want {
		t.Fatalf("bootstrap target = %q, want retained %q", got, want)
	}
	assertFileContents(t, sentinel, "preserve")
	if _, err := os.Lstat(filepath.Join(driftHome, ".agm")); !os.IsNotExist(err) {
		t.Fatalf("bootstrap mutated drift HOME: %v", err)
	}
}

func TestEnsureSymlinkBootstrapRepairsExistingCompatibilityLinks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dangling bool
	}{
		{name: "wrong existing target"},
		{name: "dangling wrong target", dangling: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearWorkspaceDiscoveryEnv(t)
			root := t.TempDir()
			home := makeAuthorityDir(t, filepath.Join(root, "home"))
			workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
			t.Setenv("HOME", home)
			cfg, err := Load(writeRuntimeAuthorityConfig(t, root, workspace, ".agm-work"))
			if err != nil {
				t.Fatal(err)
			}

			wrongTarget := filepath.Join(root, "wrong-target")
			if !tc.dangling {
				if err := os.Mkdir(wrongTarget, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(wrongTarget, "sentinel"), []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			compatibilityLink := filepath.Join(home, ".agm")
			symlinkOrSkip(t, wrongTarget, compatibilityLink)

			if err := EnsureSymlinkBootstrap(cfg); err != nil {
				t.Fatalf("EnsureSymlinkBootstrap() error = %v", err)
			}
			resolved, err := filepath.EvalSymlinks(compatibilityLink)
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(workspace, ".agm-work"); resolved != want {
				t.Fatalf("compatibility link = %q, want retained storage %q", resolved, want)
			}
			if tc.dangling {
				if _, err := os.Lstat(wrongTarget); !os.IsNotExist(err) {
					t.Fatalf("bootstrap created dangling wrong target: %v", err)
				}
			} else {
				assertFileContents(t, filepath.Join(wrongTarget, "sentinel"), "preserve")
			}
		})
	}
}

func TestEnsureSymlinkBootstrapHealsCorrectDanglingCompatibilityLink(t *testing.T) {
	clearWorkspaceDiscoveryEnv(t)
	root := t.TempDir()
	home := makeAuthorityDir(t, filepath.Join(root, "home"))
	workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
	storagePath := filepath.Join(workspace, ".agm-work")
	t.Setenv("HOME", home)
	cfg, err := Load(writeRuntimeAuthorityConfig(t, root, workspace, ".agm-work"))
	if err != nil {
		t.Fatal(err)
	}

	compatibilityLink := filepath.Join(home, ".agm")
	symlinkOrSkip(t, storagePath, compatibilityLink)
	if _, err := os.Stat(storagePath); !os.IsNotExist(err) {
		t.Fatalf("precondition: storage target exists before bootstrap: %v", err)
	}

	if err := EnsureSymlinkBootstrap(cfg); err != nil {
		t.Fatalf("EnsureSymlinkBootstrap() error = %v", err)
	}
	if info, err := os.Stat(storagePath); err != nil {
		t.Fatalf("storage target was not healed: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("storage target mode = %v, want directory", info.Mode())
	}
	if resolved, err := filepath.EvalSymlinks(compatibilityLink); err != nil {
		t.Fatal(err)
	} else if resolved != storagePath {
		t.Fatalf("compatibility link = %q, want retained storage %q", resolved, storagePath)
	}
	if err := VerifyStorageIntegrity(cfg); err != nil {
		t.Fatalf("VerifyStorageIntegrity() error = %v", err)
	}
}

func TestVerifyStorageIntegrityRejectsRetargetedCompatibilityLink(t *testing.T) {
	clearWorkspaceDiscoveryEnv(t)
	root := t.TempDir()
	home := makeAuthorityDir(t, filepath.Join(root, "home"))
	workspace := makeAuthorityDir(t, filepath.Join(root, "workspace"))
	wrongTarget := makeAuthorityDir(t, filepath.Join(root, "wrong-target"))
	sentinel := filepath.Join(wrongTarget, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	cfg, err := Load(writeRuntimeAuthorityConfig(t, root, workspace, ".agm-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureSymlinkBootstrap(cfg); err != nil {
		t.Fatal(err)
	}

	compatibilityLink := filepath.Join(home, ".agm")
	if err := os.Remove(compatibilityLink); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, wrongTarget, compatibilityLink)
	if err := VerifyStorageIntegrity(cfg); err == nil {
		t.Fatal("VerifyStorageIntegrity() error = nil, want wrong-target rejection")
	} else if !strings.Contains(err.Error(), "symlink points to wrong location") {
		t.Fatalf("VerifyStorageIntegrity() error = %v, want wrong-location context", err)
	}
	assertFileContents(t, sentinel, "preserve")
}

func TestVerifyStorageIntegrityUsesLoadedDotfileAuthority(t *testing.T) {
	home := physicalPath(t, t.TempDir())
	t.Setenv("HOME", home)
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".agm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyStorageIntegrity(cfg); err != nil {
		t.Fatalf("VerifyStorageIntegrity() error = %v", err)
	}
}

func TestCopyDir(t *testing.T) {
	// Create source directory with test files
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create test structure
	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	os.Mkdir(filepath.Join(srcDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)

	// Copy directory
	err := copyDir(srcDir, dstDir)
	if err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify files copied
	tests := []struct {
		path    string
		content string
	}{
		{"file1.txt", "content1"},
		{"subdir/file2.txt", "content2"},
	}

	for _, tt := range tests {
		dstFile := filepath.Join(dstDir, tt.path)
		content, err := os.ReadFile(dstFile)
		if err != nil {
			t.Errorf("failed to read copied file %s: %v", tt.path, err)
			continue
		}
		if string(content) != tt.content {
			t.Errorf("file %s: expected content %q, got %q", tt.path, tt.content, string(content))
		}
	}
}
