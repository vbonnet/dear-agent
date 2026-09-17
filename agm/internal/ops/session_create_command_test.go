package ops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/pkg/override"
)

func TestProjectPrivateLaunchAuthorityUsesStableSessionIDAcrossCreateAndColdResume(t *testing.T) {
	sandboxRoot, rootPath, workspacePath, workDir := testPrivateLaunchSandboxRoot(t, "stable-agm-session")

	for _, lifecycle := range []string{"create", "cold-resume"} {
		for _, harness := range []string{"claude-code", "codex-cli"} {
			t.Run(lifecycle+"/"+harness, func(t *testing.T) {
				spec := HarnessLaunchSpec{
					Harness:        harness,
					SessionID:      "stable-agm-session",
					SandboxRoot:    sandboxRoot,
					SandboxEnabled: true,
					WorkDir:        workDir,
				}
				if lifecycle == "cold-resume" {
					spec.ResumeID = "provider-native-conversation-id"
				}

				authority, err := projectPrivateLaunchAuthority(spec)
				if err != nil {
					t.Fatalf("projectPrivateLaunchAuthority() error = %v", err)
				}
				if authority.diskRoot != rootPath {
					t.Fatalf("disk root = %q, want configured parent %q", authority.diskRoot, rootPath)
				}
				if authority.sandboxWorkspace != workspacePath {
					t.Fatalf("sandbox workspace = %q, want stable AGM session workspace %q", authority.sandboxWorkspace, workspacePath)
				}
				if authority.workDir != workDir {
					t.Fatalf("workdir = %q, want physical %q", authority.workDir, workDir)
				}
				if strings.Contains(authority.sandboxWorkspace, spec.ResumeID) && spec.ResumeID != "" {
					t.Fatalf("sandbox workspace %q was derived from provider identity %q", authority.sandboxWorkspace, spec.ResumeID)
				}
			})
		}
	}
}

func TestProjectPrivateLaunchAuthorityRejectsWorkDirOutsideWorkspace(t *testing.T) {
	sandboxRoot, _, _, _ := testPrivateLaunchSandboxRoot(t, "stable-agm-session")
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := projectPrivateLaunchAuthority(HarnessLaunchSpec{
		Harness:        "codex-cli",
		SessionID:      "stable-agm-session",
		SandboxRoot:    sandboxRoot,
		SandboxEnabled: true,
		WorkDir:        outside,
	})
	if err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("projectPrivateLaunchAuthority() error = %v, want outside-workspace rejection", err)
	}
}

