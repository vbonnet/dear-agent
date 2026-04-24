package enrichment

import (
	"context"
	"testing"
)

func TestSanityCheckEnricher_SessionEnd(t *testing.T) {
	emittedEvents := make([]*TelemetryEvent, 0)
	eventEmitter := func(ctx context.Context, event *TelemetryEvent) error {
		emittedEvents = append(emittedEvents, event)
		return nil
	}

	enricher := NewSanityCheckEnricher(eventEmitter)

	event := &TelemetryEvent{
		ID:    "test-id",
		Type:  EventTypeSessionEnd,
		Agent: "test-agent",
	}

	availablePlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
	}

	loadedPlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
	}

	ec := EnrichmentContext{
		Prompt:           "research: test",
		AvailablePlugins: availablePlugins,
		LoadedPlugins:    loadedPlugins,
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, ec)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Original event should be unchanged
	if enrichedEvent.ID != "test-id" {
		t.Errorf("Expected original event unchanged, got ID: %s", enrichedEvent.ID)
	}

	// Should have emitted sanity check events
	// (plugin_loading + version_compatibility = 2 events, no ecphory check without result)
	if len(emittedEvents) != 2 {
		t.Errorf("Expected 2 sanity check events, got: %d", len(emittedEvents))
	}

	// Check first event (plugin_loading)
	if len(emittedEvents) > 0 {
		evt := emittedEvents[0]
		if evt.Type != EventTypeSanityCheck {
			t.Errorf("Expected type sanity_check, got: %s", evt.Type)
		}

		checkType, ok := evt.Data["check_type"].(string)
		if !ok || checkType != CheckTypePluginLoading {
			t.Errorf("Expected check_type plugin_loading, got: %v", evt.Data["check_type"])
		}

		status, ok := evt.Data["status"].(string)
		if !ok || status != StatusPass {
			t.Errorf("Expected status pass (all plugins loaded), got: %v", evt.Data["status"])
		}
	}
}

func TestSanityCheckEnricher_NonSessionEnd(t *testing.T) {
	emittedEvents := make([]*TelemetryEvent, 0)
	eventEmitter := func(ctx context.Context, event *TelemetryEvent) error {
		emittedEvents = append(emittedEvents, event)
		return nil
	}

	enricher := NewSanityCheckEnricher(eventEmitter)

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "other_event_type",
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, EnrichmentContext{})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Should not emit events for non-session_end events
	if len(emittedEvents) != 0 {
		t.Errorf("Expected no events for non-session_end, got: %d", len(emittedEvents))
	}

	// Original event unchanged
	if enrichedEvent.ID != "test-id" {
		t.Errorf("Expected original event unchanged")
	}
}

func TestCheckPluginLoading_Pass(t *testing.T) {
	ec := EnrichmentContext{
		Prompt:           "research: test",
		AvailablePlugins: []Plugin{{Name: "research", Version: "1.0.0"}},
		LoadedPlugins:    []Plugin{{Name: "research", Version: "1.0.0"}},
	}

	result := checkPluginLoading(ec)

	if result.CheckType != CheckTypePluginLoading {
		t.Errorf("Expected check_type plugin_loading, got: %s", result.CheckType)
	}

	if result.Status != StatusPass {
		t.Errorf("Expected status pass, got: %s", result.Status)
	}
}

func TestCheckPluginLoading_Fail(t *testing.T) {
	ec := EnrichmentContext{
		Prompt:           "research: test",
		AvailablePlugins: []Plugin{{Name: "research", Version: "1.0.0"}},
		LoadedPlugins:    []Plugin{}, // No plugins loaded
	}

	result := checkPluginLoading(ec)

	if result.CheckType != CheckTypePluginLoading {
		t.Errorf("Expected check_type plugin_loading, got: %s", result.CheckType)
	}

	if result.Status != StatusFail {
		t.Errorf("Expected status fail (missing plugin), got: %s", result.Status)
	}

	// Should have recommendations
	if len(result.Recommendations) == 0 {
		t.Error("Expected recommendations for failed plugin loading check")
	}

	// Should have context with missing plugins
	context, ok := result.Context["missing_plugins"]
	if !ok {
		t.Error("Expected missing_plugins in context")
	}

	missingPlugins, ok := context.([]string)
	if !ok || len(missingPlugins) != 1 || missingPlugins[0] != "research" {
		t.Errorf("Expected missing_plugins [research], got: %v", context)
	}
}

func TestCheckVersionCompatibility_Pass(t *testing.T) {
	ec := EnrichmentContext{
		LoadedPlugins: []Plugin{
			{Name: "research", Version: "1.0.0", Deprecated: false},
			{Name: "wayfinder", Version: "1.0.0", Deprecated: false},
		},
	}

	result := checkVersionCompatibility(ec)

	if result.CheckType != CheckTypeVersionCompatibility {
		t.Errorf("Expected check_type version_compatibility, got: %s", result.CheckType)
	}

	if result.Status != StatusPass {
		t.Errorf("Expected status pass (no deprecated plugins), got: %s", result.Status)
	}
}

