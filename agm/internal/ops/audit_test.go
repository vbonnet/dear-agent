package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuditLogger(t *testing.T) {
	t.Run("custom path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "audit.jsonl")
		logger, err := NewAuditLogger(path)
		require.NoError(t, err)
		assert.Equal(t, path, logger.FilePath())
	})

	t.Run("default path", func(t *testing.T) {
		logger, err := NewAuditLogger("")
		require.NoError(t, err)
		assert.Contains(t, logger.FilePath(), "audit.jsonl")
	})
}

func TestAuditLoggerLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := NewAuditLogger(path)
	require.NoError(t, err)

	now := time.Date(2026, 4, 13, 7, 15, 0, 0, time.UTC)
	event := AuditEvent{
		Timestamp:  now,
		Command:    "session.new",
		Session:    "worker-1",
		User:       "orchestrator-v3",
		Args:       map[string]string{"model": "opus"},
		Result:     "success",
		DurationMs: 1234,
	}

	require.NoError(t, logger.Log(event))

	// Read back and verify
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var parsed AuditEvent
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "session.new", parsed.Command)
	assert.Equal(t, "worker-1", parsed.Session)
	assert.Equal(t, "orchestrator-v3", parsed.User)
	assert.Equal(t, "success", parsed.Result)
	assert.Equal(t, int64(1234), parsed.DurationMs)
	assert.Equal(t, "opus", parsed.Args["model"])
}

func TestAuditLoggerMultipleEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := NewAuditLogger(path)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		require.NoError(t, logger.Log(AuditEvent{
			Command: "session.list",
			Result:  "success",
		}))
	}

	events, err := ReadRecentEvents(path, 0)
	require.NoError(t, err)
	assert.Len(t, events, 3)
}

func TestReadRecentEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := NewAuditLogger(path)
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		require.NoError(t, logger.Log(AuditEvent{
			Command:    "session.list",
			Result:     "success",
			DurationMs: int64(i),
		}))
	}

	t.Run("all events", func(t *testing.T) {
		events, err := ReadRecentEvents(path, 0)
		require.NoError(t, err)
		assert.Len(t, events, 10)
	})

	t.Run("limited to last 3", func(t *testing.T) {
		events, err := ReadRecentEvents(path, 3)
		require.NoError(t, err)
		assert.Len(t, events, 3)
		// Should be the last 3 events (duration_ms 7, 8, 9)
		assert.Equal(t, int64(7), events[0].DurationMs)
	})

	t.Run("non-existent file returns nil", func(t *testing.T) {
		events, err := ReadRecentEvents(filepath.Join(dir, "missing.jsonl"), 10)
		require.NoError(t, err)
		assert.Nil(t, events)
	})
}

func TestSearchEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := NewAuditLogger(path)
	require.NoError(t, err)

	events := []AuditEvent{
		{Command: "session.new", Session: "worker-1", Result: "success"},
		{Command: "session.archive", Session: "worker-1", Result: "success"},
		{Command: "session.new", Session: "worker-2", Result: "failure", Error: "timeout"},
		{Command: "session.list", Result: "success"},
		{Command: "admin.gc", Session: "worker-1", Result: "success"},
	}
	for _, ev := range events {
		require.NoError(t, logger.Log(ev))
	}

	t.Run("filter by command", func(t *testing.T) {
		results, err := SearchEvents(path, AuditSearchParams{Command: "session.new"})
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("filter by session", func(t *testing.T) {
		results, err := SearchEvents(path, AuditSearchParams{Session: "worker-1"})
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("filter by both", func(t *testing.T) {
		results, err := SearchEvents(path, AuditSearchParams{
			Command: "session.new",
			Session: "worker-2",
		})
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "failure", results[0].Result)
	})

	t.Run("with limit", func(t *testing.T) {
		results, err := SearchEvents(path, AuditSearchParams{
			Session: "worker-1",
			Limit:   2,
		})
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("no matches", func(t *testing.T) {
		results, err := SearchEvents(path, AuditSearchParams{Command: "nonexistent"})
		require.NoError(t, err)
		assert.Nil(t, results)
	})
}

func TestAuditEventTimestampDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := NewAuditLogger(path)
	require.NoError(t, err)

	before := time.Now()
	require.NoError(t, logger.Log(AuditEvent{
		Command: "test",
		Result:  "success",
	}))
	after := time.Now()

	events, err := ReadRecentEvents(path, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.False(t, events[0].Timestamp.IsZero())
	assert.True(t, events[0].Timestamp.After(before) || events[0].Timestamp.Equal(before))
	assert.True(t, events[0].Timestamp.Before(after) || events[0].Timestamp.Equal(after))
}

func TestReadRecentEventsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	// Write some valid and invalid lines
	content := `{"command":"good","result":"success"}
not json at all
{"command":"also-good","result":"success"}
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	events, err := ReadRecentEvents(path, 0)
	require.NoError(t, err)
	assert.Len(t, events, 2) // skips malformed line
}
