package enrichment

import (
	"context"
	"testing"
)

func TestPluginContextEnricher_EnrichPluginExecution(t *testing.T) {
	enricher := NewPluginContextEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: EventTypePluginExecution,
	}

	availablePlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
		{Name: "personas", Version: "1.0.0"},
	}

	loadedPlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
	}

	ec := EnrichmentContext{
		Prompt:           "research: find papers on AI safety",
		AvailablePlugins: availablePlugins,
		LoadedPlugins:    loadedPlugins,
		SessionSalt:      "test-salt",
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, ec)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Check prompt hash exists
	if enrichedEvent.Data["prompt_hash"] == nil {
		t.Error("Expected prompt_hash to be set")
	}

	// Check expected plugins
	expected, ok := enrichedEvent.Data["plugins_expected"].([]string)
	if !ok {
		t.Fatalf("Expected plugins_expected to be []string, got: %T", enrichedEvent.Data["plugins_expected"])
	}

	if len(expected) != 1 || expected[0] != "research" {
		t.Errorf("Expected plugins_expected to be [research], got: %v", expected)
	}

	// Check loaded plugins
	loaded, ok := enrichedEvent.Data["plugins_loaded"].([]string)
	if !ok {
		t.Fatalf("Expected plugins_loaded to be []string, got: %T", enrichedEvent.Data["plugins_loaded"])
	}

	if len(loaded) != 1 || loaded[0] != "research" {
		t.Errorf("Expected plugins_loaded to be [research], got: %v", loaded)
	}

	// Check no missing plugins (research was both expected and loaded)
	if enrichedEvent.Data["plugins_missing"] != nil {
		t.Errorf("Expected no plugins_missing, got: %v", enrichedEvent.Data["plugins_missing"])
	}
}

func TestPluginContextEnricher_MissingPlugin(t *testing.T) {
	enricher := NewPluginContextEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: EventTypePluginExecution,
	}

	availablePlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
		{Name: "personas", Version: "1.0.0"},
	}

	loadedPlugins := []Plugin{} // No plugins loaded

	ec := EnrichmentContext{
		Prompt:           "research: find papers on AI safety",
		AvailablePlugins: availablePlugins,
		LoadedPlugins:    loadedPlugins,
		SessionSalt:      "test-salt",
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, ec)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Check missing plugins
	missing, ok := enrichedEvent.Data["plugins_missing"].([]string)
	if !ok {
		t.Fatalf("Expected plugins_missing to be []string, got: %T", enrichedEvent.Data["plugins_missing"])
	}

	if len(missing) != 1 || missing[0] != "research" {
		t.Errorf("Expected plugins_missing to be [research], got: %v", missing)
	}
}

func TestPluginContextEnricher_NonPluginExecution(t *testing.T) {
	enricher := NewPluginContextEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: "other_event_type",
	}

	enrichedEvent, err := enricher.Enrich(context.Background(), event, EnrichmentContext{})
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Non-plugin_execution events should not be enriched
	if enrichedEvent.Data != nil && len(enrichedEvent.Data) > 0 {
		t.Errorf("Expected no enrichment for non-plugin_execution event, got: %v", enrichedEvent.Data)
	}
}

func TestDetectExpectedPlugins_Research(t *testing.T) {
	availablePlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
	}

	expected := detectExpectedPlugins("research: find papers", availablePlugins)

	if len(expected) != 1 || expected[0] != "research" {
		t.Errorf("Expected [research], got: %v", expected)
	}
}

func TestDetectExpectedPlugins_Wayfinder(t *testing.T) {
	availablePlugins := []Plugin{
		{Name: "wayfinder", Version: "1.0.0"},
	}

	expected := detectExpectedPlugins("wayfinder: help me plan", availablePlugins)

	if len(expected) != 1 || expected[0] != "wayfinder" {
		t.Errorf("Expected [wayfinder], got: %v", expected)
	}
}

func TestDetectExpectedPlugins_Personas(t *testing.T) {
	availablePlugins := []Plugin{
		{Name: "personas", Version: "1.0.0"},
	}

	// Valid persona mention (@ followed by uppercase)
	expected := detectExpectedPlugins("Ask @TechLead about architecture", availablePlugins)

	if len(expected) != 1 || expected[0] != "personas" {
		t.Errorf("Expected [personas], got: %v", expected)
	}
}

