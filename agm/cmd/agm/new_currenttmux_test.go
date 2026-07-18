package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

func TestCommitCurrentTmuxManifestLogsFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := debug.Init(true, "current-tmux-test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		debug.Close()
		if err := debug.Init(false, ""); err != nil {
			t.Errorf("disable debug logger: %v", err)
		}
	})

	repo := filepath.Join(t.TempDir(), "repo")
	if output, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	manifestPath := filepath.Join(repo, "sessions", "current", "manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("name: current\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidIndex := filepath.Join(t.TempDir(), "index-directory")
	if err := os.Mkdir(invalidIndex, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_INDEX_FILE", invalidIndex)

	commitCurrentTmuxManifest(manifestPath, "current")
	debug.Close()
	if err := debug.Init(false, ""); err != nil {
		t.Fatal(err)
	}

	logs, err := filepath.Glob(filepath.Join(home, ".agm", "debug", "new-current-tmux-test-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("debug logs = %v, want one", logs)
	}
	contents, err := os.ReadFile(logs[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "manifest commit skipped") {
		t.Fatalf("debug log = %q, want manifest commit failure", contents)
	}
}
