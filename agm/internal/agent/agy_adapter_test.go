package agent

import (
	"strings"
	"testing"
	"time"
)

// TestAgyAdapterImplementsAgentInterface verifies AgyAdapter implements Agent interface.
func TestAgyAdapterImplementsAgentInterface(t *testing.T) {
	// Create adapter with mock store
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	// Verify adapter implements Agent interface (type already Agent from NewAgyAdapter)
	_ = adapter
}

// TestAgyAdapterName tests Name() method.
func TestAgyAdapterName(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	if got := adapter.Name(); got != "agy" {
		t.Errorf("Name() = %q, want %q", got, "agy")
	}
}

// TestAgyAdapterVersion tests Version() method.
func TestAgyAdapterVersion(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	version := adapter.Version()
	if version == "" {
		t.Errorf("Version() returned empty string")
	}
}

// TestAgyAdapterCapabilities tests Capabilities() method.
func TestAgyAdapterCapabilities(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewAgyAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewAgyAdapter failed: %v", err)
	}

	caps := adapter.Capabilities()

	// Verify expected capabilities
	if !caps.SupportsSlashCommands {
		t.Error("SupportsSlashCommands should be true for Agy")
	}

	if !caps.SupportsTools {
		t.Error("SupportsTools should be true for Agy")
	}

	if !caps.SupportsHooks {
		t.Error("SupportsHooks should be true for Agy")
	}

	if caps.MaxContextWindow != 200000 {
		t.Errorf("MaxContextWindow = %d, want 200000", caps.MaxContextWindow)
	}

	if caps.ModelName != "antigravity-1.0" {
		t.Errorf("ModelName = %q, want %q", caps.ModelName, "antigravity-1.0")
	}
}

func TestAgyCreateSessionWaitsForPrompt(t *testing.T) {
	origHasSession := agyHasSession
	origNewSession := agyNewSession
	origSendCommand := agySendCommand
	origWaitForPrompt := agyWaitForPrompt
	t.Cleanup(func() {
		agyHasSession = origHasSession
		agyNewSession = origNewSession
		agySendCommand = origSendCommand
		agyWaitForPrompt = origWaitForPrompt
	})

	agyHasSession = func(string) (bool, error) { return false, nil }
	agyNewSession = func(string, string) error { return nil }

	var sent []string
	agySendCommand = func(_ string, cmd string) error {
		sent = append(sent, cmd)
		return nil
	}

	waited := false
	agyWaitForPrompt = func(sessionName string, timeout time.Duration) error {
		waited = true
		if sessionName != "agy-wait-test" {
			t.Fatalf("agyWaitForPrompt session = %q, want agy-wait-test", sessionName)
		}
		if timeout != 30*time.Second {
			t.Fatalf("agyWaitForPrompt timeout = %v, want 30s", timeout)
		}
		return nil
	}

	adapter := &AgyAdapter{sessionStore: &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}}
	_, err := adapter.CreateSession(SessionContext{
		Name:             "agy-wait-test",
		WorkingDirectory: "/work",
		Environment:      map[string]string{"AGM_PERMISSION_MODE": "auto"},
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}
	if !waited {
		t.Fatal("CreateSession did not wait for the AGY prompt")
	}
	if len(sent) == 0 || !strings.Contains(sent[0], "agy --dangerously-skip-permissions") {
		t.Fatalf("CreateSession sent commands = %v, want AGY launch with auto permission flag", sent)
	}
}

func TestAgyAdapterExecuteCommandRunHook(t *testing.T) {
	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{}}
	sessionID := SessionID("agy-hook-session")
	if err := store.Set(sessionID, &SessionMetadata{TmuxName: "agy-hook"}); err != nil {
		t.Fatalf("store.Set: %v", err)
	}

	adapter := &AgyAdapter{sessionStore: store}
	if err := adapter.ExecuteCommand(Command{
		Type: CommandRunHook,
		Params: map[string]any{
			"session_id": string(sessionID),
			"hook_name":  "SessionStart",
		},
	}); err != nil {
		t.Fatalf("ExecuteCommand(CommandRunHook) returned error: %v", err)
	}
}
