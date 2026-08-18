package coordinator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MockSandboxManager for testing
type MockSandboxManager struct {
	sandboxes map[string]*Sandbox
	createErr error
}

func NewMockSandboxManager() *MockSandboxManager {
	return &MockSandboxManager{
		sandboxes: make(map[string]*Sandbox),
	}
}

func (m *MockSandboxManager) CreateSandbox(name string) (*Sandbox, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}

	sb := &Sandbox{
		ID:   fmt.Sprintf("mock-%s-%d", name, time.Now().Unix()),
		Name: name,
	}
	m.sandboxes[sb.ID] = sb
	return sb, nil
}

func (m *MockSandboxManager) ListSandboxes() ([]*Sandbox, error) {
	var list []*Sandbox
	for _, sb := range m.sandboxes {
		list = append(list, sb)
	}
	return list, nil
}

func (m *MockSandboxManager) CleanupSandbox(nameOrID string) error {
	delete(m.sandboxes, nameOrID)
	return nil
}

func TestNewCoordinator(t *testing.T) {
	cfg := DefaultConfig()
	mockSandbox := NewMockSandboxManager()
	coord := NewCoordinator(cfg, mockSandbox)

	if coord == nil {
		t.Fatal("NewCoordinator returned nil")
		return
	}

	if coord.maxConcurrent != 4 {
		t.Errorf("Expected maxConcurrent=4, got %d", coord.maxConcurrent)
	}

	if coord.monitor == nil {
		t.Error("Monitor not initialized")
	}
}

func TestCoordinatorDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxConcurrent != 4 {
		t.Errorf("Expected MaxConcurrent=4, got %d", cfg.MaxConcurrent)
	}

	if cfg.MonitorInterval != 10*time.Second {
		t.Errorf("Expected MonitorInterval=10s, got %v", cfg.MonitorInterval)
	}
}

func TestCoordinatorConfigValidation(t *testing.T) {
	cfg := Config{
		MaxConcurrent:   0, // Invalid
		MonitorInterval: 0, // Invalid
	}

	mockSandbox := NewMockSandboxManager()
	coord := NewCoordinator(cfg, mockSandbox)

	// Should apply defaults
	if coord.maxConcurrent != 4 {
		t.Errorf("Expected default maxConcurrent=4, got %d", coord.maxConcurrent)
	}

	if coord.monitor.interval != 10*time.Second {
		t.Errorf("Expected default interval=10s, got %v", coord.monitor.interval)
	}
}

func TestCoordinatorStatus(t *testing.T) {
	cfg := DefaultConfig()
	mockSandbox := NewMockSandboxManager()
	coord := NewCoordinator(cfg, mockSandbox)

	// Initially empty
	status := coord.Status()
	if len(status) != 0 {
		t.Errorf("Expected empty status, got %d projects", len(status))
	}

	// Add a project
	coord.mu.Lock()
	coord.projects["/test/project"] = &ProjectExecution{
		ProjectDir: "/test/project",
		Status:     StatusQueued,
	}
	coord.mu.Unlock()

	status = coord.Status()
	if len(status) != 1 {
		t.Errorf("Expected 1 project, got %d", len(status))
	}

	if status["/test/project"].Status != StatusQueued {
		t.Errorf("Expected StatusQueued, got %v", status["/test/project"].Status)
	}
}

func TestUpdateProjectStatus(t *testing.T) {
	cfg := DefaultConfig()
	mockSandbox := NewMockSandboxManager()
	coord := NewCoordinator(cfg, mockSandbox)

	// Add project
	projectDir := "/test/project"
	coord.mu.Lock()
	coord.projects[projectDir] = &ProjectExecution{
		ProjectDir: projectDir,
		Status:     StatusQueued,
	}
	coord.mu.Unlock()

	// Update to running
	coord.updateProjectStatus(projectDir, StatusRunning, nil)

	status := coord.Status()
	if status[projectDir].Status != StatusRunning {
		t.Errorf("Expected StatusRunning, got %v", status[projectDir].Status)
	}

	// Update to completed
	coord.updateProjectStatus(projectDir, StatusCompleted, nil)

	status = coord.Status()
	if status[projectDir].Status != StatusCompleted {
		t.Errorf("Expected StatusCompleted, got %v", status[projectDir].Status)
	}

	if status[projectDir].CompletedAt.IsZero() {
		t.Error("CompletedAt should be set")
	}
}

func TestGetOrCreateSandbox_Create(t *testing.T) {
	cfg := DefaultConfig()
	mockSandbox := NewMockSandboxManager()
	coord := NewCoordinator(cfg, mockSandbox)

	sb, err := coord.getOrCreateSandbox("/test/oss-wp12")
	if err != nil {
		t.Fatalf("getOrCreateSandbox failed: %v", err)
	}

	if sb == nil {
		t.Fatal("Expected sandbox, got nil")
		return
	}

	if sb.Name != "oss-wp12" {
		t.Errorf("Expected name=oss-wp12, got %s", sb.Name)
	}
}

