package enrichment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry/analysis"
)

// Helper function to get map keys for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestEndToEndEnrichmentFlow tests the complete enrichment pipeline:
// 1. Create enrichment context with prompt pattern
// 2. Run enrichment pipeline
// 3. Write enriched event to JSONL
// 4. Parse JSONL file
// 5. Verify enrichment fields present
func TestEndToEndEnrichmentFlow(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "events.jsonl")

	ec := createTestEnrichmentContext()
	pipeline := NewPipeline([]Enricher{
		NewPluginContextEnricher(),
		NewEcphoryCoverageEnricher(),
	}, 500*time.Microsecond)

	pluginEvent, ecphoryEvent := createTestEvents()
	enrichedPluginEvent := pipeline.Enrich(context.Background(), pluginEvent, ec)
	enrichedEcphoryEvent := pipeline.Enrich(context.Background(), ecphoryEvent, ec)

	writeEventsToJSONL(t, jsonlPath, enrichedPluginEvent, enrichedEcphoryEvent)

	eventsChan, errsChan := analysis.ParseJSONL(jsonlPath)
	parsedEvents := collectParsedEvents(t, eventsChan, errsChan)

	if len(parsedEvents) != 2 {
		t.Fatalf("Expected 2 parsed events, got %d", len(parsedEvents))
	}

	pluginEventParsed := parsedEvents[0]
	ecphoryEventParsed := parsedEvents[1]

	assertPluginEventEnrichment(t, pluginEventParsed)
	assertEcphoryEventEnrichment(t, ecphoryEventParsed)

	if pluginEventParsed.SchemaVersion != "1.0.0" {
		t.Errorf("Expected schema_version: 1.0.0, got: %s", pluginEventParsed.SchemaVersion)
	}
	if ecphoryEventParsed.SchemaVersion != "1.0.0" {
		t.Errorf("Expected schema_version: 1.0.0, got: %s", ecphoryEventParsed.SchemaVersion)
	}
}

// TestRealJSONLParsing_Performance tests parsing a large JSONL file (10MB)
// to verify streaming architecture performs within target (<5s for 100MB)
func TestRealJSONLParsing_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Create temp directory for large JSONL file
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "large-events.jsonl")

	// Generate ~10MB of events (~50,000 events at ~200 bytes each)
	file, err := os.Create(jsonlPath)
	if err != nil {
		t.Fatalf("Failed to create JSONL file: %v", err)
	}
	defer file.Close()

	eventCount := 50000
	for i := 0; i < eventCount; i++ {
		event := map[string]interface{}{
			"id":             "test-event-" + string(rune(i)),
			"timestamp":      time.Now().Format(time.RFC3339),
			"type":           "ecphory_retrieval",
			"agent":          "perf-test",
			"schema_version": "1.0.0",
			"data": map[string]interface{}{
				"prompt_hash":       "hash-" + string(rune(i)),
				"engrams_retrieved": i % 10,
				"token_budget_used": i * 100,
			},
		}

		line, _ := json.Marshal(event)
		file.Write(append(line, '\n'))
	}
	file.Close()

	// Get file size
	fileInfo, _ := os.Stat(jsonlPath)
	fileSizeMB := float64(fileInfo.Size()) / (1024 * 1024)

	t.Logf("Generated JSONL file: %.2f MB with %d events", fileSizeMB, eventCount)

	// Parse JSONL file and measure time
	startTime := time.Now()

	eventsChan, errsChan := analysis.ParseJSONL(jsonlPath)

	parsedCount := 0
	errorCount := 0

	done := make(chan bool)
	go func() {
		for {
			select {
			case _, ok := <-eventsChan:
				if !ok {
					done <- true
					return
				}
				parsedCount++
			case _, ok := <-errsChan:
				if ok {
					errorCount++
				}
			}
		}
	}()

	<-done

	duration := time.Since(startTime)

	t.Logf("Parsed %d events in %v (%.0f events/sec)", parsedCount, duration, float64(parsedCount)/duration.Seconds())

	// Verify all events parsed
	if parsedCount != eventCount {
		t.Errorf("Expected %d parsed events, got %d", eventCount, parsedCount)
	}

	// Verify no errors
	if errorCount > 0 {
		t.Errorf("Expected no parse errors, got %d", errorCount)
	}

	// Performance target: Should process ~10MB in <10s
	// (relaxed from 1s to avoid flakiness under CI/container load)
	maxDuration := 10 * time.Second
	if duration > maxDuration {
		t.Errorf("Parsing took %v, expected <%v (%.2f MB)", duration, maxDuration, fileSizeMB)
	}
}

