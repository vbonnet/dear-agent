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

func TestRewindLockPathUsesFilesystemIdentityAcrossCaseAlias(t *testing.T) {
	projectDir := t.TempDir()
	alias := caseAliasForTest(projectDir)
	if alias == projectDir {
		t.Skip("filesystem does not expose a case-insensitive path alias")
	}
	originalLock, err := rewindLockFilePath(projectDir)
	if err != nil {
		t.Fatalf("resolve original lock path: %v", err)
	}
	aliasLock, err := rewindLockFilePath(alias)
	if err != nil {
		t.Fatalf("resolve alias lock path: %v", err)
	}
	if aliasLock != originalLock {
		t.Fatalf("same directory identity produced different locks: %q != %q", aliasLock, originalLock)
	}
	t.Cleanup(func() { _ = os.Remove(originalLock) })
}

func TestRewindTransitionLockRejectsConcurrentProcessBeforeMutation(t *testing.T) {
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
	childTemp := t.TempDir()
	helperProjectDir := caseAliasForTest(projectDir)
	cmd.Env = append(environmentWithoutTempOverrides(),
		"GO_WANT_REWIND_LOCK_HELPER="+helperProjectDir,
		"TMPDIR="+childTemp,
		"TMP="+childTemp,
		"TEMP="+childTemp,
		"HOME="+childTemp,
		"USERPROFILE="+childTemp,
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

func environmentWithoutTempOverrides() []string {
	environment := os.Environ()
	return slices.DeleteFunc(environment, func(assignment string) bool {
		key, _, _ := strings.Cut(assignment, "=")
		return key == "TMPDIR" || key == "TMP" || key == "TEMP" || key == "HOME" || key == "USERPROFILE" || key == "GO_WANT_REWIND_LOCK_HELPER"
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
