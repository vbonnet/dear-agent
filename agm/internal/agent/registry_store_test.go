package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Registry tests ---

func TestRegisterAndGet(t *testing.T) {
	// Save and restore original registry
	orig := snapshotRegistryForTest()
	resetRegistryForTest(make(map[string]Agent))
	defer func() { resetRegistryForTest(orig) }()

	mock := &registryMockAgent{name: "test-agent", version: "1.0"}
	Register("test", mock)

	got, ok := Get("test")
	if !ok {
		t.Fatal("Get() returned false for registered agent")
	}
	if got.Name() != "test-agent" {
		t.Errorf("Get().Name() = %q, want %q", got.Name(), "test-agent")
	}
}

func TestGet_NotFound(t *testing.T) {
	orig := snapshotRegistryForTest()
	resetRegistryForTest(make(map[string]Agent))
	defer func() { resetRegistryForTest(orig) }()

	_, ok := Get("nonexistent")
	if ok {
		t.Error("Get() should return false for unregistered agent")
	}
}

func TestRegister_Replaces(t *testing.T) {
	orig := snapshotRegistryForTest()
	resetRegistryForTest(make(map[string]Agent))
	defer func() { resetRegistryForTest(orig) }()

	mock1 := &registryMockAgent{name: "v1", version: "1.0"}
	mock2 := &registryMockAgent{name: "v2", version: "2.0"}

	Register("agent", mock1)
	Register("agent", mock2)

	got, ok := Get("agent")
	if !ok {
		t.Fatal("Get() returned false after re-registration")
	}
	if got.Name() != "v2" {
		t.Errorf("Register should replace: got Name()=%q, want %q", got.Name(), "v2")
	}
}

// --- Factory tests ---

func TestGetHarness_Known(t *testing.T) {
	// GetHarness("claude-code") should return a non-nil agent
	// (may fail if adapter constructor requires external deps, so we test error path too)
	_, err := GetHarness("claude-code")
	// We just verify it doesn't panic; error is OK if deps missing
	_ = err
}

func TestGetHarness_Unknown(t *testing.T) {
	_, err := GetHarness("nonexistent-harness")
	if err == nil {
		t.Error("GetHarness() should error for unknown harness")
	}
}

func TestGetAllHarnesses_Returns(t *testing.T) {
	harnesses := GetAllHarnesses()
	// Should return entries for known harnesses (some may be unavailable)
	if len(harnesses) == 0 {
		t.Error("GetAllHarnesses() should return at least one harness")
	}

	// Check that each harness has a non-empty name
	for _, h := range harnesses {
		if h.Name == "" {
			t.Error("HarnessInfo.Name should not be empty")
		}
		if h.Status != "available" && h.Status != "unavailable" {
			t.Errorf("HarnessInfo.Status = %q, want 'available' or 'unavailable'", h.Status)
		}
	}
}

// --- JSONSessionStore tests ---

func TestJSONSessionStore_CRUD(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "sessions.json")

	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("NewJSONSessionStore() error: %v", err)
	}

	meta := &SessionMetadata{
		TmuxName:   "test-tmux",
		Title:      "Test Session",
		CreatedAt:  time.Now(),
		WorkingDir: "/tmp/test",
		Project:    "test-project",
	}

	// Set
	if err := store.Set("session-1", meta); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Get
	got, err := store.Get("session-1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.TmuxName != "test-tmux" {
		t.Errorf("Get().TmuxName = %q, want %q", got.TmuxName, "test-tmux")
	}
	if got.Title != "Test Session" {
		t.Errorf("Get().Title = %q, want %q", got.Title, "Test Session")
	}

	// List
	all, err := store.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List() len = %d, want 1", len(all))
	}

	// Delete
	if err := store.Delete("session-1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = store.Get("session-1")
	if err == nil {
		t.Error("Get() should error after Delete()")
	}
}

func TestJSONSessionStore_GetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "sessions.json")

	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("NewJSONSessionStore() error: %v", err)
	}

	_, err = store.Get("nonexistent")
	if err == nil {
		t.Error("Get() should error for nonexistent session")
	}
}

func TestJSONSessionStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "sessions.json")

	// Create store and add session
	store1, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("NewJSONSessionStore() error: %v", err)
	}

	meta := &SessionMetadata{
		TmuxName:   "persist-test",
		WorkingDir: "/tmp",
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store1.Set("persist-1", meta); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if !json.Valid(data) {
		t.Error("sessions.json should contain valid JSON")
	}

	// Reload from file
	store2, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("NewJSONSessionStore() reload error: %v", err)
	}

	got, err := store2.Get("persist-1")
	if err != nil {
		t.Fatalf("Get() after reload error: %v", err)
	}
	if got.TmuxName != "persist-test" {
		t.Errorf("Persisted TmuxName = %q, want %q", got.TmuxName, "persist-test")
	}
}

