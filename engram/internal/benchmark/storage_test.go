package benchmark

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewStorage(t *testing.T) {
	// Use temp directory for test database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewStorageAt(dbPath)
	if err != nil {
		t.Fatalf("NewStorageAt failed: %v", err)
	}
	defer storage.Close()

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file not created at %s", dbPath)
	}
}

func TestInsertAndQuery(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewStorageAt(dbPath)
	if err != nil {
		t.Fatalf("NewStorageAt failed: %v", err)
	}
	defer storage.Close()

	// Create test run
	qualityScore := 9.5
	costUSD := 4.80
	fileCount := 13

	run := BenchmarkRun{
		RunID:        uuid.New().String(),
		Timestamp:    time.Now(),
		Variant:      "wayfinder",
		ProjectSize:  "small",
		ProjectName:  "test-project",
		QualityScore: &qualityScore,
		CostUSD:      &costUSD,
		FileCount:    &fileCount,
		Successful:   true,
		Metadata: map[string]interface{}{
			"test_key": "test_value",
		},
	}

	// Insert run
	if err := storage.InsertRun(run); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	// Query back
	results, err := storage.Query(QueryParams{
		Variants: []string{"wayfinder"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	// Verify data
	result := results[0]
	if result.RunID != run.RunID {
		t.Errorf("RunID mismatch: got %s, want %s", result.RunID, run.RunID)
	}
	if result.Variant != "wayfinder" {
		t.Errorf("Variant mismatch: got %s, want wayfinder", result.Variant)
	}
	if result.ProjectSize != "small" {
		t.Errorf("ProjectSize mismatch: got %s, want small", result.ProjectSize)
	}
	if result.QualityScore == nil || *result.QualityScore != 9.5 {
		t.Errorf("QualityScore mismatch")
	}

	// Verify metadata round-trip
	if result.Metadata == nil {
		t.Errorf("Metadata is nil")
	} else if val, ok := result.Metadata["test_key"]; !ok || val != "test_value" {
		t.Errorf("Metadata key 'test_key' not found or incorrect")
	}
}

func TestInsertValidation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewStorageAt(dbPath)
	if err != nil {
		t.Fatalf("NewStorageAt failed: %v", err)
	}
	defer storage.Close()

	tests := []struct {
		name    string
		run     BenchmarkRun
		wantErr bool
	}{
		{
			name: "empty run_id",
			run: BenchmarkRun{
				RunID:       "",
				Timestamp:   time.Now(),
				Variant:     "raw",
				ProjectSize: "small",
			},
			wantErr: true,
		},
		{
			name: "empty variant",
			run: BenchmarkRun{
				RunID:       uuid.New().String(),
				Timestamp:   time.Now(),
				Variant:     "",
				ProjectSize: "small",
			},
			wantErr: true,
		},
		{
			name: "invalid variant",
			run: BenchmarkRun{
				RunID:       uuid.New().String(),
				Timestamp:   time.Now(),
				Variant:     "invalid",
				ProjectSize: "small",
			},
			wantErr: true,
		},
		{
			name: "valid run",
			run: BenchmarkRun{
				RunID:       uuid.New().String(),
				Timestamp:   time.Now(),
				Variant:     "engram",
				ProjectSize: "medium",
				Successful:  true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := storage.InsertRun(tt.run)
			if (err != nil) != tt.wantErr {
				t.Errorf("InsertRun() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueryFilters(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	storage, err := NewStorageAt(dbPath)
	if err != nil {
		t.Fatalf("NewStorageAt failed: %v", err)
	}
	defer storage.Close()

	// Insert multiple runs
	runs := []BenchmarkRun{
		{
			RunID:        uuid.New().String(),
			Timestamp:    time.Now(),
			Variant:      "raw",
			ProjectSize:  "small",
			QualityScore: ptrFloat64(6.9),
			Successful:   true,
		},
		{
			RunID:        uuid.New().String(),
			Timestamp:    time.Now(),
			Variant:      "engram",
			ProjectSize:  "small",
			QualityScore: ptrFloat64(8.3),
			Successful:   true,
		},
		{
			RunID:        uuid.New().String(),
			Timestamp:    time.Now(),
			Variant:      "wayfinder",
			ProjectSize:  "medium",
			QualityScore: ptrFloat64(10.0),
			Successful:   true,
		},
	}

	for _, run := range runs {
		if err := storage.InsertRun(run); err != nil {
			t.Fatalf("InsertRun failed: %v", err)
		}
	}

	// Test quality filter
	results, err := storage.Query(QueryParams{
		QualityMin: ptrFloat64(9.0),
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result with quality >= 9.0, got %d", len(results))
	}

	// Test variant + project size filter
	results, err = storage.Query(QueryParams{
		Variants:     []string{"raw", "engram"},
		ProjectSizes: []string{"small"},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}
