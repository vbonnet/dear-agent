package analysis

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestParseJSONL_ValidFile tests parsing a well-formed JSONL file
func TestParseJSONL_ValidFile(t *testing.T) {
	// Create temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:00Z","type":"plugin_executed","agent":"claude-code","schema_version":"1.0.0","data":{"plugin":"wayfinder"}}
{"id":"evt-2","timestamp":"2025-11-26T14:31:00Z","type":"engram_loaded","agent":"claude-code","schema_version":"1.0.0","data":{"engram":"test"}}
{"id":"evt-3","timestamp":"2025-11-26T14:32:00Z","type":"session_end","agent":"claude-code","schema_version":"1.0.0"}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Parse file and collect results
	eventsChan, errsChan := ParseJSONL(testFile)
	events, errors := collectEventsAndErrors(eventsChan, errsChan)

	// Verify results
	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}

	if len(events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(events))
	}

	// Verify events using helpers
	assertEventFields(t, events[0], "evt-1", "plugin_executed", "claude-code", "1.0.0")
	assertEventDataString(t, events[0], "plugin", "wayfinder")

	assertEventFields(t, events[1], "evt-2", "engram_loaded", "claude-code", "1.0.0")

	assertEventFields(t, events[2], "evt-3", "session_end", "claude-code", "1.0.0")
}

