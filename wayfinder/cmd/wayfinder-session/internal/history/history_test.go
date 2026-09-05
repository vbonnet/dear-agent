package history

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	h := New("/tmp/test")
	expected := filepath.Join("/tmp/test", HistoryFilename)
	if h.path != expected {
		t.Errorf("New() path = %q, want %q", h.path, expected)
	}
}

func TestAppendEvent(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Append event
	data := map[string]any{
		"key": "value",
		"num": 42,
	}
	err := h.AppendEvent(EventTypePhaseStarted, "PROBLEM", data)
	if err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(h.path); os.IsNotExist(err) {
		t.Fatal("History file was not created")
	}

	// Read back events
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Read() returned %d events, want 1", len(events))
	}

	event := events[0]
	if event.Type != EventTypePhaseStarted {
		t.Errorf("event.Type = %q, want %q", event.Type, EventTypePhaseStarted)
	}
	if event.Phase != "PROBLEM" {
		t.Errorf("event.Phase = %q, want %q", event.Phase, "PROBLEM")
	}
	if event.Data["key"] != "value" {
		t.Errorf("event.Data[key] = %v, want %q", event.Data["key"], "value")
	}
	// JSON numbers are unmarshaled as float64
	if event.Data["num"] != float64(42) {
		t.Errorf("event.Data[num] = %v, want %v", event.Data["num"], float64(42))
	}
}

func TestAppendEventSanitizesNewDataWithoutMutationOrHistoryRewrite(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := t.TempDir()
	h := New(tmpDir)
	h.userHomeDir = func() (string, error) { return homeDir, nil }

	legacyBytes := []byte("{\"data\":{\"path\":\"/legacy/absolute/path\"}}\n")
	if err := os.WriteFile(h.path, legacyBytes, 0o600); err != nil {
		t.Fatalf("write existing history: %v", err)
	}

	data := map[string]any{
		"project":       tmpDir,
		"project_child": filepath.Join(tmpDir, "nested", "file.txt"),
		"nested": []any{
			map[string]any{
				"project_message": "failed at " + tmpDir + ", retrying",
				"home_message":    "home=" + homeDir + ": unavailable",
			},
		},
		"prefix_collision": tmpDir + "-old",
		"large_number":     int64(9007199254740993),
		"number":           42,
	}
	if filepath.Separator == '/' {
		data["backslash_collision"] = tmpDir + `\unrelated`
	}
	before, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal(data) error = %v", err)
	}

	if err := h.AppendEvent(EventTypePhaseStarted, "BUILD", data); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}
	after, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal(data after append) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("AppendEvent() mutated caller data:\n got %s\nwant %s", after, before)
	}

	persisted, err := os.ReadFile(h.path)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if !bytes.HasPrefix(persisted, legacyBytes) {
		t.Fatalf("existing history bytes changed:\n got %q\nwant prefix %q", persisted, legacyBytes)
	}
	if !bytes.Contains(persisted[len(legacyBytes):], []byte(`"large_number":9007199254740993`)) {
		t.Fatalf("appended event did not preserve the exact large integer: %s", persisted[len(legacyBytes):])
	}

	var event Event
	if err := json.Unmarshal(bytes.TrimSpace(persisted[len(legacyBytes):]), &event); err != nil {
		t.Fatalf("decode appended event: %v", err)
	}
	if got, want := event.Data["project"], "$PROJECT_DIR"; got != want {
		t.Errorf("project = %q, want %q", got, want)
	}
	if got, want := event.Data["project_child"], filepath.Join("$PROJECT_DIR", "nested", "file.txt"); got != want {
		t.Errorf("project_child = %q, want %q", got, want)
	}
	nested, ok := event.Data["nested"].([]any)
	if !ok || len(nested) != 1 {
		t.Fatalf("nested = %#v, want one-element slice", event.Data["nested"])
	}
	messages, ok := nested[0].(map[string]any)
	if !ok {
		t.Fatalf("nested[0] = %#v, want map", nested[0])
	}
	if got, want := messages["project_message"], "failed at $PROJECT_DIR, retrying"; got != want {
		t.Errorf("project_message = %q, want %q", got, want)
	}
	if got, want := messages["home_message"], "home=$HOME: unavailable"; got != want {
		t.Errorf("home_message = %q, want %q", got, want)
	}
	if got, want := event.Data["prefix_collision"], tmpDir+"-old"; got != want {
		t.Errorf("prefix_collision = %q, want %q", got, want)
	}
	if filepath.Separator == '/' {
		if got, want := event.Data["backslash_collision"], tmpDir+`\unrelated`; got != want {
			t.Errorf("backslash_collision = %q, want %q", got, want)
		}
	}
}

