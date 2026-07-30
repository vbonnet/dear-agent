package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/pkg/override"
)

func testLaunchCommand(spec ops.HarnessLaunchSpec) string {
	spec.DisableOAuth = true
	return ops.BuildHarnessLaunchCommand(spec).Command
}

func TestSubmitHarnessLaunchAuthorizesAtSubmissionBoundary(t *testing.T) {
	var events []string
	hookReservation := &override.Reservation{}
	spec := ops.HarnessLaunchSpec{BeforeSpawn: func(got ...*override.Reservation) ([]*override.Reservation, error) {
		if len(got) != 1 || got[0] != hookReservation {
			t.Fatalf("launch reservations = %v, want exact Codex hook reservation", got)
		}
		events = append(events, "authorize")
		return got, nil
	}}
	launch := ops.HarnessLaunchCommand{
		Command:      "fixture",
		Reservations: []*override.Reservation{hookReservation},
		BindOverrideReservations: func(got ...*override.Reservation) error {
			if len(got) != 1 || got[0] != hookReservation {
				t.Fatalf("bound reservations = %v, want exact final reservation", got)
			}
			events = append(events, "bind")
			return nil
		},
		Cancel: func() error {
			events = append(events, "cancel")
			return nil
		},
	}
	err := submitHarnessLaunch("fixture", spec, launch, func() error {
		events = append(events, "submit")
		return nil
	})
	if err != nil {
		t.Fatalf("submitHarnessLaunch() error = %v", err)
	}
	if got, want := strings.Join(events, ","), "authorize,bind,submit"; got != want {
		t.Fatalf("launch boundary events = %q, want %q", got, want)
	}
}

func TestSubmitHarnessLaunchAdmissionFailureCancelsWithoutSubmission(t *testing.T) {
	refusal := errors.New("admission refused")
	var events []string
	spec := ops.HarnessLaunchSpec{BeforeSpawn: func(...*override.Reservation) ([]*override.Reservation, error) {
		events = append(events, "authorize")
		return nil, refusal
	}}
	launch := ops.HarnessLaunchCommand{
		Command: "fixture",
		Cancel: func() error {
			events = append(events, "cancel")
			return nil
		},
	}
	err := submitHarnessLaunch("fixture", spec, launch, func() error {
		events = append(events, "submit")
		return nil
	})
	if !errors.Is(err, refusal) {
		t.Fatalf("submitHarnessLaunch() error = %v, want %v", err, refusal)
	}
	if got, want := strings.Join(events, ","), "authorize,cancel"; got != want {
		t.Fatalf("refused launch boundary events = %q, want %q", got, want)
	}
}

func TestSubmitHarnessLaunchBindingFailureCancelsWithoutSubmission(t *testing.T) {
	refusal := errors.New("private handoff binding failed")
	var events []string
	launch := ops.HarnessLaunchCommand{
		Command: "fixture",
		BindOverrideReservations: func(...*override.Reservation) error {
			events = append(events, "bind")
			return refusal
		},
		Cancel: func() error {
			events = append(events, "cancel")
			return nil
		},
	}
	err := submitHarnessLaunch("fixture", ops.HarnessLaunchSpec{}, launch, func() error {
		events = append(events, "submit")
		return nil
	})
	if !errors.Is(err, refusal) {
		t.Fatalf("submitHarnessLaunch() error = %v, want %v", err, refusal)
	}
	if got, want := strings.Join(events, ","), "bind,cancel"; got != want {
		t.Fatalf("failed binding events = %q, want %q", got, want)
	}
}

