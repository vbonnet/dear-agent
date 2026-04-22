package costtrack

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdoutSink_Record(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	sink := NewStdoutSink()

	cost := &CostInfo{
		Provider: "anthropic",
		Model:    "claude-3-5-haiku-20241022",
		Tokens: Tokens{
			Input:      1000,
			Output:     500,
			CacheRead:  200,
			CacheWrite: 100,
		},
		Cost: Cost{
			Input:      0.001,
			Output:     0.0025,
			CacheRead:  0.00006,
			CacheWrite: 0.000125,
			Total:      0.003785,
		},
		Cache: &Cache{
			HitRate: 0.67,
			Savings: 0.002,
		},
	}

	meta := &CostMetadata{
		Operation: "rank",
		Timestamp: time.Date(2025, 3, 17, 12, 0, 0, 0, time.UTC),
		Context:   "test query",
		RequestID: "req-123",
	}

	err = sink.Record(context.Background(), cost, meta)
	require.NoError(t, err)

	// Close write pipe and restore stderr
	w.Close()
	os.Stderr = oldStderr

	// Read captured output
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	output := buf.String()

	// Verify prefix
	assert.Contains(t, output, "[COST_TRACKING]")

	// Parse JSON
	jsonStart := bytes.Index(buf.Bytes(), []byte("{"))
	require.NotEqual(t, -1, jsonStart, "JSON object not found in output")

	var event map[string]any
	err = json.Unmarshal(buf.Bytes()[jsonStart:], &event)
	require.NoError(t, err)

	// Verify event structure
	assert.Equal(t, "rank", event["operation"])
	assert.Equal(t, "anthropic", event["provider"])
	assert.Equal(t, "claude-3-5-haiku-20241022", event["model"])
	assert.Equal(t, "test query", event["context"])
	assert.Equal(t, "req-123", event["request_id"])

	// Verify tokens
	tokens, ok := event["tokens"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1000), tokens["input"])
	assert.Equal(t, float64(500), tokens["output"])
	assert.Equal(t, float64(200), tokens["cache_read"])
	assert.Equal(t, float64(100), tokens["cache_write"])

	// Verify costs
	costMap, ok := event["cost"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 0.001, costMap["input"], 0.000001)
	assert.InDelta(t, 0.0025, costMap["output"], 0.000001)
	assert.InDelta(t, 0.003785, costMap["total"], 0.000001)

	// Verify cache metrics
	cacheMap, ok := event["cache"].(map[string]any)
	require.True(t, ok)
	assert.InDelta(t, 0.67, cacheMap["hit_rate"], 0.01)
	assert.InDelta(t, 0.002, cacheMap["savings"], 0.000001)
}

func TestStdoutSink_RecordWithoutCache(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	sink := NewStdoutSink()

	cost := &CostInfo{
		Provider: "vertexai-gemini",
		Model:    "gemini-2.0-flash-exp",
		Tokens: Tokens{
			Input:  500,
			Output: 250,
		},
		Cost: Cost{
			Input:  0,
			Output: 0,
			Total:  0,
		},
		Cache: nil, // No caching
	}

	meta := &CostMetadata{
		Operation: "search",
		Timestamp: time.Now(),
	}

	err = sink.Record(context.Background(), cost, meta)
	require.NoError(t, err)

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)

	jsonStart := bytes.Index(buf.Bytes(), []byte("{"))
	require.NotEqual(t, -1, jsonStart)

	var event map[string]any
	err = json.Unmarshal(buf.Bytes()[jsonStart:], &event)
	require.NoError(t, err)

	// Verify no cache field when Cache is nil
	_, hasCacheField := event["cache"]
	assert.False(t, hasCacheField)

	// Verify no context/request_id when empty
	_, hasContext := event["context"]
	assert.False(t, hasContext)
	_, hasRequestID := event["request_id"]
	assert.False(t, hasRequestID)
}

func TestStdoutSink_Close(t *testing.T) {
	sink := NewStdoutSink()
	err := sink.Close(context.Background())
	assert.NoError(t, err)
}
