package commands

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/retrospective"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestRewindTransitionLockHelper(t *testing.T) {
	projectDir := os.Getenv("GO_WANT_REWIND_LOCK_HELPER")
	if projectDir == "" {
		return
	}
	lock, err := acquireRewindTransitionLock(projectDir)
	if err != nil {
		t.Fatalf("acquire helper lock: %v", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, "locked"); err != nil {
		t.Fatalf("signal lock acquisition: %v", err)
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatalf("wait for helper release: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("release helper lock: %v", err)
	}
}

func TestRewindTransitionLockUsesOneNamespaceAcrossProjectAliases(t *testing.T) {
	projectDir := t.TempDir()
	aliases := make([]string, 0, 2)
	symlinkAlias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(projectDir, symlinkAlias); err == nil {
		aliases = append(aliases, symlinkAlias)
	}
	if caseAlias := caseAliasForTest(projectDir); caseAlias != projectDir {
		aliases = append(aliases, caseAlias)
	}
	if len(aliases) == 0 {
		t.Skip("filesystem does not expose a project path alias")
	}

	cleanupRewindLockFile(t, projectDir)
	lock, err := acquireRewindTransitionLock(projectDir)
	if err != nil {
		t.Fatalf("acquire original lock: %v", err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Errorf("release original lock: %v", err)
		}
	})

	for _, alias := range aliases {
		aliasLock, err := acquireRewindTransitionLock(alias)
		if err == nil {
			if closeErr := aliasLock.Close(); closeErr != nil {
				t.Errorf("release unexpectedly acquired alias lock: %v", closeErr)
			}
			t.Fatalf("acquire lock through alias %q succeeded, want in-progress error", alias)
		}
		if !errors.Is(err, errRewindTransitionInProgress) {
			t.Fatalf("acquire lock through alias %q = %v, want in-progress error", alias, err)
		}
	}
}

func TestRewindTransitionLockUsesProjectStorageWithoutWritableHome(t *testing.T) {
	t.Run("missing home", func(t *testing.T) {
		homeDir := filepath.Join(t.TempDir(), "missing-home")
		assertProjectLocalRewindLock(t, homeDir)
	})

	t.Run("read-only home", func(t *testing.T) {
		homeDir := filepath.Join(t.TempDir(), "read-only-home")
		if err := os.Mkdir(homeDir, 0o500); err != nil {
			t.Fatalf("create read-only home: %v", err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(homeDir, 0o700); err != nil {
				t.Errorf("restore test home permissions: %v", err)
			}
		})
		assertProjectLocalRewindLock(t, homeDir)
	})
}

func assertProjectLocalRewindLock(t *testing.T, homeDir string) {
	t.Helper()
	projectDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("TMPDIR", t.TempDir())

	lockPath, err := rewindLockFilePath(projectDir)
	if err != nil {
		t.Fatalf("resolve project-local lock path: %v", err)
	}
	wantPath := filepath.Join(projectDir, ".wayfinder", "locks", rewindLockFilename)
	if lockPath != wantPath {
		t.Fatalf("lock path = %q, want project-owned path %q", lockPath, wantPath)
	}

	lock, err := acquireRewindTransitionLock(projectDir)
	if err != nil {
		t.Fatalf("acquire project-local lock: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("release project-local lock: %v", err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove project-local lock: %v", err)
	}
}

