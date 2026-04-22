package simple

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

func TestSimpleFileProvider_StoreAndRetrieveMemory(t *testing.T) {
	// Setup temp storage
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()
	namespace := []string{"user", "alice", "project", "test"}

	// Store a memory
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-123",
		Type:          consolidation.Episodic,
		Namespace:     namespace,
		Content:       "Test memory content",
		Metadata:      map[string]interface{}{"source": "test"},
		Timestamp:     time.Now(),
		Importance:    0.8,
	}

	err := provider.StoreMemory(ctx, namespace, memory)
	if err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Verify file was created
	expectedPath := filepath.Join(tempDir, "user", "alice", "project", "test", "mem-123.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Memory file was not created at %s", expectedPath)
	}

	// Verify file contains valid JSON
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read memory file: %v", err)
	}

	var stored consolidation.Memory
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("Invalid JSON in memory file: %v", err)
	}

	if stored.ID != memory.ID {
		t.Errorf("Stored memory ID = %s, want %s", stored.ID, memory.ID)
	}

	// Retrieve the memory
	query := consolidation.Query{Limit: 10}
	memories, err := provider.RetrieveMemory(ctx, namespace, query)
	if err != nil {
		t.Fatalf("RetrieveMemory failed: %v", err)
	}

	if len(memories) != 1 {
		t.Fatalf("Expected 1 memory, got %d", len(memories))
	}

	if memories[0].ID != "mem-123" {
		t.Errorf("Retrieved memory ID = %s, want mem-123", memories[0].ID)
	}
}

func TestSimpleFileProvider_UpdateMemory(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()
	namespace := []string{"user", "bob"}

	// Store initial memory
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-update-test",
		Type:          consolidation.Semantic,
		Namespace:     namespace,
		Content:       "Initial content",
		Metadata:      map[string]interface{}{"version": 1},
		Timestamp:     time.Now(),
		Importance:    0.5,
	}

	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	tests := []struct {
		name    string
		updates consolidation.MemoryUpdate
		verify  func(*testing.T, consolidation.Memory)
	}{
		{
			name: "set content",
			updates: consolidation.MemoryUpdate{
				SetContent: func() *interface{} {
					var c interface{} = "Updated content"
					return &c
				}(),
			},
			verify: func(t *testing.T, m consolidation.Memory) {
				if m.Content != "Updated content" {
					t.Errorf("Content = %v, want 'Updated content'", m.Content)
				}
			},
		},
		{
			name: "append content",
			updates: consolidation.MemoryUpdate{
				AppendContent: func() *string {
					s := " - additional info"
					return &s
				}(),
			},
			verify: func(t *testing.T, m consolidation.Memory) {
				expected := "Updated content - additional info"
				if m.Content != expected {
					t.Errorf("Content = %v, want %q", m.Content, expected)
				}
			},
		},
		{
			name: "update metadata",
			updates: consolidation.MemoryUpdate{
				SetMetadata: map[string]interface{}{
					"version":  2,
					"reviewed": true,
				},
			},
			verify: func(t *testing.T, m consolidation.Memory) {
				// JSON numbers are float64
				if v, ok := m.Metadata["version"].(float64); !ok || v != 2.0 {
					t.Errorf("Metadata[version] = %v, want 2", m.Metadata["version"])
				}
				if m.Metadata["reviewed"] != true {
					t.Errorf("Metadata[reviewed] = %v, want true", m.Metadata["reviewed"])
				}
			},
		},
		{
			name: "update importance",
			updates: consolidation.MemoryUpdate{
				SetImportance: func() *float64 {
					i := 0.95
					return &i
				}(),
			},
			verify: func(t *testing.T, m consolidation.Memory) {
				if m.Importance != 0.95 {
					t.Errorf("Importance = %f, want 0.95", m.Importance)
				}
			},
		},
		{
			name: "change type",
			updates: consolidation.MemoryUpdate{
				SetType: func() *consolidation.MemoryType {
					typ := consolidation.Episodic
					return &typ
				}(),
			},
			verify: func(t *testing.T, m consolidation.Memory) {
				if m.Type != consolidation.Episodic {
					t.Errorf("Type = %s, want episodic", m.Type)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply update
			err := provider.UpdateMemory(ctx, namespace, "mem-update-test", tt.updates)
			if err != nil {
				t.Fatalf("UpdateMemory failed: %v", err)
			}

			// Retrieve and verify
			query := consolidation.Query{Limit: 10}
			memories, err := provider.RetrieveMemory(ctx, namespace, query)
			if err != nil {
				t.Fatalf("RetrieveMemory failed: %v", err)
			}

			if len(memories) != 1 {
				t.Fatalf("Expected 1 memory, got %d", len(memories))
			}

			tt.verify(t, memories[0])
		})
	}
}

