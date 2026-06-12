package tracker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_CreatesTrackerWithTempFile(t *testing.T) {
	dir := t.TempDir()
	telPath := filepath.Join(dir, "telemetry.jsonl")
	t.Setenv("ENGRAM_TELEMETRY_PATH", telPath)

	tr, err := New("test-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	if _, err := os.Stat(telPath); err != nil {
		t.Errorf("telemetry file not created: %v", err)
	}
}

func TestNew_SessionID_NonEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENGRAM_TELEMETRY_PATH", filepath.Join(dir, "tel.jsonl"))

	tr, err := New("my-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	if got := tr.SessionID(); got == "" {
		t.Error("SessionID() returned empty string")
	}
}

func TestNew_FullLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENGRAM_TELEMETRY_PATH", filepath.Join(dir, "tel.jsonl"))

	tr, err := New("lifecycle-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := tr.StartSession("/some/project"); err != nil {
		t.Errorf("StartSession: %v", err)
	}
	if err := tr.StartPhase("D"); err != nil {
		t.Errorf("StartPhase: %v", err)
	}
	if err := tr.CompletePhase("D", "success", nil); err != nil {
		t.Errorf("CompletePhase: %v", err)
	}
	if err := tr.EndSession("success"); err != nil {
		t.Errorf("EndSession: %v", err)
	}
	if err := tr.Close(context.Background()); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestClose_NilTelemetry(t *testing.T) {
	tr := &Tracker{}
	if err := tr.Close(context.Background()); err != nil {
		t.Errorf("Close with nil telemetry: %v", err)
	}
}
