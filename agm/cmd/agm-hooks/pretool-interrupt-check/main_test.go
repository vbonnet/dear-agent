package main

import (
	"os"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
)

func TestRunNoSession(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_NAME", "")
	os.Unsetenv("CLAUDE_SESSION_NAME")
	t.Setenv("AGM_SESSION_NAME", "") // restored on test cleanup
	os.Unsetenv("AGM_SESSION_NAME")

	code := run()
	if code != 0 {
		t.Errorf("run() with no session = %d, want 0", code)
	}
}

func TestRunNoFlag(t *testing.T) {
	dir := t.TempDir()
	// Point DefaultDir to temp dir via env isn't possible directly,
	// so we test the interrupt package directly
	session := "hook-test"
	flag, err := interrupt.Read(dir, session)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if flag != nil {
		t.Error("expected nil flag")
	}
}

func TestStopFlagBlocks(t *testing.T) {
	dir := t.TempDir()
	session := "stop-test"

	flag := &interrupt.Flag{
		Type:     interrupt.TypeStop,
		Reason:   "budget exceeded",
		IssuedBy: "orchestrator",
		IssuedAt: time.Now().UTC(),
	}
	if err := interrupt.Write(dir, session, flag); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// After consuming, flag should be gone
	consumed, err := interrupt.Consume(dir, session)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if consumed == nil {
		t.Fatal("Consume() returned nil for stop flag")
	}
	if consumed.Type != interrupt.TypeStop {
		t.Errorf("consumed type = %v, want stop", consumed.Type)
	}

	// Flag should be deleted
	again, _ := interrupt.Read(dir, session)
	if again != nil {
		t.Error("stop flag should be consumed (deleted) after Consume()")
	}
}

func TestKillFlagPersists(t *testing.T) {
	dir := t.TempDir()
	session := "kill-test"

	flag := &interrupt.Flag{
		Type:     interrupt.TypeKill,
		Reason:   "emergency",
		IssuedBy: "user",
		IssuedAt: time.Now().UTC(),
	}
	if err := interrupt.Write(dir, session, flag); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Read without consuming (kill behavior — hook reads but does not consume)
	read, err := interrupt.Read(dir, session)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if read == nil {
		t.Fatal("Read() returned nil for kill flag")
	}

	// Flag should still be there
	again, _ := interrupt.Read(dir, session)
	if again == nil {
		t.Error("kill flag should persist after Read() (not consumed)")
	}
}

func TestSteerFlagConsumed(t *testing.T) {
	dir := t.TempDir()
	session := "steer-test"

	flag := &interrupt.Flag{
		Type:     interrupt.TypeSteer,
		Reason:   "focus on tests",
		IssuedBy: "orchestrator",
		IssuedAt: time.Now().UTC(),
	}
	if err := interrupt.Write(dir, session, flag); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	consumed, err := interrupt.Consume(dir, session)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if consumed == nil {
		t.Fatal("Consume() returned nil")
	}
	if consumed.Type != interrupt.TypeSteer {
		t.Errorf("consumed type = %v, want steer", consumed.Type)
	}

	// Should be gone
	again, _ := interrupt.Read(dir, session)
	if again != nil {
		t.Error("steer flag should be consumed after Consume()")
	}
}
