//go:build integration

package tmux_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"golang.org/x/term"
)

func TestAgyMultilinePasteIntegrationPreservesOneBracketedSubmission(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve integration test executable: %v", err)
	}
	fixtureDir := t.TempDir()
	socketDir, err := os.MkdirTemp("", "agm-agy-paste-")
	if err != nil {
		t.Fatalf("create short tmux socket directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(socketDir); err != nil {
			t.Logf("remove tmux socket directory: %v", err)
		}
	})
	socketPath := filepath.Join(socketDir, "agm.sock")
	outputPath := filepath.Join(fixtureDir, "input.bin")
	fixtureExecutable := filepath.Join(fixtureDir, "agy")
	if err := copyAgyFixtureExecutable(executable, fixtureExecutable); err != nil {
		t.Fatalf("copy AGY bracketed-paste fixture executable: %v", err)
	}
	t.Setenv("AGM_TMUX_SOCKET", socketPath)

	const sessionName = "agy-multiline-paste-fixture"
	if err := tmux.NewSession(sessionName, fixtureDir); err != nil {
		t.Fatalf("create isolated tmux session: %v", err)
	}
	t.Cleanup(func() { cleanupAgyFixtureTmuxServer(t, socketPath) })
	for _, export := range []string{
		"export AGY_BRACKETED_PASTE_HELPER=1",
		"export AGY_BRACKETED_PASTE_OUTPUT=input.bin",
	} {
		if err := tmux.SendCommand(sessionName, export); err != nil {
			t.Fatalf("set fixture environment with %q: %v", export, err)
		}
	}
	// Keep the shell composer input short. A long inline environment prefix can
	// wrap across the zsh prompt and make the setup itself exercise Enter retry
	// heuristics instead of the AGY bracketed-paste behavior this test owns.
	command := "./agy -test.run '^TestAgyBracketedPasteHelper$'"
	if err := tmux.SendCommand(sessionName, command); err != nil {
		t.Fatalf("launch bracketed-paste fixture: %v", err)
	}
	if err := tmux.WaitForExpectedHarnessReady(t.Context(), sessionName, "agy", 5*time.Second); err != nil {
		output, _ := tmux.CapturePaneOutput(sessionName, 30)
		t.Fatalf("wait for bracketed-paste fixture: %v\npane output:\n%s", err, output)
	}

	prompt := "[From: codex | ID: regression]\n\nReply exactly: AGM_AGY_MULTILINE_OK"
	readiness, err := tmux.CheckExpectedHarnessInputAndSend(t.Context(), sessionName, "agy", prompt, tmux.InputDeliveryOptions{})
	if err != nil {
		output, _ := tmux.CapturePaneOutput(sessionName, 30)
		t.Fatalf("send AGY multiline prompt: %v\npane output:\n%s", err, output)
	}
	if !readiness.Ready || readiness.TargetPane == "" {
		t.Fatalf("AGY atomic delivery readiness = %+v, want ready exact pane", readiness)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, readErr := os.ReadFile(outputPath)
		if readErr == nil {
			want := "\x1b[200~" + prompt + "\x1b[201~\r"
			if string(got) != want {
				t.Fatalf("AGY received %q, want one bracketed submission %q", got, want)
			}
			return
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			t.Fatalf("read fixture input: %v", readErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for AGY bracketed-paste fixture input")
}

func copyAgyFixtureExecutable(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func TestAgyBracketedPasteHelper(t *testing.T) {
	if os.Getenv("AGY_BRACKETED_PASTE_HELPER") != "1" {
		return
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		t.Fatalf("make fixture terminal raw: %v", err)
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()
	if _, err := fmt.Fprint(os.Stdout, "\x1b[?2004h>\r\n"); err != nil {
		t.Fatalf("enable bracketed paste: %v", err)
	}
	var input []byte
	one := make([]byte, 1)
	for {
		if _, err := os.Stdin.Read(one); err != nil {
			t.Fatalf("read fixture input: %v", err)
		}
		input = append(input, one[0])
		if one[0] == '\r' {
			break
		}
	}
	if err := os.WriteFile(os.Getenv("AGY_BRACKETED_PASTE_OUTPUT"), input, 0o600); err != nil {
		t.Fatalf("persist fixture input: %v", err)
	}
}

func TestAgyLifecycleIntegrationRejectsOnboardingWithoutInput(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}

	fixtureDir := t.TempDir()
	socketDir, err := os.MkdirTemp("", "agm-agy-onboarding-")
	if err != nil {
		t.Fatalf("create short tmux socket directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(socketDir); err != nil {
			t.Logf("remove tmux socket directory: %v", err)
		}
	})
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
	t.Cleanup(func() { cleanupAgyFixtureTmuxServer(t, socketPath) })
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
		output, captureErr := tmux.CapturePaneOutput(sessionName, 30)
		if captureErr != nil {
			t.Fatalf("onboarding wait error = %v, want ErrAgyOnboardingRequired (capture failed: %v)", err, captureErr)
		}
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

func TestAgyResumeLifecycleDistinguishesTranscriptFromPersistentOnboarding(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}

	fixtureDir := t.TempDir()
	socketDir, err := os.MkdirTemp("", "agm-agy-resume-onboarding-")
	if err != nil {
		t.Fatalf("create short tmux socket directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(socketDir); err != nil {
			t.Logf("remove tmux socket directory: %v", err)
		}
	})
	socketPath := filepath.Join(socketDir, "agm.sock")
	inputPath := filepath.Join(fixtureDir, "unexpected-resume-input.txt")
	binDir := filepath.Join(fixtureDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create fixture bin directory: %v", err)
	}
	fixturePath := filepath.Join(binDir, "agy")
	fixture := `#!/bin/sh
if [ "$1" = "transient" ]; then
  printf 'previous composer\n>\n> you: quote this screen\nWelcome to Antigravity CLI!\nChoose your color scheme:\n> terminal\n'
  sleep 1
  printf 'resume complete\n>\n'
  sleep 30
  exit
fi
printf 'Welcome to Antigravity CLI!\nChoose your color scheme:\n> terminal\n'
if IFS= read -r line; then
  printf '%s\n' "$line" > "$AGY_UNEXPECTED_INPUT"
fi
`
	if err := os.WriteFile(fixturePath, []byte(fixture), 0o755); err != nil {
		t.Fatalf("write AGY resume fixture: %v", err)
	}

	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	t.Setenv("AGY_UNEXPECTED_INPUT", inputPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() { cleanupAgyFixtureTmuxServer(t, socketPath) })
	fixturePathExport := "export PATH='" + strings.ReplaceAll(binDir, "'", "'\"'\"'") + "':\"$PATH\""

	const transcriptSession = "agy-resume-transcript-fixture"
	if err := tmux.NewSession(transcriptSession, fixtureDir); err != nil {
		t.Fatalf("create transient transcript session: %v", err)
	}
	if err := tmux.SendCommand(transcriptSession, fixturePathExport); err != nil {
		t.Fatalf("prepend AGY fixture to transient pane PATH: %v", err)
	}
	if err := tmux.SendCommand(transcriptSession, "agy transient"); err != nil {
		t.Fatalf("launch transient AGY resume fixture: %v", err)
	}
	if err := tmux.WaitForAgyPromptOnResume(t.Context(), transcriptSession, 15*time.Second); err != nil {
		output, captureErr := tmux.CapturePaneOutput(transcriptSession, 30)
		if captureErr != nil {
			t.Fatalf("resume wait rejected transient transcript: %v (capture failed: %v)", err, captureErr)
		}
		t.Fatalf("resume wait rejected transient transcript: %v\npane output:\n%s", err, output)
	}

	const onboardingSession = "agy-resume-onboarding-fixture"
	if err := tmux.NewSession(onboardingSession, fixtureDir); err != nil {
		t.Fatalf("create persistent onboarding session: %v", err)
	}
	if err := tmux.SendCommand(onboardingSession, fixturePathExport); err != nil {
		t.Fatalf("prepend AGY fixture to onboarding pane PATH: %v", err)
	}
	if err := tmux.SendCommand(onboardingSession, "agy persistent"); err != nil {
		t.Fatalf("launch persistent AGY resume fixture: %v", err)
	}
	err = tmux.WaitForAgyPromptOnResume(t.Context(), onboardingSession, 15*time.Second)
	if !errors.Is(err, tmux.ErrAgyOnboardingRequired) {
		output, captureErr := tmux.CapturePaneOutput(onboardingSession, 30)
		if captureErr != nil {
			t.Fatalf("persistent onboarding wait error = %v, want ErrAgyOnboardingRequired (capture failed: %v)", err, captureErr)
		}
		t.Fatalf("persistent onboarding wait error = %v, want ErrAgyOnboardingRequired\npane output:\n%s", err, output)
	}
	time.Sleep(250 * time.Millisecond)
	if input, readErr := os.ReadFile(inputPath); readErr == nil {
		t.Fatalf("AGM sent input to persistent onboarding fixture: %q", string(input))
	} else if !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("inspect persistent onboarding input evidence: %v", readErr)
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
	t.Cleanup(func() {
		if err := os.RemoveAll(socketDir); err != nil {
			t.Logf("remove tmux socket directory: %v", err)
		}
	})
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
	t.Cleanup(func() { cleanupAgyFixtureTmuxServer(t, socketPath) })
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
		output, captureErr := tmux.CapturePaneOutput(sessionName, 30)
		if captureErr != nil {
			t.Fatalf("wait through AGY trust prompt: %v (capture failed: %v)\nlaunch: %s", err, captureErr, launch)
		}
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
	output, captureErr := tmux.CapturePaneOutput(sessionName, 30)
	if captureErr != nil {
		t.Fatalf("AGY fixture did not receive post-readiness prompt and pane capture failed: %v", captureErr)
	}
	t.Fatalf("AGY fixture did not receive post-readiness prompt; pane output:\n%s", output)
}

func cleanupAgyFixtureTmuxServer(t *testing.T, socketPath string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "kill-server")
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		t.Logf("kill isolated tmux server: %v", err)
	}
}
