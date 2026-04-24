package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
	"github.com/vbonnet/dear-agent/engram/internal/providers/simple"
)

// TestMemoryCLI_EndToEnd tests the complete CLI workflow: store -> retrieve -> update -> delete
func TestMemoryCLI_EndToEnd(t *testing.T) {
	// Setup temp directory and provider for testing
	tmpDir := t.TempDir()
	ctx := context.Background()
	provider := setupMemoryProvider(t, tmpDir)
	defer provider.Close(ctx)

	namespace := []string{"test", "user"}
	memoryID := "mem-" + uuid.New().String()

	t.Run("store memory", func(t *testing.T) {
		memory := consolidation.Memory{
			SchemaVersion: "1.0",
			ID:            memoryID,
			Type:          consolidation.Episodic,
			Namespace:     namespace,
			Content:       "Test memory content",
			Importance:    0.8,
		}

		err := provider.StoreMemory(ctx, namespace, memory)
		if err != nil {
			t.Errorf("Failed to store memory: %v", err)
		}
	})

	t.Run("retrieve memory", func(t *testing.T) {
		query := consolidation.Query{
			Type:  consolidation.Episodic,
			Limit: 10,
		}

		memories := retrieveMemories(t, provider, namespace, query)

		if len(memories) == 0 {
			t.Fatal("Expected at least one memory, got none")
		}

		memory, found := findMemoryByID(memories, memoryID)
		if !found {
			t.Fatalf("Memory %s not found in retrieved memories", memoryID)
		}

		if memory.Content != "Test memory content" {
			t.Errorf("Content = %v, want %v", memory.Content, "Test memory content")
		}
		if memory.Importance != 0.8 {
			t.Errorf("Importance = %v, want %v", memory.Importance, 0.8)
		}
	})

	t.Run("update memory", func(t *testing.T) {
		appendText := " - Updated"
		updates := consolidation.MemoryUpdate{
			AppendContent: &appendText,
		}

		err := provider.UpdateMemory(ctx, namespace, memoryID, updates)
		if err != nil {
			t.Errorf("Failed to update memory: %v", err)
		}

		// Verify update
		query := consolidation.Query{Limit: 10}
		memories := retrieveMemories(t, provider, namespace, query)

		memory, found := findMemoryByID(memories, memoryID)
		if !found {
			t.Fatalf("Memory %s not found after update", memoryID)
		}

		expected := "Test memory content - Updated"
		if memory.Content != expected {
			t.Errorf("Updated content = %v, want %v", memory.Content, expected)
		}
	})

	t.Run("delete memory", func(t *testing.T) {
		err := provider.DeleteMemory(ctx, namespace, memoryID)
		if err != nil {
			t.Errorf("Failed to delete memory: %v", err)
		}

		// Verify deletion
		query := consolidation.Query{Limit: 10}
		memories := retrieveMemories(t, provider, namespace, query)
		assertMemoryNotFound(t, memories, memoryID)
	})
}

// TestFormatStoreOutput tests the store output formatting
func TestFormatStoreOutput(t *testing.T) {
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-test-123",
		Type:          consolidation.Episodic,
		Namespace:     []string{"user", "alice"},
		Content:       "Test content",
		Importance:    0.9,
		Metadata: map[string]interface{}{
			"source": "test",
		},
	}

	tests := []struct {
		name       string
		format     string
		wantErr    bool
		wantSubstr string
	}{
		{
			name:       "json format",
			format:     "json",
			wantErr:    false,
			wantSubstr: `"id": "mem-test-123"`,
		},
		{
			name:       "text format",
			format:     "text",
			wantErr:    false,
			wantSubstr: "Memory stored successfully",
		},
		{
			name:    "invalid format",
			format:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := formatStoreOutput(memory, tt.format)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			if (err != nil) != tt.wantErr {
				t.Errorf("formatStoreOutput() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && !strings.Contains(output, tt.wantSubstr) {
				t.Errorf("Output does not contain %q\nGot: %s", tt.wantSubstr, output)
			}
		})
	}
}