func TestAppendEventSucceedsWhenHomeDirectoryCannotBeResolved(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)
	h.userHomeDir = func() (string, error) {
		return "", errors.New("home unavailable")
	}

	err := h.AppendEvent(EventTypePhaseStarted, "BUILD", map[string]any{"path": "/host/private/path"})
	if err != nil {
		t.Fatalf("AppendEvent() unexpected error = %v", err)
	}
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Read() returned %d events, want 1", len(events))
	}
}

func TestReplacePathRootBoundaries(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "project")
	descendant := root + string(filepath.Separator) + "child"
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "exact root", value: root, want: "$PROJECT_DIR"},
		{name: "descendant", value: descendant, want: "$PROJECT_DIR" + string(filepath.Separator) + "child"},
		{name: "comma terminator", value: "failed at " + root + ", retrying", want: "failed at $PROJECT_DIR, retrying"},
		{name: "colon terminator", value: "path=" + root + ": unavailable", want: "path=$PROJECT_DIR: unavailable"},
		{name: "sentence terminator", value: "failed at " + root + ".", want: "failed at $PROJECT_DIR."},
		{name: "hyphen continuation", value: root + "-old", want: root + "-old"},
		{name: "dot continuation", value: root + ".old", want: root + ".old"},
		{name: "underscore continuation", value: root + "_old", want: root + "_old"},
		{name: "embedded prefix", value: "x" + root, want: "x" + root},
		{name: "repeated roots", value: root + " then " + descendant, want: "$PROJECT_DIR then $PROJECT_DIR" + string(filepath.Separator) + "child"},
	}
	if filepath.Separator == '/' {
		tests = append(tests, struct {
			name  string
			value string
			want  string
		}{name: "backslash is not a POSIX descendant", value: root + `\unrelated`, want: root + `\unrelated`})
	}
	if filepath.Separator == '\\' {
		tests = append(tests, struct {
			name  string
			value string
			want  string
		}{name: "forward slashes throughout Windows root", value: filepath.ToSlash(root) + "/child", want: "$PROJECT_DIR/child"})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replacePathRoot(tt.value, root, "$PROJECT_DIR"); got != tt.want {
				t.Errorf("replacePathRoot(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestMatchWindowsPathRootTreatsCaseAndSeparatorsAsEquivalent(t *testing.T) {
	value := `c:/users/agent/project/private.txt`
	root := `C:\Users\Agent\Project`
	matchedLength, ok := matchWindowsPathRoot(value, root)
	if !ok {
		t.Fatal("matchWindowsPathRoot() did not match a case-folded forward-slash root")
	}
	if want := len(`c:/users/agent/project`); matchedLength != want {
		t.Errorf("matchWindowsPathRoot() length = %d, want %d", matchedLength, want)
	}
}

func TestSanitizeValuePrefersProjectPlaceholderWithinHome(t *testing.T) {
	homeDir := filepath.Join(string(filepath.Separator), "users", "agent")
	projectDir := filepath.Join(homeDir, "project")
	input := map[string]any{
		"project": filepath.Join(projectDir, "artifact.md"),
		"home":    filepath.Join(homeDir, "other.md"),
	}

	got, ok := sanitizeValue(input, projectDir, homeDir).(map[string]any)
	if !ok {
		t.Fatalf("sanitizeValue() = %#v, want map", got)
	}
	if want := filepath.Join("$PROJECT_DIR", "artifact.md"); got["project"] != want {
		t.Errorf("project = %q, want %q", got["project"], want)
	}
	if want := filepath.Join("$HOME", "other.md"); got["home"] != want {
		t.Errorf("home = %q, want %q", got["home"], want)
	}
}

func TestAppendEventMigratesLegacyHistoryBeforeWriting(t *testing.T) {
	tmpDir := t.TempDir()
	legacyPath := filepath.Join(tmpDir, LegacyHistoryFilename)
	legacy := Event{Timestamp: time.Now(), Type: EventTypeSessionStarted}
	legacyData, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	if err := os.WriteFile(legacyPath, append(legacyData, '\n'), 0o600); err != nil {
		t.Fatalf("write legacy history: %v", err)
	}

	h := New(tmpDir)
	if err := h.AppendEvent(EventTypePhaseStarted, "PROBLEM", nil); err != nil {
		t.Fatalf("AppendEvent() error: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy history stat error = %v, want not exists", err)
	}
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if len(events) != 2 || events[0].Type != EventTypeSessionStarted || events[1].Type != EventTypePhaseStarted {
		t.Fatalf("migrated events = %#v, want legacy event followed by appended event", events)
	}
}

func TestAppendEventRejectsAmbiguousDualHistoryFiles(t *testing.T) {
	tmpDir := t.TempDir()
	currentPath := filepath.Join(tmpDir, HistoryFilename)
	legacyPath := filepath.Join(tmpDir, LegacyHistoryFilename)
	currentData := []byte("{\"type\":\"current\"}\n")
	legacyData := []byte("{\"type\":\"legacy\"}\n")
	if err := os.WriteFile(currentPath, currentData, 0o600); err != nil {
		t.Fatalf("write current history: %v", err)
	}
	if err := os.WriteFile(legacyPath, legacyData, 0o600); err != nil {
		t.Fatalf("write legacy history: %v", err)
	}

	err := New(tmpDir).AppendEvent(EventTypePhaseStarted, "PROBLEM", nil)
	if err == nil || !strings.Contains(err.Error(), "ambiguous history state") {
		t.Fatalf("AppendEvent() error = %v, want ambiguous history state", err)
	}
	if !errors.Is(err, ErrAmbiguousHistory) {
		t.Fatalf("AppendEvent() error = %v, want ErrAmbiguousHistory", err)
	}
	for path, want := range map[string][]byte{currentPath: currentData, legacyPath: legacyData} {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s changed: got %q, want %q", path, got, want)
		}
	}
}

func TestAppendEvent_Multiple(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Append multiple events
	events := []struct {
		eventType string
		phase     string
	}{
		{EventTypeSessionStarted, ""},
		{EventTypePhaseStarted, "PROBLEM"},
		{EventTypePhaseCompleted, "PROBLEM"},
		{EventTypePhaseStarted, "RESEARCH"},
	}

	for _, e := range events {
		if err := h.AppendEvent(e.eventType, e.phase, nil); err != nil {
			t.Fatalf("AppendEvent(%q, %q) error = %v", e.eventType, e.phase, err)
		}
	}

	// Read all events
	readEvents, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(readEvents) != len(events) {
		t.Fatalf("Read() returned %d events, want %d", len(readEvents), len(events))
	}

	// Verify events in order
	for i, expected := range events {
		if readEvents[i].Type != expected.eventType {
			t.Errorf("event[%d].Type = %q, want %q", i, readEvents[i].Type, expected.eventType)
		}
		if readEvents[i].Phase != expected.phase {
			t.Errorf("event[%d].Phase = %q, want %q", i, readEvents[i].Phase, expected.phase)
		}
	}
}

func TestRead_EmptyHistory(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Read without creating file
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Read() on non-existent file returned %d events, want 0", len(events))
	}
}

