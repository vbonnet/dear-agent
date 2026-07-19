package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestActiveHarnessesHaveCurrentTmuxLauncher(t *testing.T) {
	t.Parallel()

	for _, harness := range agent.ActiveHarnesses() {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()

			calls := 0
			record := func(ops.HarnessLaunchSpec) error {
				calls++
				return nil
			}
			runtime := currentTmuxHarnessRuntime{
				startClaude:   record,
				startCodex:    func(ops.HarnessLaunchSpec) (bool, error) { calls++; return false, nil },
				startOpenCode: record,
				startGemini:   record,
				startAgy:      record,
				validateCodex: func() error { return nil },
			}

			if err := startCurrentTmuxHarnessWithRuntime(ops.HarnessLaunchSpec{Harness: harness}, runtime); err != nil {
				t.Fatalf("current-tmux dispatch for %q failed: %v", harness, err)
			}
			if calls != 1 {
				t.Fatalf("current-tmux dispatch for %q called %d launchers, want 1", harness, calls)
			}
		})
	}
}

func TestStartCurrentTmuxHarnessCodexUsesRealLauncherContract(t *testing.T) {
	t.Parallel()

	var validated bool
	var launched *ops.HarnessLaunchSpec
	runtime := currentTmuxHarnessRuntime{
		validateCodex: func() error {
			validated = true
			return nil
		},
		startCodex: func(spec ops.HarnessLaunchSpec) (bool, error) {
			launched = &spec
			return true, nil
		},
	}
	spec := ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.5", SessionName: "codex-current", WorkDir: "/tmp/codex-current",
	}

	if err := startCurrentTmuxHarnessWithRuntime(spec, runtime); err != nil {
		t.Fatalf("startCurrentTmuxHarnessWithRuntime() error = %v", err)
	}
	if !validated {
		t.Fatal("Codex credentials were not validated")
	}
	if launched == nil {
		t.Fatal("Codex launcher was not called; current-tmux creation would report false success")
	}
	if !reflect.DeepEqual(*launched, spec) {
		t.Fatalf("Codex launch spec = %#v, want %#v", *launched, spec)
	}
}

func TestStartCurrentTmuxHarnessCodexStopsAfterCredentialFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("missing Codex authentication")
	launched := false
	runtime := currentTmuxHarnessRuntime{
		validateCodex: func() error { return wantErr },
		startCodex: func(ops.HarnessLaunchSpec) (bool, error) {
			launched = true
			return false, nil
		},
	}

	err := startCurrentTmuxHarnessWithRuntime(ops.HarnessLaunchSpec{Harness: "codex-cli"}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if launched {
		t.Fatal("Codex launcher ran after credential validation failed")
	}
}

func TestStartCurrentTmuxHarnessCodexPropagatesReadinessFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("Codex composer not ready")
	runtime := currentTmuxHarnessRuntime{
		validateCodex: func() error { return nil },
		startCodex: func(ops.HarnessLaunchSpec) (bool, error) {
			return false, wantErr
		},
	}

	err := startCurrentTmuxHarnessWithRuntime(ops.HarnessLaunchSpec{Harness: "codex-cli"}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want readiness failure %v", err, wantErr)
	}
}

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
