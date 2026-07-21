package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestFinalizeCLICreateSessionStopsCancellationAfterLiveness(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	completed := false

	err := finalizeCLICreateSession(ctx, "agy-create", cliCreateFinalizationRuntime{
		checkLiveness: func(got context.Context, _, _ string) (tmux.PaneLiveness, error) {
			if got != ctx {
				t.Fatal("liveness scan did not receive the caller context")
			}
			cancel()
			return tmux.PaneLiveness{SessionExists: true, HarnessAlive: true}, nil
		},
		updateTitle: func(string) { completed = true },
		attach:      func(string) { completed = true },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("finalizeCLICreateSession() error = %v, want context.Canceled", err)
	}
	if completed {
		t.Fatal("creation completed or attached after liveness scan canceled its caller")
	}
}

func TestValidateFinalStartupLiveness(t *testing.T) {
	tests := []struct {
		name    string
		verdict tmux.PaneLiveness
		err     error
		wantErr string
	}{
		{name: "live", verdict: tmux.PaneLiveness{SessionExists: true, HarnessAlive: true}},
		{name: "probe failure", err: errors.New("ps failed"), wantErr: "liveness check failed"},
		{name: "session gone", verdict: tmux.PaneLiveness{}, wantErr: "session disappeared"},
		{name: "harness dead", verdict: tmux.PaneLiveness{SessionExists: true, Evidence: "descendants: zsh"}, wantErr: "no active harness process"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := launchparity.ValidateFinalLiveness(tt.verdict, tt.err)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