// TestFormatRetrieveOutput tests the retrieve output formatting
func TestFormatRetrieveOutput(t *testing.T) {
	memories := []consolidation.Memory{
		{
			ID:         "mem-1",
			Type:       consolidation.Episodic,
			Namespace:  []string{"user", "alice"},
			Content:    "First memory",
			Importance: 0.8,
		},
		{
			ID:        "mem-2",
			Type:      consolidation.Semantic,
			Namespace: []string{"user", "alice"},
			Content:   "Second memory",
		},
	}

	tests := []struct {
		name       string
		format     string
		wantErr    bool
		wantSubstr string
	}{
		{
			name:       "json format",
			format:     "json",
			wantErr:    false,
			wantSubstr: `"id": "mem-1"`,
		},
		{
			name:       "text format",
			format:     "text",
			wantErr:    false,
			wantSubstr: "Memory 1/2",
		},
		{
			name:       "table format",
			format:     "table",
			wantErr:    false,
			wantSubstr: "Total: 2 memories",
		},
		{
			name:    "invalid format",
			format:  "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := formatRetrieveOutput(memories, tt.format)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			if (err != nil) != tt.wantErr {
				t.Errorf("formatRetrieveOutput() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && !strings.Contains(output, tt.wantSubstr) {
				t.Errorf("Output does not contain %q\nGot: %s", tt.wantSubstr, output)
			}
		})
	}
}

// TestLoadMemoryProvider tests provider loading and initialization
func TestLoadMemoryProvider(t *testing.T) {
	ctx := context.Background()

	// Save original values
	origProvider := memoryProvider
	origConfig := memoryConfig
	defer func() {
		memoryProvider = origProvider
		memoryConfig = origConfig
	}()

	t.Run("load simple provider", func(t *testing.T) {
		// Create test directory under allowed path
		tmpDir := filepath.Join("/tmp/engram-memory-test", t.Name())
		os.MkdirAll(tmpDir, 0755)
		defer os.RemoveAll(tmpDir)

		memoryProvider = "simple"
		memoryConfig = tmpDir

		// Register provider
		consolidation.Register("simple", simple.NewProvider)

		provider, err := loadMemoryProvider(ctx)
		if err != nil {
			t.Fatalf("Failed to load provider: %v", err)
		}
		defer provider.Close(ctx)

		// Test health check
		if err := provider.HealthCheck(ctx); err != nil {
			t.Errorf("Health check failed: %v", err)
		}
	})

	t.Run("unsupported provider type", func(t *testing.T) {
		memoryProvider = "unsupported"
		memoryConfig = ""

		_, err := loadMemoryProvider(ctx)
		if err == nil {
			t.Error("Expected error for unsupported provider, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported provider type") {
			t.Errorf("Error message = %v, want 'unsupported provider type'", err)
		}
	})
}

// TestMemoryWorkflow_WithMetadata tests complete workflow with metadata
func TestMemoryWorkflow_WithMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	provider := setupMemoryProvider(t, tmpDir)
	defer provider.Close(ctx)

	namespace := []string{"project", "myapp"}
	memoryID := "mem-with-metadata"

	// Store with metadata
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            memoryID,
		Type:          consolidation.Semantic,
		Namespace:     namespace,
		Content:       "API uses REST architecture",
		Importance:    0.95,
		Metadata: map[string]interface{}{
			"source": "architecture-review",
			"phase":  "D3",
			"tags":   []string{"api", "rest", "architecture"},
		},
	}
	storeMemory(t, provider, namespace, memory)

	// Retrieve and verify metadata
	query := consolidation.Query{Limit: 10}
	memories := retrieveMemories(t, provider, namespace, query)

	retrieved, found := findMemoryByID(memories, memoryID)
	if !found {
		t.Fatal("Memory with metadata not found")
	}

	assertMetadataField(t, retrieved, "source", "architecture-review")
	assertMetadataField(t, retrieved, "phase", "D3")

	// Update metadata
	newMetadata := map[string]interface{}{
		"reviewed": true,
		"reviewer": "alice",
	}
	updateMemoryMetadata(t, provider, namespace, memoryID, newMetadata)

	// Verify merged metadata
	memories = retrieveMemories(t, provider, namespace, query)
	updated, _ := findMemoryByID(memories, memoryID)

	// Original metadata should be preserved
	assertMetadataField(t, updated, "source", "architecture-review")
	// New metadata should be added
	assertMetadataField(t, updated, "reviewed", true)
}

