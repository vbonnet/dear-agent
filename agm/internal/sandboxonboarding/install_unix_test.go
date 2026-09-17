//go:build darwin || linux

package sandboxonboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestInstallCreatesPrivateChainAndPreservesExistingInstructions(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	projectDirectory := privateDirectory(t, filepath.Dir(outputPath(home, request.WorkingDir)))
	for _, directory := range []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".claude", "projects"),
		projectDirectory,
	} {
		if err := os.Chmod(directory, privateDirectoryMode); err != nil {
			t.Fatal(err)
		}
	}
	original := "# Existing user instructions\nkeep this\n"
	if err := os.WriteFile(outputPath(home, request.WorkingDir), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(request); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	installed, err := os.ReadFile(outputPath(home, request.WorkingDir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(installed), "test-session") || !strings.HasSuffix(string(installed), original) {
		t.Fatalf("installed content did not prepend onboarding and preserve original:\n%s", installed)
	}
	info, err := os.Stat(outputPath(home, request.WorkingDir))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != privateFileMode {
		t.Fatalf("installed mode = %04o, want %04o", got, privateFileMode)
	}
}

func TestInstallCreatesMissingDirectoryChainAt0700(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	if err := Install(request); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	for _, directory := range []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".claude", "projects"),
		filepath.Dir(outputPath(home, request.WorkingDir)),
	} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != privateDirectoryMode {
			t.Fatalf("directory %q mode = %04o, want %04o", directory, got, privateDirectoryMode)
		}
	}
}

func TestInstallCleansCreatedDirectoryRejectedAfterMkdir(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))

	priorMask := unix.Umask(0o777)
	err := Install(request)
	unix.Umask(priorMask)
	if err == nil || !strings.Contains(err.Error(), "has mode 0000") {
		t.Fatalf("Install() error = %v, want created-directory mode rejection", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".claude")); !os.IsNotExist(statErr) {
		t.Fatalf("rejected transaction-created directory was not cleaned: %v", statErr)
	}
}