func TestProjectPrivateLaunchAuthorityAcceptsAPFSMergedWorkDir(t *testing.T) {
	sandboxRoot, rootPath, workspacePath, _ := testPrivateLaunchSandboxRoot(t, "stable-agm-session")
	mergedPath := filepath.Join(workspacePath, "merged")
	if err := os.Remove(mergedPath); err != nil {
		t.Fatal(err)
	}
	upperWorkDir := filepath.Join(workspacePath, "upper", "repo")
	if err := os.MkdirAll(upperWorkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(workspacePath, "upper"), mergedPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	workDir := filepath.Join(mergedPath, "repo")

	authority, err := projectPrivateLaunchAuthority(HarnessLaunchSpec{
		Harness:        "codex-cli",
		SessionID:      "stable-agm-session",
		SandboxRoot:    sandboxRoot,
		SandboxEnabled: true,
		WorkDir:        workDir,
	})
	if err != nil {
		t.Fatalf("projectPrivateLaunchAuthority() error = %v", err)
	}
	if authority.diskRoot != rootPath || authority.sandboxWorkspace != workspacePath || authority.workDir != upperWorkDir {
		t.Fatalf("authority = (%q, %q, %q), want (%q, %q, %q)",
			authority.diskRoot, authority.sandboxWorkspace, authority.workDir,
			rootPath, workspacePath, upperWorkDir)
	}
}

func TestProjectPrivateLaunchAuthorityRequiresRootOnlyForSandboxWorkspace(t *testing.T) {
	if _, err := projectPrivateLaunchAuthority(HarnessLaunchSpec{
		SessionID: "stable-agm-session", SandboxEnabled: true, WorkDir: "/tmp/work",
	}); !errors.Is(err, config.ErrRuntimeAuthorityUnavailable) {
		t.Fatalf("sandboxed zero authority error = %v, want fail-closed authority error", err)
	}

	authority, err := projectPrivateLaunchAuthority(HarnessLaunchSpec{})
	if err != nil {
		t.Fatalf("legacy unsandboxed zero authority error = %v", err)
	}
	if authority.diskRoot != "" || authority.sandboxWorkspace != "" || authority.workDir != "" {
		t.Fatalf("legacy unsandboxed zero authority = (%q, %q, %q), want empty compatibility projection",
			authority.diskRoot, authority.sandboxWorkspace, authority.workDir)
	}

	sandboxRoot, rootPath, _, workDir := testPrivateLaunchSandboxRoot(t, "stable-agm-session")
	authority, err = projectPrivateLaunchAuthority(HarnessLaunchSpec{
		SessionID: "stable-agm-session", SandboxRoot: sandboxRoot, WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("unsandboxed configured authority error = %v", err)
	}
	if authority.diskRoot != rootPath || authority.sandboxWorkspace != "" || authority.workDir != workDir {
		t.Fatalf("unsandboxed configured authority = (%q, %q, %q), want (%q, empty, %q)",
			authority.diskRoot, authority.sandboxWorkspace, authority.workDir, rootPath, workDir)
	}
}

func testPrivateLaunchSandboxRoot(t *testing.T, sessionID string) (config.SandboxRoot, string, string, string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve synthetic home: %v", err)
	}
	rootPath := filepath.Join(home, ".agm", "sandboxes")
	workspacePath := filepath.Join(rootPath, sessionID)
	workDir := filepath.Join(workspacePath, "merged")
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	cfg, err := config.Load("")
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	authority, err := cfg.RuntimeAuthority()
	if err != nil {
		t.Fatalf("RuntimeAuthority() error = %v", err)
	}
	sandboxRoot, err := authority.Sandboxes()
	if err != nil {
		t.Fatalf("Sandboxes() error = %v", err)
	}
	return sandboxRoot, rootPath, workspacePath, workDir
}

func TestResolveHarnessLaunchSubmissionPreservesUncertainAndCancelsConfirmedFailure(t *testing.T) {
	cases := []struct {
		name          string
		submissionErr error
		wantUncertain bool
		wantCancel    bool
		wantErr       bool
	}{
		{
			name:          "uncertain acknowledgement preserves launch",
			submissionErr: tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement")),
			wantUncertain: true,
		},
		{
			name:          "confirmed failure cancels launch",
			submissionErr: errors.New("send rejected"),
			wantCancel:    true,
			wantErr:       true,
		},
		{name: "confirmed success", submissionErr: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cancelled := false
			command := HarnessLaunchCommand{Cancel: func() error {
				cancelled = true
				return nil
			}}
			uncertain, err := ResolveHarnessLaunchSubmission(command, tc.submissionErr)
			if uncertain != tc.wantUncertain {
				t.Fatalf("uncertain = %v, want %v", uncertain, tc.wantUncertain)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v", err, tc.wantErr)
			}
			if cancelled != tc.wantCancel {
				t.Fatalf("cancelled = %v, want %v", cancelled, tc.wantCancel)
			}
		})
	}
}

func TestPrepareHarnessLaunchCommandRejectsControlsForSharedHarnesses(t *testing.T) {
	for _, value := range []struct {
		name    string
		workDir string
	}{
		{name: "escape and newline", workDir: "/safe\x1b[201~\nunsafe"},
		{name: "NUL", workDir: "/safe\x00unsafe"},
	} {
		for _, harness := range []string{"agy", "pi-cli", "opencode-cli"} {
			t.Run(harness+"/"+value.name, func(t *testing.T) {
				_, err := PrepareHarnessLaunchCommand(HarnessLaunchSpec{
					Harness: harness,
					WorkDir: value.workDir,
				})
				if err == nil || !strings.Contains(err.Error(), "control characters") {
					t.Fatalf("PrepareHarnessLaunchCommand() error = %v, want terminal-control rejection", err)
				}
			})
		}
	}
}

func TestReserveCodexLaunchCarriesExactReservationToSubmission(t *testing.T) {
	originalReserve := reserveCodexHookTrust
	t.Cleanup(func() { reserveCodexHookTrust = originalReserve })

	const (
		reason  = "sandbox path rotates per spawn so hooks cannot be pre-trusted"
		actor   = "vroom-dispatch"
		session = "worker-ce-6xfu"
	)
	reservation := &override.Reservation{}
	var wantProof override.AuthorizationProof
	reserveCodexHookTrust = func(gotReason, gotActor, gotSession, subject string) (
		*override.Reservation, override.AuthorizationProof, error,
	) {
		if gotReason != reason || gotActor != actor || gotSession != session || subject == "" {
			t.Fatalf("reservation request = (%q, %q, %q, %q)", gotReason, gotActor, gotSession, subject)
		}
		wantProof = override.AuthorizationProof{
			Kind:            override.KindCodexHookTrust,
			Reason:          gotReason,
			Actor:           gotActor,
			Session:         gotSession,
			Subject:         subject,
			AuthorizationID: "0123456789abcdef0123456789abcdef",
		}
		return reservation, wantProof, nil
	}
	launch, _ := codexLaunch(HarnessLaunchSpec{
		Harness:               "codex-cli",
		Model:                 "gpt-test",
		SessionName:           session,
		WorkDir:               "/tmp/work",
		BypassCodexHookTrust:  true,
		CodexHookRoot:         "/trusted/hooks/digest",
		CodexHookTrustReason:  reason,
		CodexHookTrustActor:   actor,
		CodexHookSourceRepo:   "/reviewed/dear-agent",
		CodexHookSourceCommit: strings.Repeat("a", 40),
		CodexHookDigest:       strings.Repeat("b", 64),
	})
	prepared, reservations, err := reserveCodexLaunch(launch)
	if err != nil {
		t.Fatalf("reserve Codex launch: %v", err)
	}
	if prepared.HookTrustProof != wantProof || prepared.HookTrustSubject != wantProof.Subject {
		t.Fatalf("prepared Codex proof = %+v, subject %q; want %+v", prepared.HookTrustProof, prepared.HookTrustSubject, wantProof)
	}
	if len(reservations) != 1 || reservations[0] != reservation {
		t.Fatalf("prepared reservations = %v, want exact hook-trust reservation", reservations)
	}
}