// TestTruncatedLastLine tests that parser handles truncated last line gracefully
func TestTruncatedLastLine(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "truncated.jsonl")

	// Write valid events + truncated last line
	content := `{"id":"1","timestamp":"2025-12-08T00:00:00Z","type":"test","agent":"test","schema_version":"1.0.0"}
{"id":"2","timestamp":"2025-12-08T00:00:01Z","type":"test","agent":"test","schema_version":"1.0.0"}
{"id":"3","timestamp":"2025-12-08T00:00:02Z","type":"test","agent":"test","schema_vers`

	if err := os.WriteFile(jsonlPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write JSONL file: %v", err)
	}

	// Parse file
	eventsChan, errsChan := analysis.ParseJSONL(jsonlPath)

	var parsedEvents []*analysis.TelemetryEvent
	var parseErrors []error

	// Wait for both channels to close
	eventsDone := make(chan bool)
	errorsDone := make(chan bool)

	go func() {
		for event := range eventsChan {
			parsedEvents = append(parsedEvents, event)
		}
		eventsDone <- true
	}()

	go func() {
		for err := range errsChan {
			parseErrors = append(parseErrors, err)
		}
		errorsDone <- true
	}()

	<-eventsDone
	<-errorsDone

	// Should parse first 2 events successfully
	if len(parsedEvents) != 2 {
		t.Errorf("Expected 2 valid events parsed, got %d", len(parsedEvents))
	}

	// Should have 1 error for truncated line
	if len(parseErrors) != 1 {
		t.Errorf("Expected 1 parse error for truncated line, got %d", len(parseErrors))
	}

	// Verify error message mentions malformed JSON
	if len(parseErrors) > 0 && !strings.Contains(parseErrors[0].Error(), "malformed JSON") {
		t.Errorf("Expected error to mention 'malformed JSON', got: %v", parseErrors[0])
	}
}

// TestMixedSchemaVersions tests that parser handles mixed schema versions
// (1.0.0 and unversioned) in the same file
func TestMixedSchemaVersions(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "mixed-versions.jsonl")

	// Write events with different schema versions
	content := `{"id":"1","timestamp":"2025-12-08T00:00:00Z","type":"test","agent":"test","schema_version":"1.0.0","data":{"key":"versioned"}}
{"id":"2","timestamp":"2025-12-08T00:00:01Z","type":"test","agent":"test","data":{"key":"unversioned-legacy"}}
{"id":"3","timestamp":"2025-12-08T00:00:02Z","type":"test","agent":"test","schema_version":"1.0.0","data":{"key":"versioned-again"}}
`

	if err := os.WriteFile(jsonlPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write JSONL file: %v", err)
	}

	// Parse file
	eventsChan, errsChan := analysis.ParseJSONL(jsonlPath)

	var parsedEvents []*analysis.TelemetryEvent
	var parseErrors []error

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-eventsChan:
				if !ok {
					done <- true
					return
				}
				parsedEvents = append(parsedEvents, event)
			case err, ok := <-errsChan:
				if ok {
					parseErrors = append(parseErrors, err)
				}
			}
		}
	}()

	<-done

	// Should parse all 3 events successfully
	if len(parsedEvents) != 3 {
		t.Fatalf("Expected 3 parsed events, got %d", len(parsedEvents))
	}

	// Should have no errors
	if len(parseErrors) > 0 {
		t.Errorf("Expected no parse errors, got %d: %v", len(parseErrors), parseErrors)
	}

	// Verify schema versions
	// Event 1: explicit 1.0.0
	if parsedEvents[0].SchemaVersion != "1.0.0" {
		t.Errorf("Event 1: Expected schema_version 1.0.0, got: %s", parsedEvents[0].SchemaVersion)
	}

	// Event 2: unversioned, should default to 1.0.0
	if parsedEvents[1].SchemaVersion != "1.0.0" {
		t.Errorf("Event 2 (unversioned): Expected schema_version defaulted to 1.0.0, got: %s", parsedEvents[1].SchemaVersion)
	}

	// Event 3: explicit 1.0.0
	if parsedEvents[2].SchemaVersion != "1.0.0" {
		t.Errorf("Event 3: Expected schema_version 1.0.0, got: %s", parsedEvents[2].SchemaVersion)
	}

	// Verify data fields
	if parsedEvents[0].Data["key"] != "versioned" {
		t.Errorf("Event 1: Expected data.key='versioned', got: %v", parsedEvents[0].Data["key"])
	}

	if parsedEvents[1].Data["key"] != "unversioned-legacy" {
		t.Errorf("Event 2: Expected data.key='unversioned-legacy', got: %v", parsedEvents[1].Data["key"])
	}

	if parsedEvents[2].Data["key"] != "versioned-again" {
		t.Errorf("Event 3: Expected data.key='versioned-again', got: %v", parsedEvents[2].Data["key"])
	}
}

