package dockerbackend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
)

// mockDockerClient is an in-memory implementation of ContainerClient for testing.
type mockDockerClient struct {
	mu         sync.Mutex
	containers map[string]*mockContainer
	nextID     int
}

type mockContainer struct {
	id      string
	name    string
	running bool
	labels  map[string]string
	output  string
}

func newMockDockerClient() *mockDockerClient {
	return &mockDockerClient{
		containers: make(map[string]*mockContainer),
	}
}

func (m *mockDockerClient) CreateContainer(_ context.Context, opts ContainerCreateOpts) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextID++
	id := fmt.Sprintf("container-%d", m.nextID)
	m.containers[id] = &mockContainer{
		id:     id,
		name:   opts.Name,
		labels: opts.Labels,
	}
	return id, nil
}

func (m *mockDockerClient) StartContainer(_ context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[containerID]
	if !ok {
		return fmt.Errorf("container %q not found", containerID)
	}
	c.running = true
	return nil
}

func (m *mockDockerClient) StopContainer(_ context.Context, containerID string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[containerID]
	if !ok {
		return fmt.Errorf("container %q not found", containerID)
	}
	c.running = false
	return nil
}

func (m *mockDockerClient) RemoveContainer(_ context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.containers, containerID)
	return nil
}

func (m *mockDockerClient) InspectContainer(_ context.Context, containerID string) (ContainerState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[containerID]
	if !ok {
		return ContainerState{}, fmt.Errorf("container %q not found", containerID)
	}
	return ContainerState{
		Running:   c.running,
		StartedAt: time.Now(),
	}, nil
}

