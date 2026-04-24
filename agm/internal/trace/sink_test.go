package trace

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

// memoryBackend captures records in memory for testing.
type memoryBackend struct {
	mu      sync.Mutex
	records []*TraceRecord
	flushed bool
	closed  bool
	writeErr error
}

func (m *memoryBackend) Write(_ context.Context, rec *TraceRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.records = append(m.records, rec)
	return nil
}

func (m *memoryBackend) Flush(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushed = true
	return nil
}

func (m *memoryBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func makeEvent(t *testing.T, typ eventbus.EventType, sessionID string) *eventbus.Event {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"key": "value"})
	return &eventbus.Event{
		Type:      typ,
		Timestamp: time.Now(),
		SessionID: sessionID,
		Payload:   payload,
	}
}

func TestAuditSink_HandleEvent(t *testing.T) {
	b := &memoryBackend{}
	sink := NewAuditSink([]Backend{b})
	defer sink.Close()

	ev := makeEvent(t, eventbus.EventSessionStuck, "s1")
	err := sink.HandleEvent(ev)
	require.NoError(t, err)

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Len(t, b.records, 1)
	assert.Equal(t, "s1", b.records[0].SessionID)
}

func TestAuditSink_FanOut(t *testing.T) {
	b1 := &memoryBackend{}
	b2 := &memoryBackend{}
	sink := NewAuditSink([]Backend{b1, b2})
	defer sink.Close()

	ev := makeEvent(t, eventbus.EventSessionCompleted, "s2")
	err := sink.HandleEvent(ev)
	require.NoError(t, err)

	b1.mu.Lock()
	assert.Len(t, b1.records, 1)
	b1.mu.Unlock()

	b2.mu.Lock()
	assert.Len(t, b2.records, 1)
	b2.mu.Unlock()
}

func TestAuditSink_BackendError_ContinuesDelivery(t *testing.T) {
	failing := &memoryBackend{writeErr: errors.New("disk full")}
	healthy := &memoryBackend{}
	sink := NewAuditSink([]Backend{failing, healthy})
	defer sink.Close()

	ev := makeEvent(t, eventbus.EventSessionStuck, "s3")
	err := sink.HandleEvent(ev)
	assert.Error(t, err) // first error returned

	healthy.mu.Lock()
	assert.Len(t, healthy.records, 1, "healthy backend should still receive the record")
	healthy.mu.Unlock()
}

func TestAuditSink_Close_FlushesAndCloses(t *testing.T) {
	b := &memoryBackend{}
	sink := NewAuditSink([]Backend{b})

	err := sink.Close()
	require.NoError(t, err)

	b.mu.Lock()
	assert.True(t, b.flushed)
	assert.True(t, b.closed)
	b.mu.Unlock()
}

func TestAuditSink_HandleEvent_AfterClose(t *testing.T) {
	b := &memoryBackend{}
	sink := NewAuditSink([]Backend{b})
	sink.Close()

	ev := makeEvent(t, eventbus.EventSessionStuck, "s4")
	err := sink.HandleEvent(ev)
	assert.ErrorIs(t, err, ErrSinkClosed)
}

func TestAuditSink_DoubleClose(t *testing.T) {
	b := &memoryBackend{}
	sink := NewAuditSink([]Backend{b})
	require.NoError(t, sink.Close())
	require.NoError(t, sink.Close()) // idempotent
}
