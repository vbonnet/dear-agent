package tracker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_WithEnvPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	t.Setenv("ENGRAM_TELEMETRY_PATH", path)

	tr, err := New("test-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	if tr.SessionID() == "" {
		t.Error("SessionID should be non-empty")
	}
}

func TestNew_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	t.Setenv("ENGRAM_TELEMETRY_PATH", path)

	tr, err := New("test-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("telemetry file should exist after New: %v", err)
	}
}

func TestNew_InvalidPath(t *testing.T) {
	t.Setenv("ENGRAM_TELEMETRY_PATH", "/nonexistent/dir/telemetry.jsonl")
	_, err := New("test-session")
	if err == nil {
		t.Error("expected error for unwritable path")
	}
}

func TestLifecycle_NoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	t.Setenv("ENGRAM_TELEMETRY_PATH", path)

	tr, err := New("test-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	if err := tr.StartSession("/test/project"); err != nil {
		t.Errorf("StartSession: %v", err)
	}
	if err := tr.StartPhase("PROBLEM"); err != nil {
		t.Errorf("StartPhase: %v", err)
	}
	if err := tr.CompletePhase("PROBLEM", "success", map[string]any{"key": "val"}); err != nil {
		t.Errorf("CompletePhase: %v", err)
	}
	if err := tr.EndSession("success"); err != nil {
		t.Errorf("EndSession: %v", err)
	}
}

func TestClose_NoError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	t.Setenv("ENGRAM_TELEMETRY_PATH", path)

	tr, err := New("test-session")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := tr.Close(context.Background()); err != nil {
		t.Errorf("Close: %v", err)
	}
}
