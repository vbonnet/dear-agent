package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestLogCommandAudit_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := ops.NewAuditLogger(path)
	require.NoError(t, err)

	// Save and restore global state
	origLogger := auditLogger
	origHandled := auditHandled
	origStartTime := commandStartTime
	defer func() {
		auditLogger = origLogger
		auditHandled = origHandled
		commandStartTime = origStartTime
	}()

	auditLogger = logger
	auditHandled = false
	commandStartTime = time.Now().Add(-100 * time.Millisecond)

	logCommandAudit("session.kill", "worker-1", map[string]string{
		"mode":  "soft",
		"force": "false",
	}, nil)

	// Verify audit was logged
	assert.True(t, auditHandled)

	events, err := ops.ReadRecentEvents(path, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, "session.kill", ev.Command)
	assert.Equal(t, "worker-1", ev.Session)
	assert.Equal(t, "success", ev.Result)
	assert.Equal(t, "", ev.Error)
	assert.Equal(t, "soft", ev.Args["mode"])
	assert.Equal(t, "false", ev.Args["force"])
	assert.Greater(t, ev.DurationMs, int64(0))
}

func TestLogCommandAudit_Error(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := ops.NewAuditLogger(path)
	require.NoError(t, err)

	origLogger := auditLogger
	origHandled := auditHandled
	origStartTime := commandStartTime
	defer func() {
		auditLogger = origLogger
		auditHandled = origHandled
		commandStartTime = origStartTime
	}()

	auditLogger = logger
	auditHandled = false
	commandStartTime = time.Now()

	cmdErr := errors.New("session 'worker-1' does not exist")
	logCommandAudit("send.enter", "worker-1", map[string]string{
		"force": "true",
	}, cmdErr)

	assert.True(t, auditHandled)

	events, err := ops.ReadRecentEvents(path, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, "send.enter", ev.Command)
	assert.Equal(t, "error", ev.Result)
	assert.Equal(t, "session 'worker-1' does not exist", ev.Error)
}

func TestLogCommandAudit_NilLogger(t *testing.T) {
	origLogger := auditLogger
	origHandled := auditHandled
	defer func() {
		auditLogger = origLogger
		auditHandled = origHandled
	}()

	auditLogger = nil
	auditHandled = false

	// Should not panic with nil logger
	logCommandAudit("session.new", "test", nil, nil)

	assert.False(t, auditHandled)
}

func TestLogCommandAudit_AllKeyCommands(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := ops.NewAuditLogger(path)
	require.NoError(t, err)

	origLogger := auditLogger
	origHandled := auditHandled
	origStartTime := commandStartTime
	defer func() {
		auditLogger = origLogger
		auditHandled = origHandled
		commandStartTime = origStartTime
	}()

	auditLogger = logger
	commandStartTime = time.Now()

	tests := []struct {
		command string
		session string
		args    map[string]string
		cmdErr  error
	}{
		{
			command: "session.select-option",
			session: "worker-1",
			args:    map[string]string{"option": "2", "force": "false"},
		},
		{
			command: "send.enter",
			session: "worker-2",
			args:    map[string]string{"force": "true"},
		},
		{
			command: "send.msg",
			session: "worker-3",
			args:    map[string]string{"recipient": "worker-3", "sender": "orchestrator", "priority": "urgent"},
		},
		{
			command: "session.kill",
			session: "worker-4",
			args:    map[string]string{"mode": "hard", "force": "true"},
		},
		{
			command: "session.archive",
			session: "worker-5",
			args:    map[string]string{"async": "true", "force": "false"},
		},
		{
			command: "session.new",
			session: "worker-6",
			args:    map[string]string{"harness": "claude-code", "model": "opus", "detached": "true"},
		},
	}

	for _, tt := range tests {
		auditHandled = false
		logCommandAudit(tt.command, tt.session, tt.args, tt.cmdErr)
		assert.True(t, auditHandled, "auditHandled should be true for %s", tt.command)
	}

	events, err := ops.ReadRecentEvents(path, 0)
	require.NoError(t, err)
	assert.Len(t, events, 6)

	// Verify each command was logged in order
	for i, tt := range tests {
		assert.Equal(t, tt.command, events[i].Command)
		assert.Equal(t, tt.session, events[i].Session)
		assert.Equal(t, "success", events[i].Result)
		for k, v := range tt.args {
			assert.Equal(t, v, events[i].Args[k], "arg %s for command %s", k, tt.command)
		}
	}
}

func TestLogCommandAudit_AppendOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := ops.NewAuditLogger(path)
	require.NoError(t, err)

	origLogger := auditLogger
	origHandled := auditHandled
	origStartTime := commandStartTime
	defer func() {
		auditLogger = origLogger
		auditHandled = origHandled
		commandStartTime = origStartTime
	}()

	auditLogger = logger
	commandStartTime = time.Now()

	// Log multiple events
	logCommandAudit("send.enter", "s1", map[string]string{"force": "false"}, nil)
	logCommandAudit("send.enter", "s2", map[string]string{"force": "true"}, nil)
	logCommandAudit("send.enter", "s3", map[string]string{"force": "false"}, errors.New("empty"))

	// Read raw file and verify it's valid JSONL (one JSON object per line)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := splitJSONL(data)
	assert.Len(t, lines, 3)

	// Verify each line is valid JSON
	for i, line := range lines {
		var ev ops.AuditEvent
		err := json.Unmarshal(line, &ev)
		require.NoError(t, err, "line %d should be valid JSON", i)
		assert.Equal(t, "send.enter", ev.Command)
	}
}

// splitJSONL splits JSONL content into individual lines, skipping empty lines.
func splitJSONL(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			line := data[start:i]
			if len(line) > 0 {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(data) {
		line := data[start:]
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestLogCommandAudit_UserFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger, err := ops.NewAuditLogger(path)
	require.NoError(t, err)

	origLogger := auditLogger
	origHandled := auditHandled
	origStartTime := commandStartTime
	defer func() {
		auditLogger = origLogger
		auditHandled = origHandled
		commandStartTime = origStartTime
	}()

	auditLogger = logger
	auditHandled = false
	commandStartTime = time.Now()

	// Set AGM_SESSION_NAME to verify it's captured
	t.Setenv("AGM_SESSION_NAME", "orchestrator-v3")

	logCommandAudit("session.kill", "worker-1", nil, nil)

	events, err := ops.ReadRecentEvents(path, 0)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "orchestrator-v3", events[0].User)
}