func TestGetOrCreateSandbox_Reuse(t *testing.T) {
	cfg := DefaultConfig()
	mockSandbox := NewMockSandboxManager()
	coord := NewCoordinator(cfg, mockSandbox)

	// Create sandbox
	sb1, err := coord.getOrCreateSandbox("/test/oss-wp12")
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	// Try to create again (should reuse)
	sb2, err := coord.getOrCreateSandbox("/test/oss-wp12")
	if err != nil {
		t.Fatalf("Second create failed: %v", err)
	}

	if sb1.ID != sb2.ID {
		t.Errorf("Expected same sandbox ID, got %s and %s", sb1.ID, sb2.ID)
	}
}

func TestGetOrCreateSandbox_Fallback(t *testing.T) {
	cfg := DefaultConfig()
	mockSandbox := NewMockSandboxManager()
	mockSandbox.createErr = fmt.Errorf("sandbox creation failed")
	coord := NewCoordinator(cfg, mockSandbox)

	// Should return error (not nil sandbox)
	_, err := coord.getOrCreateSandbox("/test/oss-wp12")
	if err == nil {
		t.Error("Expected error when sandbox creation fails")
	}
}

func TestCoordinatorConcurrencyLimit(t *testing.T) {
	// This test is hard to verify deterministically without mocking exec
	// In integration tests, we'll verify actual process concurrency
	cfg := Config{
		MaxConcurrent:   2,
		MonitorInterval: 1 * time.Second,
	}
	mockSandbox := NewMockSandboxManager()
	coord := NewCoordinator(cfg, mockSandbox)

	if cap(coord.semaphore) != 2 {
		t.Errorf("Expected semaphore capacity=2, got %d", cap(coord.semaphore))
	}
}

func TestMonitorGetStatus(t *testing.T) {
	monitor := NewMonitor(1*time.Second, "/tmp/test-logs")

	// No status initially
	_, err := monitor.GetStatus("/test/project")
	if err == nil {
		t.Error("Expected error for unknown project")
	}

	// Add status
	monitor.statusPoller.mu.Lock()
	monitor.statusPoller.projects["/test/project"] = &ProjectStatus{
		ProjectDir:   "/test/project",
		CurrentPhase: "BUILD",
		Progress:     50,
	}
	monitor.statusPoller.mu.Unlock()

	status, err := monitor.GetStatus("/test/project")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.CurrentPhase != "BUILD" {
		t.Errorf("Expected phase=BUILD, got %s", status.CurrentPhase)
	}

	if status.Progress != 50 {
		t.Errorf("Expected progress=50, got %d", status.Progress)
	}
}

func TestEventSubscriptionAndEmit(t *testing.T) {
	monitor := NewMonitor(1*time.Second, "/tmp/test-logs")

	received := make(chan Event, 1)
	monitor.Subscribe(EventProjectStarted, func(e Event) {
		received <- e
	})

	event := Event{
		Type:       EventProjectStarted,
		ProjectDir: "/test/project",
		Timestamp:  time.Now(),
	}

	monitor.Emit(event)

	// Wait for event with timeout
	select {
	case e := <-received:
		if e.Type != EventProjectStarted {
			t.Errorf("Expected EventProjectStarted, got %v", e.Type)
		}
		if e.ProjectDir != "/test/project" {
			t.Errorf("Expected /test/project, got %s", e.ProjectDir)
		}
	case <-time.After(1 * time.Second):
		t.Error("Event not received within timeout")
	}
}

func TestParseWayfinderStatus(t *testing.T) {
	content := `---
schema_version: "2.0"
project_name: test
project_type: feature
risk_level: M
current_waypoint: BUILD
status: in-progress
created_at: 2026-07-20T12:00:00Z
updated_at: 2026-07-20T12:30:00Z
waypoint_history:
  - {name: CHARTER, status: completed, started_at: 2026-07-20T12:00:00Z, completed_at: 2026-07-20T12:01:00Z}
  - {name: PROBLEM, status: completed, started_at: 2026-07-20T12:01:00Z, completed_at: 2026-07-20T12:02:00Z}
  - {name: RESEARCH, status: completed, started_at: 2026-07-20T12:02:00Z, completed_at: 2026-07-20T12:03:00Z}
  - {name: DESIGN, status: completed, started_at: 2026-07-20T12:03:00Z, completed_at: 2026-07-20T12:04:00Z}
  - {name: SPEC, status: completed, started_at: 2026-07-20T12:04:00Z, completed_at: 2026-07-20T12:05:00Z}
  - {name: PLAN, status: completed, started_at: 2026-07-20T12:05:00Z, completed_at: 2026-07-20T12:06:00Z}
  - {name: SETUP, status: completed, started_at: 2026-07-20T12:06:00Z, completed_at: 2026-07-20T12:07:00Z}
---
`

	status := parseWayfinderStatus("/test/project", []byte(content))
	if status == nil {
		t.Fatal("parseWayfinderStatus rejected valid canonical status")
	}

	if status.CurrentPhase != "BUILD" {
		t.Errorf("Expected phase=BUILD, got %s", status.CurrentPhase)
	}

	if status.Progress != 77 {
		t.Errorf("Expected progress=77, got %d", status.Progress)
	}

	if status.Message != "in-progress" {
		t.Errorf("Expected message='in-progress', got %s", status.Message)
	}
}