func TestCheckVersionCompatibility_Warn(t *testing.T) {
	ec := EnrichmentContext{
		LoadedPlugins: []Plugin{
			{Name: "research", Version: "0.9.0", Deprecated: true},
			{Name: "wayfinder", Version: "1.0.0", Deprecated: false},
		},
	}

	result := checkVersionCompatibility(ec)

	if result.CheckType != CheckTypeVersionCompatibility {
		t.Errorf("Expected check_type version_compatibility, got: %s", result.CheckType)
	}

	if result.Status != StatusWarn {
		t.Errorf("Expected status warn (deprecated plugin), got: %s", result.Status)
	}

	// Should have recommendations
	if len(result.Recommendations) == 0 {
		t.Error("Expected recommendations for deprecated plugins")
	}
}

func TestCheckEcphoryCoverage_Pass(t *testing.T) {
	ec := EnrichmentContext{
		EcphoryResult: &EcphoryResult{
			TokenBudgetUsed:    25000, // 25% utilization (below 80% threshold)
			EngramsRetrieved:   10,
			CandidatesFiltered: 30,
			Strategy:           "api",
		},
	}

	result := checkEcphoryCoverage(ec)

	if result.CheckType != CheckTypeEcphoryCoverage {
		t.Errorf("Expected check_type ecphory_coverage, got: %s", result.CheckType)
	}

	if result.Status != StatusPass {
		t.Errorf("Expected status pass (low utilization), got: %s", result.Status)
	}
}

func TestCheckEcphoryCoverage_Warn(t *testing.T) {
	ec := EnrichmentContext{
		EcphoryResult: &EcphoryResult{
			TokenBudgetUsed:    85000, // 85% utilization (above 80% threshold)
			EngramsRetrieved:   50,
			CandidatesFiltered: 200,
			Strategy:           "inverted-tags",
		},
	}

	result := checkEcphoryCoverage(ec)

	if result.CheckType != CheckTypeEcphoryCoverage {
		t.Errorf("Expected check_type ecphory_coverage, got: %s", result.CheckType)
	}

	if result.Status != StatusWarn {
		t.Errorf("Expected status warn (high utilization), got: %s", result.Status)
	}

	// Should have recommendations
	if len(result.Recommendations) == 0 {
		t.Error("Expected recommendations for high token utilization")
	}

	// Check utilization percentage in context
	utilization, ok := result.Context["token_utilization_percent"].(float64)
	if !ok || utilization != 85.0 {
		t.Errorf("Expected token_utilization_percent 85.0, got: %v", result.Context["token_utilization_percent"])
	}
}

func TestCheckEcphoryCoverage_NoResult(t *testing.T) {
	ec := EnrichmentContext{
		EcphoryResult: nil,
	}

	result := checkEcphoryCoverage(ec)

	if result.CheckType != CheckTypeEcphoryCoverage {
		t.Errorf("Expected check_type ecphory_coverage, got: %s", result.CheckType)
	}

	if result.Status != StatusPass {
		t.Errorf("Expected status pass (no ecphory in session), got: %s", result.Status)
	}
}

func TestRunSanityChecks(t *testing.T) {
	ec := EnrichmentContext{
		Prompt:           "research: test",
		AvailablePlugins: []Plugin{{Name: "research", Version: "1.0.0"}},
		LoadedPlugins:    []Plugin{{Name: "research", Version: "1.0.0"}},
		EcphoryResult: &EcphoryResult{
			TokenBudgetUsed: 25000,
			Strategy:        "api",
		},
	}

	results := runSanityChecks(ec)

	// Should return 3 checks: plugin_loading, version_compatibility, ecphory_coverage
	if len(results) != 3 {
		t.Errorf("Expected 3 check results, got: %d", len(results))
	}

	// Verify all check types present
	checkTypes := make(map[string]bool)
	for _, result := range results {
		checkTypes[result.CheckType] = true
	}

	if !checkTypes[CheckTypePluginLoading] {
		t.Error("Expected plugin_loading check")
	}
	if !checkTypes[CheckTypeVersionCompatibility] {
		t.Error("Expected version_compatibility check")
	}
	if !checkTypes[CheckTypeEcphoryCoverage] {
		t.Error("Expected ecphory_coverage check")
	}
}

func TestSanityCheckEnricher_Name(t *testing.T) {
	enricher := NewSanityCheckEnricher(nil)
	if enricher.Name() != "sanity_check" {
		t.Errorf("Expected name 'sanity_check', got: %s", enricher.Name())
	}
}
