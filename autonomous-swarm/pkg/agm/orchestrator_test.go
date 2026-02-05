package csm

import (
	"testing"
)

func TestNewOrchestrator(t *testing.T) {
	orch := NewOrchestrator()
	if orch == nil {
		t.Fatal("NewOrchestrator should not return nil")
	}

	if orch.csmBinary != "csm" {
		t.Errorf("default csmBinary: got %s, want csm", orch.csmBinary)
	}
}

func TestNewOrchestratorWithBinary(t *testing.T) {
	customPath := "/usr/local/bin/csm"
	orch := NewOrchestratorWithBinary(customPath)

	if orch.csmBinary != customPath {
		t.Errorf("csmBinary: got %s, want %s", orch.csmBinary, customPath)
	}
}

func TestCreate_EmptySessionName(t *testing.T) {
	orch := NewOrchestrator()
	err := orch.Create("")

	if err == nil {
		t.Fatal("Create should fail for empty session name")
	}

	if err.Error() != "session name cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMonitor_EmptySessionName(t *testing.T) {
	orch := NewOrchestrator()
	_, err := orch.Monitor("")

	if err == nil {
		t.Fatal("Monitor should fail for empty session name")
	}

	if err.Error() != "session name cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtract_EmptySessionName(t *testing.T) {
	orch := NewOrchestrator()
	_, err := orch.Extract("")

	if err == nil {
		t.Fatal("Extract should fail for empty session name")
	}

	if err.Error() != "session name cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestArchive_EmptySessionName(t *testing.T) {
	orch := NewOrchestrator()
	err := orch.Archive("")

	if err == nil {
		t.Fatal("Archive should fail for empty session name")
	}

	if err.Error() != "session name cannot be empty" {
		t.Errorf("unexpected error: %v", err)
	}
}

// Integration tests - require actual CSM installation
// These tests are skipped in CI environments without CSM

func TestCreate_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	orch := NewOrchestrator()
	sessionName := "test-autonomous-swarm-create"

	// Cleanup any existing session
	_ = orch.Archive(sessionName)

	// Create session
	err := orch.Create(sessionName)
	if err != nil {
		t.Skipf("CSM not available or test failed: %v", err)
	}

	// Cleanup
	defer func() {
		_ = orch.Archive(sessionName)
	}()

	// Verify session exists
	healthy, err := orch.Monitor(sessionName)
	if err != nil {
		t.Fatalf("Monitor failed: %v", err)
	}

	if !healthy {
		t.Error("newly created session should be healthy")
	}
}

func TestMonitor_NonexistentSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	orch := NewOrchestrator()
	sessionName := "nonexistent-session-12345"

	healthy, err := orch.Monitor(sessionName)
	if err != nil {
		t.Skipf("CSM not available: %v", err)
	}

	if healthy {
		t.Error("nonexistent session should not be healthy")
	}
}

func TestExtract_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	orch := NewOrchestrator()
	sessionName := "test-autonomous-swarm-extract"

	// Cleanup any existing session
	_ = orch.Archive(sessionName)

	// Create session
	if err := orch.Create(sessionName); err != nil {
		t.Skipf("CSM not available: %v", err)
	}

	defer func() {
		_ = orch.Archive(sessionName)
	}()

	// Extract UUID
	uuid, err := orch.Extract(sessionName)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if uuid == "" {
		t.Error("UUID should not be empty")
	}

	// UUID should be valid format (basic check)
	if len(uuid) < 10 {
		t.Errorf("UUID seems invalid: %s", uuid)
	}
}

func TestArchive_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	orch := NewOrchestrator()
	sessionName := "test-autonomous-swarm-archive"

	// Cleanup any existing session
	_ = orch.Archive(sessionName)

	// Create session
	if err := orch.Create(sessionName); err != nil {
		t.Skipf("CSM not available: %v", err)
	}

	// Archive session
	if err := orch.Archive(sessionName); err != nil {
		t.Fatalf("Archive failed: %v", err)
	}

	// Verify session no longer exists
	healthy, err := orch.Monitor(sessionName)
	if err != nil {
		t.Fatalf("Monitor failed: %v", err)
	}

	if healthy {
		t.Error("archived session should not be healthy")
	}
}

func TestWaitForSession_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	orch := NewOrchestrator()
	sessionName := "nonexistent-wait-session"

	// Should timeout for nonexistent session
	err := orch.WaitForSession(sessionName, 500*1000000) // 500ms
	if err == nil {
		t.Fatal("WaitForSession should timeout for nonexistent session")
	}
}

func TestWaitForSession_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	orch := NewOrchestrator()
	sessionName := "test-autonomous-swarm-wait"

	// Cleanup
	_ = orch.Archive(sessionName)

	// Create session
	if err := orch.Create(sessionName); err != nil {
		t.Skipf("CSM not available: %v", err)
	}

	defer func() {
		_ = orch.Archive(sessionName)
	}()

	// Should succeed immediately
	err := orch.WaitForSession(sessionName, 5*1000000000) // 5s
	if err != nil {
		t.Fatalf("WaitForSession failed: %v", err)
	}
}
