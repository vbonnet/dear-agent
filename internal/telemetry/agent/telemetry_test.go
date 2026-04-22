package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

func TestTelemetry_LogAgentLaunch(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	bus := eventbus.NewBus(nil)
	telemetry := NewTelemetry(bus, storage)
	defer telemetry.Close()

	ctx := context.Background()
	prompt := "Create a function calculateTotal() with limit 100"
	model := "claude-sonnet-4.5"

	id, err := telemetry.LogAgentLaunch(ctx, prompt, model)
	if err != nil {
		t.Fatalf("LogAgentLaunch() failed: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero launch ID")
	}

	// Verify stored in database
	filters := QueryFilters{Limit: 10}
	launches, err := telemetry.Query(ctx, filters)
	if err != nil {
		t.Fatalf("Query() failed: %v", err)
	}

	if len(launches) != 1 {
		t.Errorf("Expected 1 launch, got %d", len(launches))
	}

	if launches[0].PromptText != prompt {
		t.Errorf("PromptText = %q, want %q", launches[0].PromptText, prompt)
	}
}

func TestTelemetry_LogAgentLaunchFull(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	bus := eventbus.NewBus(nil)
	telemetry := NewTelemetry(bus, storage)
	defer telemetry.Close()

	ctx := context.Background()
	prompt := "Test prompt"
	model := "test-model"
	taskDesc := "Test task"
	sessionID := "session-123"
	parentID := "parent-456"

	id, err := telemetry.LogAgentLaunchFull(ctx, prompt, model, taskDesc, sessionID, parentID)
	if err != nil {
		t.Fatalf("LogAgentLaunchFull() failed: %v", err)
	}

	// Verify all metadata stored
	filters := QueryFilters{Limit: 10}
	launches, err := telemetry.Query(ctx, filters)
	if err != nil {
		t.Fatalf("Query() failed: %v", err)
	}

	if len(launches) != 1 {
		t.Fatalf("Expected 1 launch, got %d", len(launches))
	}

	launch := launches[0]
	if launch.ID != id {
		t.Errorf("ID = %d, want %d", launch.ID, id)
	}
	if launch.TaskDescription != taskDesc {
		t.Errorf("TaskDescription = %q, want %q", launch.TaskDescription, taskDesc)
	}
	if launch.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", launch.SessionID, sessionID)
	}
	if launch.ParentAgentID != parentID {
		t.Errorf("ParentAgentID = %q, want %q", launch.ParentAgentID, parentID)
	}
}

func TestTelemetry_LogAgentCompletion(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	telemetry := NewTelemetry(nil, storage)
	defer telemetry.Close()

	ctx := context.Background()

	// Log launch
	id, err := telemetry.LogAgentLaunch(ctx, "test prompt", "test-model")
	if err != nil {
		t.Fatalf("LogAgentLaunch() failed: %v", err)
	}

	// Log completion
	err = telemetry.LogAgentCompletion(ctx, id, "success", 1500)
	if err != nil {
		t.Fatalf("LogAgentCompletion() failed: %v", err)
	}

	// Verify outcome updated
	filters := QueryFilters{Outcome: "success", Limit: 10}
	launches, err := telemetry.Query(ctx, filters)
	if err != nil {
		t.Fatalf("Query() failed: %v", err)
	}

	if len(launches) != 1 {
		t.Errorf("Expected 1 success launch, got %d", len(launches))
	}

	if launches[0].TokensUsed != 1500 {
		t.Errorf("TokensUsed = %d, want 1500", launches[0].TokensUsed)
	}
}

func TestTelemetry_LogAgentCompletionFull(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	telemetry := NewTelemetry(nil, storage)
	defer telemetry.Close()

	ctx := context.Background()

	// Log launch
	id, _ := telemetry.LogAgentLaunch(ctx, "test prompt", "test-model")

	// Log full completion
	err := telemetry.LogAgentCompletionFull(ctx, id, "failure", 500, 2, "timeout error", 5000)
	if err != nil {
		t.Fatalf("LogAgentCompletionFull() failed: %v", err)
	}

	// Verify all completion data
	filters := QueryFilters{Outcome: "failure", Limit: 10}
	launches, err := telemetry.Query(ctx, filters)
	if err != nil {
		t.Fatalf("Query() failed: %v", err)
	}

	if len(launches) != 1 {
		t.Fatalf("Expected 1 failure launch, got %d", len(launches))
	}

	launch := launches[0]
	if launch.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", launch.RetryCount)
	}
	if launch.ErrorMessage != "timeout error" {
		t.Errorf("ErrorMessage = %q, want %q", launch.ErrorMessage, "timeout error")
	}
	if launch.DurationMs != 5000 {
		t.Errorf("DurationMs = %d, want 5000", launch.DurationMs)
	}
}

func TestTelemetry_Stats(t *testing.T) {
	storage := setupTestStorage(t)
	defer storage.Close()

	telemetry := NewTelemetry(nil, storage)
	defer telemetry.Close()

	ctx := context.Background()

	// Log test data
	id1, _ := telemetry.LogAgentLaunch(ctx, "prompt 1", "test-model")
	telemetry.LogAgentCompletion(ctx, id1, "success", 1000)

	id2, _ := telemetry.LogAgentLaunch(ctx, "prompt 2", "test-model")
	telemetry.LogAgentCompletion(ctx, id2, "success", 2000)

	id3, _ := telemetry.LogAgentLaunch(ctx, "prompt 3", "test-model")
	telemetry.LogAgentCompletion(ctx, id3, "failure", 500)

	// Get stats
	stats, err := telemetry.Stats(ctx, "test-model")
	if err != nil {
		t.Fatalf("Stats() failed: %v", err)
	}

	if stats.Total != 3 {
		t.Errorf("Total = %d, want 3", stats.Total)
	}

	if stats.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", stats.SuccessCount)
	}

	wantRate := 2.0 / 3.0
	if abs(stats.SuccessRate-wantRate) > 0.01 {
		t.Errorf("SuccessRate = %.2f, want %.2f", stats.SuccessRate, wantRate)
	}
}

func TestTelemetry_NoStorage(t *testing.T) {
	bus := eventbus.NewBus(nil)
	telemetry := NewTelemetry(bus, nil)
	defer telemetry.Close()

	ctx := context.Background()

	// Log launch (should succeed but not persist)
	id, err := telemetry.LogAgentLaunch(ctx, "test prompt", "test-model")
	if err != nil {
		t.Fatalf("LogAgentLaunch() failed: %v", err)
	}

	if id != 0 {
		t.Error("Expected ID=0 when no storage")
	}

	// Query should fail
	_, err = telemetry.Query(ctx, QueryFilters{})
	if err == nil {
		t.Error("Expected error from Query() with no storage")
	}

	// Stats should fail
	_, err = telemetry.Stats(ctx, "")
	if err == nil {
		t.Error("Expected error from Stats() with no storage")
	}
}

func TestTelemetry_NoBus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewStorageAt(dbPath)
	if err != nil {
		t.Fatalf("NewStorageAt() failed: %v", err)
	}
	defer storage.Close()

	telemetry := NewTelemetry(nil, storage)
	defer telemetry.Close()

	ctx := context.Background()

	// Should work without bus
	id, err := telemetry.LogAgentLaunch(ctx, "test prompt", "test-model")
	if err != nil {
		t.Fatalf("LogAgentLaunch() failed: %v", err)
	}

	if id == 0 {
		t.Error("Expected non-zero ID")
	}
}
