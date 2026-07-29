package commands

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

type fakeRewindCommitter struct {
	isRepo bool
	err    error
	called bool
}

func (f *fakeRewindCommitter) IsGitRepo() bool { return f.isRepo }

func (f *fakeRewindCommitter) CommitRewind(_, _ string) error {
	f.called = true
	return f.err
}

func TestCommitRewindStateTreatsCommitFailureAsWarning(t *testing.T) {
	integrator := &fakeRewindCommitter{isRepo: true, err: errors.New("hook rejected commit")}
	var stdout, stderr bytes.Buffer

	commitRewindState(integrator, status.WaypointV2Build, status.WaypointV2Plan, &stdout, &stderr)

	if !integrator.called {
		t.Fatal("CommitRewind was not called")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "rewind persisted") || !strings.Contains(got, "hook rejected commit") {
		t.Fatalf("stderr = %q, want persisted-state warning", got)
	}
}

func TestResetForRewindResetsTargetAndLaterPhases(t *testing.T) {
	now := time.Now()
	outcome := status.OutcomeSuccess
	st := &status.StatusV2{
		WaypointHistory: []status.WaypointHistory{
			{Name: status.WaypointV2Research, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
			{Name: status.WaypointV2Design, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
			{Name: status.WaypointV2Spec, Status: status.PhaseStatusV2Completed, StartedAt: now, CompletedAt: &now, Outcome: &outcome},
		},
		Roadmap: &status.Roadmap{Phases: []status.RoadmapPhase{
			{ID: status.WaypointV2Research, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
			{ID: status.WaypointV2Design, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
			{ID: status.WaypointV2Spec, Status: status.PhaseStatusV2Completed, StartedAt: &now, CompletedAt: &now},
		}},
	}
	phases := status.AllPhasesV2Schema()
	resetForRewind(st, phases, 3)

	if got := st.WaypointHistory[0].Status; got != status.PhaseStatusV2Completed {
		t.Fatalf("earlier phase status = %q, want completed", got)
	}
	if got := len(st.WaypointHistory); got != 1 {
		t.Fatalf("waypoint history length = %d, want only the earlier completed entry", got)
	}
	for _, phase := range st.Roadmap.Phases[1:] {
		if phase.Status != status.PhaseStatusV2Pending || phase.StartedAt != nil || phase.CompletedAt != nil {
			t.Errorf("rewound roadmap phase = %+v, want clean pending state", phase)
		}
	}
	st.UpdatePhase(status.WaypointV2Design, status.PhaseStatusV2InProgress, "")
	restarted := st.GetPhaseHistory(status.WaypointV2Design)
	if restarted == nil || restarted.StartedAt.IsZero() {
		t.Fatalf("restarted target history = %+v, want a valid start timestamp", restarted)
	}
}

func TestResetLifecycleForRewindReopensTerminalStatus(t *testing.T) {
	completedAt := time.Now().Add(-time.Hour)
	rewoundAt := time.Now()
	st := &status.StatusV2{
		Status:         status.StatusV2Completed,
		LifecycleState: status.LifecycleCompleted,
		CompletionDate: &completedAt,
		BlockedReason:  "stale block",
		BlockedOn:      "stale-dependency",
		ErrorMessage:   "stale error",
		InputNeeded:    "stale input",
	}

	resetLifecycleForRewind(st, rewoundAt)

	if st.Status != status.StatusV2InProgress || st.LifecycleState != status.LifecycleWorking {
		t.Fatalf("rewound lifecycle = %q/%q, want in-progress/working", st.Status, st.LifecycleState)
	}
	if st.CompletionDate != nil || st.BlockedReason != "" || st.BlockedOn != "" || st.ErrorMessage != "" || st.InputNeeded != "" {
		t.Fatalf("rewound lifecycle retained terminal metadata: %+v", st)
	}
	if !st.UpdatedAt.Equal(rewoundAt) {
		t.Fatalf("updated_at = %s, want %s", st.UpdatedAt, rewoundAt)
	}
}

func TestValidateRewindTargetRejectsConfiguredSkip(t *testing.T) {
	now := time.Now()
	st := &status.StatusV2{
		SkipPhases: []string{status.WaypointV2Design},
		WaypointHistory: []status.WaypointHistory{
			{Name: status.WaypointV2Design, Status: status.PhaseStatusV2Skipped, StartedAt: now},
		},
	}

	err := validateRewindTarget(st, status.WaypointV2Design)
	if err == nil || err.Error() != "cannot rewind to phase DESIGN: phase is configured to be skipped" {
		t.Fatalf("validateRewindTarget() error = %v, want configured-skip rejection", err)
	}
}

func TestRunRewindLeavesLegacyHistoryUntouchedWhenTargetIsRejected(t *testing.T) {
	projectDir := t.TempDir()
	previous := projectDirectory
	projectDirectory = projectDir
	t.Cleanup(func() { projectDirectory = previous })

	st := status.NewStatusV2("rewind-guard", "service", "low")
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("seed status file: %v", err)
	}

	legacyPath := filepath.Join(projectDir, history.LegacyHistoryFilename)
	if err := os.WriteFile(legacyPath, []byte("{\"event\":\"seed\"}\n"), 0o600); err != nil {
		t.Fatalf("seed legacy history: %v", err)
	}

	if err := runRewind(nil, []string{"NOT-A-PHASE"}); err == nil {
		t.Fatal("runRewind accepted an invalid target phase")
	}

	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("rejected rewind migrated the legacy history file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, history.HistoryFilename)); !os.IsNotExist(err) {
		t.Fatalf("rejected rewind created %s (stat err: %v)", history.HistoryFilename, err)
	}
}