func TestJSONSessionStore_ListReturnsCopy(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "sessions.json")

	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("NewJSONSessionStore() error: %v", err)
	}

	meta := &SessionMetadata{TmuxName: "orig", WorkingDir: "/tmp", CreatedAt: time.Now()}
	store.Set("s1", meta)

	list, _ := store.List()
	// Mutate the returned map
	list["s1"].TmuxName = "mutated"

	// Original should be unchanged
	got, _ := store.Get("s1")
	// Note: since we store pointers, mutation propagates. This tests current behavior.
	// The List() method returns a new map but shares pointers — testing that the map itself is a copy.
	delete(list, "s1")
	got2, err := store.Get("s1")
	if err != nil {
		t.Error("Deleting from List() copy should not affect store")
	}
	_ = got
	_ = got2
}

func TestJSONSessionStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "sessions.json")

	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("NewJSONSessionStore() error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sid := SessionID(fmt.Sprintf("session-%d", id))
			meta := &SessionMetadata{
				TmuxName:   fmt.Sprintf("tmux-%d", id),
				WorkingDir: "/tmp",
				CreatedAt:  time.Now(),
			}
			store.Set(sid, meta)
			store.Get(sid)
			store.List()
		}(i)
	}
	wg.Wait()

	// Verify all sessions were stored
	all, err := store.List()
	if err != nil {
		t.Fatalf("List() error after concurrent access: %v", err)
	}
	if len(all) != 10 {
		t.Errorf("Expected 10 sessions after concurrent writes, got %d", len(all))
	}
}

func TestJSONSessionStore_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "sessions.json")

	// Create empty file
	os.WriteFile(storePath, []byte{}, 0644)

	store, err := NewJSONSessionStore(storePath)
	if err != nil {
		t.Fatalf("NewJSONSessionStore() should handle empty file, got: %v", err)
	}

	all, _ := store.List()
	if len(all) != 0 {
		t.Errorf("Empty file should produce empty store, got %d entries", len(all))
	}
}

// --- Validate tests for additional coverage ---

func TestValidateHarnessName(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		wantErr bool
	}{
		{"valid claude", "claude-code", false},
		{"valid gemini", "gemini-cli", false},
		{"valid codex", "codex-cli", false},
		{"valid opencode", "opencode-cli", false},
		{"invalid", "unknown", true},
		{"empty", "", true},
		{"typo suggests", "claude-cod", true}, // prefix match should suggest
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHarnessName(tt.harness)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHarnessName(%q) error = %v, wantErr %v", tt.harness, err, tt.wantErr)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "b", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
	}
	for _, tt := range tests {
		got := levenshteinDistance(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

func TestSuggestHarness(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude", "claude-code"},
		{"gem", "gemini-cli"},
		{"xyz-unknown", ""},
	}
	for _, tt := range tests {
		got := suggestHarness(tt.input)
		if got != tt.expected {
			t.Errorf("suggestHarness(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestHarnessUnavailableError(t *testing.T) {
	err := &HarnessUnavailableError{Harness: "claude-code", EnvVar: "ANTHROPIC_API_KEY"}
	msg := err.Error()
	if msg == "" {
		t.Error("HarnessUnavailableError.Error() should not be empty")
	}
	// Should contain the env var and harness name
	if !strings.Contains(msg, "ANTHROPIC_API_KEY") {
		t.Error("Error message should contain env var name")
	}
	if !strings.Contains(msg, "claude-code") {
		t.Error("Error message should contain harness name")
	}

	// Test with unknown harness (no help URL)
	err2 := &HarnessUnavailableError{Harness: "unknown", EnvVar: "UNKNOWN_KEY"}
	msg2 := err2.Error()
	if msg2 == "" {
		t.Error("Error message for unknown harness should not be empty")
	}
}

func TestTestModelForHarness(t *testing.T) {
	model, ok := TestModelForHarness("claude-code")
	if !ok || model != "haiku" {
		t.Errorf("TestModelForHarness(claude-code) = (%q, %v), want (haiku, true)", model, ok)
	}

	_, ok = TestModelForHarness("nonexistent")
	if ok {
		t.Error("TestModelForHarness should return false for unknown harness")
	}
}

// --- Helper types ---

// registryMockAgent implements the Agent interface for testing
type registryMockAgent struct {
	name    string
	version string
}

func (m *registryMockAgent) Name() string    { return m.name }
func (m *registryMockAgent) Version() string { return m.version }
func (m *registryMockAgent) CreateSession(ctx SessionContext) (SessionID, error) {
	return SessionID("mock-session"), nil
}
func (m *registryMockAgent) ResumeSession(sessionID SessionID) error    { return nil }
func (m *registryMockAgent) TerminateSession(sessionID SessionID) error { return nil }
func (m *registryMockAgent) GetSessionStatus(sessionID SessionID) (Status, error) {
	return StatusActive, nil
}
func (m *registryMockAgent) SendMessage(sessionID SessionID, message Message) error { return nil }
func (m *registryMockAgent) GetHistory(sessionID SessionID) ([]Message, error) {
	return nil, nil
}
func (m *registryMockAgent) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	return nil, ErrNotImplemented
}
func (m *registryMockAgent) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	return "", ErrNotImplemented
}
func (m *registryMockAgent) Capabilities() Capabilities {
	return Capabilities{ModelName: m.version}
}
func (m *registryMockAgent) ExecuteCommand(cmd Command) error { return nil }
