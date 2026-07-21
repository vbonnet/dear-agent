//go:build integration

package tmux_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestAgyLifecycleIntegrationRejectsOnboardingWithoutInput(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}

	fixtureDir := t.TempDir()
	socketDir, err := os.MkdirTemp("", "agm-agy-onboarding-")
	if err != nil {
		t.Fatalf("create short tmux socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "agm.sock")
	inputPath := filepath.Join(fixtureDir, "unexpected-input.txt")
	binDir := filepath.Join(fixtureDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create fixture bin directory: %v", err)
	}
	fixturePath := filepath.Join(binDir, "agy")
	fixture := `#!/bin/sh
printf 'Welcome to Antigravity CLI!\nChoose your color scheme:\n> terminal\n'
if IFS= read -r line; then
  printf '%s\n' "$line" > "$AGY_UNEXPECTED_INPUT"
fi
`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o755); err != nil {
		t.Fatalf("write AGY onboarding fixture: %v", err)
	}

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Setenv("AGY_UNEXPECTED_INPUT", inputPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	const sessionName = "agy-onboarding-fixture"
	if err := tmux.NewSession(sessionName, fixtureDir); err != nil {
		t.Fatalf("create isolated tmux session: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
	})
	fixturePathExport := "export PATH='" + strings.ReplaceAll(binDir, "'", "'\"'\"'") + "':\"$PATH\""
	if err := tmux.SendCommand(sessionName, fixturePathExport); err != nil {
		t.Fatalf("prepend AGY fixture to pane PATH: %v", err)
	}
	if err := tmux.SendCommand(sessionName, "agy"); err != nil {
		t.Fatalf("launch AGY onboarding fixture: %v", err)
	}

	started := time.Now()
	err = tmux.WaitForAgyPrompt(t.Context(), sessionName, 10*time.Second)
	if !errors.Is(err, tmux.ErrAgyOnboardingRequired) {
		output, _ := tmux.CapturePaneOutput(sessionName, 30)
		t.Fatalf("onboarding wait error = %v, want ErrAgyOnboardingRequired\npane output:\n%s", err, output)
	}
	if elapsed := time.Since(started); elapsed >= 3*time.Second {
		t.Fatalf("onboarding detection took %v, want prompt failure before generic readiness timeout", elapsed)
	}
	time.Sleep(250 * time.Millisecond)
	if input, readErr := os.ReadFile(inputPath); readErr == nil {
		t.Fatalf("AGM sent input to onboarding fixture: %q", string(input))
	} else if !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("inspect onboarding input evidence: %v", readErr)
	}
}

func TestAgyLifecycleThroughIsolatedTmuxFixture(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}

	fixtureDir := t.TempDir()
	socketDir, err := os.MkdirTemp("", "agm-agy-tmux-")
	if err != nil {
		t.Fatalf("create short tmux socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "agm.sock")
	argsPath := filepath.Join(fixtureDir, "agy-args.txt")
	binDir := filepath.Join(fixtureDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create fixture bin directory: %v", err)
	}
	fixturePath := filepath.Join(binDir, "agy")
	fixture := `#!/bin/sh
printf '%s\n' "$@" > "$AGY_FIXTURE_ARGS"
printf 'Do you trust the contents of this project?\nYes, I trust this folder\n'
IFS= read -r _trust_answer
printf 'READY\n>\n'
while IFS= read -r line; do
  printf 'fixture-response:%s\n>\n' "$line"
done
`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o755); err != nil {
		t.Fatalf("write AGY fixture: %v", err)
	}

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Setenv("AGY_FIXTURE_ARGS", argsPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	const sessionName = "agy-lifecycle-fixture"
	if err := tmux.NewSession(sessionName, fixtureDir); err != nil {
		t.Fatalf("create isolated tmux session: %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socketPath, "kill-server").Run()
	})
	fixturePathExport := "export PATH='" + strings.ReplaceAll(binDir, "'", "'\"'\"'") + "':\"$PATH\""
	if err := tmux.SendCommand(sessionName, fixturePathExport); err != nil {
		t.Fatalf("prepend AGY fixture to pane PATH: %v", err)
	}

	launch := ops.BuildHarnessLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash-low", SessionName: sessionName,
		WorkDir: fixtureDir, Persistent: true, PermissionMode: "auto",
		ExtraAddDirs: []string{filepath.Join(fixtureDir, "extra dir")},
	}).Command
	if strings.Contains(launch, "--prompt-interactive") {
		t.Fatalf("launch command used string-valued prompt flag: %q", launch)
	}
	if err := tmux.SendCommand(sessionName, launch); err != nil {
		t.Fatalf("send canonical AGY launch command: %v", err)
	}
	if err := tmux.WaitForAgyPrompt(t.Context(), sessionName, 10*time.Second); err != nil {
		output, _ := tmux.CapturePaneOutput(sessionName, 30)
		t.Fatalf("wait through AGY trust prompt: %v\npane output:\n%s\nlaunch: %s", err, output, launch)
	}

	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read AGY fixture arguments: %v", err)
	}
	for _, want := range []string{
		"--model\nGemini 3.5 Flash (Low)\n",
		"--dangerously-skip-permissions\n",
		"--add-dir\n" + filepath.Join(fixtureDir, "extra dir") + "\n",
	} {
		if !strings.Contains(string(args), want) {
			t.Errorf("AGY fixture args %q missing %q", string(args), want)
		}
	}

	if err := tmux.SendCommand(sessionName, "hello from AGM"); err != nil {
		t.Fatalf("send prompt after readiness: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		output, captureErr := tmux.CapturePaneOutput(sessionName, 30)
		if captureErr == nil && strings.Contains(output, "fixture-response:hello from AGM") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	output, _ := tmux.CapturePaneOutput(sessionName, 30)
	t.Fatalf("AGY fixture did not receive post-readiness prompt; pane output:\n%s", output)
}