func TestDetectExpectedPlugins_EmailFalsePositive(t *testing.T) {
	availablePlugins := []Plugin{
		{Name: "personas", Version: "1.0.0"},
	}

	// Email address should not trigger personas plugin
	expected := detectExpectedPlugins("Contact user@example.com", availablePlugins)

	if len(expected) != 0 {
		t.Errorf("Expected no plugins for email address, got: %v", expected)
	}
}

func TestDetectExpectedPlugins_MinimumLength(t *testing.T) {
	availablePlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
	}

	// Very short prompt should not trigger detection
	expected := detectExpectedPlugins("@", availablePlugins)

	if len(expected) != 0 {
		t.Errorf("Expected no plugins for short prompt, got: %v", expected)
	}
}

func TestDetectExpectedPlugins_Multiple(t *testing.T) {
	availablePlugins := []Plugin{
		{Name: "research", Version: "1.0.0"},
		{Name: "personas", Version: "1.0.0"},
	}

	// Prompt with multiple plugin patterns
	expected := detectExpectedPlugins("research: Ask @TechLead to find papers", availablePlugins)

	if len(expected) != 2 {
		t.Errorf("Expected 2 plugins, got: %v", expected)
	}

	// Check both plugins detected (order may vary)
	hasResearch := false
	hasPersonas := false
	for _, plugin := range expected {
		if plugin == "research" {
			hasResearch = true
		}
		if plugin == "personas" {
			hasPersonas = true
		}
	}

	if !hasResearch || !hasPersonas {
		t.Errorf("Expected both research and personas, got: %v", expected)
	}
}

func TestHashPrompt_Privacy(t *testing.T) {
	// Note: Hash is now non-deterministic due to crypto/rand nonce (P1-1 security fix)
	// This is better for security - each hash is unique even for same input

	hash1 := hashPrompt("test prompt", "salt1")
	hash2 := hashPrompt("test prompt", "salt2")

	// Different salts should produce different hashes (privacy)
	if hash1 == hash2 {
		t.Error("Expected different hashes for different salts")
	}

	// Hashes should be 64 characters (SHA-256 hex)
	if len(hash1) != 64 {
		t.Errorf("Expected hash length 64, got: %d", len(hash1))
	}

	// Multiple calls with same input should produce different hashes (nonce randomness)
	hash3 := hashPrompt("test prompt", "salt1")
	if hash1 == hash3 {
		t.Error("Expected different hashes for same input (nonce should be random)")
	}
}

func TestCalculateMissingPlugins(t *testing.T) {
	expected := []string{"research", "personas", "wayfinder"}
	loaded := []string{"research"}

	missing := calculateMissingPlugins(expected, loaded)

	if len(missing) != 2 {
		t.Errorf("Expected 2 missing plugins, got: %v", missing)
	}

	// Check personas and wayfinder are missing
	hasPersonas := false
	hasWayfinder := false
	for _, plugin := range missing {
		if plugin == "personas" {
			hasPersonas = true
		}
		if plugin == "wayfinder" {
			hasWayfinder = true
		}
	}

	if !hasPersonas || !hasWayfinder {
		t.Errorf("Expected personas and wayfinder to be missing, got: %v", missing)
	}
}

func TestPluginContextEnricher_ThreadSafety(t *testing.T) {
	enricher := NewPluginContextEnricher()

	event := &TelemetryEvent{
		ID:   "test-id",
		Type: EventTypePluginExecution,
	}

	ec := EnrichmentContext{
		Prompt:           "research: test",
		AvailablePlugins: []Plugin{{Name: "research", Version: "1.0.0"}},
		LoadedPlugins:    []Plugin{{Name: "research", Version: "1.0.0"}},
		SessionSalt:      "test-salt",
	}

	// Run concurrent enrichments
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			_, err := enricher.Enrich(context.Background(), event, ec)
			if err != nil {
				t.Errorf("Enrich failed: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Test passes if no race condition detected with -race flag
}

func TestPluginContextEnricher_Name(t *testing.T) {
	enricher := NewPluginContextEnricher()
	if enricher.Name() != "plugin_context" {
		t.Errorf("Expected name 'plugin_context', got: %s", enricher.Name())
	}
}
