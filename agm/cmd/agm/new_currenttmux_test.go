package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestSupportedHarnessesHaveCurrentTmuxLauncher(t *testing.T) {
	t.Parallel()

	for _, harness := range []string{"claude-code", "codex-cli", "opencode-cli", "gemini-cli", "pi-cli"} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()

			calls := 0
			record := func(context.Context, ops.HarnessLaunchSpec) error {
				calls++
				return nil
			}
			runtime := currentTmuxHarnessRuntime{
				startClaude:   record,
				startCodex:    func(ops.HarnessLaunchSpec) (bool, error) { calls++; return false, nil },
				startPi:       func(ops.HarnessLaunchSpec) (bool, error) { calls++; return false, nil },
				startOpenCode: record,
				startGemini:   record,
				validateCodex: func() error { return nil },
			}

			if err := startCurrentTmuxHarnessWithRuntime(t.Context(), ops.HarnessLaunchSpec{Harness: harness}, runtime); err != nil {
				t.Fatalf("current-tmux dispatch for %q failed: %v", harness, err)
			}
			if calls != 1 {
				t.Fatalf("current-tmux dispatch for %q called %d launchers, want 1", harness, calls)
			}
		})
	}
}

func TestQueueCurrentTmuxPiUsesManagedLaunchContract(t *testing.T) {
	t.Parallel()

	var gotSession, gotCommand string
	spec := ops.HarnessLaunchSpec{
		Harness: "pi-cli", SessionName: "pi-current", WorkDir: "/tmp/pi-current",
		PiLaunchID:  "launch-current",
		Pi:          &manifest.Pi{SessionID: "pi-current", SessionDir: "/tmp/agm/pi"},
		PiExtension: "/tmp/agm/pi/authorization.js",
	}
	want := ops.BuildHarnessLaunchCommand(spec)
	modeApplied, err := queueCurrentTmuxPiWithRuntime(spec, currentTmuxPiQueueRuntime{
		lookPath: func(string) (string, error) { return "/usr/local/bin/pi", nil },
		sendCommand: func(sessionName, command string) error {
			gotSession, gotCommand = sessionName, command
			return nil
		},
	})
	if err != nil {
		t.Fatalf("queueCurrentTmuxPiWithRuntime() error = %v", err)
	}
	if modeApplied != want.ModeAppliedAtStartup {
		t.Fatalf("mode applied at startup = %v, want %v", modeApplied, want.ModeAppliedAtStartup)
	}
	if gotSession != spec.SessionName || gotCommand != want.Command {
		t.Fatalf("queued (%q, %q), want (%q, %q)", gotSession, gotCommand, spec.SessionName, want.Command)
	}
}

