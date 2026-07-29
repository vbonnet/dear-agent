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
	for _, harness := range []string{"agy", "pi-cli", "opencode-cli"} {
		t.Run(harness, func(t *testing.T) {
			_, err := PrepareHarnessLaunchCommand(HarnessLaunchSpec{
				Harness: harness,
				WorkDir: "/safe\x1b[201~\nunsafe",
			})
			if err == nil || !strings.Contains(err.Error(), "control characters") {
				t.Fatalf("PrepareHarnessLaunchCommand() error = %v, want terminal-control rejection", err)
			}
		})
	}
}
