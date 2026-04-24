package trace

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
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

func TestJSONLBackend_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	b, err := NewJSONLBackend(path)
	require.NoError(t, err)

	rec := &TraceRecord{
		Timestamp: time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
		EventType: eventbus.EventSessionStuck,
		SessionID: "sess-1",
		Payload:   map[string]interface{}{"reason": "timeout"},
	}

	require.NoError(t, b.Write(context.Background(), rec))
	require.NoError(t, b.Close())

	// Read back and verify
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	require.True(t, scanner.Scan())

	var got TraceRecord
	require.NoError(t, json.Unmarshal(scanner.Bytes(), &got))
	assert.Equal(t, "sess-1", got.SessionID)
	assert.Equal(t, eventbus.EventSessionStuck, got.EventType)
	assert.Equal(t, "timeout", got.Payload["reason"])
}

func TestJSONLBackend_MultipleWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	b, err := NewJSONLBackend(path)
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		rec := &TraceRecord{
			Timestamp: time.Now(),
			EventType: eventbus.EventSessionStateChange,
			SessionID: "sess-multi",
		}
		require.NoError(t, b.Write(context.Background(), rec))
	}
	require.NoError(t, b.Flush(context.Background()))
	require.NoError(t, b.Close())

	// Count lines
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	assert.Equal(t, 5, count)
}

func TestJSONLBackend_InvalidPath(t *testing.T) {
	_, err := NewJSONLBackend("/nonexistent/dir/file.jsonl")
	assert.Error(t, err)
}