func TestSimpleFileProvider_UpdateMemory_NonexistentMemory(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()
	namespace := []string{"user", "charlie"}

	updates := consolidation.MemoryUpdate{
		SetContent: func() *interface{} {
			var c interface{} = "new content"
			return &c
		}(),
	}

	err := provider.UpdateMemory(ctx, namespace, "nonexistent", updates)
	if err == nil {
		t.Fatal("Expected error for nonexistent memory, got nil")
	}

	// Should return ErrNotFound
	if !errors.Is(err, consolidation.ErrNotFound) {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestSimpleFileProvider_UpdateMemory_AppendNonString(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()
	namespace := []string{"user", "dave"}

	// Store memory with non-string content
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-struct",
		Type:          consolidation.Semantic,
		Namespace:     namespace,
		Content:       map[string]interface{}{"key": "value"}, // Struct, not string
		Timestamp:     time.Now(),
	}

	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Try to append to non-string content
	updates := consolidation.MemoryUpdate{
		AppendContent: func() *string {
			s := " - append"
			return &s
		}(),
	}

	err := provider.UpdateMemory(ctx, namespace, "mem-struct", updates)
	if err == nil {
		t.Fatal("Expected error for appending to non-string content, got nil")
	}

	if !strings.Contains(err.Error(), "cannot append to non-string content") {
		t.Errorf("Expected 'cannot append to non-string content' error, got: %v", err)
	}
}

func TestSimpleFileProvider_DeleteMemory(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()
	namespace := []string{"user", "eve"}

	// Store a memory
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "mem-delete-test",
		Type:          consolidation.Working,
		Namespace:     namespace,
		Content:       "To be deleted",
		Timestamp:     time.Now(),
	}

	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		t.Fatalf("StoreMemory failed: %v", err)
	}

	// Delete the memory
	err := provider.DeleteMemory(ctx, namespace, "mem-delete-test")
	if err != nil {
		t.Fatalf("DeleteMemory failed: %v", err)
	}

	// Verify file was deleted
	memoryPath := provider.getMemoryPath(namespace, "mem-delete-test")
	if _, err := os.Stat(memoryPath); !os.IsNotExist(err) {
		t.Error("Memory file still exists after deletion")
	}

	// Verify retrieval returns empty
	query := consolidation.Query{Limit: 10}
	memories, err := provider.RetrieveMemory(ctx, namespace, query)
	if err != nil {
		t.Fatalf("RetrieveMemory failed: %v", err)
	}

	if len(memories) != 0 {
		t.Errorf("Expected 0 memories after deletion, got %d", len(memories))
	}

	// Try to delete again (should return ErrNotFound)
	err = provider.DeleteMemory(ctx, namespace, "mem-delete-test")
	if !errors.Is(err, consolidation.ErrNotFound) {
		t.Errorf("Expected ErrNotFound for second delete, got %v", err)
	}
}