func TestQueueCurrentTmuxPiRejectsMissingExecutable(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("pi not found")
	sent := false
	_, err := queueCurrentTmuxPiWithRuntime(ops.HarnessLaunchSpec{Harness: "pi-cli"}, currentTmuxPiQueueRuntime{
		lookPath:    func(string) (string, error) { return "", wantErr },
		sendCommand: func(string, string) error { sent = true; return nil },
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want executable lookup error %v", err, wantErr)
	}
	if sent {
		t.Fatal("Pi command was queued after executable preflight failed")
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

	if err := startCurrentTmuxHarnessWithRuntime(t.Context(), spec, runtime); err != nil {
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

func TestQueueCurrentTmuxCodexDoesNotWaitForReadiness(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())

	var gotSession, gotCommand string
	spec := ops.HarnessLaunchSpec{
		Harness: "codex-cli", SessionName: "codex-current", WorkDir: "/tmp/codex-current",
	}
	modeApplied, err := queueCurrentTmuxCodexWithRuntime(spec, currentTmuxCodexQueueRuntime{
		lookPath: func(string) (string, error) { return "/usr/local/bin/codex", nil },
		sendCommand: func(sessionName, command string) error {
			gotSession, gotCommand = sessionName, command
			return nil
		},
	})
	if err != nil {
		t.Fatalf("queueCurrentTmuxCodexWithRuntime() error = %v", err)
	}
	if modeApplied {
		t.Fatal("mode applied at startup = true, want false for default launch spec")
	}
	if gotSession != spec.SessionName {
		t.Fatalf("queued session = %q, want %q", gotSession, spec.SessionName)
	}
	assertPreparedHarnessCommand(t, gotCommand, "__exec-codex")
}

func TestQueueCurrentTmuxCodexPreservesHandoffAfterUncertainSubmission(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	var handoffPath string
	_, err := queueCurrentTmuxCodexWithRuntime(
		ops.HarnessLaunchSpec{
			Harness: "codex-cli", SessionName: "codex-uncertain", WorkDir: "/tmp/codex-uncertain",
		},
		currentTmuxCodexQueueRuntime{
			lookPath: func(string) (string, error) { return "/usr/local/bin/codex", nil },
			sendCommand: func(_, command string) error {
				handoffPath = preparedHandoffPath(t, command)
				return tmux.MarkPromptSubmissionUncertain(errors.New("lost tmux acknowledgement"))
			},
		},
	)
	if err != nil {
		t.Fatalf("uncertain Codex submission returned an error: %v", err)
	}
	if _, err := os.Stat(handoffPath); err != nil {
		t.Fatalf("uncertain Codex submission removed its handoff: %v", err)
	}
}

func TestQueueCurrentTmuxClaudePreservesHandoffAfterUncertainSubmission(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	var handoffPath string
	err := queueCurrentTmuxHarnessCommand(
		t.Context(),
		ops.HarnessLaunchSpec{
			Harness: "claude-code", SessionName: "claude-uncertain", WorkDir: "/tmp/claude-uncertain",
		},
		currentTmuxCommandQueueRuntime{
			lookPath: func(string) (string, error) { return "/usr/local/bin/claude", nil },
			sendCommand: func(_, command string) error {
				handoffPath = preparedHandoffPath(t, command)
				return tmux.MarkPromptSubmissionUncertain(errors.New("lost tmux acknowledgement"))
			},
		},
	)
	if err != nil {
		t.Fatalf("uncertain Claude submission returned an error: %v", err)
	}
	if _, err := os.Stat(handoffPath); err != nil {
		t.Fatalf("uncertain Claude submission removed its handoff: %v", err)
	}
}

func TestCurrentTmuxLaunchResultDefersEveryQueuedHarness(t *testing.T) {
	t.Parallel()

	for _, harness := range []string{"claude-code", "codex-cli", "opencode-cli", "pi-cli", "gemini-cli"} {
		if got := currentTmuxLaunchResult(harness).Readiness; got != ops.CreateSessionReadinessDeferredUntilCallerExit {
			t.Errorf("currentTmuxLaunchResult(%q) readiness = %q, want deferred", harness, got)
		}
	}
	if got := currentTmuxLaunchResult("agy").Readiness; got != "" {
		t.Fatalf("current-tmux AGY readiness = %q, want unsupported/unverified", got)
	}
}

func TestQueueCurrentTmuxHarnessCommandUsesCanonicalCommandWithoutWaiting(t *testing.T) {
	for harness, executable := range map[string]string{"claude-code": "claude", "opencode-cli": "opencode", "gemini-cli": "gemini"} {
		t.Run(harness, func(t *testing.T) {
			if harness == "claude-code" {
				t.Setenv("AGM_STATE_DIR", t.TempDir())
			}
			spec := ops.HarnessLaunchSpec{Harness: harness, SessionName: "current", WorkDir: "/tmp/current"}
			var gotExecutable, gotSession, gotCommand string
			err := queueCurrentTmuxHarnessCommand(t.Context(), spec, currentTmuxCommandQueueRuntime{
				lookPath: func(file string) (string, error) {
					gotExecutable = file
					return "/usr/local/bin/" + file, nil
				},
				sendCommand: func(sessionName, command string) error {
					gotSession, gotCommand = sessionName, command
					return nil
				},
			})
			if err != nil {
				t.Fatalf("queueCurrentTmuxHarnessCommand() error = %v", err)
			}
			if gotExecutable != executable {
				t.Fatalf("executable lookup = %q, want %q", gotExecutable, executable)
			}
			if gotSession != spec.SessionName {
				t.Fatalf("queued session = %q, want %q", gotSession, spec.SessionName)
			}
			if harness == "claude-code" {
				assertPreparedHarnessCommand(t, gotCommand, "__exec-claude")
			} else if gotCommand != ops.BuildHarnessLaunchCommand(spec).Command {
				t.Fatalf("queued command = %q, want canonical command for %#v", gotCommand, spec)
			}
		})
	}
}

func assertPreparedHarnessCommand(t *testing.T, command, protocol string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	for _, want := range []string{executable, protocol, "--handoff"} {
		if !strings.Contains(command, want) {
			t.Fatalf("prepared command %q does not contain %q", command, want)
		}
	}
	handoffPath := preparedHandoffPath(t, command)
	payload, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("read current-tmux handoff: %v", err)
	}
	var handoff struct {
		DeferredUntilProducerExit bool `json:"deferred_until_producer_exit"`
	}
	if err := json.Unmarshal(payload, &handoff); err != nil {
		t.Fatalf("decode current-tmux handoff: %v", err)
	}
	if !handoff.DeferredUntilProducerExit {
		t.Fatal("current-tmux handoff omitted its producer-liveness lease marker")
	}
}

func preparedHandoffPath(t *testing.T, command string) string {
	t.Helper()
	match := regexp.MustCompile(`--handoff '([^']+)'`).FindStringSubmatch(command)
	if len(match) != 2 {
		t.Fatalf("prepared command %q does not contain a quoted handoff path", command)
	}
	return match[1]
}

func TestQueueCurrentTmuxHarnessCommandRejectsMissingExecutable(t *testing.T) {
	t.Parallel()

	for harness, executable := range map[string]string{"claude-code": "claude", "opencode-cli": "opencode", "gemini-cli": "gemini"} {
		t.Run(harness, func(t *testing.T) {
			t.Parallel()
			wantErr := errors.New(executable + " not found")
			var gotExecutable string
			sent := false
			err := queueCurrentTmuxHarnessCommand(t.Context(), ops.HarnessLaunchSpec{Harness: harness}, currentTmuxCommandQueueRuntime{
				lookPath: func(file string) (string, error) {
					gotExecutable = file
					return "", wantErr
				},
				sendCommand: func(string, string) error {
					sent = true
					return nil
				},
			})
			if !errors.Is(err, wantErr) {
				t.Fatalf("error = %v, want executable lookup error %v", err, wantErr)
			}
			if gotExecutable != executable {
				t.Fatalf("executable lookup = %q, want %q", gotExecutable, executable)
			}
			if sent {
				t.Fatal("command was queued after executable preflight failed")
			}
		})
	}
}

func TestQueueCurrentTmuxCodexRejectsMissingExecutable(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("codex not found")
	sent := false
	_, err := queueCurrentTmuxCodexWithRuntime(ops.HarnessLaunchSpec{Harness: "codex-cli"}, currentTmuxCodexQueueRuntime{
		lookPath: func(string) (string, error) { return "", wantErr },
		sendCommand: func(string, string) error {
			sent = true
			return nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want executable lookup error %v", err, wantErr)
	}
	if sent {
		t.Fatal("Codex command was queued after executable preflight failed")
	}
}

func TestStartNewSessionForContextRoutesCurrentTmux(t *testing.T) {
	t.Parallel()

	callerCtx := t.Context()
	var current, separate int
	err := startNewSessionForContext(callerCtx, true, false, "current", "claude-code", newSessionStartRuntime{
		currentTmux: func(ctx context.Context, sessionName string) error {
			current++
			if ctx != callerCtx {
				t.Fatal("current-tmux route did not receive the command context")
			}
			if sessionName != "current" {
				t.Fatalf("session name = %q, want current", sessionName)
			}
			return nil
		},
		separateTmux: func(context.Context, string) error { separate++; return nil },
	})
	if err != nil {
		t.Fatalf("startNewSessionForContext() error = %v", err)
	}
	if current != 1 || separate != 0 {
		t.Fatalf("route calls current=%d separate=%d, want 1/0", current, separate)
	}
}

func TestStartNewSessionForContextRejectsCurrentTmuxAgyBeforeLaunch(t *testing.T) {
	t.Parallel()

	var current, separate int
	err := startNewSessionForContext(t.Context(), true, false, "current", "agy", newSessionStartRuntime{
		currentTmux:  func(context.Context, string) error { current++; return nil },
		separateTmux: func(context.Context, string) error { separate++; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "use --detached") {
		t.Fatalf("startNewSessionForContext() error = %v, want detached AGY guidance", err)
	}
	if current != 0 || separate != 0 {
		t.Fatalf("rejected AGY route launched current=%d separate=%d", current, separate)
	}
	if dispatcherErr := startCurrentTmuxHarnessWithRuntime(t.Context(), ops.HarnessLaunchSpec{Harness: "agy"}, currentTmuxHarnessRuntime{}); dispatcherErr == nil || !strings.Contains(dispatcherErr.Error(), "use --detached") {
		t.Fatalf("direct current-tmux AGY dispatch error = %v, want detached guidance", dispatcherErr)
	}

	err = startNewSessionForContext(t.Context(), true, true, "detached", "antigravity", newSessionStartRuntime{
		currentTmux: func(context.Context, string) error { current++; return nil },
		separateTmux: func(_ context.Context, sessionName string) error {
			separate++
			if sessionName != "detached" {
				t.Fatalf("detached session name = %q, want detached", sessionName)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("detached AGY route failed: %v", err)
	}
	if current != 0 || separate != 1 {
		t.Fatalf("detached AGY route launched current=%d separate=%d, want 0/1", current, separate)
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

	err := startCurrentTmuxHarnessWithRuntime(t.Context(), ops.HarnessLaunchSpec{Harness: "codex-cli"}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if launched {
		t.Fatal("Codex launcher ran after credential validation failed")
	}
}

func TestStartCurrentTmuxHarnessCodexPropagatesQueueFailure(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("Codex command could not be queued")
	runtime := currentTmuxHarnessRuntime{
		validateCodex: func() error { return nil },
		startCodex: func(ops.HarnessLaunchSpec) (bool, error) {
			return false, wantErr
		},
	}

	err := startCurrentTmuxHarnessWithRuntime(t.Context(), ops.HarnessLaunchSpec{Harness: "codex-cli"}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want queue failure %v", err, wantErr)
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