func TestParseWayfinderStatus_Defaults(t *testing.T) {
	content := `# Some random content without status info`

	if status := parseWayfinderStatus("/test/project", []byte(content)); status != nil {
		t.Fatalf("parseWayfinderStatus accepted invalid content: %+v", status)
	}
}

func TestParseWayfinderStatus_RejectsInvalidCanonicalFields(t *testing.T) {
	content := []byte("---\nschema_version: \"1.0\"\ncurrent_waypoint: UNKNOWN\nstatus: typo\n---\n")
	if status := parseWayfinderStatus("/test/project", content); status != nil {
		t.Fatalf("parseWayfinderStatus accepted invalid canonical state: %+v", status)
	}
}

func TestStatusPollerEvictsCachedStatusAfterParseFailure(t *testing.T) {
	dir := t.TempDir()
	statusPath := filepath.Join(dir, "WAYFINDER-STATUS.md")
	valid := []byte(`---
schema_version: "2.0"
project_name: cache-eviction-test
project_type: feature
risk_level: S
current_waypoint: BUILD
status: in-progress
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
waypoint_history:
  - {name: CHARTER, status: completed, started_at: 2026-07-20T00:00:00Z, completed_at: 2026-07-20T00:01:00Z}
  - {name: PROBLEM, status: completed, started_at: 2026-07-20T00:01:00Z, completed_at: 2026-07-20T00:02:00Z}
  - {name: RESEARCH, status: completed, started_at: 2026-07-20T00:02:00Z, completed_at: 2026-07-20T00:03:00Z}
  - {name: DESIGN, status: completed, started_at: 2026-07-20T00:03:00Z, completed_at: 2026-07-20T00:04:00Z}
  - {name: SPEC, status: completed, started_at: 2026-07-20T00:04:00Z, completed_at: 2026-07-20T00:05:00Z}
  - {name: PLAN, status: completed, started_at: 2026-07-20T00:05:00Z, completed_at: 2026-07-20T00:06:00Z}
  - {name: SETUP, status: completed, started_at: 2026-07-20T00:06:00Z, completed_at: 2026-07-20T00:07:00Z}
---
`)
	if err := os.WriteFile(statusPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}

	monitor := NewMonitor(time.Second, t.TempDir())
	monitor.statusPoller.pollOnce([]string{dir})
	if _, err := monitor.GetStatus(dir); err != nil {
		t.Fatalf("GetStatus() after valid poll: %v", err)
	}

	if err := os.WriteFile(statusPath, []byte("not canonical status\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	monitor.statusPoller.pollOnce([]string{dir})
	if cached, err := monitor.GetStatus(dir); err == nil {
		t.Fatalf("GetStatus() retained stale cache after parse failure: %+v", cached)
	}
}

// TestStop_NoConcurrentDeadlock is a regression test for the deadlock where
// Stop() held c.mu across wg.Wait(). runProject goroutines need c.mu to call
// updateProjectStatus before calling wg.Done(), so holding the lock made
// wg.Wait() hang until the 10-second forced-kill timeout fired.
//
// Fix: Stop() releases c.mu before entering the wg.Wait() select loop.
// This test fails (hangs >2s) against the old code and passes quickly with the fix.
func TestStop_NoConcurrentDeadlock(t *testing.T) {
	const N = 4
	cfg := DefaultConfig()
	coord := NewCoordinator(cfg, nil)

	// Register N "running" projects and spawn goroutines that simulate runProject:
	// sleep briefly (so Stop enters its wait path first), then call updateProjectStatus
	// (which acquires c.mu) before calling wg.Done.
	for i := range N {
		dir := fmt.Sprintf("/test/deadlock-%d", i)
		coord.mu.Lock()
		coord.projects[dir] = &ProjectExecution{
			ProjectDir: dir,
			Status:     StatusRunning,
		}
		coord.mu.Unlock()

		coord.wg.Add(1)
		go func(d string) {
			defer coord.wg.Done()
			time.Sleep(10 * time.Millisecond)
			coord.updateProjectStatus(d, StatusCompleted, nil)
		}(dir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stopDone := make(chan error, 1)
	go func() { stopDone <- coord.Stop(ctx) }()

	select {
	case <-stopDone:
		// passed: Stop returned before the 2s deadline
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() deadlocked: did not return within 2s under concurrent updateProjectStatus calls")
	}
}