func TestStartAgyHarnessUsesCanonicalLaunchAndWaits(t *testing.T) {
	callerCtx := t.Context()
	var sentCommand string
	var waitedSession string
	var waitedTimeout time.Duration
	runtime := agyHarnessRuntime{
		lookPath: func(file string) (string, error) {
			if file != "agy" {
				t.Fatalf("lookPath(%q), want agy", file)
			}
			return "/fixture/agy", nil
		},
		sendCommand: func(sessionName, command string) error {
			if sessionName != "agy-production-seam" {
				t.Fatalf("send session = %q", sessionName)
			}
			sentCommand = command
			return nil
		},
		waitForPrompt: func(ctx context.Context, sessionName string, timeout time.Duration) error {
			if ctx != callerCtx {
				t.Fatal("readiness wait did not receive the caller context")
			}
			waitedSession, waitedTimeout = sessionName, timeout
			return nil
		},
		sleep: func(time.Duration) {},
	}

	modeApplied, err := startAgyHarnessWithRuntime(callerCtx, ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash-low", SessionName: "agy-production-seam",
		WorkDir: "/tmp/agy work", ExtraAddDirs: []string{"/tmp/extra dir"}, PermissionMode: "auto",
	}, runtime)
	if err != nil {
		t.Fatalf("startAgyHarnessWithRuntime: %v", err)
	}
	if !modeApplied {
		t.Fatal("auto permission mode was not reported as applied")
	}
	for _, want := range []string{
		"cd '/tmp/agy work' && agy --model 'Gemini 3.5 Flash (Low)'",
		"--dangerously-skip-permissions",
		"--add-dir '/tmp/extra dir'",
		"&& exit",
	} {
		if !strings.Contains(sentCommand, want) {
			t.Errorf("launch command %q missing %q", sentCommand, want)
		}
	}
	if strings.Contains(sentCommand, "--prompt-interactive") {
		t.Errorf("launch used string-valued prompt flag without a prompt: %q", sentCommand)
	}
	if waitedSession != "agy-production-seam" || waitedTimeout != 90*time.Second {
		t.Fatalf("readiness wait = (%q, %s), want (agy-production-seam, 90s)", waitedSession, waitedTimeout)
	}
}

func TestStartAgyHarnessPropagatesReadinessFailure(t *testing.T) {
	wantErr := errors.New("fixture AGY never became ready")
	runtime := agyHarnessRuntime{
		lookPath:      func(string) (string, error) { return "/fixture/agy", nil },
		sendCommand:   func(string, string) error { return nil },
		waitForPrompt: func(context.Context, string, time.Duration) error { return wantErr },
		sleep:         func(time.Duration) {},
	}
	_, err := startAgyHarnessWithRuntime(t.Context(), ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash", SessionName: "agy-not-ready", WorkDir: "/tmp",
	}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("readiness error = %v, want %v", err, wantErr)
	}
}

func TestStartAgyHarnessDoesNotAuthorizeBeforeExecutableResolution(t *testing.T) {
	wantErr := errors.New("agy is unavailable")
	authorized := false
	submitted := false
	runtime := agyHarnessRuntime{
		lookPath:    func(string) (string, error) { return "", wantErr },
		sendCommand: func(string, string) error { submitted = true; return nil },
	}
	_, err := startAgyHarnessWithRuntime(t.Context(), ops.HarnessLaunchSpec{
		Harness: "agy", SessionName: "agy-missing", WorkDir: "/tmp",
		BeforeSpawn: func(...*override.Reservation) ([]*override.Reservation, error) {
			authorized = true
			return nil, nil
		},
	}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want executable lookup error %v", err, wantErr)
	}
	if authorized {
		t.Fatal("launch admission was consumed before executable resolution")
	}
	if submitted {
		t.Fatal("launch was submitted after executable resolution failed")
	}
}

func TestStartPiHarnessUsesManagedExtensionAndFatalReadiness(t *testing.T) {
	callerCtx := t.Context()
	var sent, waitedLaunchID string
	runtime := piHarnessRuntime{
		lookPath: func(file string) (string, error) {
			if file != "pi" {
				t.Fatalf("lookPath(%q), want pi", file)
			}
			return "/fixture/pi", nil
		},
		sendCommand: func(session, command string) error {
			if session != "pi-worker" {
				t.Fatalf("session = %q", session)
			}
			sent = command
			return nil
		},
		waitForPrompt: func(ctx context.Context, session, launchID string, timeout time.Duration) error {
			if ctx != callerCtx || session != "pi-worker" || timeout != 90*time.Second {
				t.Fatalf("readiness = ctx %v session %q timeout %s", ctx == callerCtx, session, timeout)
			}
			waitedLaunchID = launchID
			return nil
		},
		sleep: func(time.Duration) {},
	}
	modeApplied, err := startPiHarnessWithRuntime(callerCtx, ops.HarnessLaunchSpec{
		Harness: "pi-cli", Model: "sonnet", SessionName: "pi-worker", WorkDir: "/tmp/work",
		PermissionMode: "plan", Pi: &manifest.Pi{SessionID: "native-id", SessionDir: "/tmp/pi"},
		PiExtension: "/tmp/agm-auth.js", PiPolicyFile: "/tmp/pi-policy.json",
	}, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if !modeApplied || !strings.Contains(sent, "--extension '/tmp/agm-auth.js'") || !strings.Contains(sent, "--tools 'read,grep,find,ls'") {
		t.Fatalf("Pi launch = mode %v command %q", modeApplied, sent)
	}
	if waitedLaunchID == "" || !strings.Contains(sent, "AGM_PI_LAUNCH_ID='"+waitedLaunchID+"'") {
		t.Fatalf("Pi launch/readiness correlation = command %q launch %q", sent, waitedLaunchID)
	}
}

func TestStartPiHarnessPropagatesManagedReadinessFailure(t *testing.T) {
	wantErr := errors.New("authorization extension failed")
	runtime := piHarnessRuntime{
		lookPath:      func(string) (string, error) { return "/fixture/pi", nil },
		sendCommand:   func(string, string) error { return nil },
		waitForPrompt: func(context.Context, string, string, time.Duration) error { return wantErr },
		sleep:         func(time.Duration) {},
	}
	_, err := startPiHarnessWithRuntime(t.Context(), ops.HarnessLaunchSpec{
		Harness: "pi-cli", SessionName: "pi-fail", WorkDir: "/tmp", Pi: &manifest.Pi{SessionID: "id", SessionDir: "/tmp/pi"},
	}, runtime)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

// TestBuildCodexCommand_ModelResolved verifies that a registry alias is resolved
// to its full Codex model name and passed via the `-m` flag.
func TestBuildCodexCommand_ModelResolved(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "test-session", WorkDir: "/tmp/work",
	})

	if !strings.Contains(cmd, "--model 'gpt-5.4'") {
		t.Errorf("resolved model not present in command: %s", cmd)
	}
	// Cross-harness alias should resolve too (opus -> 5.5 -> gpt-5.5).
	cmd = testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "opus", SessionName: "test-session", WorkDir: "/tmp/work",
	})
	if !strings.Contains(cmd, "--model 'gpt-5.5'") {
		t.Errorf("cross-harness alias not resolved in command: %s", cmd)
	}
}