func TestPrepareGeminiLaunchCommandValidatesOnlyPastedModel(t *testing.T) {
	command, err := PrepareHarnessLaunchCommand(HarnessLaunchSpec{
		Harness: "gemini-cli",
		Model:   "gemini-2.5-pro",
		WorkDir: "/unused\x1b[201~\nworkdir",
	})
	if err != nil {
		t.Fatalf("unused Gemini workdir rejected: %v", err)
	}
	if strings.Contains(command.Command, "unused") {
		t.Fatalf("Gemini command unexpectedly contains workdir: %q", command.Command)
	}

	_, err = PrepareHarnessLaunchCommand(HarnessLaunchSpec{
		Harness: "gemini-cli",
		Model:   "gemini-2.5-pro\x1b[201~\nunsafe",
		WorkDir: "/safe",
	})
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("PrepareHarnessLaunchCommand() error = %v, want model control rejection", err)
	}
}

func TestPrepareNonCodexLaunchWithAdmissionUsesPrivateExecutor(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	launch, err := PrepareHarnessLaunchCommand(HarnessLaunchSpec{
		Harness:     "agy",
		SessionName: "admission-bound-agy",
		WorkDir:     "/tmp",
		BeforeSpawn: func(
			reservations ...*override.Reservation,
		) ([]*override.Reservation, error) {
			return reservations, nil
		},
		AfterAuthorization: func() {},
	})
	if err != nil {
		t.Fatalf("PrepareHarnessLaunchCommand() error = %v", err)
	}
	t.Cleanup(func() { _ = launch.CancelUndelivered() })
	if !strings.Contains(launch.Command, harnessexec.AgyProtocol) {
		t.Fatalf("prepared launch = %q, want private harness executor", launch.Command)
	}
	if launch.BindOverrideReservations == nil || launch.Cancel == nil {
		t.Fatal("admission-bound launch omitted handoff binding or cancellation")
	}
}

func TestPrepareClaudeLaunchCarriesAdmissionBinding(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	launch, err := PrepareHarnessLaunchCommand(HarnessLaunchSpec{
		Harness:     "claude-code",
		SessionName: "admission-bound-claude",
		WorkDir:     "/tmp",
		BeforeSpawn: func(
			reservations ...*override.Reservation,
		) ([]*override.Reservation, error) {
			return reservations, nil
		},
		AfterAuthorization: func() {},
		DisableOAuth:       true,
	})
	if err != nil {
		t.Fatalf("PrepareHarnessLaunchCommand() error = %v", err)
	}
	t.Cleanup(func() { _ = launch.CancelUndelivered() })
	if !strings.Contains(launch.Command, harnessexec.ClaudeProtocol) {
		t.Fatalf("prepared launch = %q, want private Claude executor", launch.Command)
	}
	if launch.BindOverrideReservations == nil || launch.Cancel == nil {
		t.Fatal("admission-bound Claude launch omitted handoff binding or cancellation")
	}
}

func TestPrepareAgyResumeCommandRejectsConversationControls(t *testing.T) {
	_, err := PrepareAgyResumeCommand(HarnessLaunchSpec{
		Harness: "agy",
		WorkDir: "/safe",
	}, "conversation\x1b[201~\nunsafe")
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("PrepareAgyResumeCommand() error = %v, want terminal-control rejection", err)
	}
}

func TestPrepareFallbackResumeCommandRejectsWorkdirControls(t *testing.T) {
	_, err := PrepareFallbackResumeCommand("/safe\x1b[201~\nunsafe")
	if err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("PrepareFallbackResumeCommand() error = %v, want terminal-control rejection", err)
	}
}
