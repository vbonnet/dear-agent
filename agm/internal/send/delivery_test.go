package send

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSequentialDeliver_SingleJob(t *testing.T) {
	job := &DeliveryJob{
		Recipient:        "session1",
		Sender:           "sender1",
		MessageID:        "msg-001",
		FormattedMessage: "test message",
		ShouldInterrupt:  false,
	}

	deliveryCalled := false
	deliveryFunc := func(j *DeliveryJob) error {
		deliveryCalled = true
		if j.Recipient != "session1" {
			t.Errorf("expected recipient 'session1', got '%s'", j.Recipient)
		}
		return nil
	}

	results := SequentialDeliver(context.Background(), []*DeliveryJob{job}, deliveryFunc)

	if !deliveryCalled {
		t.Error("delivery function was not called")
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !results[0].Success {
		t.Error("expected success, got failure")
	}

	if results[0].Recipient != "session1" {
		t.Errorf("expected recipient 'session1', got '%s'", results[0].Recipient)
	}

	if results[0].MessageID != "msg-001" {
		t.Errorf("expected message ID 'msg-001', got '%s'", results[0].MessageID)
	}
}

func TestSequentialDeliver_MultipleJobs(t *testing.T) {
	jobs := []*DeliveryJob{
		{Recipient: "session1", Sender: "sender1", MessageID: "msg-001"},
		{Recipient: "session2", Sender: "sender1", MessageID: "msg-002"},
		{Recipient: "session3", Sender: "sender1", MessageID: "msg-003"},
	}

	deliveryCount := 0

	deliveryFunc := func(j *DeliveryJob) error {
		deliveryCount++
		return nil
	}

	results := SequentialDeliver(context.Background(), jobs, deliveryFunc)

	if deliveryCount != 3 {
		t.Errorf("expected 3 deliveries, got %d", deliveryCount)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// All should succeed
	for i, result := range results {
		if !result.Success {
			t.Errorf("result[%d] expected success, got failure", i)
		}
	}

	// Sequential delivery preserves order
	for i, result := range results {
		expected := jobs[i].Recipient
		if result.Recipient != expected {
			t.Errorf("result[%d] expected recipient '%s', got '%s'", i, expected, result.Recipient)
		}
	}
}

func TestSequentialDeliver_PartialFailures(t *testing.T) {
	jobs := []*DeliveryJob{
		{Recipient: "session1", Sender: "sender1", MessageID: "msg-001"},
		{Recipient: "session2", Sender: "sender1", MessageID: "msg-002"},
		{Recipient: "session3", Sender: "sender1", MessageID: "msg-003"},
	}

	deliveryFunc := func(j *DeliveryJob) error {
		// Fail session2
		if j.Recipient == "session2" {
			return fmt.Errorf("delivery failed")
		}
		return nil
	}

	results := SequentialDeliver(context.Background(), jobs, deliveryFunc)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	successCount := 0
	failureCount := 0
	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
			if result.Recipient == "session2" {
				if result.Error == nil {
					t.Error("expected error for session2, got nil")
				}
			}
		}
	}

	if successCount != 2 {
		t.Errorf("expected 2 successes, got %d", successCount)
	}

	if failureCount != 1 {
		t.Errorf("expected 1 failure, got %d", failureCount)
	}
}

func TestSequentialDeliver_EmptyJobs(t *testing.T) {
	deliveryFunc := func(j *DeliveryJob) error {
		t.Error("delivery function should not be called for empty jobs")
		return nil
	}

	results := SequentialDeliver(context.Background(), []*DeliveryJob{}, deliveryFunc)

	if results != nil {
		t.Errorf("expected nil results for empty jobs, got %v", results)
	}
}

func TestSequentialDeliver_DurationTracking(t *testing.T) {
	job := &DeliveryJob{
		Recipient: "session1",
		Sender:    "sender1",
		MessageID: "msg-001",
	}

	deliveryFunc := func(j *DeliveryJob) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	results := SequentialDeliver(context.Background(), []*DeliveryJob{job}, deliveryFunc)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Duration should be at least 50ms
	if results[0].Duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", results[0].Duration)
	}

	// But not too much more (accounting for overhead)
	if results[0].Duration > 200*time.Millisecond {
		t.Errorf("expected duration < 200ms, got %v", results[0].Duration)
	}
}

func TestSequentialDeliver_MessageIDPreserved(t *testing.T) {
	jobs := []*DeliveryJob{
		{Recipient: "session1", MessageID: "msg-001"},
		{Recipient: "session2", MessageID: "msg-002"},
		{Recipient: "session3", MessageID: "msg-003"},
	}

	deliveryFunc := func(j *DeliveryJob) error {
		return nil
	}

	results := SequentialDeliver(context.Background(), jobs, deliveryFunc)

	// Sequential delivery preserves order, so we can check directly
	for i, result := range results {
		if result.MessageID != jobs[i].MessageID {
			t.Errorf("result[%d]: expected message ID '%s', got '%s'",
				i, jobs[i].MessageID, result.MessageID)
		}
	}
}

func TestSequentialDeliver_NilDeliveryFunc(t *testing.T) {
	job := &DeliveryJob{
		Recipient: "session1",
		MessageID: "msg-001",
	}

	// Should use DefaultDeliveryFunc when nil is passed
	results := SequentialDeliver(context.Background(), []*DeliveryJob{job}, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Default function should fail with "not initialized" error
	if results[0].Success {
		t.Error("expected failure with default delivery func, got success")
	}

	if results[0].Error == nil {
		t.Error("expected error with default delivery func, got nil")
	}
}

func TestSequentialDeliver_ContextCancellation(t *testing.T) {
	jobs := []*DeliveryJob{
		{Recipient: "session1", MessageID: "msg-001"},
		{Recipient: "session2", MessageID: "msg-002"},
		{Recipient: "session3", MessageID: "msg-003"},
	}

	ctx, cancel := context.WithCancel(context.Background())

	deliveryCount := 0
	deliveryFunc := func(j *DeliveryJob) error {
		deliveryCount++
		if deliveryCount == 1 {
			// Cancel after first delivery
			cancel()
		}
		return nil
	}

	results := SequentialDeliver(ctx, jobs, deliveryFunc)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First should succeed
	if !results[0].Success {
		t.Error("expected first delivery to succeed")
	}

	// Remaining should be cancelled
	cancelledCount := 0
	for _, r := range results {
		if !r.Success && r.Method == "cancelled" {
			cancelledCount++
		}
	}

	if cancelledCount < 1 {
		t.Errorf("expected at least 1 cancelled delivery, got %d", cancelledCount)
	}
}

func TestSetDefaultDeliveryFunc(t *testing.T) {
	// Save original
	original := DefaultDeliveryFunc

	// Set new function
	called := false
	SetDefaultDeliveryFunc(func(j *DeliveryJob) error {
		called = true
		return nil
	})

	// Test it was set
	job := &DeliveryJob{Recipient: "test"}
	_ = DefaultDeliveryFunc(job)

	if !called {
		t.Error("custom delivery function was not called")
	}

	// Restore original
	DefaultDeliveryFunc = original
}