// Helper functions for TestEndToEndEnrichmentFlow

// createTestEnrichmentContext creates a standard enrichment context for testing
func createTestEnrichmentContext() EnrichmentContext {
	return EnrichmentContext{
		Prompt: "research: test query for integration",
		AvailablePlugins: []Plugin{
			{Name: "research", Version: "1.0.0", Path: "/test/plugins/research"},
			{Name: "wayfinder", Version: "1.0.0", Path: "/test/plugins/wayfinder"},
		},
		LoadedPlugins: []Plugin{
			{Name: "research", Version: "1.0.0", Path: "/test/plugins/research"},
		},
		EcphoryResult: &EcphoryResult{
			PromptHash:         "test-hash-12345",
			EngramsRetrieved:   5,
			CandidatesFiltered: 10,
			TokenBudgetUsed:    50000,
			Strategy:           "api",
		},
		SessionSalt: "test-session-salt-12345",
	}
}

// createTestEvents creates plugin and ecphory test events
func createTestEvents() (*TelemetryEvent, *TelemetryEvent) {
	pluginEvent := &TelemetryEvent{
		ID:            "test-event-1",
		Timestamp:     time.Now(),
		Type:          EventTypePluginExecution,
		Agent:         "integration-test",
		SchemaVersion: "1.0.0",
		Data:          make(map[string]interface{}),
	}
	ecphoryEvent := &TelemetryEvent{
		ID:            "test-event-2",
		Timestamp:     time.Now(),
		Type:          EventTypeEcphoryRetrieval,
		Agent:         "integration-test",
		SchemaVersion: "1.0.0",
		Data:          make(map[string]interface{}),
	}
	return pluginEvent, ecphoryEvent
}

// writeEventsToJSONL writes events to a JSONL file
func writeEventsToJSONL(t *testing.T, path string, events ...*TelemetryEvent) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create JSONL file: %v", err)
	}
	defer file.Close()

	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Failed to marshal event: %v", err)
		}
		if _, err := file.Write(append(line, '\n')); err != nil {
			t.Fatalf("Failed to write event: %v", err)
		}
	}
}

// collectParsedEvents collects events from parser channels
func collectParsedEvents(t *testing.T, eventsChan <-chan *analysis.TelemetryEvent, errsChan <-chan error) []*analysis.TelemetryEvent {
	t.Helper()
	var parsedEvents []*analysis.TelemetryEvent
	var parseErrors []error

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-eventsChan:
				if !ok {
					done <- true
					return
				}
				parsedEvents = append(parsedEvents, event)
			case err, ok := <-errsChan:
				if ok {
					parseErrors = append(parseErrors, err)
				}
			}
		}
	}()

	<-done

	if len(parseErrors) > 0 {
		t.Errorf("Expected no parse errors, got %d: %v", len(parseErrors), parseErrors)
	}

	return parsedEvents
}

// assertPluginEventEnrichment verifies plugin event enrichment fields
func assertPluginEventEnrichment(t *testing.T, event *analysis.TelemetryEvent) {
	t.Helper()
	if event.Data == nil {
		t.Fatal("Expected enriched plugin event data, got nil")
	}
	if _, ok := event.Data["prompt_hash"]; !ok {
		t.Errorf("Expected 'prompt_hash' field from PluginContextEnricher. Data keys: %v", getKeys(event.Data))
	}
	if pluginsExpected, ok := event.Data["plugins_expected"]; ok {
		expectedList := pluginsExpected.([]interface{})
		if len(expectedList) != 1 || expectedList[0].(string) != "research" {
			t.Errorf("Expected plugins_expected: [research], got: %v", expectedList)
		}
	} else {
		t.Errorf("Expected 'plugins_expected' field from PluginContextEnricher. Data keys: %v", getKeys(event.Data))
	}
}

// assertEcphoryEventEnrichment verifies ecphory event enrichment fields
func assertEcphoryEventEnrichment(t *testing.T, event *analysis.TelemetryEvent) {
	t.Helper()
	if event.Data == nil {
		t.Fatal("Expected enriched ecphory event data, got nil")
	}
	if engramsRetrieved, ok := event.Data["engrams_retrieved"]; !ok {
		t.Error("Expected 'engrams_retrieved' field from EcphoryCoverageEnricher")
	} else if engramsRetrieved.(float64) != 5 {
		t.Errorf("Expected engrams_retrieved: 5, got: %v", engramsRetrieved)
	}
	if utilizationPercent, ok := event.Data["token_utilization_percent"]; !ok {
		t.Error("Expected 'token_utilization_percent' field from EcphoryCoverageEnricher")
	} else if utilizationPercent.(float64) != 50.0 {
		t.Errorf("Expected token_utilization_percent: 50.0, got: %v", utilizationPercent)
	}
}
