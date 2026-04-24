package enforcement

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/engram/internal/identity"
	"github.com/vbonnet/dear-agent/internal/telemetry"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// mockTelemetry implements eventbus.TelemetryRecorder for testing
type mockTelemetry struct{}

func (m *mockTelemetry) Record(eventType string, agent string, level telemetry.Level, data map[string]interface{}) error {
	return nil
}

// testEventCollector captures events published to the bus
type testEventCollector struct {
	events []*eventbus.Event
}

func newTestEventCollector() *testEventCollector {
	return &testEventCollector{
		events: make([]*eventbus.Event, 0),
	}
}

func (c *testEventCollector) handler(ctx context.Context, event *eventbus.Event) (*eventbus.Response, error) {
	c.events = append(c.events, event)
	return nil, nil
}

func (c *testEventCollector) getEventsByTopic(topic string) []*eventbus.Event {
	var found []*eventbus.Event
	for _, event := range c.events {
		if event.Type == topic {
			found = append(found, event)
		}
	}
	return found
}

// Test successful validation emits success telemetry
func TestValidateWithTelemetry_Success(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       true,
			AllowedDomains: []string{"@company.com"},
		},
	}

	id := &identity.Identity{
		Email:  "user@company.com",
		Domain: "@company.com",
		Method: "gcp_adc",
	}

	validator := NewValidator(cfg, id)

	// Create eventbus and collector
	bus := eventbus.NewBus(&mockTelemetry{})
	collector := newTestEventCollector()
	bus.Subscribe(EventEnforcementValidation, "test", collector.handler)

	err := validator.ValidateWithTelemetry(context.Background(), bus)

	if err != nil {
		t.Fatalf("Expected successful validation, got error: %v", err)
	}

	// Give events time to be published (async)
	time.Sleep(10 * time.Millisecond)

	// Check validation event emitted
	events := collector.getEventsByTopic(EventEnforcementValidation)
	if len(events) != 1 {
		t.Fatalf("Expected 1 validation event, got %d", len(events))
	}

	event := events[0]
	if result, ok := event.Data["result"].(string); !ok || result != "success" {
		t.Errorf("Expected result 'success', got '%v'", event.Data["result"])
	}
	if phase, ok := event.Data["phase"].(string); !ok || phase != "enforcement" {
		t.Errorf("Expected phase 'enforcement', got '%v'", event.Data["phase"])
	}

	// Should not emit violation event on success
	violations := collector.getEventsByTopic(EventViolation)
	if len(violations) != 0 {
		t.Errorf("Expected no violation events, got %d", len(violations))
	}

	bus.Close()
}

// Test failed validation emits failure telemetry
func TestValidateWithTelemetry_Failure(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       true,
			AllowedDomains: []string{"@company.com"},
		},
	}

	// Identity with wrong domain
	id := &identity.Identity{
		Email:  "user@personal.com",
		Domain: "@personal.com",
		Method: "git_config",
	}

	validator := NewValidator(cfg, id)

	// Create eventbus and collector
	bus := eventbus.NewBus(&mockTelemetry{})
	collector := newTestEventCollector()
	bus.Subscribe(EventEnforcementValidation, "test", collector.handler)
	bus.Subscribe(EventViolation, "test", collector.handler)

	err := validator.ValidateWithTelemetry(context.Background(), bus)

	if err == nil {
		t.Fatal("Expected validation to fail, got nil error")
	}

	// Give events time to be published (async)
	time.Sleep(10 * time.Millisecond)

	// Check validation event emitted
	events := collector.getEventsByTopic(EventEnforcementValidation)
	if len(events) != 1 {
		t.Fatalf("Expected 1 validation event, got %d", len(events))
	}

	event := events[0]
	if result, ok := event.Data["result"].(string); !ok || result != "failure" {
		t.Errorf("Expected result 'failure', got '%v'", event.Data["result"])
	}
	if _, ok := event.Data["error"].(string); !ok {
		t.Error("Expected error message in event data")
	}

	// Should emit violation event on failure
	violations := collector.getEventsByTopic(EventViolation)
	if len(violations) != 1 {
		t.Fatalf("Expected 1 violation event, got %d", len(violations))
	}

	violation := violations[0]
	if vtype, ok := violation.Data["type"].(string); !ok || vtype != "domain" {
		t.Errorf("Expected violation type 'domain', got '%v'", violation.Data["type"])
	}

	bus.Close()
}

// Test nil eventbus falls back to standard validation
func TestValidateWithTelemetry_NilBus(t *testing.T) {
	cfg := &config.EnforcementConfig{
		Enabled: true,
		Identity: config.EnforcementIdentityConfig{
			Required:       true,
			AllowedDomains: []string{"@company.com"},
		},
	}

	id := &identity.Identity{
		Email:  "user@company.com",
		Domain: "@company.com",
	}

	validator := NewValidator(cfg, id)

	// Pass nil eventbus - should still work
	err := validator.ValidateWithTelemetry(context.Background(), nil)

	if err != nil {
		t.Fatalf("Expected successful validation, got error: %v", err)
	}
}

// Test extractPluginNames helper
func TestExtractPluginNames(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected []string
	}{
		{
			name:     "single plugin",
			errMsg:   "missing required plugins: foo",
			expected: []string{"foo"},
		},
		{
			name:     "multiple plugins",
			errMsg:   "missing required plugins: foo, bar, baz",
			expected: []string{"foo", "bar", "baz"},
		},
		{
			name:     "with extra text",
			errMsg:   "Required Plugins Missing: missing required plugins: foo, bar; version mismatches: baz",
			expected: []string{"foo", "bar"},
		},
		{
			name:     "no match",
			errMsg:   "some other error",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPluginNames(tt.errMsg)

			if result == nil && tt.expected != nil {
				t.Errorf("Expected result, got nil")
				return
			}

			if result != nil && tt.expected == nil {
				t.Errorf("Expected nil, got %v", result)
				return
			}

			if result == nil && tt.expected == nil {
				return // Both nil, pass
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d plugins, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected plugin[%d] = %s, got %s", i, expected, result[i])
				}
			}
		})
	}
}

// Test extractVersionMismatch helper
func TestExtractVersionMismatch(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected []string
	}{
		{
			name:     "version mismatch",
			errMsg:   "foo (have 1.0.0, need >= 2.0.0)",
			expected: []string{"foo", "1.0.0", "2.0.0"},
		},
		{
			name:     "with prefix text",
			errMsg:   "version mismatches: foo (have 1.0.0, need >= 2.0.0)",
			expected: []string{"foo", "1.0.0", "2.0.0"},
		},
		{
			name:     "no match",
			errMsg:   "missing required plugins: foo",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionMismatch(tt.errMsg)

			if result == nil && tt.expected != nil {
				t.Errorf("Expected result, got nil")
				return
			}

			if result != nil && tt.expected == nil {
				t.Errorf("Expected nil, got %v", result)
				return
			}

			if result == nil && tt.expected == nil {
				return // Both nil, pass
			}

			if len(result) < len(tt.expected) {
				t.Errorf("Expected at least %d parts, got %d: %v", len(tt.expected), len(result), result)
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected part[%d] = %s, got %s", i, expected, result[i])
				}
			}
		})
	}
}
