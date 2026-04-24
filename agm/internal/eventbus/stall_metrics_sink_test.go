package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStallMetricsSink_HandleEvent_Detected(t *testing.T) {
	sink := NewStallMetricsSink()

	event, err := NewEvent(EventStallDetected, "worker-1", StallDetectedPayload{
		StallType: "permission_prompt",
		Session:   "worker-1",
		Duration:  10 * time.Minute,
		Details:   "Permission dialog open for 10m",
		Severity:  "critical",
	})
	require.NoError(t, err)

	err = sink.HandleEvent(event)
	require.NoError(t, err)

	m := sink.Snapshot()
	assert.Equal(t, int64(1), m.Detected)
	assert.Equal(t, int64(0), m.Recovered)
	assert.Equal(t, int64(0), m.Escalated)
	assert.Equal(t, int64(1), m.ByStallType["permission_prompt"])
	assert.Equal(t, int64(1), m.BySession["worker-1"])
}

func TestStallMetricsSink_HandleEvent_Recovered(t *testing.T) {
	sink := NewStallMetricsSink()

	event, err := NewEvent(EventStallRecovered, "worker-1", StallRecoveredPayload{
		StallType:      "no_commit",
		Session:        "worker-1",
		RecoveryAction: "nudge",
		Duration:       15 * time.Minute,
	})
	require.NoError(t, err)

	err = sink.HandleEvent(event)
	require.NoError(t, err)

	m := sink.Snapshot()
	assert.Equal(t, int64(0), m.Detected)
	assert.Equal(t, int64(1), m.Recovered)
	assert.Equal(t, int64(0), m.Escalated)
}

func TestStallMetricsSink_HandleEvent_Escalated(t *testing.T) {
	sink := NewStallMetricsSink()

	event, err := NewEvent(EventStallEscalated, "worker-2", StallEscalatedPayload{
		StallType:    "error_loop",
		Session:      "worker-2",
		Reason:       "max retries exceeded",
		AttemptCount: 3,
	})
	require.NoError(t, err)

	err = sink.HandleEvent(event)
	require.NoError(t, err)

	m := sink.Snapshot()
	assert.Equal(t, int64(0), m.Detected)
	assert.Equal(t, int64(0), m.Recovered)
	assert.Equal(t, int64(1), m.Escalated)
}

func TestStallMetricsSink_MultipleEvents(t *testing.T) {
	sink := NewStallMetricsSink()

	// 2 detections, 1 recovery, 1 escalation
	events := []struct {
		eventType EventType
		sessionID string
		payload   any
	}{
		{EventStallDetected, "w-1", StallDetectedPayload{StallType: "permission_prompt", Session: "w-1"}},
		{EventStallDetected, "w-2", StallDetectedPayload{StallType: "error_loop", Session: "w-2"}},
		{EventStallRecovered, "w-1", StallRecoveredPayload{StallType: "permission_prompt", Session: "w-1"}},
		{EventStallEscalated, "w-2", StallEscalatedPayload{StallType: "error_loop", Session: "w-2", AttemptCount: 3}},
	}

	for _, e := range events {
		event, err := NewEvent(e.eventType, e.sessionID, e.payload)
		require.NoError(t, err)
		require.NoError(t, sink.HandleEvent(event))
	}

	m := sink.Snapshot()
	assert.Equal(t, int64(2), m.Detected)
	assert.Equal(t, int64(1), m.Recovered)
	assert.Equal(t, int64(1), m.Escalated)
	assert.Equal(t, int64(1), m.ByStallType["permission_prompt"])
	assert.Equal(t, int64(1), m.ByStallType["error_loop"])
	assert.Equal(t, int64(1), m.BySession["w-1"])
	assert.Equal(t, int64(1), m.BySession["w-2"])
}

func TestStallMetricsSink_IgnoresNonStallEvents(t *testing.T) {
	sink := NewStallMetricsSink()

	event, err := NewEvent(EventSessionStuck, "session-1", SessionStuckPayload{
		Reason:   "test",
		Duration: time.Minute,
	})
	require.NoError(t, err)

	err = sink.HandleEvent(event)
	require.NoError(t, err)

	m := sink.Snapshot()
	assert.Equal(t, int64(0), m.Detected)
	assert.Equal(t, int64(0), m.Recovered)
	assert.Equal(t, int64(0), m.Escalated)
}

func TestStallMetricsSink_Close(t *testing.T) {
	sink := NewStallMetricsSink()
	assert.NoError(t, sink.Close())
}

func TestStallMetricsSink_SnapshotIsCopy(t *testing.T) {
	sink := NewStallMetricsSink()

	event, _ := NewEvent(EventStallDetected, "w-1", StallDetectedPayload{StallType: "no_commit", Session: "w-1"})
	sink.HandleEvent(event)

	snap1 := sink.Snapshot()
	snap1.ByStallType["no_commit"] = 999 // mutate snapshot

	snap2 := sink.Snapshot()
	assert.Equal(t, int64(1), snap2.ByStallType["no_commit"], "snapshot mutation should not affect sink")
}
