package ops

import (
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
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