func TestInstallIgnoresAmbientHomeDrift(t *testing.T) {
	retainedHome := privateDirectory(t, filepath.Join(t.TempDir(), "retained"))
	driftHome := privateDirectory(t, filepath.Join(t.TempDir(), "drift"))
	homeRoot := loadHomeRoot(t, retainedHome)
	t.Setenv("HOME", driftHome)
	request := testRequest(homeRoot)

	if err := Install(request); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(outputPath(retainedHome, request.WorkingDir)); err != nil {
		t.Fatalf("retained HOME output missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(driftHome, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("ambient HOME was mutated: %v", err)
	}
}

func TestInstallRejectsSymlinkedOutputNodesWithoutExternalMutation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, home, outside, workingDir string)
	}{
		{
			name: ".claude",
			setup: func(t *testing.T, home, outside, _ string) {
				t.Helper()
				if err := os.Symlink(outside, filepath.Join(home, ".claude")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
		{
			name: "projects",
			setup: func(t *testing.T, home, outside, _ string) {
				t.Helper()
				privateDirectory(t, filepath.Join(home, ".claude"))
				if err := os.Symlink(outside, filepath.Join(home, ".claude", "projects")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
		{
			name: "encoded project directory",
			setup: func(t *testing.T, home, outside, workingDir string) {
				t.Helper()
				privateDirectory(t, filepath.Join(home, ".claude", "projects"))
				if err := os.Symlink(outside, filepath.Join(home, ".claude", "projects", encodedProjectName(workingDir))); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
		{
			name: "final CLAUDE.md",
			setup: func(t *testing.T, home, outside, workingDir string) {
				t.Helper()
				projectDirectory := privateDirectory(t, filepath.Join(home, ".claude", "projects", encodedProjectName(workingDir)))
				if err := os.Symlink(filepath.Join(outside, "sentinel"), filepath.Join(projectDirectory, outputName)); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
			outside := privateDirectory(t, filepath.Join(t.TempDir(), "outside"))
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			request := testRequest(loadHomeRoot(t, home))
			test.setup(t, home, outside, request.WorkingDir)

			if err := Install(request); err == nil {
				t.Fatal("Install() error = nil, want unsafe-node rejection")
			}
			content, err := os.ReadFile(sentinel)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "preserve" {
				t.Fatalf("outside sentinel = %q, want preserved", content)
			}
			entries, err := os.ReadDir(outside)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "sentinel" {
				t.Fatalf("outside directory mutated: %v", entries)
			}
		})
	}
}

func TestInstallRejectsFIFOFinal(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	target := outputPath(home, request.WorkingDir)
	privateDirectory(t, filepath.Dir(target))
	if err := unix.Mkfifo(target, 0o600); err != nil {
		t.Skipf("FIFO unavailable: %v", err)
	}

	if err := Install(request); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Install() error = %v, want FIFO rejection", err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("final node mode = %v, want preserved FIFO", info.Mode())
	}
}

func TestInstallRejectsFIFOOutputDirectoriesWithoutExternalMutation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, home, workingDir string) string
	}{
		{
			name: ".claude",
			setup: func(_ *testing.T, home, _ string) string {
				return filepath.Join(home, ".claude")
			},
		},
		{
			name: "projects",
			setup: func(t *testing.T, home, _ string) string {
				privateDirectory(t, filepath.Join(home, ".claude"))
				return filepath.Join(home, ".claude", "projects")
			},
		},
		{
			name: "encoded project directory",
			setup: func(t *testing.T, home, workingDir string) string {
				privateDirectory(t, filepath.Join(home, ".claude", "projects"))
				return filepath.Join(home, ".claude", "projects", encodedProjectName(workingDir))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
			outside := privateDirectory(t, filepath.Join(t.TempDir(), "outside"))
			sentinel := filepath.Join(outside, "sentinel")
			if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			request := testRequest(loadHomeRoot(t, home))
			fifo := test.setup(t, home, request.WorkingDir)
			if err := unix.Mkfifo(fifo, 0o600); err != nil {
				t.Skipf("FIFO unavailable: %v", err)
			}

			if err := Install(request); err == nil || !strings.Contains(err.Error(), "not a real directory") {
				t.Fatalf("Install() error = %v, want directory FIFO rejection", err)
			}
			info, err := os.Lstat(fifo)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("directory node mode = %v, want preserved FIFO", info.Mode())
			}
			content, err := os.ReadFile(sentinel)
			if err != nil {
				t.Fatal(err)
			}
			if string(content) != "preserve" {
				t.Fatalf("outside sentinel = %q, want preserved", content)
			}
			entries, err := os.ReadDir(outside)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "sentinel" {
				t.Fatalf("outside directory mutated: %v", entries)
			}
		})
	}
}

func TestValidateDirectoryRejectsForeignEffectiveUID(t *testing.T) {
	foreignUID := uint32(os.Geteuid()) + 1
	stat := unix.Stat_t{Mode: unix.S_IFDIR | privateDirectoryMode, Uid: foreignUID}

	err := validateDirectory("synthetic foreign directory", &stat, false)
	if err == nil || !strings.Contains(err.Error(), "owned by uid") {
		t.Fatalf("validateDirectory() error = %v, want foreign-owner rejection", err)
	}
}

func TestInstallRejectsUnsafeExistingDirectoryAndFinal(t *testing.T) {
	t.Run("group-writable directory", func(t *testing.T) {
		home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
		request := testRequest(loadHomeRoot(t, home))
		claudeDirectory := privateDirectory(t, filepath.Join(home, ".claude"))
		if err := os.Chmod(claudeDirectory, 0o770); err != nil {
			t.Fatal(err)
		}
		if err := Install(request); err == nil || !strings.Contains(err.Error(), "group- or world-writable") {
			t.Fatalf("Install() error = %v, want unsafe-directory rejection", err)
		}
	})

	t.Run("hard-linked final", func(t *testing.T) {
		home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
		request := testRequest(loadHomeRoot(t, home))
		target := outputPath(home, request.WorkingDir)
		privateDirectory(t, filepath.Dir(target))
		sentinel := filepath.Join(t.TempDir(), "sentinel")
		if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(sentinel, target); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		if err := Install(request); err == nil || !strings.Contains(err.Error(), "hard links") {
			t.Fatalf("Install() error = %v, want hard-link rejection", err)
		}
		content, err := os.ReadFile(sentinel)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "preserve" {
			t.Fatalf("hard-link sentinel = %q, want preserved", content)
		}
	})

	t.Run("group-writable final", func(t *testing.T) {
		home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
		request := testRequest(loadHomeRoot(t, home))
		target := outputPath(home, request.WorkingDir)
		privateDirectory(t, filepath.Dir(target))
		if err := os.WriteFile(target, []byte("preserve"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(target, 0o620); err != nil {
			t.Fatal(err)
		}
		if err := Install(request); err == nil || !strings.Contains(err.Error(), "group- or world-writable") {
			t.Fatalf("Install() error = %v, want unsafe-final rejection", err)
		}
		content, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "preserve" {
			t.Fatalf("unsafe final = %q, want preserved", content)
		}
	})
}

func TestInstallRejectsTargetIdentityChangeBeforeCommit(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	target := outputPath(home, request.WorkingDir)
	privateDirectory(t, filepath.Dir(target))
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	hooks := installHooks{beforeCommit: func() {
		if err := os.Remove(target); err != nil {
			t.Errorf("remove target in commit hook: %v", err)
			return
		}
		if err := os.WriteFile(target, []byte("replacement"), 0o600); err != nil {
			t.Errorf("replace target in commit hook: %v", err)
		}
	}}

	if err := installWithHooks(request, hooks); err == nil || !strings.Contains(err.Error(), "target changed before commit") {
		t.Fatalf("Install() error = %v, want target identity-change rejection", err)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "replacement" {
		t.Fatalf("replacement target = %q, want preserved", content)
	}
	assertNoTemporaryFiles(t, filepath.Dir(target))
}

func TestInstallRejectsParentIdentityChangeAndPreservesReplacement(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	projectDirectory := filepath.Dir(outputPath(home, request.WorkingDir))
	movedDirectory := projectDirectory + "-moved"
	replacementSentinel := filepath.Join(projectDirectory, "replacement-sentinel")

	hooks := installHooks{beforeCommit: func() {
		if err := os.Rename(projectDirectory, movedDirectory); err != nil {
			t.Errorf("move authenticated project directory in commit hook: %v", err)
			return
		}
		if err := os.Mkdir(projectDirectory, privateDirectoryMode); err != nil {
			t.Errorf("create replacement project directory in commit hook: %v", err)
			return
		}
		if err := os.WriteFile(replacementSentinel, []byte("replacement"), privateFileMode); err != nil {
			t.Errorf("write replacement sentinel in commit hook: %v", err)
		}
	}}

	err := installWithHooks(request, hooks)
	if err == nil || !strings.Contains(err.Error(), "visible onboarding directory") ||
		!strings.Contains(err.Error(), "changed identity") ||
		!strings.Contains(err.Error(), "visible entry no longer matches the created directory") {
		t.Fatalf("Install() error = %v, want parent identity and cleanup refusal errors", err)
	}
	content, readErr := os.ReadFile(replacementSentinel)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "replacement" {
		t.Fatalf("replacement sentinel = %q, want preserved", content)
	}
}

func TestInstallRejectsStagedTemporaryReplacementAndPreservesIt(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	projectDirectory := privateDirectory(t, filepath.Dir(outputPath(home, request.WorkingDir)))
	var replacementPath string

	hooks := installHooks{beforeCommit: func() {
		entries, err := os.ReadDir(projectDirectory)
		if err != nil {
			t.Errorf("read project directory in commit hook: %v", err)
			return
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".CLAUDE.md.agm-") {
				replacementPath = filepath.Join(projectDirectory, entry.Name())
				break
			}
		}
		if replacementPath == "" {
			t.Error("onboarding temporary file not found in commit hook")
			return
		}
		replacementSource := filepath.Join(projectDirectory, ".replacement-temporary")
		if err := os.WriteFile(replacementSource, []byte("replacement"), privateFileMode); err != nil {
			t.Errorf("write replacement temporary source in commit hook: %v", err)
			return
		}
		if err := os.Rename(replacementSource, replacementPath); err != nil {
			t.Errorf("replace staged temporary file in commit hook: %v", err)
		}
	}}

	err := installWithHooks(request, hooks)
	if err == nil || !strings.Contains(err.Error(), "temporary file changed before commit") ||
		!strings.Contains(err.Error(), "visible entry no longer matches the created file") {
		t.Fatalf("Install() error = %v, want temporary identity and cleanup refusal errors", err)
	}
	content, readErr := os.ReadFile(replacementPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "replacement" {
		t.Fatalf("replacement temporary file = %q, want preserved", content)
	}
}

func TestInstallJoinsCleanupErrorsWithoutRemovingNewContent(t *testing.T) {
	home := privateDirectory(t, filepath.Join(t.TempDir(), "home"))
	request := testRequest(loadHomeRoot(t, home))
	target := outputPath(home, request.WorkingDir)

	hooks := installHooks{beforeCommit: func() {
		if err := os.WriteFile(target, []byte("concurrent"), 0o600); err != nil {
			t.Errorf("create concurrent target in commit hook: %v", err)
		}
	}}

	err := installWithHooks(request, hooks)
	if err == nil || !strings.Contains(err.Error(), "target changed before commit") ||
		!strings.Contains(err.Error(), "cleanup onboarding directory") {
		t.Fatalf("Install() error = %v, want primary and cleanup failures", err)
	}
	content, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "concurrent" {
		t.Fatalf("concurrent target = %q, want preserved", content)
	}
	assertNoTemporaryFiles(t, filepath.Dir(target))
}

func assertNoTemporaryFiles(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".CLAUDE.md.agm-") {
			t.Fatalf("temporary file was not cleaned up: %s", entry.Name())
		}
	}
}
