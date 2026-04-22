package consolidation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMemory_JSONSerialization(t *testing.T) {
	tests := []struct {
		name       string
		memory     Memory
		wantFields []string // Fields that must be present
		omitFields []string // Fields that should be omitted
	}{
		{
			name: "episodic memory with all fields",
			memory: Memory{
				SchemaVersion: "1.0",
				ID:            "mem-123",
				Type:          Episodic,
				Namespace:     []string{"user", "alice"},
				Content:       "Implemented authentication",
				Metadata:      map[string]interface{}{"source": "manual"},
				Timestamp:     time.Date(2025, 12, 15, 10, 30, 0, 0, time.UTC),
				Embedding:     []float64{0.1, 0.2, 0.3},
				Importance:    0.9,
			},
			wantFields: []string{`"schema_version":"1.0"`, `"id":"mem-123"`, `"type":"episodic"`,
				`"importance":0.9`, `"embedding"`},
			omitFields: nil,
		},
		{
			name: "memory with omitted optional fields",
			memory: Memory{
				SchemaVersion: "1.0",
				ID:            "mem-456",
				Type:          Semantic,
				Namespace:     []string{"user", "bob"},
				Content:       "Learned about Go interfaces",
				Timestamp:     time.Date(2025, 12, 15, 11, 0, 0, 0, time.UTC),
			},
			wantFields: []string{`"schema_version":"1.0"`, `"type":"semantic"`},
			omitFields: []string{"embedding", "importance", "metadata"},
		},
		{
			name: "working memory type",
			memory: Memory{
				SchemaVersion: "1.0",
				ID:            "mem-789",
				Type:          Working,
				Namespace:     []string{"system"},
				Content:       "Currently implementing tests",
				Timestamp:     time.Now(),
			},
			wantFields: []string{`"type":"working"`},
			omitFields: []string{"embedding", "importance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.memory)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			jsonStr := string(data)

			// Check required fields present
			for _, field := range tt.wantFields {
				if !strings.Contains(jsonStr, field) {
					t.Errorf("JSON missing expected field: %s\nGot: %s", field, jsonStr)
				}
			}

			// Check omitted fields absent
			for _, field := range tt.omitFields {
				if strings.Contains(jsonStr, field) {
					t.Errorf("JSON contains field that should be omitted: %s\nGot: %s", field, jsonStr)
				}
			}

			// Test round-trip
			var decoded Memory
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if decoded.ID != tt.memory.ID {
				t.Errorf("ID mismatch: got %s, want %s", decoded.ID, tt.memory.ID)
			}
			if decoded.Type != tt.memory.Type {
				t.Errorf("Type mismatch: got %s, want %s", decoded.Type, tt.memory.Type)
			}
			if decoded.SchemaVersion != tt.memory.SchemaVersion {
				t.Errorf("SchemaVersion mismatch: got %s, want %s", decoded.SchemaVersion, tt.memory.SchemaVersion)
			}
		})
	}
}

func TestMemoryUpdate_JSONSerialization(t *testing.T) {
	tests := []struct {
		name   string
		update MemoryUpdate
	}{
		{
			name: "set content only",
			update: MemoryUpdate{
				SetContent: func() *interface{} {
					var c interface{} = "new content"
					return &c
				}(),
			},
		},
		{
			name: "append content only",
			update: MemoryUpdate{
				AppendContent: func() *string {
					s := " - additional info"
					return &s
				}(),
			},
		},
		{
			name: "update metadata and importance",
			update: MemoryUpdate{
				SetMetadata: map[string]interface{}{"reviewed": true, "priority": "high"},
				SetImportance: func() *float64 {
					i := 0.95
					return &i
				}(),
			},
		},
		{
			name: "change type",
			update: MemoryUpdate{
				SetType: func() *MemoryType {
					t := Semantic
					return &t
				}(),
			},
		},
		{
			name: "multiple updates",
			update: MemoryUpdate{
				AppendContent: func() *string {
					s := " - updated"
					return &s
				}(),
				SetMetadata: map[string]interface{}{"status": "complete"},
				SetImportance: func() *float64 {
					i := 0.9
					return &i
				}(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal and unmarshal
			data, err := json.Marshal(tt.update)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var decoded MemoryUpdate
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			// Verify non-nil fields match
			if tt.update.SetContent != nil && decoded.SetContent == nil {
				t.Error("SetContent was lost in round-trip")
			}
			if tt.update.AppendContent != nil && decoded.AppendContent == nil {
				t.Error("AppendContent was lost in round-trip")
			}
			if tt.update.SetImportance != nil && decoded.SetImportance == nil {
				t.Error("SetImportance was lost in round-trip")
			}
		})
	}
}

func TestMemoryType_Constants(t *testing.T) {
	types := []MemoryType{Episodic, Semantic, Procedural, Working}

	for _, typ := range types {
		if string(typ) == "" {
			t.Errorf("MemoryType constant is empty: %v", typ)
		}
	}

	// Test uniqueness
	seen := make(map[MemoryType]bool)
	for _, typ := range types {
		if seen[typ] {
			t.Errorf("Duplicate MemoryType: %s", typ)
		}
		seen[typ] = true
	}
}