func TestRewindTransitionLockRejectsLinkedMetadataComponents(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, projectDir, externalDir string) (linkPath, escapedLockPath string)
	}{
		{
			name: "wayfinder metadata",
			setup: func(t *testing.T, projectDir, externalDir string) (string, string) {
				t.Helper()
				return filepath.Join(projectDir, ".wayfinder"), filepath.Join(externalDir, "locks", rewindLockFilename)
			},
		},
		{
			name: "lock directory",
			setup: func(t *testing.T, projectDir, externalDir string) (string, string) {
				t.Helper()
				wayfinderDir := filepath.Join(projectDir, ".wayfinder")
				if err := os.Mkdir(wayfinderDir, 0o700); err != nil {
					t.Fatalf("create Wayfinder metadata directory: %v", err)
				}
				return filepath.Join(wayfinderDir, "locks"), filepath.Join(externalDir, rewindLockFilename)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			externalDir := t.TempDir()
			linkPath, escapedLockPath := test.setup(t, projectDir, externalDir)
			if err := os.Symlink(externalDir, linkPath); err != nil {
				t.Skipf("filesystem does not permit directory symlinks: %v", err)
			}

			lock, err := acquireRewindTransitionLock(projectDir)
			if err == nil {
				if closeErr := lock.Close(); closeErr != nil {
					t.Errorf("release unexpectedly acquired redirected lock: %v", closeErr)
				}
				t.Fatal("acquire rewind lock succeeded through linked metadata")
			}
			if _, statErr := os.Stat(escapedLockPath); !os.IsNotExist(statErr) {
				t.Fatalf("redirected lock exists outside project: %v", statErr)
			}
		})
	}
}

func TestRewindTransitionLockRejectsLinkedLockFile(t *testing.T) {
	tests := []struct {
		name       string
		createLink func(oldPath, newPath string) error
	}{
		{name: "symbolic link", createLink: os.Symlink},
		{name: "multiple hard links", createLink: os.Link},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			lockPath, err := rewindLockFilePath(projectDir)
			if err != nil {
				t.Fatalf("prepare project lock path: %v", err)
			}
			externalPath := filepath.Join(t.TempDir(), "external.txt")
			wantContent := []byte("must remain an unrelated file\n")
			if err := os.WriteFile(externalPath, wantContent, 0o600); err != nil {
				t.Fatalf("write external file: %v", err)
			}
			if err := test.createLink(externalPath, lockPath); err != nil {
				t.Skipf("filesystem does not permit %s: %v", test.name, err)
			}

			lock, err := acquireRewindTransitionLock(projectDir)
			if err == nil {
				if closeErr := lock.Close(); closeErr != nil {
					t.Errorf("release unexpectedly acquired linked lock: %v", closeErr)
				}
				t.Fatal("acquire rewind lock succeeded through linked lock file")
			}
			gotContent, readErr := os.ReadFile(externalPath)
			if readErr != nil {
				t.Fatalf("read external file: %v", readErr)
			}
			if !bytes.Equal(gotContent, wantContent) {
				t.Fatalf("external file changed: got %q, want %q", gotContent, wantContent)
			}
		})
	}
}