func (m *mockDockerClient) ListContainers(_ context.Context, labels map[string]string) ([]ContainerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var results []ContainerInfo
	for _, c := range m.containers {
		match := true
		for k, v := range labels {
			if c.labels[k] != v {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		state := "exited"
		if c.running {
			state = "running"
		}
		results = append(results, ContainerInfo{
			ID:     c.id,
			Name:   c.name,
			Labels: c.labels,
			State:  state,
		})
	}
	return results, nil
}

func (m *mockDockerClient) Exec(_ context.Context, containerID string, cmd []string, stdin string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[containerID]
	if !ok {
		return "", fmt.Errorf("container %q not found", containerID)
	}
	if !c.running {
		return "", fmt.Errorf("container %q is not running", containerID)
	}

	// Simulate: "cat" appends stdin to output, "tail" returns output, "kill" is a no-op
	if len(cmd) > 0 {
		switch cmd[0] {
		case "cat":
			c.output += stdin
			return "", nil
		case "tail":
			return c.output, nil
		case "kill":
			return "", nil
		}
	}
	return "", nil
}

// --- Tests ---

func newTestBackend() (*DockerBackend, *mockDockerClient) {
	client := newMockDockerClient()
	return New(client, DefaultConfig()), client
}

func TestDockerBackend_Name(t *testing.T) {
	b, _ := newTestBackend()
	if b.Name() != "docker" {
		t.Errorf("expected 'docker', got %q", b.Name())
	}
}

func TestDockerBackend_Capabilities(t *testing.T) {
	b, _ := newTestBackend()
	caps := b.Capabilities()
	if caps.SupportsAttach {
		t.Error("Docker backend should not support attach")
	}
	if !caps.SupportsStructuredIO {
		t.Error("Docker backend should support structured IO")
	}
	if !caps.SupportsInterrupt {
		t.Error("Docker backend should support interrupt")
	}
}

func TestDockerBackend_CreateSession(t *testing.T) {
	b, client := newTestBackend()
	ctx := context.Background()

	id, err := b.CreateSession(ctx, manager.SessionConfig{
		Name:             "test-session",
		WorkingDirectory: "/tmp/workspace",
		Harness:          "claude-code",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if id == "" {
		t.Fatal("CreateSession returned empty ID")
	}

	// Verify container was created and started
	client.mu.Lock()
	if len(client.containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(client.containers))
	}
	for _, c := range client.containers {
		if !c.running {
			t.Error("container should be running after create")
		}
	}
	client.mu.Unlock()
}

func TestDockerBackend_CreateSession_EmptyName(t *testing.T) {
	b, _ := newTestBackend()
	_, err := b.CreateSession(context.Background(), manager.SessionConfig{})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestDockerBackend_CreateSession_Duplicate(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	_, _ = b.CreateSession(ctx, manager.SessionConfig{Name: "dup"})
	_, err := b.CreateSession(ctx, manager.SessionConfig{Name: "dup"})
	if err == nil {
		t.Error("expected error for duplicate session")
	}
}

func TestDockerBackend_TerminateSession(t *testing.T) {
	b, client := newTestBackend()
	ctx := context.Background()

	id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "term-test"})
	err := b.TerminateSession(ctx, id)
	if err != nil {
		t.Fatalf("TerminateSession failed: %v", err)
	}

	// Container should be removed
	client.mu.Lock()
	if len(client.containers) != 0 {
		t.Errorf("expected 0 containers after terminate, got %d", len(client.containers))
	}
	client.mu.Unlock()

	// Session should not be found
	_, err = b.GetSession(ctx, id)
	if err == nil {
		t.Error("GetSession should fail after TerminateSession")
	}
}

func TestDockerBackend_TerminateSession_Idempotent(t *testing.T) {
	b, _ := newTestBackend()
	err := b.TerminateSession(context.Background(), "nonexistent")
	if err != nil {
		t.Errorf("TerminateSession on nonexistent should not error: %v", err)
	}
}

func TestDockerBackend_ListSessions(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	b.CreateSession(ctx, manager.SessionConfig{Name: "s1"})
	b.CreateSession(ctx, manager.SessionConfig{Name: "s2"})

	sessions, err := b.ListSessions(ctx, manager.SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestDockerBackend_ListSessions_NameFilter(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	b.CreateSession(ctx, manager.SessionConfig{Name: "alpha"})
	b.CreateSession(ctx, manager.SessionConfig{Name: "beta"})

	sessions, err := b.ListSessions(ctx, manager.SessionFilter{NameMatch: "alpha"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
}

func TestDockerBackend_ListSessions_Limit(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	b.CreateSession(ctx, manager.SessionConfig{Name: "a"})
	b.CreateSession(ctx, manager.SessionConfig{Name: "b"})
	b.CreateSession(ctx, manager.SessionConfig{Name: "c"})

	sessions, err := b.ListSessions(ctx, manager.SessionFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) > 2 {
		t.Errorf("expected at most 2 sessions, got %d", len(sessions))
	}
}

func TestDockerBackend_GetSession(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "get-test", Harness: "test"})

	info, err := b.GetSession(ctx, id)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if info.Name != "get-test" {
		t.Errorf("expected name 'get-test', got %q", info.Name)
	}
	if info.Harness != "test" {
		t.Errorf("expected harness 'test', got %q", info.Harness)
	}
}

func TestDockerBackend_GetSession_NotFound(t *testing.T) {
	b, _ := newTestBackend()
	_, err := b.GetSession(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestDockerBackend_RenameSession(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "old-name"})
	err := b.RenameSession(ctx, id, "new-name")
	if err != nil {
		t.Fatalf("RenameSession failed: %v", err)
	}

	info, _ := b.GetSession(ctx, id)
	if info.Name != "new-name" {
		t.Errorf("expected 'new-name', got %q", info.Name)
	}
}

func TestDockerBackend_SendAndReadMessage(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "msg-test"})

	result, err := b.SendMessage(ctx, id, "hello world")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if !result.Delivered {
		t.Error("SendMessage reported not delivered")
	}

	output, err := b.ReadOutput(ctx, id, 10)
	if err != nil {
		t.Fatalf("ReadOutput failed: %v", err)
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("expected output to contain 'hello world', got %q", output)
	}
}

func TestDockerBackend_SendMessage_NotFound(t *testing.T) {
	b, _ := newTestBackend()
	_, err := b.SendMessage(context.Background(), "nonexistent", "hello")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestDockerBackend_Interrupt(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "int-test"})
	err := b.Interrupt(ctx, id)
	if err != nil {
		t.Fatalf("Interrupt failed: %v", err)
	}
}

func TestDockerBackend_Interrupt_NotFound(t *testing.T) {
	b, _ := newTestBackend()
	err := b.Interrupt(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestDockerBackend_GetState_Running(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "state-test"})
	state, err := b.GetState(ctx, id)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.State != manager.StateIdle {
		t.Errorf("expected IDLE for running container, got %q", state.State)
	}
	if state.Confidence < 0.5 {
		t.Errorf("expected confidence >= 0.5, got %f", state.Confidence)
	}
}

func TestDockerBackend_GetState_Offline(t *testing.T) {
	b, _ := newTestBackend()
	state, err := b.GetState(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetState should not error for nonexistent: %v", err)
	}
	if state.State != manager.StateOffline {
		t.Errorf("expected OFFLINE, got %q", state.State)
	}
}

func TestDockerBackend_CheckDelivery_Yes(t *testing.T) {
	b, _ := newTestBackend()
	ctx := context.Background()

	id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "delivery-test"})
	canRecv, err := b.CheckDelivery(ctx, id)
	if err != nil {
		t.Fatalf("CheckDelivery failed: %v", err)
	}
	if canRecv != manager.CanReceiveYes {
		t.Errorf("expected CanReceiveYes, got %d", canRecv)
	}
}

