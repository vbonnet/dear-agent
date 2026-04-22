// Package dockerbackend implements the manager.Backend interface using Docker
// containers as the session runtime. Each agent session runs in its own container.
package dockerbackend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
)

// Compile-time interface check.
var _ manager.Backend = (*DockerBackend)(nil)

// ContainerClient abstracts Docker operations for testability.
type ContainerClient interface {
	// CreateContainer creates a new container and returns its ID.
	CreateContainer(ctx context.Context, opts ContainerCreateOpts) (string, error)

	// StartContainer starts a stopped container.
	StartContainer(ctx context.Context, containerID string) error

	// StopContainer stops a running container with a timeout.
	StopContainer(ctx context.Context, containerID string, timeout time.Duration) error

	// RemoveContainer removes a container.
	RemoveContainer(ctx context.Context, containerID string) error

	// InspectContainer returns the current state of a container.
	InspectContainer(ctx context.Context, containerID string) (ContainerState, error)

	// ListContainers returns containers matching label filters.
	ListContainers(ctx context.Context, labels map[string]string) ([]ContainerInfo, error)

	// Exec runs a command inside a container and returns stdout.
	Exec(ctx context.Context, containerID string, cmd []string, stdin string) (string, error)
}

// ContainerCreateOpts holds options for creating a container.
type ContainerCreateOpts struct {
	Name   string
	Image  string
	Env    map[string]string
	Mounts []Mount
	Labels map[string]string
	Cmd    []string
}

// Mount describes a bind mount for the container.
type Mount struct {
	Source   string // Host path
	Target   string // Container path
	ReadOnly bool
}

// ContainerState holds the inspected state of a container.
type ContainerState struct {
	Running    bool
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
}

// ContainerInfo holds metadata about a listed container.
type ContainerInfo struct {
	ID     string
	Name   string
	Labels map[string]string
	State  string // "running", "exited", "created", etc.
}

// Config holds Docker backend configuration.
type Config struct {
	// Image is the default container image for agent sessions.
	Image string

	// StopTimeout is how long to wait for graceful container stop.
	StopTimeout time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Image:       "ghcr.io/anthropics/claude-code:latest",
		StopTimeout: 10 * time.Second,
	}
}

const (
	labelPrefix  = "agm."
	labelManaged = labelPrefix + "managed"
	labelSession = labelPrefix + "session"
	labelHarness = labelPrefix + "harness"
)

// DockerBackend manages agent sessions as Docker containers.
type DockerBackend struct {
	client ContainerClient
	config Config

	mu       sync.RWMutex
	sessions map[manager.SessionID]*sessionRecord
}

// sessionRecord tracks the mapping between session ID and container ID.
type sessionRecord struct {
	containerID string
	name        string
	harness     string
	createdAt   time.Time
}

// New creates a new DockerBackend with the given client and configuration.
func New(client ContainerClient, config Config) *DockerBackend {
	return &DockerBackend{
		client:   client,
		config:   config,
		sessions: make(map[manager.SessionID]*sessionRecord),
	}
}

// Name returns the backend identifier.
func (b *DockerBackend) Name() string { return "docker" }

// Capabilities returns what the Docker backend supports.
func (b *DockerBackend) Capabilities() manager.BackendCapabilities {
	return manager.BackendCapabilities{
		SupportsAttach:        false,
		SupportsStructuredIO:  true,
		SupportsInterrupt:     true,
		MaxConcurrentSessions: 0, // unlimited
	}
}

// --- SessionManager ---

// CreateSession creates a new Docker container for the agent session.
func (b *DockerBackend) CreateSession(ctx context.Context, config manager.SessionConfig) (manager.SessionID, error) {
	if config.Name == "" {
		return "", fmt.Errorf("session name is required")
	}

	id := manager.SessionID(config.Name)

	b.mu.Lock()
	if _, exists := b.sessions[id]; exists {
		b.mu.Unlock()
		return "", fmt.Errorf("session %q already exists", config.Name)
	}
	b.mu.Unlock()

	image := b.config.Image
	if config.Harness != "" {
		// Allow harness-specific images in the future
		_ = config.Harness
	}

	env := make(map[string]string)
	for k, v := range config.Environment {
		env[k] = v
	}

	containerName := "agm-" + config.Name

	var mounts []Mount
	if config.WorkingDirectory != "" {
		mounts = append(mounts, Mount{
			Source: config.WorkingDirectory,
			Target: "/workspace",
		})
	}

	labels := map[string]string{
		labelManaged: "true",
		labelSession: config.Name,
		labelHarness: config.Harness,
	}

	containerID, err := b.client.CreateContainer(ctx, ContainerCreateOpts{
		Name:   containerName,
		Image:  image,
		Env:    env,
		Mounts: mounts,
		Labels: labels,
		Cmd:    []string{"sleep", "infinity"}, // Keep container alive
	})
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := b.client.StartContainer(ctx, containerID); err != nil {
		// Clean up on start failure
		_ = b.client.RemoveContainer(ctx, containerID)
		return "", fmt.Errorf("start container: %w", err)
	}

	now := time.Now()
	b.mu.Lock()
	b.sessions[id] = &sessionRecord{
		containerID: containerID,
		name:        config.Name,
		harness:     config.Harness,
		createdAt:   now,
	}
	b.mu.Unlock()

	return id, nil
}

// TerminateSession stops and removes the container for the session.
func (b *DockerBackend) TerminateSession(ctx context.Context, id manager.SessionID) error {
	b.mu.Lock()
	rec, exists := b.sessions[id]
	if exists {
		delete(b.sessions, id)
	}
	b.mu.Unlock()

	if !exists {
		return nil // Idempotent
	}

	_ = b.client.StopContainer(ctx, rec.containerID, b.config.StopTimeout)
	_ = b.client.RemoveContainer(ctx, rec.containerID)
	return nil
}

