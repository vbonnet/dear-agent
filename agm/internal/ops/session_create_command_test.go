package ops

import (
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/pkg/override"
)

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