// TestBuildCodexCommand_ShellQuoting verifies that a working directory containing
// shell-special characters is single-quoted so it cannot break out of the command
// line or trigger word-splitting/globbing when pasted into the pane's shell.
func TestBuildCodexCommand_ShellQuoting(t *testing.T) {
	// A path with a space and a single quote (injection attempt).
	workDir := "/tmp/work dir/'; rm -rf ~ #"
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "test-session", WorkDir: workDir,
		ExtraAddDirs: []string{"/extra/add dir"},
	})

	// The -C value must be wrapped in single quotes with the embedded quote escaped.
	if !strings.Contains(cmd, "--workdir '/tmp/work dir/'\"'\"'; rm -rf ~ #'") {
		t.Errorf("workdir not safely shell-quoted: %s", cmd)
	}
	// The raw unquoted injection must NOT appear as a bare token.
	if strings.Contains(cmd, "--workdir /tmp/work dir") {
		t.Errorf("unquoted workdir would allow shell word-splitting/injection: %s", cmd)
	}
	// Extra add-dir must be quoted as well.
	if !strings.Contains(cmd, "--add-dir '/extra/add dir'") {
		t.Errorf("extra add-dir not shell-quoted: %s", cmd)
	}
	// Session name with special chars must be quoted in the env assignment.
	cmd = testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "my session", WorkDir: workDir,
	})
	if !strings.Contains(cmd, "--session 'my session'") {
		t.Errorf("session name not shell-quoted: %s", cmd)
	}
}

// TestBuildCodexCommand_NoClaudeEnvLeak is the security regression for the design
// invariant: the Codex command must never carry credentials or telemetry env.
// Authentication is injected only by the private executor.
func TestBuildCodexCommand_NoClaudeEnvLeak(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "test-session",
		SessionID: "engram-uuid-1234", WorkDir: "/tmp/work", ForwardTelemetry: true,
	})

	for _, banned := range []string{
		"CLAUDE_CODE_OAUTH_TOKEN",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_",
		"ENGRAM_SESSION_ID",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
	} {
		if strings.Contains(cmd, banned) {
			t.Errorf("Codex command leaked Claude/engram env %q: %s", banned, cmd)
		}
	}

	if !strings.Contains(cmd, "agm __exec-codex") {
		t.Errorf("expected private Codex executor: %s", cmd)
	}
	if !strings.Contains(cmd, "--sandbox 'workspace-write'") {
		t.Errorf("expected default workspace-write sandbox: %s", cmd)
	}
	if strings.Contains(cmd, "dangerously-bypass") {
		t.Errorf("full-autonomy bypass must not be a silent default: %s", cmd)
	}
}

