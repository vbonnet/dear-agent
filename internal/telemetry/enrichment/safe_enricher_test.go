package enrichment

import (
	"context"
	"testing"
	"time"
)

// mockEnricher is a mock enricher for testing
type mockEnricher struct {
	name        string
	delay       time.Duration
	shouldError bool
}

func (m *mockEnricher) Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error) {
	// Simulate processing time
	select {
	case <-time.After(m.delay):
		if m.shouldError {
			return nil, context.DeadlineExceeded
		}
		// Add mock enrichment
		enriched := *event
		if enriched.Data == nil {
			enriched.Data = make(map[string]interface{})
		}
		enriched.Data["enriched_by"] = m.name
		return &enriched, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *mockEnricher) Name() string {
	return m.name
}

func TestSafeEnricher_Success(t *testing.T) {
	// Create enricher that completes within timeout
	enricher := &mockEnricher{
		name:  "test-enricher",
		delay: 1 * time.Microsecond, // Very fast to ensure it completes
	}

	safeEnricher := NewSafeEnricher(enricher, 10*time.Millisecond) // Generous timeout

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "test",
	}

	ctx := context.Background()
	enrichedEvent, err := safeEnricher.Enrich(ctx, event, EnrichmentContext{})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if enrichedEvent.Data["enriched_by"] != "test-enricher" {
		t.Errorf("Expected enrichment data, got: %v", enrichedEvent.Data)
	}
}

func TestSafeEnricher_Timeout(t *testing.T) {
	// Create enricher that exceeds timeout
	enricher := &mockEnricher{
		name:  "slow-enricher",
		delay: 1 * time.Second, // Much longer than timeout
	}

	safeEnricher := NewSafeEnricher(enricher, 100*time.Microsecond)

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "test",
	}

	ctx := context.Background()
	enrichedEvent, err := safeEnricher.Enrich(ctx, event, EnrichmentContext{})

	if err == nil {
		t.Fatal("Expected timeout error, got nil")
	}

	// Original event should be returned on timeout
	if enrichedEvent.ID != "test-id" {
		t.Errorf("Expected original event on timeout, got different event")
	}

	// Should not have enrichment data
	if enrichedEvent.Data != nil && enrichedEvent.Data["enriched_by"] != nil {
		t.Errorf("Expected no enrichment data on timeout, got: %v", enrichedEvent.Data)
	}
}

func TestSafeEnricher_Name(t *testing.T) {
	enricher := &mockEnricher{name: "test-enricher"}
	safeEnricher := NewSafeEnricher(enricher, 500*time.Microsecond)

	if safeEnricher.Name() != "test-enricher" {
		t.Errorf("Expected name 'test-enricher', got: %s", safeEnricher.Name())
	}
}
