package coordinator

import (
	"fmt"
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
		CurrentPhase: "S8",
		Progress:     50,
	}
	monitor.statusPoller.mu.Unlock()

	status, err := monitor.GetStatus("/test/project")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.CurrentPhase != "S8" {
		t.Errorf("Expected phase=S8, got %s", status.CurrentPhase)
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
**Current Phase**: S8 - Implementation
**Status**: In progress
**Progress**: 75%
---`

	status := parseWayfinderStatus("/test/project", content)

	if status.CurrentPhase != "S8" {
		t.Errorf("Expected phase=S8, got %s", status.CurrentPhase)
	}

	if status.Progress != 75 {
		t.Errorf("Expected progress=75, got %d", status.Progress)
	}

	if status.Message != "In progress" {
		t.Errorf("Expected message='In progress', got %s", status.Message)
	}
}

func TestParseWayfinderStatus_Defaults(t *testing.T) {
	content := `# Some random content without status info`

	status := parseWayfinderStatus("/test/project", content)

	if status.CurrentPhase != "unknown" {
		t.Errorf("Expected phase=unknown, got %s", status.CurrentPhase)
	}

	if status.Progress != 0 {
		t.Errorf("Expected progress=0, got %d", status.Progress)
	}
}