func TestBuildCodexCommand_RemoteThreadResume(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "codex-cli", Model: "5.4", SessionName: "codex-session", WorkDir: "/tmp/work",
		ExtraAddDirs: []string{"/tmp/extra dir"}, Codex: &manifest.Codex{SessionID: "thr_123"},
	})

	for _, want := range []string{
		"agm __exec-codex",
		"--session 'codex-session'",
		"--model 'gpt-5.4'",
		"--workdir '/tmp/work'",
		"--sandbox 'workspace-write'",
		"--add-dir '/tmp/extra dir'",
		"--resume-id 'thr_123'",
		"--remote",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("remote Codex command missing %q: %s", want, cmd)
		}
	}
}

func TestBuildAgyCommand_AutoPermissionMode(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash-low", SessionName: "agy", WorkDir: "/tmp/agy work",
		ExtraAddDirs: []string{"/tmp/extra dir"}, PermissionMode: "auto",
	})

	for _, want := range []string{
		"cd '/tmp/agy work' && agy --model 'Gemini 3.5 Flash (Low)' --dangerously-skip-permissions",
		"--add-dir '/tmp/extra dir'",
		"&& exit",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("command %q missing %q", cmd, want)
		}
	}
}

func TestBuildAgyCommand_DefaultPermissionMode(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "agy", Model: "3.5-flash", SessionName: "agy", WorkDir: "/tmp/agy-work",
		PermissionMode: "default",
	})

	if strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Errorf("default AGY command should not skip permissions: %q", cmd)
	}
	if !strings.Contains(cmd, "cd '/tmp/agy-work' && agy --model 'Gemini 3.5 Flash (Medium)' && exit") {
		t.Errorf("unexpected default AGY launch command: %q", cmd)
	}
	if strings.Contains(cmd, "--prompt-interactive") {
		t.Errorf("bare AGY lifecycle must not use the string-valued prompt flag: %q", cmd)
	}
}

func TestBuildPiCommandCarriesExactNativeIdentityAndAuthorization(t *testing.T) {
	cmd := testLaunchCommand(ops.HarnessLaunchSpec{
		Harness: "pi-cli", Model: "sonnet", SessionName: "pi-worker", SessionID: "agm-id",
		WorkDir: "/tmp/pi work", PermissionMode: "plan", Persistent: true,
		Pi:          &manifest.Pi{SessionID: "native-id", SessionDir: "/tmp/pi sessions"},
		PiExtension: "/tmp/agm auth.js", PiPolicyFile: "/tmp/pi policy.json",
	})
	for _, want := range []string{
		"cd '/tmp/pi work'", "pi --session-id 'native-id'", "--session-dir '/tmp/pi sessions'",
		"--name 'pi-worker'", "--model 'anthropic/claude-sonnet-4-6'",
		"--extension '/tmp/agm auth.js'", "AGM_PI_PERMISSION_MODE='plan'",
		"AGM_PI_PERMISSION_POLICY_FILE='/tmp/pi policy.json'", "--tools 'read,grep,find,ls'",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("Pi launch %q missing %q", cmd, want)
		}
	}
	if strings.Contains(cmd, "&& exit") {
		t.Fatalf("persistent Pi command exits pane shell: %q", cmd)
	}
}

func TestActiveHarnessBuildersHonorPersistentStartupContracts(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{name: "Codex", cmd: testLaunchCommand(ops.HarnessLaunchSpec{Harness: "codex-cli", Model: "5.4", SessionName: "worker", WorkDir: "/tmp/work", Persistent: true, PermissionMode: "auto"}), want: "--approval 'never'"},
		{name: "AGY", cmd: testLaunchCommand(ops.HarnessLaunchSpec{Harness: "agy", Model: "3.5-flash", SessionName: "worker", WorkDir: "/tmp/work", Persistent: true, PermissionMode: "auto"}), want: "agy --model 'Gemini 3.5 Flash (Medium)' --dangerously-skip-permissions"},
		{name: "OpenCode", cmd: testLaunchCommand(ops.HarnessLaunchSpec{Harness: "opencode-cli", Model: "glm-5.2", SessionName: "worker", WorkDir: "/tmp/work", Persistent: true}), want: "opencode attach"},
		{name: "Pi", cmd: testLaunchCommand(ops.HarnessLaunchSpec{Harness: "pi-cli", Model: "sonnet", SessionName: "worker", SessionID: "native", WorkDir: "/tmp/work", Persistent: true, Pi: &manifest.Pi{SessionID: "native", SessionDir: "/tmp/pi"}}), want: "pi --session-id 'native'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.cmd, tt.want) {
				t.Fatalf("command %q missing startup contract %q", tt.cmd, tt.want)
			}
			if strings.Contains(tt.cmd, "&& exit") {
				t.Fatalf("persistent command exits pane shell: %q", tt.cmd)
			}
		})
	}
}
