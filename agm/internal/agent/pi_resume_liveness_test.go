package agent

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

func TestDecidePiPaneResumeFailsClosed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		exactPi  bool
		liveness tmux.PaneLiveness
		want     PiPaneResumeAction
		wantErr  string
	}{
		{name: "exact Pi", exactPi: true, want: PiPanePreserve},
		{name: "bare shell", liveness: tmux.PaneLiveness{SessionExists: true, RestartableShell: true, Evidence: "zsh"}, want: PiPaneRelaunch},
		{name: "other harness", liveness: tmux.PaneLiveness{SessionExists: true, HarnessAlive: true, Evidence: "zsh,claude"}, wantErr: "another live harness"},
		{name: "foreground process", liveness: tmux.PaneLiveness{SessionExists: true, Evidence: "zsh,vim"}, wantErr: "not a proven restartable shell"},
		{name: "missing pane", liveness: tmux.PaneLiveness{}, wantErr: "disappeared"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecidePiPaneResume(test.exactPi, test.liveness)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("DecidePiPaneResume() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("DecidePiPaneResume() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