func TestRead_CorruptedLine(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Write valid event
	if err := h.AppendEvent(EventTypePhaseStarted, "PROBLEM", nil); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	// Manually append corrupted line
	file, err := os.OpenFile(h.path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	file.WriteString("{ corrupted json\n")
	file.Close()

	// Write another valid event
	if err := h.AppendEvent(EventTypePhaseCompleted, "PROBLEM", nil); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	// Read should skip corrupted line and return valid events
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	// Should have 2 valid events (corrupted line skipped)
	if len(events) != 2 {
		t.Errorf("Read() returned %d events, want 2 (corrupted line should be skipped)", len(events))
	}
}

func TestAppendEvent_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Concurrent writes using O_APPEND should be safe
	const numGoroutines = 10
	const eventsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				data := map[string]interface{}{
					"goroutine": id,
					"event":     j,
				}
				if err := h.AppendEvent(EventTypePhaseStarted, "PROBLEM", data); err != nil {
					t.Errorf("AppendEvent() error in goroutine %d: %v", id, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Read all events
	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	expectedCount := numGoroutines * eventsPerGoroutine
	if len(events) != expectedCount {
		t.Errorf("Read() returned %d events, want %d", len(events), expectedCount)
	}
}

func TestGetEventsByPhase(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Add events for different phases
	h.AppendEvent(EventTypePhaseStarted, "PROBLEM", nil)
	h.AppendEvent(EventTypePhaseCompleted, "PROBLEM", nil)
	h.AppendEvent(EventTypePhaseStarted, "RESEARCH", nil)
	h.AppendEvent(EventTypePhaseCompleted, "RESEARCH", nil)
	h.AppendEvent(EventTypePhaseStarted, "DESIGN", nil)

	// Get events for RESEARCH
	events, err := h.GetEventsByPhase("RESEARCH")
	if err != nil {
		t.Fatalf("GetEventsByPhase() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("GetEventsByPhase(RESEARCH) returned %d events, want 2", len(events))
	}

	// Verify all events are for RESEARCH
	for i, event := range events {
		if event.Phase != "RESEARCH" {
			t.Errorf("event[%d].Phase = %q, want %q", i, event.Phase, "RESEARCH")
		}
	}
}

func TestGetEventsByType(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	// Add events of different types
	h.AppendEvent(EventTypeSessionStarted, "", nil)
	h.AppendEvent(EventTypePhaseStarted, "PROBLEM", nil)
	h.AppendEvent(EventTypePhaseCompleted, "PROBLEM", nil)
	h.AppendEvent(EventTypePhaseStarted, "RESEARCH", nil)
	h.AppendEvent(EventTypeValidationFailed, "DESIGN", nil)

	// Get all wayfinder.phase.started events
	events, err := h.GetEventsByType(EventTypePhaseStarted)
	if err != nil {
		t.Fatalf("GetEventsByType() error = %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("GetEventsByType(wayfinder.phase.started) returned %d events, want 2", len(events))
	}

	// Verify all events are wayfinder.phase.started
	for i, event := range events {
		if event.Type != EventTypePhaseStarted {
			t.Errorf("event[%d].Type = %q, want %q", i, event.Type, EventTypePhaseStarted)
		}
	}
}

func TestEvent_Timestamp(t *testing.T) {
	tmpDir := t.TempDir()
	h := New(tmpDir)

	before := time.Now()
	time.Sleep(1 * time.Millisecond) // Ensure timestamp is after 'before'

	if err := h.AppendEvent(EventTypePhaseStarted, "PROBLEM", nil); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	time.Sleep(1 * time.Millisecond) // Ensure timestamp is before 'after'
	after := time.Now()

	events, err := h.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("Read() returned %d events, want 1", len(events))
	}

	timestamp := events[0].Timestamp
	if timestamp.Before(before) || timestamp.After(after) {
		t.Errorf("event.Timestamp = %v, want between %v and %v", timestamp, before, after)
	}
}
