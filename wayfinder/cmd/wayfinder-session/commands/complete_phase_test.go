package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestRunCompletePhaseLeavesLegacyHistoryUntouchedWhenTrackerSetupFails(t *testing.T) {
	projectDir := t.TempDir()
	previousProjectDir := projectDirectory
	previousOutcome := phaseOutcome
	projectDirectory = projectDir
	phaseOutcome = status.OutcomeSuccess
	t.Cleanup(func() {
		projectDirectory = previousProjectDir
		phaseOutcome = previousOutcome
	})

	st := status.NewStatusV2("complete-phase-guard", "service", "low")
	st.UpdatePhase(status.WaypointV2Charter, status.PhaseStatusInProgress, "")
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		t.Fatalf("seed status file: %v", err)
	}

	legacyPath := filepath.Join(projectDir, history.LegacyHistoryFilename)
	if err := os.WriteFile(legacyPath, []byte("{\"event\":\"seed\"}\n"), 0o600); err != nil {
		t.Fatalf("seed legacy history: %v", err)
	}

	notDirectory := filepath.Join(projectDir, "not-a-directory")
	if err := os.WriteFile(notDirectory, []byte("block mkdir"), 0o600); err != nil {
		t.Fatalf("seed telemetry path blocker: %v", err)
	}
	t.Setenv("ENGRAM_TELEMETRY_PATH", filepath.Join(notDirectory, "telemetry.jsonl"))

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runCompletePhase(cmd, []string{status.WaypointV2Charter}); err == nil {
		t.Fatal("runCompletePhase succeeded despite invalid telemetry path")
	}

	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("failed completion migrated the legacy history file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, history.HistoryFilename)); !os.IsNotExist(err) {
		t.Fatalf("failed completion created %s (stat err: %v)", history.HistoryFilename, err)
	}
}