// ListSessions returns sessions matching the filter criteria.
func (b *DockerBackend) ListSessions(_ context.Context, filter manager.SessionFilter) ([]manager.SessionInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var results []manager.SessionInfo
	for id, rec := range b.sessions {
		info := manager.SessionInfo{
			ID:        id,
			Name:      rec.name,
			State:     manager.StateIdle,
			CreatedAt: rec.createdAt,
			Harness:   rec.harness,
		}

		if filter.NameMatch != "" && rec.name != filter.NameMatch {
			continue
		}

		results = append(results, info)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			break
		}
	}
	return results, nil
}

// GetSession returns metadata for a single session.
func (b *DockerBackend) GetSession(_ context.Context, id manager.SessionID) (manager.SessionInfo, error) {
	b.mu.RLock()
	rec, exists := b.sessions[id]
	b.mu.RUnlock()

	if !exists {
		return manager.SessionInfo{}, fmt.Errorf("session %q not found", id)
	}

	return manager.SessionInfo{
		ID:        id,
		Name:      rec.name,
		State:     manager.StateIdle,
		CreatedAt: rec.createdAt,
		Harness:   rec.harness,
	}, nil
}

// RenameSession changes the human-readable name of a session.
func (b *DockerBackend) RenameSession(_ context.Context, id manager.SessionID, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	rec, exists := b.sessions[id]
	if !exists {
		return fmt.Errorf("session %q not found", id)
	}
	rec.name = name
	return nil
}

// --- MessageBroker ---

// SendMessage delivers a message to the agent session via docker exec.
func (b *DockerBackend) SendMessage(ctx context.Context, id manager.SessionID, message string) (manager.SendResult, error) {
	b.mu.RLock()
	rec, exists := b.sessions[id]
	b.mu.RUnlock()

	if !exists {
		return manager.SendResult{Delivered: false}, fmt.Errorf("session %q not found", id)
	}

	_, err := b.client.Exec(ctx, rec.containerID, []string{"cat"}, message+"\n")
	if err != nil {
		return manager.SendResult{Delivered: false, Error: err}, fmt.Errorf("send message: %w", err)
	}

	return manager.SendResult{Delivered: true}, nil
}

// ReadOutput returns recent output from the container.
func (b *DockerBackend) ReadOutput(ctx context.Context, id manager.SessionID, lines int) (string, error) {
	b.mu.RLock()
	rec, exists := b.sessions[id]
	b.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("session %q not found", id)
	}

	if lines <= 0 {
		lines = 30
	}

	output, err := b.client.Exec(ctx, rec.containerID,
		[]string{"tail", "-n", fmt.Sprintf("%d", lines), "/tmp/agent-output.log"}, "")
	if err != nil {
		return "", fmt.Errorf("read output: %w", err)
	}

	return output, nil
}

// Interrupt sends a signal to cancel the current operation in the container.
func (b *DockerBackend) Interrupt(ctx context.Context, id manager.SessionID) error {
	b.mu.RLock()
	rec, exists := b.sessions[id]
	b.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %q not found", id)
	}

	_, err := b.client.Exec(ctx, rec.containerID,
		[]string{"kill", "-INT", "1"}, "")
	if err != nil {
		return fmt.Errorf("interrupt session: %w", err)
	}

	return nil
}

// --- StateReader ---

// GetState returns the current state of a session based on container status.
func (b *DockerBackend) GetState(ctx context.Context, id manager.SessionID) (manager.StateResult, error) {
	b.mu.RLock()
	rec, exists := b.sessions[id]
	b.mu.RUnlock()

	if !exists {
		return manager.StateResult{
			State:      manager.StateOffline,
			Confidence: 1.0,
			Evidence:   "session not found",
		}, nil
	}

	state, err := b.client.InspectContainer(ctx, rec.containerID)
	if err != nil {
		return manager.StateResult{
			State:      manager.StateError,
			Confidence: 0.8,
			Evidence:   fmt.Sprintf("inspect failed: %v", err),
		}, nil
	}

	if !state.Running {
		return manager.StateResult{
			State:      manager.StateOffline,
			Confidence: 1.0,
			Evidence:   fmt.Sprintf("container exited with code %d", state.ExitCode),
		}, nil
	}

	return manager.StateResult{
		State:      manager.StateIdle,
		Confidence: 0.8,
		Evidence:   "container running",
	}, nil
}

// CheckDelivery determines if a session can receive input.
func (b *DockerBackend) CheckDelivery(ctx context.Context, id manager.SessionID) (manager.CanReceive, error) {
	b.mu.RLock()
	rec, exists := b.sessions[id]
	b.mu.RUnlock()

	if !exists {
		return manager.CanReceiveNotFound, nil
	}

	state, err := b.client.InspectContainer(ctx, rec.containerID)
	if err != nil {
		return manager.CanReceiveQueue, nil
	}

	if !state.Running {
		return manager.CanReceiveNo, nil
	}

	return manager.CanReceiveYes, nil
}

// HealthCheck verifies Docker is available by listing containers.
func (b *DockerBackend) HealthCheck(ctx context.Context) error {
	_, err := b.client.ListContainers(ctx, map[string]string{labelManaged: "true"})
	if err != nil {
		return fmt.Errorf("docker health check failed: %w", err)
	}
	return nil
}

// containerNameForSession returns the Docker container name for a session.
func containerNameForSession(name string) string {
	// Replace characters not valid in container names
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	return "agm-" + safe
}
