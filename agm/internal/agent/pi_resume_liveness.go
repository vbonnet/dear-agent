package agent

import (
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// PiPaneResumeAction is the only safe action after classifying an existing
// tmux pane recorded for a Pi session.
type PiPaneResumeAction string

const (
	// PiPanePreserve reuses the exact live Pi process without injecting input.
	PiPanePreserve PiPaneResumeAction = "preserve"
	// PiPaneRelaunch starts Pi only after positively proving a bare shell.
	PiPaneRelaunch PiPaneResumeAction = "relaunch"
)

// DecidePiPaneResume centralizes the fail-closed existing-pane invariant used
// by the Pi adapter, root CLI resume path, and cross-harness BDD suite.
func DecidePiPaneResume(exactPiProcess bool, liveness tmux.PaneLiveness) (PiPaneResumeAction, error) {
	if exactPiProcess {
		return PiPanePreserve, nil
	}
	evidence := strings.TrimSpace(liveness.Evidence)
	if evidence == "" {
		evidence = "unavailable"
	}
	if !liveness.SessionExists {
		return "", fmt.Errorf("tmux pane disappeared during Pi liveness classification (pane tree: %s)", evidence)
	}
	if liveness.HarnessAlive {
		return "", fmt.Errorf("tmux pane contains another live harness (pane tree: %s)", evidence)
	}
	if !liveness.RestartableShell {
		return "", fmt.Errorf("tmux pane is not a proven restartable shell (pane tree: %s)", evidence)
	}
	return PiPaneRelaunch, nil
}
