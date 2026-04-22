package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

// mockQueuePoller implements QueuePoller for testing.
type mockQueuePoller struct {
	stats map[string]int
	err   error
}

func (m *mockQueuePoller) GetStats() (map[string]int, error) {
	return m.stats, m.err
}

func (m *mockQueuePoller) Close() error { return nil }

func TestWatcherDirectiveEvent(t *testing.T) {
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	var received []WatchEvent
	callback := func(ev WatchEvent) {
		received = append(received, ev)
		cancel() // stop after first event
	}

	// Use a mock queue that always returns 0 queued
	factory := func() (QueuePoller, error) {
		return &mockQueuePoller{stats: map[string]int{messages.StatusQueued: 0}}, nil
	}

	w := NewWatcher(dir, 10*time.Minute, callback, WithQueueFactory(factory))

	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Create a directive file
	if err := os.WriteFile(filepath.Join(dir, "test-directive.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("failed to write directive file: %v", err)
	}

	// Wait for callback or timeout
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("watcher returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for directive event")
	}

	if len(received) == 0 {
		t.Fatal("expected at least one event, got none")
	}
	if received[0].Type != "directive" {
		t.Errorf("expected event type 'directive', got %q", received[0].Type)
	}
	if received[0].Source != filepath.Join(dir, "test-directive.txt") {
		t.Errorf("unexpected source: %s", received[0].Source)
	}
}

func TestWatcherHeartbeatFallback(t *testing.T) {
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	var received []WatchEvent
	callback := func(ev WatchEvent) {
		if ev.Type == "heartbeat" {
			received = append(received, ev)
			cancel()
		}
	}

	factory := func() (QueuePoller, error) {
		return &mockQueuePoller{stats: map[string]int{messages.StatusQueued: 0}}, nil
	}

	// Short heartbeat interval for test
	w := NewWatcher(dir, 200*time.Millisecond, callback, WithQueueFactory(factory))

	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("watcher returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for heartbeat event")
	}

	if len(received) == 0 {
		t.Fatal("expected heartbeat event, got none")
	}
	if received[0].Type != "heartbeat" {
		t.Errorf("expected event type 'heartbeat', got %q", received[0].Type)
	}
}

func TestWatcherMessageQueueEvent(t *testing.T) {
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	var received []WatchEvent
	callback := func(ev WatchEvent) {
		if ev.Type == "message" {
			received = append(received, ev)
			cancel()
		}
	}

	// Simulate increasing queue depth on second poll
	factory := func() (QueuePoller, error) {
		callCount++
		queued := 0
		if callCount >= 2 {
			queued = 3 // simulate 3 new messages
		}
		return &mockQueuePoller{stats: map[string]int{messages.StatusQueued: queued}}, nil
	}

	w := NewWatcher(dir, 10*time.Minute, callback, WithQueueFactory(factory))

	errCh := make(chan error, 1)
	go func() { errCh <- w.Run(ctx) }()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("watcher returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		cancel()
		t.Fatal("timed out waiting for message event")
	}

	if len(received) == 0 {
		t.Fatal("expected message event, got none")
	}
	if received[0].Type != "message" {
		t.Errorf("expected event type 'message', got %q", received[0].Type)
	}
	if received[0].Detail != "3 new message(s) queued" {
		t.Errorf("unexpected detail: %s", received[0].Detail)
	}
}

func TestWatchEventRecording(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	factory := func() (QueuePoller, error) {
		return &mockQueuePoller{stats: map[string]int{messages.StatusQueued: 0}}, nil
	}

	// Use short heartbeat to get an event quickly
	w := NewWatcher(dir, 100*time.Millisecond, nil, WithQueueFactory(factory))

	go func() {
		time.Sleep(250 * time.Millisecond)
		cancel()
	}()

	if err := w.Run(ctx); err != nil {
		t.Fatalf("watcher returned error: %v", err)
	}

	events := w.Events()
	if len(events) == 0 {
		t.Fatal("expected recorded events, got none")
	}

	foundHeartbeat := false
	for _, ev := range events {
		if ev.Type == "heartbeat" {
			foundHeartbeat = true
			break
		}
	}
	if !foundHeartbeat {
		t.Error("expected at least one heartbeat event in recorded events")
	}
}