func TestRewindTransitionLockRejectsConcurrentEnvironmentAliasBeforeMutation(t *testing.T) {
	projectDir := setupRewindCommandProject(t, "concurrent-rewind", status.WaypointV2Retro)
	statusPath := filepath.Join(projectDir, status.StatusFilename)
	historyPath := filepath.Join(projectDir, history.HistoryFilename)
	statusBefore, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status before contention: %v", err)
	}
	historyBefore, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("read history before contention: %v", err)
	}
	archiveRoot := filepath.Join(projectDir, ".wayfinder", "archives")
	archivesBefore := readEntryNames(t, archiveRoot)
	stagedBefore := gittest.Run(t, projectDir, "diff", "--cached", "--name-only")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRewindTransitionLockHelper$")
	parentRoot := t.TempDir()
	parentCache := t.TempDir()
	t.Setenv("HOME", filepath.Join(parentRoot, "missing-home"))
	t.Setenv("USERPROFILE", filepath.Join(parentRoot, "missing-profile"))
	t.Setenv("XDG_CACHE_HOME", parentCache)
	t.Setenv("LOCALAPPDATA", parentCache)
	t.Setenv("TMPDIR", filepath.Join(parentRoot, "tmp"))

	childRoot := t.TempDir()
	childHome := filepath.Join(childRoot, "read-only-home")
	if err := os.Mkdir(childHome, 0o500); err != nil {
		t.Fatalf("create child read-only home: %v", err)
	}
	childCache := t.TempDir()
	helperProjectDir := caseAliasForTest(projectDir)
	cmd.Env = append(environmentWithoutRewindLockLocationOverrides(),
		"GO_WANT_REWIND_LOCK_HELPER="+helperProjectDir,
		"TMPDIR="+childRoot,
		"TMP="+childRoot,
		"TEMP="+childRoot,
		"HOME="+childHome,
		"USERPROFILE="+childHome,
		"XDG_CACHE_HOME="+childCache,
		"LOCALAPPDATA="+childCache,
		"APPDATA="+childCache,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("create helper stdout pipe: %v", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("create helper stdin pipe: %v", err)
	}
	var helperStderr bytes.Buffer
	cmd.Stderr = &helperStderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lock helper: %v", err)
	}
	finished := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !finished {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("lock helper signal = %q, err = %v, stderr = %s", line, err, helperStderr.String())
	}

	previousDir, previousNoPrompt := projectDirectory, rewindNoPrompt
	projectDirectory, rewindNoPrompt = projectDir, true
	t.Cleanup(func() {
		projectDirectory, rewindNoPrompt = previousDir, previousNoPrompt
	})
	err = runRewind(nil, []string{status.WaypointV2Retro})
	if !errors.Is(err, errRewindTransitionInProgress) {
		t.Fatalf("contending runRewind() error = %v, want in-progress lock error", err)
	}

	statusAfter, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status after contention: %v", err)
	}
	historyAfter, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("read history after contention: %v", err)
	}
	if !bytes.Equal(statusAfter, statusBefore) || !bytes.Equal(historyAfter, historyBefore) {
		t.Fatal("contending rewind mutated status or history before acquiring the lock")
	}
	if got := readEntryNames(t, archiveRoot); !slices.Equal(got, archivesBefore) {
		t.Fatalf("contending rewind changed archives: got %v, want %v", got, archivesBefore)
	}
	if _, err := os.Stat(filepath.Join(projectDir, retrospective.RetroFilename)); !os.IsNotExist(err) {
		t.Fatalf("contending rewind created retrospective evidence: %v", err)
	}
	stagedAfter := gittest.Run(t, projectDir, "diff", "--cached", "--name-only")
	if stagedAfter != stagedBefore {
		t.Fatalf("contending rewind changed staged files: got %q, want %q", stagedAfter, stagedBefore)
	}

	if err := stdin.Close(); err != nil {
		t.Fatalf("release lock helper: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait for lock helper: %v; stderr = %s", err, helperStderr.String())
	}
	finished = true

}

func readEntryNames(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read directory %s: %v", directory, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func environmentWithoutRewindLockLocationOverrides() []string {
	environment := os.Environ()
	return slices.DeleteFunc(environment, func(assignment string) bool {
		key, _, _ := strings.Cut(assignment, "=")
		switch key {
		case "TMPDIR", "TMP", "TEMP", "HOME", "USERPROFILE", "XDG_CACHE_HOME", "LOCALAPPDATA", "APPDATA", "GO_WANT_REWIND_LOCK_HELPER":
			return true
		default:
			return false
		}
	})
}

func cleanupRewindLockFile(t *testing.T, projectDir string) {
	t.Helper()
	lockPath, err := rewindLockFilePath(projectDir)
	if err != nil {
		t.Fatalf("resolve test lock path: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(lockPath) })
}

func caseAliasForTest(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	originalInfo, err := os.Stat(resolved)
	if err != nil {
		return path
	}
	for index := range len(resolved) {
		character := resolved[index]
		var replacement byte
		switch {
		case character >= 'a' && character <= 'z':
			replacement = character - ('a' - 'A')
		case character >= 'A' && character <= 'Z':
			replacement = character + ('a' - 'A')
		default:
			continue
		}
		candidate := resolved[:index] + string(replacement) + resolved[index+1:]
		candidateInfo, statErr := os.Stat(candidate)
		if statErr == nil && os.SameFile(originalInfo, candidateInfo) {
			return candidate
		}
	}
	return path
}
