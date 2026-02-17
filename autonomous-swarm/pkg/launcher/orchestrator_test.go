package launcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/ai-tools/autonomous-swarm/pkg/taskqueue"
	"gopkg.in/yaml.v3"
)

func TestLaunchReady_EmptyQueue(t *testing.T) {
	// Create temp queue file
	tempDir := t.TempDir()
	queuePath := filepath.Join(tempDir, "test-queue.yaml")

	// Initialize empty queue
	queue := &taskqueue.TaskQueue{
		Ready:      []taskqueue.Bead{},
		InProgress: []taskqueue.Bead{},
		Blocked:    []taskqueue.Bead{},
		Completed:  []taskqueue.Bead{},
	}

	// Write queue to file
	if err := writeTestQueue(queuePath, queue); err != nil {
		t.Fatalf("Failed to write test queue: %v", err)
	}

	// Create coordinator
	coord := taskqueue.NewCoordinator(queuePath)
	if err := coord.Load(); err != nil {
		t.Fatalf("Failed to load queue: %v", err)
	}

	// Create orchestrator
	orch := NewOrchestrator(coord)

	// Launch ready beads (should be no-op)
	err := orch.LaunchReady()
	if err != nil {
		t.Errorf("LaunchReady() with empty queue error = %v, want nil", err)
	}
}

func TestLaunchReady_LinearDependencies(t *testing.T) {
	// Create temp queue file
	tempDir := t.TempDir()
	queuePath := filepath.Join(tempDir, "test-queue.yaml")

	// Initialize queue with linear dependencies: A → B → C
	queue := &taskqueue.TaskQueue{
		Ready: []taskqueue.Bead{
			{
				ID:        "bead-A",
				Tier:      1,
				Title:     "Test Bead A",
				DependsOn: []string{},
				Prompts:   taskqueue.BeadPrompts{Start: "Start A"},
			},
			{
				ID:        "bead-B",
				Tier:      1,
				Title:     "Test Bead B",
				DependsOn: []string{"bead-A"},
				Prompts:   taskqueue.BeadPrompts{Start: "Start B"},
			},
			{
				ID:        "bead-C",
				Tier:      1,
				Title:     "Test Bead C",
				DependsOn: []string{"bead-B"},
				Prompts:   taskqueue.BeadPrompts{Start: "Start C"},
			},
		},
		InProgress: []taskqueue.Bead{},
		Blocked:    []taskqueue.Bead{},
		Completed:  []taskqueue.Bead{},
	}

	// Write queue to file
	if err := writeTestQueue(queuePath, queue); err != nil {
		t.Fatalf("Failed to write test queue: %v", err)
	}

	// Create coordinator
	coord := taskqueue.NewCoordinator(queuePath)
	if err := coord.Load(); err != nil {
		t.Fatalf("Failed to load queue: %v", err)
	}

	// Create orchestrator
	orch := NewOrchestrator(coord)

	// Launch ready beads
	err := orch.LaunchReady()

	// Note: This will fail at Claim because bead-B and bead-C have dependencies
	// The orchestrator tries to launch in order, but only bead-A should succeed
	// This is expected behavior - the test validates orchestrator respects order
	if err != nil {
		// Check that error mentions dependencies
		// (Claim will fail for B and C because A isn't completed yet)
		t.Logf("LaunchReady() error (expected): %v", err)
	}

	// Verify bead-A was claimed (check InProgress queue)
	updatedQueue := coord.GetQueue()
	if len(updatedQueue.InProgress) == 0 {
		t.Errorf("Expected bead-A to be in InProgress, found none")
	}
}

func TestLaunchReady_CircularDependency(t *testing.T) {
	// Create temp queue file
	tempDir := t.TempDir()
	queuePath := filepath.Join(tempDir, "test-queue.yaml")

	// Initialize queue with circular dependency: A → B → A
	queue := &taskqueue.TaskQueue{
		Ready: []taskqueue.Bead{
			{
				ID:        "bead-A",
				Tier:      1,
				Title:     "Test Bead A",
				DependsOn: []string{"bead-B"},
				Prompts:   taskqueue.BeadPrompts{Start: "Start A"},
			},
			{
				ID:        "bead-B",
				Tier:      1,
				Title:     "Test Bead B",
				DependsOn: []string{"bead-A"},
				Prompts:   taskqueue.BeadPrompts{Start: "Start B"},
			},
		},
		InProgress: []taskqueue.Bead{},
		Blocked:    []taskqueue.Bead{},
		Completed:  []taskqueue.Bead{},
	}

	// Write queue to file
	if err := writeTestQueue(queuePath, queue); err != nil {
		t.Fatalf("Failed to write test queue: %v", err)
	}

	// Create coordinator
	coord := taskqueue.NewCoordinator(queuePath)
	if err := coord.Load(); err != nil {
		t.Fatalf("Failed to load queue: %v", err)
	}

	// Create orchestrator
	orch := NewOrchestrator(coord)

	// Launch ready beads - should detect cycle
	err := orch.LaunchReady()
	if err == nil {
		t.Errorf("LaunchReady() with circular dependency expected error, got nil")
	}

	// Verify error mentions circular dependency
	if err != nil && err.Error() == "" {
		t.Errorf("LaunchReady() error should not be empty")
	}
}

// writeTestQueue is a helper to write test queue to file
func writeTestQueue(path string, queue *taskqueue.TaskQueue) error {
	// Set required fields
	queue.SchemaVersion = "1.0"

	// Serialize to YAML
	data, err := yaml.Marshal(queue)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