// TestParseJSONL_EmptyLines tests resilience to empty lines
func TestParseJSONL_EmptyLines(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:00Z","type":"test","agent":"test"}

{"id":"evt-2","timestamp":"2025-11-26T14:31:00Z","type":"test","agent":"test"}

`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	eventsChan, errsChan := ParseJSONL(testFile)

	events := make([]*TelemetryEvent, 0)
	errors := make([]error, 0)

	for {
		select {
		case event, ok := <-eventsChan:
			if !ok {
				eventsChan = nil
			} else {
				events = append(events, event)
			}
		case err, ok := <-errsChan:
			if !ok {
				errsChan = nil
			} else {
				errors = append(errors, err)
			}
		}

		if eventsChan == nil && errsChan == nil {
			break
		}
	}

	// Should skip empty lines without errors
	if len(errors) > 0 {
		t.Errorf("Expected no errors for empty lines, got %d: %v", len(errors), errors)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events (empty lines skipped), got %d", len(events))
	}
}

// TestParseJSONL_MalformedJSON tests resilience to malformed JSON
func TestParseJSONL_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:00Z","type":"test","agent":"test"}
{invalid json here}
{"id":"evt-2","timestamp":"2025-11-26T14:31:00Z","type":"test","agent":"test"}
{"incomplete":
{"id":"evt-3","timestamp":"2025-11-26T14:32:00Z","type":"test","agent":"test"}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	eventsChan, errsChan := ParseJSONL(testFile)

	events := make([]*TelemetryEvent, 0)
	errors := make([]error, 0)

	for {
		select {
		case event, ok := <-eventsChan:
			if !ok {
				eventsChan = nil
			} else {
				events = append(events, event)
			}
		case err, ok := <-errsChan:
			if !ok {
				errsChan = nil
			} else {
				errors = append(errors, err)
			}
		}

		if eventsChan == nil && errsChan == nil {
			break
		}
	}

	// Should parse valid lines and report errors for invalid ones
	if len(errors) == 0 {
		t.Error("Expected errors for malformed JSON, got none")
	}

	// Should successfully parse 3 valid events despite 2 malformed lines
	if len(events) != 3 {
		t.Fatalf("Expected 3 valid events, got %d", len(events))
	}

	// Verify we got the correct events
	expectedIDs := []string{"evt-1", "evt-2", "evt-3"}
	for i, event := range events {
		if event.ID != expectedIDs[i] {
			t.Errorf("Event %d: expected ID '%s', got '%s'", i, expectedIDs[i], event.ID)
		}
	}
}

// TestParseJSONL_MissingSchemaVersion tests backward compatibility
func TestParseJSONL_MissingSchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	// Events without schema_version field (old format)
	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:00Z","type":"test","agent":"test"}
{"id":"evt-2","timestamp":"2025-11-26T14:31:00Z","type":"test","agent":"test","schema_version":"2.0.0"}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	eventsChan, errsChan := ParseJSONL(testFile)

	events := make([]*TelemetryEvent, 0)

	for event := range eventsChan {
		events = append(events, event)
	}

	// Drain errors channel
	for range errsChan {
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// First event should default to 1.0.0
	if events[0].SchemaVersion != "1.0.0" {
		t.Errorf("Event 1: expected default schema version '1.0.0', got '%s'", events[0].SchemaVersion)
	}

	// Second event should preserve its version
	if events[1].SchemaVersion != "2.0.0" {
		t.Errorf("Event 2: expected schema version '2.0.0', got '%s'", events[1].SchemaVersion)
	}
}

// TestParseJSONL_FileNotFound tests error handling for missing files
func TestParseJSONL_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nonexistent.jsonl")

	eventsChan, errsChan := ParseJSONL(testFile)

	events := make([]*TelemetryEvent, 0)
	errors := make([]error, 0)

	for {
		select {
		case event, ok := <-eventsChan:
			if !ok {
				eventsChan = nil
			} else {
				events = append(events, event)
			}
		case err, ok := <-errsChan:
			if !ok {
				errsChan = nil
			} else {
				errors = append(errors, err)
			}
		}

		if eventsChan == nil && errsChan == nil {
			break
		}
	}

	// Should report file not found error
	if len(errors) != 1 {
		t.Fatalf("Expected 1 error (file not found), got %d", len(errors))
	}

	// Should have no events
	if len(events) != 0 {
		t.Errorf("Expected 0 events, got %d", len(events))
	}
}

// TestParseJSONLSync_ValidFile tests synchronous parsing
func TestParseJSONLSync_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:00Z","type":"test","agent":"test"}
{"id":"evt-2","timestamp":"2025-11-26T14:31:00Z","type":"test","agent":"test"}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	events, err := ParseJSONLSync(testFile)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("Expected 2 events, got %d", len(events))
	}

	// Verify events
	if events[0].ID != "evt-1" {
		t.Errorf("Event 1: expected ID 'evt-1', got '%s'", events[0].ID)
	}
	if events[1].ID != "evt-2" {
		t.Errorf("Event 2: expected ID 'evt-2', got '%s'", events[1].ID)
	}
}

// TestParseJSONLSync_MalformedJSON tests error reporting in sync mode
func TestParseJSONLSync_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:00Z","type":"test","agent":"test"}
{invalid json}
{"id":"evt-2","timestamp":"2025-11-26T14:31:00Z","type":"test","agent":"test"}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	events, err := ParseJSONLSync(testFile)

	// Should return both valid events and an error for the malformed line
	// The function returns an error but still provides all successfully parsed events
	if err == nil {
		t.Error("Expected error for malformed JSON, got nil")
	} else if err.Error() == "" {
		// Verify error message mentions the parsing errors
		t.Error("Error message should not be empty")
	}

	// Should still return the valid events
	if len(events) != 2 {
		t.Fatalf("Expected 2 valid events, got %d", len(events))
	}

	// Verify we got the correct events
	if events[0].ID != "evt-1" {
		t.Errorf("Event 1: expected ID 'evt-1', got '%s'", events[0].ID)
	}
	if events[1].ID != "evt-2" {
		t.Errorf("Event 2: expected ID 'evt-2', got '%s'", events[1].ID)
	}
}

// TestParseJSONL_TimestampParsing tests timestamp parsing
func TestParseJSONL_TimestampParsing(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:22.123456Z","type":"test","agent":"test"}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	eventsChan, errsChan := ParseJSONL(testFile)

	var event *TelemetryEvent
	for e := range eventsChan {
		event = e
	}

	// Drain errors
	for range errsChan {
	}

	if event == nil {
		t.Fatal("Expected 1 event, got none")
	}

	// Verify timestamp was parsed correctly
	expectedTime, _ := time.Parse(time.RFC3339Nano, "2025-11-26T14:30:22.123456Z")
	if !event.Timestamp.Equal(expectedTime) {
		t.Errorf("Expected timestamp %v, got %v", expectedTime, event.Timestamp)
	}
}

// TestParseJSONL_LargeFile tests performance with many events
func TestParseJSONL_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.jsonl")

	// Create file with 1000 events
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	for i := 0; i < 1000; i++ {
		line := `{"id":"evt-%d","timestamp":"2025-11-26T14:30:00Z","type":"test","agent":"test"}` + "\n"
		if _, err := f.WriteString(line); err != nil {
			t.Fatalf("Failed to write event %d: %v", i, err)
		}
	}
	f.Close()

	// Parse file
	eventsChan, errsChan := ParseJSONL(testFile)

	eventCount := 0
	for range eventsChan {
		eventCount++
	}

	// Drain errors
	errorCount := 0
	for range errsChan {
		errorCount++
	}

	if errorCount > 0 {
		t.Errorf("Expected no errors, got %d", errorCount)
	}

	if eventCount != 1000 {
		t.Errorf("Expected 1000 events, got %d", eventCount)
	}
}

// TestParseJSONL_DataTypes tests various data types in event.Data
func TestParseJSONL_DataTypes(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "events.jsonl")

	content := `{"id":"evt-1","timestamp":"2025-11-26T14:30:00Z","type":"test","agent":"test","data":{"string":"value","number":42,"float":3.14,"bool":true,"null":null,"array":[1,2,3],"object":{"nested":"value"}}}
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	eventsChan, errsChan := ParseJSONL(testFile)
	events, errors := collectEventsAndErrors(eventsChan, errsChan)

	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]

	// Verify data types
	assertEventDataString(t, event, "string", "value")
	assertEventDataNumber(t, event, "number", 42)
	assertEventDataFloat(t, event, "float", 3.14)
	assertEventDataBool(t, event, "bool", true)
	assertEventDataNull(t, event, "null")
	assertEventDataArray(t, event, "array", 3)
	assertEventDataObject(t, event, "object", "nested", "value")
}

