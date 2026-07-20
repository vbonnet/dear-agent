//go:build integration

package isolated_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/test/integration/helpers"
)

// TestCodexLifecycleUsesIsolatedSourceEnvironment replaces the Codex branch of
// the legacy comprehensive test. It exercises production create, list, send,
// kill, resume, and archive commands with a source-built AGM, fake Codex
// process, isolated HOME/state/SQLite/manifests, unique tmux socket, and exact
// owned cleanup.
func TestCodexLifecycleUsesIsolatedSourceEnvironment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-tmux Codex lifecycle in short mode")
	}
	if err := probeProcessTable(); err != nil {
		if helpers.IsUnavailablePrerequisite(err) {
			t.Skipf("process-table inspection is unavailable to the fail-closed spawn guard: %v", err)
		}
		t.Fatalf("probe process-table inspection: %v", err)
	}

	env := helpers.NewIsolatedEnvironment(t)
	probe := env.SessionName("tmux-probe")
	if err := env.StartTmuxServer(probe); err != nil {
		if env.TmuxUnavailable() && helpers.IsUnavailablePrerequisite(err) {
			t.Skipf("tmux cannot create an isolated server in this environment: %v", err)
		}
		t.Fatalf("start isolated tmux server: %v", err)
	}

	const fakeCodex = `package main

import (
	"bufio"
	"fmt"
	"os"
)

func composer() {
	fmt.Println("│ >_ OpenAI Codex (v0.144.0) │")
	fmt.Println("│ model: gpt-5.4 /model to change │")
	fmt.Println("›")
	fmt.Println("  gpt-5.4 high · /isolated-workdir")
}

func main() {
	composer()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Println("accepted isolated input")
		composer()
	}
}
`
	if err := env.BuildGoExecutable("codex", fakeCodex); err != nil {
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

	send := env.Command("send", "msg", sessionName, "--sender", "integration-test", "--prompt", "isolated lifecycle message")
	sendOutput, err := send.CombinedOutput()
	if err != nil {
		t.Fatalf("send to isolated Codex session with source AGM: %v\n%s", err, sendOutput)
	}
	requirePaneContains(t, env, sessionName, "accepted isolated input")

	kill := env.Command(
		"session", "kill", sessionName,
		"--confirmed-stuck", "--force",
		"--reason", "isolated lifecycle test",
		"--no-agent",
	)
	killOutput, err := kill.CombinedOutput()
	if err != nil {
		t.Fatalf("kill isolated Codex session with source AGM: %v\n%s", err, killOutput)
	}
	if env.HasSession(sessionName) {
		t.Fatalf("source AGM reported kill success while exact tmux target %q survived", sessionName)
	}

	resume := env.Command("session", "resume", sessionName, "--detached")
	resume.Env = append(resume.Env,
		"OPENAI_API_KEY=sk-test-only-not-real",
		"AGM_CODEX_REMOTE_CONTROL=0",
	)
	resumeOutput, err := resume.CombinedOutput()
	if err != nil {
		t.Fatalf("resume isolated Codex session with source AGM: %v\n%s", err, resumeOutput)
	}
	if !env.HasSession(sessionName) {
		t.Fatalf("source AGM reported resume success without exact tmux target %q", sessionName)
	}
	resumedPane, err := env.CapturePane(sessionName)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resumedPane, "│ >_ OpenAI Codex") || !strings.Contains(resumedPane, "/model to change") {
		t.Fatalf("resumed Codex composer missing from isolated pane:\n%s", resumedPane)
	}

	kill = env.Command(
		"session", "kill", sessionName,
		"--confirmed-stuck", "--force",
		"--reason", "isolated lifecycle test",
		"--no-agent",
	)
	if killOutput, err = kill.CombinedOutput(); err != nil {
		t.Fatalf("kill resumed isolated Codex session with source AGM: %v\n%s", err, killOutput)
	}

	archive := env.Command("session", "archive", sessionName, "--outcome", "killed")
	archive.Env = append(archive.Env, "AGM_CODEX_REMOTE_CONTROL=0")
	archiveOutput, err := archive.CombinedOutput()
	if err != nil {
		t.Fatalf("archive isolated Codex session with source AGM: %v\n%s", err, archiveOutput)
	}
	archivedStore, err := dolt.NewSQLiteAdapter(env.DBPath)
	if err != nil {
		t.Fatalf("reopen isolated SQLite database: %v", err)
	}
	archived, err := archivedStore.GetSession(sessionManifest.SessionID)
	if err != nil {
		_ = archivedStore.Close()
		t.Fatalf("read archived isolated Codex registration: %v", err)
	}
	if archived == nil || archived.Lifecycle != "archived" || archived.Outcome != "killed" {
		_ = archivedStore.Close()
		t.Fatalf("archived manifest = %+v, want lifecycle archived and outcome killed", archived)
	}
	if err := archivedStore.Close(); err != nil {
		t.Fatalf("close archived isolated SQLite database: %v", err)
	}

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

func TestProcessTableProbePreservesDeniedStderr(t *testing.T) {
	fakeBin := t.TempDir()
	fakePS := filepath.Join(fakeBin, "ps")
	// #nosec G306 -- the test fixture must be executable to model a denied ps command.
	if err := os.WriteFile(fakePS, []byte("#!/bin/sh\necho 'permission denied' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)

	err := probeProcessTable()
	if err == nil {
		t.Fatal("denied process-table probe succeeded")
	}
	if !helpers.IsUnavailablePrerequisite(err) {
		t.Fatalf("denied process-table stderr was not classified unavailable: %v", err)
	}
}

func probeProcessTable() error {
	output, err := exec.Command("ps", "-axo", "command=").CombinedOutput()
	if err != nil {
		return fmt.Errorf("run process-table probe: %w: %s", err, output)
	}
	return nil
}

func requirePaneContains(t *testing.T, env *helpers.IsolatedEnvironment, sessionName, marker string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastPane string
	var lastErr error
	for time.Now().Before(deadline) {
		lastPane, lastErr = env.CapturePane(sessionName)
		if lastErr == nil && strings.Contains(lastPane, marker) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("isolated pane never contained %q: err=%v\n%s", marker, lastErr, lastPane)
}
