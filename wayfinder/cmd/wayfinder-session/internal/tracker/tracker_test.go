package tracker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_SucceedsWithTempPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENGRAM_TELEMETRY_PATH", filepath.Join(dir, "telemetry.jsonl"))

	tr, err := New("test-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer tr.Close(context.Background())
}

func TestNew_SessionIDIsNonEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENGRAM_TELEMETRY_PATH", filepath.Join(dir, "telemetry.jsonl"))

	tr, err := New("my-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer tr.Close(context.Background())
	if tr.SessionID() == "" {
		t.Error("SessionID() should be non-empty")
	}
}

func TestClose_NilTelemetry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENGRAM_TELEMETRY_PATH", filepath.Join(dir, "telemetry.jsonl"))

	tr, err := New("close-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tr.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNew_CreatesFileOnWrite(t *testing.T) {
	dir := t.TempDir()
	telPath := filepath.Join(dir, "telemetry.jsonl")
	t.Setenv("ENGRAM_TELEMETRY_PATH", telPath)

	tr, err := New("file-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = tr.StartSession("/tmp/project")
	defer tr.Close(context.Background())
	if _, err := os.Stat(telPath); err != nil {
		t.Errorf("telemetry file not created: %v", err)
	}
}
