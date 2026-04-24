package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestQueryEngine_Basic(t *testing.T) {
	// Create temporary test directory
	tmpDir, err := os.MkdirTemp("", "cc-query-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test data directory
	dataDir := filepath.Join(tmpDir, ".test-component", "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}

	// Create test data files
	testData := []map[string]interface{}{
		{"id": "1", "name": "Alice", "age": 30.0},
		{"id": "2", "name": "Bob", "age": 25.0},
		{"id": "3", "name": "Charlie", "age": 35.0},
	}

	for i, data := range testData {
		dataBytes, _ := json.Marshal(data)
		fileName := fmt.Sprintf("record-%d.json", i+1)
		filePath := filepath.Join(dataDir, fileName)
		if err := os.WriteFile(filePath, dataBytes, 0644); err != nil {
			t.Fatalf("Failed to write test file: %v", err)
		}
	}

	// Create query engine with custom home dir
	engine := &QueryEngine{homeDir: tmpDir}

	// Create schema with discovery pattern
	// expandPattern prepends homeDir/.component, so pattern is relative to that
	schemaData := map[string]interface{}{
		"schemas": map[string]interface{}{
			"record": map[string]interface{}{
				"discovery_patterns": []interface{}{
					"data/*.json",
				},
			},
		},
	}

	// Test query without filter
	result, err := engine.Query(QueryParams{
		Component: "test-component",
		Schema:    "record",
		Limit:     10,
	}, schemaData)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if result.Count != 3 {
		t.Errorf("Expected 3 results, got %d", result.Count)
	}

	// Test query with filter
	result, err = engine.Query(QueryParams{
		Component: "test-component",
		Schema:    "record",
		Filter: map[string]interface{}{
			"age_gt": 28.0,
		},
		Limit: 10,
	}, schemaData)

	if err != nil {
		t.Fatalf("Query with filter failed: %v", err)
	}

	if result.Count != 2 {
		t.Errorf("Expected 2 filtered results, got %d", result.Count)
	}

	// Test query with sorting
	result, err = engine.Query(QueryParams{
		Component: "test-component",
		Schema:    "record",
		Sort: &SortConfig{
			Field: "age",
			Order: "desc",
		},
		Limit: 10,
	}, schemaData)

	if err != nil {
		t.Fatalf("Query with sorting failed: %v", err)
	}

	if result.Count != 3 {
		t.Errorf("Expected 3 sorted results, got %d", result.Count)
	}

	// Check sort order (descending by age: Charlie, Alice, Bob)
	if len(result.Data) > 0 {
		firstAge, ok := result.Data[0]["age"].(float64)
		if !ok || firstAge != 35.0 {
			t.Errorf("Expected first record to have age 35, got %v", result.Data[0]["age"])
		}
	}
}

func TestQueryEngine_NoDiscoveryPatterns(t *testing.T) {
	engine, err := NewQueryEngine()
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Schema without discovery patterns
	schemaData := map[string]interface{}{
		"schemas": map[string]interface{}{
			"record": map[string]interface{}{},
		},
	}

	_, err = engine.Query(QueryParams{
		Component: "test",
		Schema:    "record",
	}, schemaData)

	if err == nil {
		t.Error("Expected error for missing discovery patterns")
	}
}

func TestMatchesFilter(t *testing.T) {
	engine := &QueryEngine{}

	tests := []struct {
		name   string
		data   map[string]interface{}
		filter map[string]interface{}
		want   bool
	}{
		{
			name:   "exact match",
			data:   map[string]interface{}{"name": "Alice"},
			filter: map[string]interface{}{"name": "Alice"},
			want:   true,
		},
		{
			name:   "exact mismatch",
			data:   map[string]interface{}{"name": "Bob"},
			filter: map[string]interface{}{"name": "Alice"},
			want:   false,
		},
		{
			name:   "greater than - true",
			data:   map[string]interface{}{"age": 30.0},
			filter: map[string]interface{}{"age_gt": 25.0},
			want:   true,
		},
		{
			name:   "greater than - false",
			data:   map[string]interface{}{"age": 20.0},
			filter: map[string]interface{}{"age_gt": 25.0},
			want:   false,
		},
		{
			name:   "less than - true",
			data:   map[string]interface{}{"age": 20.0},
			filter: map[string]interface{}{"age_lt": 25.0},
			want:   true,
		},
		{
			name:   "less than - false",
			data:   map[string]interface{}{"age": 30.0},
			filter: map[string]interface{}{"age_lt": 25.0},
			want:   false,
		},
		{
			name:   "multiple filters - all match",
			data:   map[string]interface{}{"name": "Alice", "age": 30.0},
			filter: map[string]interface{}{"name": "Alice", "age_gt": 25.0},
			want:   true,
		},
		{
			name:   "multiple filters - one fails",
			data:   map[string]interface{}{"name": "Alice", "age": 20.0},
			filter: map[string]interface{}{"name": "Alice", "age_gt": 25.0},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.matchesFilter(tt.data, tt.filter)
			if got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}
