//go:build integration

package lifecycle_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestCodexLifecycleUsesIsolatedSourceEnvironment replaces the Codex branch of
// the legacy comprehensive test. It exercises the production CLI with a
// source-built AGM, fake Codex process, isolated HOME/state/SQLite/manifests,
// unique tmux socket, and exact owned cleanup.
func TestCodexLifecycleUsesIsolatedSourceEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-tmux Codex lifecycle in short mode")
	}
	if err := exec.Command("/bin/ps", "-axo", "command=").Run(); err != nil {
		t.Skipf("process-table inspection is unavailable to the fail-closed spawn guard: %v", err)
	}

	env := helpers.NewIsolatedEnvironment(t)
	probe := env.SessionName("tmux-probe")
	if err := env.StartTmuxServer(probe); err != nil {
		t.Skipf("tmux cannot create an isolated server in this environment: %v", err)
	}

	const fakeCodex = `#!/bin/sh
printf 'OpenAI Codex (v0.144.0)\n'
exec sleep 300
`
	if err := env.WriteExecutable("codex", fakeCodex); err != nil {
		t.Fatalf("install fake Codex: %v", err)
	}

	sessionName := env.SessionName("codex-lifecycle")
	if err := env.RegisterSession(sessionName); err != nil {
		t.Fatal(err)
	}
	command := env.Command(
		"session", "new", sessionName,
		"--detached",
		"--harness", "codex-cli",
		"--model", "5.4",
		"--mode", "default",
		"--no-sandbox",
	)
	command.Env = append(command.Env,
		"OPENAI_API_KEY=sk-test-only-not-real",
		"AGM_CODEX_REMOTE_CONTROL=0",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("create isolated Codex session with source AGM: %v\n%s", err, output)
	}
	if !env.HasSession(sessionName) {
		t.Fatalf("source AGM reported success without exact tmux target %q", sessionName)
	}
	pane, err := env.CapturePane(sessionName)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pane, "OpenAI Codex") {
		t.Fatalf("Codex composer marker missing from isolated pane:\n%s", pane)
	}

	manifestDir := filepath.Join(env.SessionsDir, sessionName)
	if info, err := os.Stat(manifestDir); err != nil || !info.IsDir() {
		t.Fatalf("isolated manifest directory missing: info=%v err=%v", info, err)
	}
	store, err := dolt.NewSQLiteAdapter(env.DBPath)
	if err != nil {
		t.Fatalf("open isolated SQLite database: %v", err)
	}
	storeClosed := false
	defer func() {
		if !storeClosed {
			_ = store.Close()
		}
	}()
	sessionManifest, err := store.GetSessionByName(sessionName)
	if err != nil {
		t.Fatalf("read isolated Codex registration: %v", err)
	}
	if sessionManifest == nil {
		t.Fatal("source AGM reported success without an isolated Codex registration")
	}
	if sessionManifest.Harness != "codex-cli" || sessionManifest.Tmux.SessionName != sessionName {
		t.Fatalf("isolated manifest = harness %q tmux %q", sessionManifest.Harness, sessionManifest.Tmux.SessionName)
	}
	if info, err := os.Stat(env.DBPath); err != nil || info.IsDir() {
		t.Fatalf("isolated SQLite database missing: info=%v err=%v", info, err)
	}

	list := env.Command("session", "list", "--all", "--json")
	listOutput, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("list isolated sessions with source AGM: %v\n%s", err, listOutput)
	}
	if !strings.Contains(string(listOutput), sessionName) {
		t.Fatalf("isolated session absent from source AGM list:\n%s", listOutput)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close isolated SQLite database: %v", err)
	}
	storeClosed = true

	if err := env.Cleanup(); err != nil {
		t.Fatalf("cleanup isolated Codex lifecycle: %v", err)
	}
	if env.HasSession(sessionName) {
		t.Fatalf("exact Codex tmux target %q survived cleanup", sessionName)
	}
	if _, err := os.Stat(env.Context.BaseDir); !os.IsNotExist(err) {
		t.Fatalf("isolated root survived cleanup: %v", err)
	}
}