func TestDockerBackend_CheckDelivery_NotFound(t *testing.T) {
	b, _ := newTestBackend()
	canRecv, err := b.CheckDelivery(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("CheckDelivery should not error: %v", err)
	}
	if canRecv != manager.CanReceiveNotFound {
		t.Errorf("expected CanReceiveNotFound, got %d", canRecv)
	}
}

func TestDockerBackend_HealthCheck(t *testing.T) {
	b, _ := newTestBackend()
	err := b.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

// TestDockerBackend_Compliance runs the shared compliance suite.
func TestDockerBackend_Compliance(t *testing.T) {
	b, _ := newTestBackend()
	runDockerComplianceSuite(t, b)
}

// runDockerComplianceSuite mirrors the compliance tests from manager_test
// but runs against our DockerBackend with mock client.
func runDockerComplianceSuite(t *testing.T, b manager.Backend) {
	t.Helper()
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		if b.Name() == "" {
			t.Error("Name() returned empty string")
		}
	})

	t.Run("Capabilities", func(t *testing.T) {
		caps := b.Capabilities()
		_ = caps.SupportsAttach
		_ = caps.SupportsStructuredIO
	})

	t.Run("HealthCheck", func(t *testing.T) {
		if err := b.HealthCheck(ctx); err != nil {
			t.Errorf("HealthCheck failed: %v", err)
		}
	})

	t.Run("CreateAndGetSession", func(t *testing.T) {
		id, err := b.CreateSession(ctx, manager.SessionConfig{
			Name:    "compliance-test",
			Harness: "test",
		})
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}
		if id == "" {
			t.Fatal("CreateSession returned empty ID")
		}

		info, err := b.GetSession(ctx, id)
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if info.Name != "compliance-test" {
			t.Errorf("expected name 'compliance-test', got %q", info.Name)
		}
		_ = b.TerminateSession(ctx, id)
	})

	t.Run("SendAndReadMessage", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "comp-msg-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		result, err := b.SendMessage(ctx, id, "hello world")
		if err != nil {
			t.Fatalf("SendMessage failed: %v", err)
		}
		if !result.Delivered {
			t.Error("SendMessage reported not delivered")
		}

		output, err := b.ReadOutput(ctx, id, 10)
		if err != nil {
			t.Fatalf("ReadOutput failed: %v", err)
		}
		if output == "" {
			t.Error("ReadOutput returned empty after SendMessage")
		}
	})

	t.Run("GetState", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "comp-state-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		state, err := b.GetState(ctx, id)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		if state.State == "" {
			t.Error("GetState returned empty state")
		}
	})

	t.Run("CheckDelivery", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "comp-delivery-test", Harness: "test"})
		defer b.TerminateSession(ctx, id)

		canRecv, err := b.CheckDelivery(ctx, id)
		if err != nil {
			t.Fatalf("CheckDelivery failed: %v", err)
		}
		if canRecv != manager.CanReceiveYes {
			t.Errorf("expected CanReceiveYes, got %d", canRecv)
		}
	})

	t.Run("CheckDelivery_NotFound", func(t *testing.T) {
		canRecv, err := b.CheckDelivery(ctx, "nonexistent-xyz")
		if err != nil {
			t.Fatalf("CheckDelivery should not error: %v", err)
		}
		if canRecv != manager.CanReceiveNotFound {
			t.Errorf("expected CanReceiveNotFound, got %d", canRecv)
		}
	})

	t.Run("GetState_Offline", func(t *testing.T) {
		state, err := b.GetState(ctx, "nonexistent-xyz")
		if err != nil {
			t.Fatalf("GetState should not error: %v", err)
		}
		if state.State != manager.StateOffline {
			t.Errorf("expected OFFLINE, got %q", state.State)
		}
	})

	t.Run("TerminateSession", func(t *testing.T) {
		id, _ := b.CreateSession(ctx, manager.SessionConfig{Name: "comp-term-test", Harness: "test"})
		if err := b.TerminateSession(ctx, id); err != nil {
			t.Fatalf("TerminateSession failed: %v", err)
		}
		_, err := b.GetSession(ctx, id)
		if err == nil {
			t.Error("GetSession should fail after TerminateSession")
		}
	})
}

func TestContainerNameForSession(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "agm-simple"},
		{"with spaces", "agm-with-spaces"},
		{"with.dots", "agm-with-dots"},
		{"a-b_c", "agm-a-b_c"},
	}
	for _, tt := range tests {
		got := containerNameForSession(tt.input)
		if got != tt.expected {
			t.Errorf("containerNameForSession(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
