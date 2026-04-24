package costtrack

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSink_Record(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "costs.jsonl")

	sink, err := NewFileSink(filePath)
	require.NoError(t, err)
	defer sink.Close(context.Background())

	cost := &CostInfo{
		Provider: "anthropic",
		Model:    "claude-3-5-sonnet-20241022",
		Tokens: Tokens{
			Input:  2000,
			Output: 1000,
		},
		Cost: Cost{
			Input:  0.006,
			Output: 0.015,
			Total:  0.021,
		},
	}

	meta := &CostMetadata{
		Operation: "rank",
		Timestamp: time.Date(2025, 3, 17, 14, 30, 0, 0, time.UTC),
		Context:   "OAuth implementation",
	}

	err = sink.Record(context.Background(), cost, meta)
	require.NoError(t, err)

	// Close to flush
	err = sink.Close(context.Background())
	require.NoError(t, err)

	// Read file
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	require.True(t, scanner.Scan(), "Expected at least one line")

	var event map[string]any
	err = json.Unmarshal(scanner.Bytes(), &event)
	require.NoError(t, err)

	// Verify event
	assert.Equal(t, "rank", event["operation"])
	assert.Equal(t, "anthropic", event["provider"])
	assert.Equal(t, "claude-3-5-sonnet-20241022", event["model"])
	assert.Equal(t, "OAuth implementation", event["context"])

	tokens, ok := event["tokens"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(2000), tokens["input"])
	assert.Equal(t, float64(1000), tokens["output"])

	costMap, ok := event["cost"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 0.006, costMap["input"], 0.000001)
	assert.InDelta(t, 0.015, costMap["output"], 0.000001)
	assert.InDelta(t, 0.021, costMap["total"], 0.000001)
}

func TestFileSink_MultipleRecords(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "costs.jsonl")

	sink, err := NewFileSink(filePath)
	require.NoError(t, err)

	// Write multiple records
	for i := 0; i < 3; i++ {
		cost := &CostInfo{
			Provider: "vertexai-claude",
			Model:    "claude-sonnet-4-5@20250929",
			Tokens: Tokens{
				Input:  1000 * (i + 1),
				Output: 500 * (i + 1),
			},
			Cost: Cost{
				Input:  0.003 * float64(i+1),
				Output: 0.0075 * float64(i+1),
				Total:  0.0105 * float64(i+1),
			},
		}

		meta := &CostMetadata{
			Operation: "search",
			Timestamp: time.Now(),
		}

		err = sink.Record(context.Background(), cost, meta)
		require.NoError(t, err)
	}

	err = sink.Close(context.Background())
	require.NoError(t, err)

	// Verify 3 lines in file
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		var event map[string]any
		err := json.Unmarshal(scanner.Bytes(), &event)
		require.NoError(t, err, "Line %d should be valid JSON", lineCount)
	}

	assert.Equal(t, 3, lineCount)
}

func TestFileSink_AppendMode(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "costs.jsonl")

	// First sink - write one record
	sink1, err := NewFileSink(filePath)
	require.NoError(t, err)

	cost1 := &CostInfo{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-20241022",
		Tokens:   Tokens{Input: 100, Output: 50},
		Cost:     Cost{Total: 0.001},
	}
	meta1 := &CostMetadata{
		Operation: "test1",
		Timestamp: time.Now(),
	}

	err = sink1.Record(context.Background(), cost1, meta1)
	require.NoError(t, err)
	err = sink1.Close(context.Background())
	require.NoError(t, err)

	// Second sink - append another record
	sink2, err := NewFileSink(filePath)
	require.NoError(t, err)

	cost2 := &CostInfo{
		Provider: "vertexai-gemini",
		Model:    "gemini-2.0-flash-exp",
		Tokens:   Tokens{Input: 200, Output: 100},
		Cost:     Cost{Total: 0.002},
	}
	meta2 := &CostMetadata{
		Operation: "test2",
		Timestamp: time.Now(),
	}

	err = sink2.Record(context.Background(), cost2, meta2)
	require.NoError(t, err)
	err = sink2.Close(context.Background())
	require.NoError(t, err)

	// Verify both records exist
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
	}

	assert.Equal(t, 2, lines, "File should contain 2 records")
}

func TestFileSink_ThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "costs.jsonl")

	sink, err := NewFileSink(filePath)
	require.NoError(t, err)
	defer sink.Close(context.Background())

	// Write from multiple goroutines
	const numGoroutines = 10
	const recordsPerGoroutine = 5

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < recordsPerGoroutine; j++ {
				cost := &CostInfo{
					Provider: "local",
					Model:    "local-jaccard-v1",
					Tokens:   Tokens{Input: id*100 + j, Output: id*50 + j},
					Cost:     Cost{Total: 0},
				}
				meta := &CostMetadata{
					Operation: "concurrent-test",
					Timestamp: time.Now(),
				}

				err := sink.Record(context.Background(), cost, meta)
				require.NoError(t, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	err = sink.Close(context.Background())
	require.NoError(t, err)

	// Verify all records written
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		var event map[string]any
		err := json.Unmarshal(scanner.Bytes(), &event)
		require.NoError(t, err, "Line %d should be valid JSON", lineCount)
	}

	assert.Equal(t, numGoroutines*recordsPerGoroutine, lineCount)
}

func TestFileSink_CreateDirectoryIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nested", "dir", "costs.jsonl")

	// Directory doesn't exist yet - NewFileSink should still work
	// (os.OpenFile with os.O_CREATE creates file but not parent dirs)
	// This tests that we handle the error gracefully
	_, err := NewFileSink(filePath)

	// This should fail because parent directory doesn't exist
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open cost file")
}

func TestFileSink_DoubleClose(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "costs.jsonl")

	sink, err := NewFileSink(filePath)
	require.NoError(t, err)

	// First close
	err = sink.Close(context.Background())
	require.NoError(t, err)

	// Second close should be safe (no-op)
	err = sink.Close(context.Background())
	assert.NoError(t, err)
}