// TestNamespaceIsolation verifies memories are properly isolated by namespace
func TestNamespaceIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	consolidation.Register("simple", simple.NewProvider)
	provider, err := consolidation.Load(config)
	if err != nil {
		t.Fatalf("Failed to load provider: %v", err)
	}
	defer provider.Close(ctx)

	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Failed to initialize provider: %v", err)
	}

	// Store memories in different namespaces
	namespaces := [][]string{
		{"user", "alice"},
		{"user", "bob"},
		{"project", "app1"},
	}

	for i, ns := range namespaces {
		memory := consolidation.Memory{
			SchemaVersion: "1.0",
			ID:            "mem-" + string(rune('a'+i)),
			Type:          consolidation.Episodic,
			Namespace:     ns,
			Content:       "Memory in namespace " + strings.Join(ns, "/"),
		}
		if err := provider.StoreMemory(ctx, ns, memory); err != nil {
			t.Fatalf("Failed to store memory in namespace %v: %v", ns, err)
		}
	}

	// Verify isolation: each namespace should only see its own memories
	for _, ns := range namespaces {
		query := consolidation.Query{Limit: 100}
		memories, err := provider.RetrieveMemory(ctx, ns, query)
		if err != nil {
			t.Errorf("Failed to retrieve from namespace %v: %v", ns, err)
			continue
		}

		if len(memories) != 1 {
			t.Errorf("Namespace %v: expected 1 memory, got %d", ns, len(memories))
		}

		if len(memories) > 0 {
			expectedContent := "Memory in namespace " + strings.Join(ns, "/")
			if memories[0].Content != expectedContent {
				t.Errorf("Wrong memory in namespace %v: got %v, want %v",
					ns, memories[0].Content, expectedContent)
			}
		}
	}
}

// Test helper functions for TestMemoryCLI_EndToEnd

// setupMemoryProvider creates and initializes a memory provider for testing
func setupMemoryProvider(t *testing.T, tmpDir string) consolidation.Provider {
	t.Helper()
	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": tmpDir,
		},
	}

	consolidation.Register("simple", simple.NewProvider)

	provider, err := consolidation.Load(config)
	if err != nil {
		t.Fatalf("Failed to load provider: %v", err)
	}

	ctx := context.Background()
	if err := provider.Initialize(ctx, config); err != nil {
		t.Fatalf("Failed to initialize provider: %v", err)
	}

	return provider
}

// retrieveMemories retrieves memories from provider with given query
func retrieveMemories(t *testing.T, provider consolidation.Provider, namespace []string, query consolidation.Query) []consolidation.Memory {
	t.Helper()
	ctx := context.Background()
	memories, err := provider.RetrieveMemory(ctx, namespace, query)
	if err != nil {
		t.Fatalf("Failed to retrieve memory: %v", err)
	}
	return memories
}

// findMemoryByID finds a memory with the specified ID in the results
func findMemoryByID(memories []consolidation.Memory, memoryID string) (consolidation.Memory, bool) {
	for _, m := range memories {
		if m.ID == memoryID {
			return m, true
		}
	}
	return consolidation.Memory{}, false
}

// assertMemoryNotFound verifies that a memory with the given ID is not in results
func assertMemoryNotFound(t *testing.T, memories []consolidation.Memory, memoryID string) {
	t.Helper()
	for _, m := range memories {
		if m.ID == memoryID {
			t.Errorf("Memory %s still exists (should be deleted)", memoryID)
		}
	}
}

// storeMemory stores a memory in the provider
func storeMemory(t *testing.T, provider consolidation.Provider, namespace []string, memory consolidation.Memory) {
	t.Helper()
	ctx := context.Background()
	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		t.Fatalf("Failed to store memory: %v", err)
	}
}

// assertMetadataField verifies a specific metadata field value
func assertMetadataField(t *testing.T, memory consolidation.Memory, key string, expectedValue interface{}) {
	t.Helper()
	actualValue := memory.Metadata[key]
	if actualValue != expectedValue {
		t.Errorf("Metadata %s = %v, want %v", key, actualValue, expectedValue)
	}
}

// updateMemoryMetadata updates memory metadata in the provider
func updateMemoryMetadata(t *testing.T, provider consolidation.Provider, namespace []string, memoryID string, newMetadata map[string]interface{}) {
	t.Helper()
	ctx := context.Background()
	updates := consolidation.MemoryUpdate{
		SetMetadata: newMetadata,
	}
	if err := provider.UpdateMemory(ctx, namespace, memoryID, updates); err != nil {
		t.Fatalf("Failed to update metadata: %v", err)
	}
}
