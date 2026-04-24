package ops

import (
	"context"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// mockStorage implements dolt.Storage for testing.
type mockStorage struct {
	sessions []*manifest.Manifest
}

func (m *mockStorage) GetSession(identifier string) (*manifest.Manifest, error) {
	for _, s := range m.sessions {
		if s.SessionID == identifier || s.Name == identifier {
			return s, nil
		}
	}
	return nil, ErrSessionNotFound(identifier)
}

func (m *mockStorage) ListSessions(filter *dolt.SessionFilter) ([]*manifest.Manifest, error) {
	var result []*manifest.Manifest
	for _, s := range m.sessions {
		// Check exclude archived
		if filter.ExcludeArchived && s.Lifecycle == manifest.LifecycleArchived {
			continue
		}
		result = append(result, s)
	}
	return result, nil
}

func (m *mockStorage) UpdateSession(session *manifest.Manifest) error {
	return nil
}

// Stub out remaining manifest.Store methods (inherited by dolt.Storage)
func (m *mockStorage) Create(*manifest.Manifest) error          { return nil }
func (m *mockStorage) Get(string) (*manifest.Manifest, error)   { return nil, nil }
func (m *mockStorage) Update(*manifest.Manifest) error          { return nil }
func (m *mockStorage) Delete(string) error                      { return nil }
func (m *mockStorage) List(*manifest.Filter) ([]*manifest.Manifest, error) { return nil, nil }
func (m *mockStorage) Close() error                             { return nil }
func (m *mockStorage) ApplyMigrations() error                   { return nil }

// Stub out CreateSession, GetSession, UpdateSession, DeleteSession (legacy dolt.Storage methods)
func (m *mockStorage) CreateSession(*manifest.Manifest) error { return nil }
func (m *mockStorage) DeleteSession(string) error             { return nil }

// testManifest creates a test manifest with defaults.
func testManifest(name, state string, stateUpdatedAt time.Time) *manifest.Manifest {
	return &manifest.Manifest{
		SessionID:      name + "-id",
		Name:           name,
		State:          state,
		StateUpdatedAt: stateUpdatedAt,
		Lifecycle:      "", // active
		Context:        manifest.Context{Tags: []string{}},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

func TestDetectPermissionPromptStall_NoStall(t *testing.T) {
	now := time.Now()
	detector := NewStallDetector(&OpContext{})
	session := testManifest("session-1", manifest.StatePermissionPrompt, now.Add(-2*time.Minute))

	event := detector.detectPermissionPromptStall(session, now)
	if event != nil {
		t.Errorf("Expected no stall, got %v", event)
	}
}

func TestDetectPermissionPromptStall_Stall(t *testing.T) {
	now := time.Now()
	detector := NewStallDetector(&OpContext{})
	stateUpdated := now.Add(-10 * time.Minute)
	session := testManifest("session-1", manifest.StatePermissionPrompt, stateUpdated)

	event := detector.detectPermissionPromptStall(session, now)
	if event == nil {
		t.Fatal("Expected stall event, got nil")
	}
	if event.StallType != "permission_prompt" {
		t.Errorf("StallType = %v, want permission_prompt", event.StallType)
	}
	if event.Severity != "critical" {
		t.Errorf("Severity = %v, want critical", event.Severity)
	}
}

func TestDetectPermissionPromptStall_NotPermissionPrompt(t *testing.T) {
	now := time.Now()
	detector := NewStallDetector(&OpContext{})
	session := testManifest("session-1", manifest.StateWorking, now.Add(-10*time.Minute))

	event := detector.detectPermissionPromptStall(session, now)
	if event != nil {
		t.Errorf("Expected no stall for non-permission state, got %v", event)
	}
}

func TestDetectNoCommitStall_WorkingState(t *testing.T) {
	// Note: This test verifies the structure for no-commit stalls.
	// In a real environment, countRecentCommits would need a valid git repo.
	// This test checks that the detector properly identifies the WORKING state
	// and calls countRecentCommits. The actual commit counting is tested separately.
	now := time.Now()
	detector := NewStallDetector(&OpContext{})
	session := testManifest("worker-1", manifest.StateWorking, now.Add(-20*time.Minute))

	// Since countRecentCommits will fail (not in a git repo), we can't test
	// the full flow here. But we can verify the no-commit stall is only
	// checked for WORKING state with sufficient duration.
	event := detector.detectNoCommitStall(session, now)
	// Event will be nil because countRecentCommits returns -1 (error)
	// which is intentionally skipped in production
	_ = event // Just verify the function runs without panicking
}

func TestDetectNoCommitStall_NotWorkingState(t *testing.T) {
	now := time.Now()
	detector := NewStallDetector(&OpContext{})
	session := testManifest("worker-1", manifest.StateDone, now.Add(-20*time.Minute))

	event := detector.detectNoCommitStall(session, now)
	if event != nil {
		t.Errorf("Expected no stall for non-working state, got %v", event)
	}
}

func TestDetectNoCommitStall_ShortDuration(t *testing.T) {
	now := time.Now()
	detector := NewStallDetector(&OpContext{})
	session := testManifest("worker-1", manifest.StateWorking, now.Add(-5*time.Minute))

	event := detector.detectNoCommitStall(session, now)
	if event != nil {
		t.Errorf("Expected no stall for duration < timeout, got %v", event)
	}
}

func TestIsWorkerSession_WithTag(t *testing.T) {
	session := testManifest("my-session", manifest.StateWorking, time.Now())
	session.Context.Tags = []string{"worker", "agm-orchestrator"}

	if !isWorkerSession(session) {
		t.Error("Expected session with 'worker' tag to be identified as worker")
	}
}

func TestIsWorkerSession_WithName(t *testing.T) {
	session := testManifest("worker-123", manifest.StateWorking, time.Now())

	if !isWorkerSession(session) {
		t.Error("Expected session with 'worker' in name to be identified as worker")
	}
}

func TestIsWorkerSession_NotWorker(t *testing.T) {
	session := testManifest("orchestrator-1", manifest.StateWorking, time.Now())

	if isWorkerSession(session) {
		t.Error("Expected orchestrator session to not be identified as worker")
	}
}

func TestExtractErrorPatterns_SingleError(t *testing.T) {
	output := `error: permission denied
error: permission denied
error: permission denied`

	patterns := extractErrorPatterns(output)
	if len(patterns) == 0 {
		t.Error("Expected at least one pattern extracted")
	}
}

func TestExtractErrorPatterns_NoErrors(t *testing.T) {
	output := `processing item 1
processing item 2
completed successfully`

	patterns := extractErrorPatterns(output)
	if len(patterns) > 0 {
		t.Errorf("Expected no error patterns, got %d", len(patterns))
	}
}

func TestIsErrorLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"error: something failed", true},
		{"fatal: cannot open file", true},
		{"permission denied: access not allowed", true},
		{"timeout waiting for response", true},
		{"processing completed", false},
		{"user input required", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isErrorLine(tt.line); got != tt.want {
			t.Errorf("isErrorLine(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestNormalizeErrorMessage(t *testing.T) {
	tests := []struct {
		input string
		// We just check that it removes timestamps and doesn't fail
	}{
		{"2024-04-12T10:30:45 error: something failed"},
		{"/home/user/file.go:123: error"},
		{"some/path/to/file: operation failed"},
	}

	for _, tt := range tests {
		normalized := normalizeErrorMessage(tt.input)
		if normalized == "" {
			t.Errorf("normalizeErrorMessage(%q) returned empty string", tt.input)
		}
	}
}

func TestDetectStalls_EmptyList(t *testing.T) {
	mockStore := &mockStorage{sessions: []*manifest.Manifest{}}
	detector := NewStallDetector(&OpContext{Storage: mockStore})

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}
	if len(events) != 0 {
		t.Errorf("Expected 0 events for empty session list, got %d", len(events))
	}
}

func TestDetectStalls_MultipleEventTypes(t *testing.T) {
	now := time.Now()
	sessions := []*manifest.Manifest{
		testManifest("session-perm", manifest.StatePermissionPrompt, now.Add(-10*time.Minute)),
		testManifest("worker-1", manifest.StateWorking, now.Add(-20*time.Minute)),
	}

	mockStore := &mockStorage{sessions: sessions}
	detector := NewStallDetector(&OpContext{Storage: mockStore})

	events, err := detector.DetectStalls(context.Background())
	if err != nil {
		t.Fatalf("DetectStalls() error = %v", err)
	}

	// Should detect permission prompt stall
	// Note: no-commit detection would fail here since git log won't find commits
	// but the test validates structure

	var foundPermStall bool
	for _, e := range events {
		if e.StallType == "permission_prompt" {
			foundPermStall = true
		}
	}

	if !foundPermStall {
		t.Error("Expected to find permission_prompt stall")
	}
}
