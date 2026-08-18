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

func TestRunStartPhaseLeavesLegacyHistoryUntouchedWhenTrackerSetupFails(t *testing.T) {
	projectDir := t.TempDir()
	previousProjectDir := projectDirectory
	previousAllowDirty := allowDirty
	projectDirectory = projectDir
	allowDirty = true
	t.Cleanup(func() {
		projectDirectory = previousProjectDir
		allowDirty = previousAllowDirty
	})

	st := status.NewStatusV2("start-phase-guard", "service", "low")
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
	if err := runStartPhase(cmd, []string{status.WaypointV2Charter}); err == nil {
		t.Fatal("runStartPhase succeeded despite invalid telemetry path")
	}

	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("failed start migrated the legacy history file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, history.HistoryFilename)); !os.IsNotExist(err) {
		t.Fatalf("failed start created %s (stat err: %v)", history.HistoryFilename, err)
	}
}
