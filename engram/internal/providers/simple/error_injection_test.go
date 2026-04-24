package simple

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

func TestSimpleFileProvider_StoreMemory_PermissionDenied(t *testing.T) {
	// Root can write to any directory regardless of permissions
	if os.Getuid() == 0 {
		t.Skip("Skipping test: requires non-root user for filesystem permission checks")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create a directory with no write permissions
	restrictedDir := filepath.Join(tmpDir, "restricted")
	if err := os.Mkdir(restrictedDir, 0555); err != nil { // Read and execute only
		t.Fatalf("Failed to create restricted directory: %v", err)
	}

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": restrictedDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "test-mem",
		Type:          consolidation.Episodic,
		Namespace:     []string{"test", "perm"},
		Content:       "Test",
	}

	// This should fail due to permission denied
	err = provider.StoreMemory(ctx, memory.Namespace, memory)
	if err == nil {
		t.Error("Expected permission denied error, got nil")
	}
}

func TestSimpleFileProvider_RetrieveMemory_CorruptedFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Store a valid memory first
	namespace := []string{"test", "corrupt"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "test-mem",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "Test",
	}

	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		t.Fatalf("StoreMemory() error = %v", err)
	}

	// Corrupt the file by writing invalid JSON
	memoryPath := filepath.Join(tmpDir, filepath.Join(namespace...), "test-mem.json")
	if err := os.WriteFile(memoryPath, []byte("invalid json{{{"), 0644); err != nil {
		t.Fatalf("Failed to corrupt file: %v", err)
	}

	// Attempt to retrieve - should handle corrupted file gracefully
	query := consolidation.Query{}
	memories, err := provider.RetrieveMemory(ctx, namespace, query)

	// We expect either an error or an empty result (depending on implementation)
	// The provider should not panic
	if err == nil && len(memories) > 0 {
		t.Error("Expected error or empty result for corrupted file")
	}
}

func TestSimpleFileProvider_UpdateMemory_NonexistentFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	namespace := []string{"test", "update"}
	var content interface{} = "new content"
	updates := consolidation.MemoryUpdate{
		SetContent: &content,
	}

	// Attempt to update a non-existent memory
	err = provider.UpdateMemory(ctx, namespace, "nonexistent-id", updates)
	if err == nil {
		t.Error("Expected error for updating non-existent memory, got nil")
	}
}

func TestSimpleFileProvider_DeleteMemory_NonexistentFile(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	namespace := []string{"test", "delete"}

	// Attempt to delete a non-existent memory
	err = provider.DeleteMemory(ctx, namespace, "nonexistent-id")
	if err == nil {
		t.Error("Expected error for deleting non-existent memory, got nil")
	}
}

func TestSimpleFileProvider_StoreMemory_EmptyContent(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	namespace := []string{"test", "empty"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "empty-mem",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "", // Empty content
	}

	// Should succeed - empty content is valid
	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		t.Errorf("StoreMemory() with empty content error = %v, want nil", err)
	}

	// Verify it was stored
	query := consolidation.Query{}
	memories, err := provider.RetrieveMemory(ctx, namespace, query)
	if err != nil {
		t.Fatalf("RetrieveMemory() error = %v", err)
	}

	if len(memories) != 1 {
		t.Errorf("Expected 1 memory, got %d", len(memories))
	}

	if memories[0].Content != "" {
		t.Errorf("Expected empty content, got %q", memories[0].Content)
	}
}

func TestSimpleFileProvider_UpdateMemory_AppendToNonString(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	namespace := []string{"test", "append"}
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "append-mem",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       123, // Non-string content
	}

	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		t.Fatalf("StoreMemory() error = %v", err)
	}

	// Attempt to append to non-string content
	appendText := " more"
	updates := consolidation.MemoryUpdate{
		AppendContent: &appendText,
	}

	err = provider.UpdateMemory(ctx, namespace, "append-mem", updates)
	if err == nil {
		t.Error("Expected error for appending to non-string content, got nil")
	}
}

func TestSimpleFileProvider_RetrieveMemory_EmptyNamespace(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	query := consolidation.Query{}
	_, err = provider.RetrieveMemory(ctx, []string{}, query)
	if err == nil {
		t.Error("Expected error for empty namespace, got nil")
	}
}

func TestSimpleFileProvider_Multiple_Concurrent_Stores(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	provider, err := NewProvider(config)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	namespace := []string{"test", "concurrent"}

	// Store multiple memories concurrently
	done := make(chan error, 5)
	for i := 0; i < 5; i++ {
		go func(id int) {
			memory := consolidation.Memory{
				SchemaVersion: "1.0",
				ID:            fmt.Sprintf("mem-%d", id),
				Type:          consolidation.Episodic,
				Namespace:     namespace,
				Content:       "concurrent test",
			}
			done <- provider.StoreMemory(ctx, namespace, memory)
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 5; i++ {
		if err := <-done; err != nil {
			t.Errorf("Concurrent store %d failed: %v", i, err)
		}
	}

	// Verify all memories were stored
	query := consolidation.Query{}
	memories, err := provider.RetrieveMemory(ctx, namespace, query)
	if err != nil {
		t.Fatalf("RetrieveMemory() error = %v", err)
	}

	if len(memories) != 5 {
		t.Errorf("Expected 5 memories after concurrent stores, got %d", len(memories))
	}
}
