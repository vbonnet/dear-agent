package telemetry

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// TestMain runs goleak to detect goroutine leaks across all tests
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// Test 2.2: Level filtering
func TestLevelFiltering(t *testing.T) {
	registry := NewListenerRegistry()

	// Create listeners with different minimum levels
	infoListener := &mockListener{minLevel: LevelInfo}
	warnListener := &mockListener{minLevel: LevelWarn}
	errorListener := &mockListener{minLevel: LevelError}

	registry.Register(infoListener)
	registry.Register(warnListener)
	registry.Register(errorListener)

	tests := []struct {
		name                string
		eventLevel          Level
		expectInfoListener  bool
		expectWarnListener  bool
		expectErrorListener bool
	}{
		{
			name:                "INFO event",
			eventLevel:          LevelInfo,
			expectInfoListener:  true,
			expectWarnListener:  false,
			expectErrorListener: false,
		},
		{
			name:                "WARN event",
			eventLevel:          LevelWarn,
			expectInfoListener:  true,
			expectWarnListener:  true,
			expectErrorListener: false,
		},
		{
			name:                "ERROR event",
			eventLevel:          LevelError,
			expectInfoListener:  true,
			expectWarnListener:  true,
			expectErrorListener: true,
		},
		{
			name:                "CRITICAL event",
			eventLevel:          LevelCritical,
			expectInfoListener:  true,
			expectWarnListener:  true,
			expectErrorListener: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset listeners
			infoListener.reset()
			warnListener.reset()
			errorListener.reset()

			// Send event
			event := &Event{
				Type:  "test.event",
				Agent: "test",
				Level: tt.eventLevel,
				Data:  map[string]interface{}{"test": "data"},
			}

			registry.Notify(event)

			// Wait for async goroutines to complete
			time.Sleep(50 * time.Millisecond)

			// Check INFO listener
			infoEvents := infoListener.getEvents()
			if tt.expectInfoListener && len(infoEvents) != 1 {
				t.Errorf("INFO listener: expected 1 event, got %d", len(infoEvents))
			}
			if !tt.expectInfoListener && len(infoEvents) != 0 {
				t.Errorf("INFO listener: expected 0 events, got %d", len(infoEvents))
			}

			// Check WARN listener
			warnEvents := warnListener.getEvents()
			if tt.expectWarnListener && len(warnEvents) != 1 {
				t.Errorf("WARN listener: expected 1 event, got %d", len(warnEvents))
			}
			if !tt.expectWarnListener && len(warnEvents) != 0 {
				t.Errorf("WARN listener: expected 0 events, got %d", len(warnEvents))
			}

			// Check ERROR listener
			errorEvents := errorListener.getEvents()
			if tt.expectErrorListener && len(errorEvents) != 1 {
				t.Errorf("ERROR listener: expected 1 event, got %d", len(errorEvents))
			}
			if !tt.expectErrorListener && len(errorEvents) != 0 {
				t.Errorf("ERROR listener: expected 0 events, got %d", len(errorEvents))
			}
		})
	}
}

// Test 2.4: Panic recovery
func TestPanicRecovery(t *testing.T) {
	registry := NewListenerRegistry()

	// Create listeners: one that panics, one that works
	panicListener := &mockListener{
		minLevel:    LevelInfo,
		shouldPanic: true,
	}
	normalListener := &mockListener{
		minLevel: LevelInfo,
	}

	registry.Register(panicListener)
	registry.Register(normalListener)

	// Send event
	event := &Event{
		Type:  "test.panic",
		Agent: "test",
		Level: LevelInfo,
		Data:  map[string]interface{}{"test": "panic"},
	}

	// Should not panic
	registry.Notify(event)

	// Wait for async goroutines
	time.Sleep(50 * time.Millisecond)

	// Normal listener should still receive event
	normalEvents := normalListener.getEvents()
	if len(normalEvents) != 1 {
		t.Errorf("Normal listener should receive event despite panic in other listener, got %d events", len(normalEvents))
	}

	// Panic listener should have been called (incremented counter before panic)
	if panicListener.callCount.Load() != 1 {
		t.Errorf("Panic listener should have been called, got %d calls", panicListener.callCount.Load())
	}
}

// Test 2.6: No goroutine leaks
func TestNoGoroutineLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)

	registry := NewListenerRegistry()
	listener := &mockListener{minLevel: LevelInfo}
	registry.Register(listener)

	// Send many events
	for i := 0; i < 1000; i++ {
		event := &Event{
			Type:  "test.leak",
			Agent: "test",
			Level: LevelInfo,
			Data:  map[string]interface{}{"iteration": i},
		}
		registry.Notify(event)
	}

	// Wait for goroutines to complete
	time.Sleep(100 * time.Millisecond)

	// goleak.VerifyNone will check for leaks on defer
}

// Test 2.6: Panic doesn't cause leaks
func TestPanicNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)

	registry := NewListenerRegistry()
	panicListener := &mockListener{
		minLevel:    LevelInfo,
		shouldPanic: true,
	}
	registry.Register(panicListener)

	// Send events that will cause panics
	for i := 0; i < 100; i++ {
		event := &Event{
			Type:  "test.panic.leak",
			Agent: "test",
			Level: LevelInfo,
			Data:  map[string]interface{}{"iteration": i},
		}
		registry.Notify(event)
	}

	// Wait for goroutines to complete (and panic)
	time.Sleep(100 * time.Millisecond)

	// goleak.VerifyNone will check for leaks on defer
}

// Test listener error handling
func TestListenerError(t *testing.T) {
	registry := NewListenerRegistry()

	errorListener := &mockListener{
		minLevel:    LevelInfo,
		shouldError: true,
	}
	normalListener := &mockListener{
		minLevel: LevelInfo,
	}

	registry.Register(errorListener)
	registry.Register(normalListener)

	event := &Event{
		Type:  "test.error",
		Agent: "test",
		Level: LevelInfo,
		Data:  map[string]interface{}{"test": "error"},
	}

	// Should not propagate error
	registry.Notify(event)

	// Wait for async goroutines
	time.Sleep(50 * time.Millisecond)

	// Both listeners should have been called
	if errorListener.callCount.Load() != 1 {
		t.Errorf("Error listener should have been called, got %d calls", errorListener.callCount.Load())
	}

	normalEvents := normalListener.getEvents()
	if len(normalEvents) != 1 {
		t.Errorf("Normal listener should receive event despite error in other listener, got %d events", len(normalEvents))
	}
}

// Test concurrent registration and notification
func TestConcurrentOperations(t *testing.T) {
	defer goleak.VerifyNone(t)

	registry := NewListenerRegistry()

	var wg sync.WaitGroup

	// Concurrent registration
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listener := &mockListener{minLevel: LevelInfo}
			registry.Register(listener)
		}()
	}

	// Concurrent notification
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			event := &Event{
				Type:  "test.concurrent",
				Agent: "test",
				Level: LevelInfo,
				Data:  map[string]interface{}{"iteration": iter},
			}
			registry.Notify(event)
		}(i)
	}

	wg.Wait()

	// Wait for async notifications to complete
	time.Sleep(100 * time.Millisecond)

	// No assertions - just checking for races and leaks
}