func TestSimpleFileProvider_RetrieveMemory_Filtering(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()
	namespace := []string{"user", "frank"}

	// Store multiple memories with different attributes
	now := time.Now()
	memories := []consolidation.Memory{
		{
			SchemaVersion: "1.0",
			ID:            "mem-1",
			Type:          consolidation.Episodic,
			Namespace:     namespace,
			Content:       "Episodic memory with authentication",
			Timestamp:     now.Add(-2 * time.Hour),
			Importance:    0.9,
		},
		{
			SchemaVersion: "1.0",
			ID:            "mem-2",
			Type:          consolidation.Semantic,
			Namespace:     namespace,
			Content:       "Semantic memory about testing",
			Timestamp:     now.Add(-1 * time.Hour),
			Importance:    0.5,
		},
		{
			SchemaVersion: "1.0",
			ID:            "mem-3",
			Type:          consolidation.Episodic,
			Namespace:     namespace,
			Content:       "Another episodic memory",
			Timestamp:     now,
			Importance:    0.7,
		},
	}

	for _, mem := range memories {
		if err := provider.StoreMemory(ctx, namespace, mem); err != nil {
			t.Fatalf("StoreMemory failed: %v", err)
		}
	}

	tests := []struct {
		name      string
		query     consolidation.Query
		wantCount int
		wantIDs   []string
	}{
		{
			name:      "retrieve all",
			query:     consolidation.Query{Limit: 10},
			wantCount: 3,
			wantIDs:   []string{"mem-3", "mem-2", "mem-1"}, // Sorted by time desc
		},
		{
			name:      "filter by type",
			query:     consolidation.Query{Type: consolidation.Episodic, Limit: 10},
			wantCount: 2,
			wantIDs:   []string{"mem-3", "mem-1"},
		},
		{
			name:      "filter by importance",
			query:     consolidation.Query{MinImportance: 0.8, Limit: 10},
			wantCount: 1,
			wantIDs:   []string{"mem-1"},
		},
		{
			name:      "filter by text",
			query:     consolidation.Query{Text: "authentication", Limit: 10},
			wantCount: 1,
			wantIDs:   []string{"mem-1"},
		},
		{
			name:      "apply limit",
			query:     consolidation.Query{Limit: 2},
			wantCount: 2,
			wantIDs:   []string{"mem-3", "mem-2"}, // Most recent 2
		},
		{
			name: "filter by time range",
			query: consolidation.Query{
				TimeRange: &consolidation.TimeRange{
					Start: now.Add(-90 * time.Minute),
					End:   now.Add(-30 * time.Minute),
				},
				Limit: 10,
			},
			wantCount: 1,
			wantIDs:   []string{"mem-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := provider.RetrieveMemory(ctx, namespace, tt.query)
			if err != nil {
				t.Fatalf("RetrieveMemory failed: %v", err)
			}

			if len(retrieved) != tt.wantCount {
				t.Errorf("Retrieved %d memories, want %d", len(retrieved), tt.wantCount)
			}

			for i, wantID := range tt.wantIDs {
				if i >= len(retrieved) {
					break
				}
				if retrieved[i].ID != wantID {
					t.Errorf("Memory[%d].ID = %s, want %s", i, retrieved[i].ID, wantID)
				}
			}
		})
	}
}

func TestSimpleFileProvider_RetrieveMemory_NonexistentNamespace(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()

	query := consolidation.Query{Limit: 10}
	memories, err := provider.RetrieveMemory(ctx, []string{"nonexistent", "namespace"}, query)

	if err != nil {
		t.Fatalf("RetrieveMemory failed: %v", err)
	}

	if len(memories) != 0 {
		t.Errorf("Expected empty result for nonexistent namespace, got %d memories", len(memories))
	}
}

func TestSimpleFileProvider_InvalidNamespace(t *testing.T) {
	tempDir := t.TempDir()
	provider := &SimpleFileProvider{storagePath: tempDir}
	ctx := context.Background()

	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            "test",
		Type:          consolidation.Episodic,
		Content:       "test",
		Timestamp:     time.Now(),
	}

	tests := []struct {
		name      string
		namespace []string
	}{
		{"empty namespace", []string{}},
		{"namespace with empty part", []string{"user", "", "project"}},
		{"namespace with dot", []string{"user", ".", "project"}},
		{"namespace with dot-dot", []string{"user", "..", "project"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.StoreMemory(ctx, tt.namespace, memory)
			if !errors.Is(err, consolidation.ErrInvalidNamespace) {
				t.Errorf("Expected ErrInvalidNamespace, got %v", err)
			}
		})
	}
}