// Test helper functions for TestParseJSONL_ValidFile

// collectEventsAndErrors collects all events and errors from parser channels
func collectEventsAndErrors(eventsChan <-chan *TelemetryEvent, errsChan <-chan error) ([]*TelemetryEvent, []error) {
	events := make([]*TelemetryEvent, 0)
	errors := make([]error, 0)

	for {
		select {
		case event, ok := <-eventsChan:
			if !ok {
				eventsChan = nil
			} else {
				events = append(events, event)
			}
		case err, ok := <-errsChan:
			if !ok {
				errsChan = nil
			} else {
				errors = append(errors, err)
			}
		}

		if eventsChan == nil && errsChan == nil {
			break
		}
	}

	return events, errors
}

// assertEventFields verifies basic event fields (ID, Type, Agent, SchemaVersion)
func assertEventFields(t *testing.T, event *TelemetryEvent, id, eventType, agent, schema string) {
	t.Helper()
	if event.ID != id {
		t.Errorf("Expected ID '%s', got '%s'", id, event.ID)
	}
	if event.Type != eventType {
		t.Errorf("Expected type '%s', got '%s'", eventType, event.Type)
	}
	if agent != "" && event.Agent != agent {
		t.Errorf("Expected agent '%s', got '%s'", agent, event.Agent)
	}
	if schema != "" && event.SchemaVersion != schema {
		t.Errorf("Expected schema version '%s', got '%s'", schema, event.SchemaVersion)
	}
}

// assertEventDataString verifies a string value in event.Data
func assertEventDataString(t *testing.T, event *TelemetryEvent, key, expectedValue string) {
	t.Helper()
	if value, ok := event.Data[key].(string); !ok || value != expectedValue {
		t.Errorf("Expected data.%s='%s', got %v", key, expectedValue, event.Data[key])
	}
}

// assertEventDataNumber verifies a number value in event.Data
func assertEventDataNumber(t *testing.T, event *TelemetryEvent, key string, expectedValue float64) {
	t.Helper()
	if value, ok := event.Data[key].(float64); !ok || value != expectedValue {
		t.Errorf("Expected data.%s=%v, got %v", key, expectedValue, event.Data[key])
	}
}

// assertEventDataFloat verifies a float value in event.Data
func assertEventDataFloat(t *testing.T, event *TelemetryEvent, key string, expectedValue float64) {
	t.Helper()
	if value, ok := event.Data[key].(float64); !ok || value != expectedValue {
		t.Errorf("Expected data.%s=%v, got %v", key, expectedValue, event.Data[key])
	}
}

// assertEventDataBool verifies a boolean value in event.Data
func assertEventDataBool(t *testing.T, event *TelemetryEvent, key string, expectedValue bool) {
	t.Helper()
	if value, ok := event.Data[key].(bool); !ok || value != expectedValue {
		t.Errorf("Expected data.%s=%v, got %v", key, expectedValue, event.Data[key])
	}
}

// assertEventDataNull verifies a null value in event.Data
func assertEventDataNull(t *testing.T, event *TelemetryEvent, key string) {
	t.Helper()
	if event.Data[key] != nil {
		t.Errorf("Expected data.%s=nil, got %v", key, event.Data[key])
	}
}

// assertEventDataArray verifies an array value in event.Data
func assertEventDataArray(t *testing.T, event *TelemetryEvent, key string, expectedLength int) {
	t.Helper()
	if arr, ok := event.Data[key].([]interface{}); !ok || len(arr) != expectedLength {
		t.Errorf("Expected data.%s to be array with length %d, got %v", key, expectedLength, event.Data[key])
	}
}

// assertEventDataObject verifies an object value with nested key in event.Data
func assertEventDataObject(t *testing.T, event *TelemetryEvent, key, nestedKey, nestedValue string) {
	t.Helper()
	if obj, ok := event.Data[key].(map[string]interface{}); !ok {
		t.Errorf("Expected data.%s to be object, got %v", key, event.Data[key])
	} else if nested, ok := obj[nestedKey].(string); !ok || nested != nestedValue {
		t.Errorf("Expected data.%s.%s='%s', got %v", key, nestedKey, nestedValue, obj[nestedKey])
	}
}
