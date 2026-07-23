package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// TestClaudeAdapterImplementsAgentInterface verifies ClaudeAdapter implements Agent interface.
func TestClaudeAdapterImplementsAgentInterface(t *testing.T) {
	// Create adapter with mock store
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	// Verify adapter implements Agent interface
	var _ = adapter
}

// TestClaudeAdapterName tests Name() method.
func TestClaudeAdapterName(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	if got := adapter.Name(); got != "claude-code" {
		t.Errorf("Name() = %q, want %q", got, "claude-code")
	}
}

// TestClaudeAdapterVersion tests Version() method.
func TestClaudeAdapterVersion(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	version := adapter.Version()
	if version == "" {
		t.Errorf("Version() returned empty string")
	}
}

// TestClaudeAdapterCapabilities tests Capabilities() method.
func TestClaudeAdapterCapabilities(t *testing.T) {
	mockStore := &MockSessionStore{
		sessions: make(map[SessionID]*SessionMetadata),
	}

	adapter, err := NewClaudeAdapter(mockStore)
	if err != nil {
		t.Fatalf("NewClaudeAdapter failed: %v", err)
	}

	caps := adapter.Capabilities()

	// Verify expected capabilities
	if !caps.SupportsSlashCommands {
		t.Error("SupportsSlashCommands should be true for Claude CLI")
	}

	if !caps.SupportsTools {
		t.Error("SupportsTools should be true for Claude")
	}

	if !caps.SupportsVision {
		t.Error("SupportsVision should be true for Claude Sonnet 4.5")
	}

	if caps.MaxContextWindow != 200000 {
		t.Errorf("MaxContextWindow = %d, want 200000", caps.MaxContextWindow)
	}

	if caps.ModelName != "claude-sonnet-4.5" {
		t.Errorf("ModelName = %q, want %q", caps.ModelName, "claude-sonnet-4.5")
	}
}

func TestClaudeAdapterCreateUsesPreparedCallerEnvironment(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "claude-adapter-oauth-canary")
	restoreClaudeAdapterRuntime(t)
	claudeHasSession = func(string) (bool, error) { return false, nil }
	claudeNewSession = func(string, string) error { return nil }
	var sent string
	claudeSendCommand = func(_ string, command string) error {
		sent = command
		return nil
	}
	claudeWaitForReady = func(string, time.Duration) error { return nil }

	adapter := &ClaudeAdapter{sessionStore: &MockSessionStore{sessions: make(map[SessionID]*SessionMetadata)}}
	if _, err := adapter.CreateSession(SessionContext{
		Name: "claude-adapter", WorkingDirectory: "/tmp/work",
		AuthorizedDirs: []string{"/tmp/work", "/tmp/extra"},
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	for _, want := range []string{
		"__exec-claude", "--handoff", "--workdir '/tmp/work'", "--add-dir '/tmp/extra'",
	} {
		if !strings.Contains(sent, want) {
			t.Errorf("prepared Claude adapter command %q missing %q", sent, want)
		}
	}
	if strings.Contains(sent, "claude-adapter-oauth-canary") {
		t.Fatalf("Claude adapter exposed caller OAuth in command: %s", sent)
	}
}

func TestClaudeAdapterCreateCancelsUndeliveredHandoff(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	restoreClaudeAdapterRuntime(t)
	claudeHasSession = func(string) (bool, error) { return false, nil }
	claudeNewSession = func(string, string) error { return nil }
	claudeSendCommand = func(_ string, command string) error {
		if strings.Contains(command, "__exec-claude") {
			return errors.New("send failed")
		}
		return nil
	}

	adapter := &ClaudeAdapter{sessionStore: &MockSessionStore{sessions: make(map[SessionID]*SessionMetadata)}}
	if _, err := adapter.CreateSession(SessionContext{Name: "claude-failed", WorkingDirectory: "/tmp/work"}); err == nil {
		t.Fatal("CreateSession succeeded after command delivery failed")
	}
	entries, err := os.ReadDir(filepath.Join(stateDir, "private-launch"))
	if err != nil {
		t.Fatalf("read private handoff directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("undelivered Claude handoff remains: %v", entries)
	}
}

func TestClaudeAdapterCreatePreservesHandoffAfterUncertainSubmission(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	restoreClaudeAdapterRuntime(t)
	claudeHasSession = func(string) (bool, error) { return false, nil }
	claudeNewSession = func(string, string) error { return nil }
	var sent []string
	claudeSendCommand = func(_ string, command string) error {
		sent = append(sent, command)
		if strings.Contains(command, "__exec-claude") {
			return tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement"))
		}
		return nil
	}
	claudeWaitForReady = func(string, time.Duration) error { return nil }

	adapter := &ClaudeAdapter{sessionStore: &MockSessionStore{sessions: make(map[SessionID]*SessionMetadata)}}
	if _, err := adapter.CreateSession(SessionContext{Name: "claude-uncertain", WorkingDirectory: "/tmp/work"}); err != nil {
		t.Fatalf("CreateSession returned uncertain submission as failure: %v", err)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "__exec-claude") {
		t.Fatalf("commands = %q, want one private launch and no compensating exit", sent)
	}
	assertOnePrivateHandoff(t, stateDir)
}

func TestClaudeAdapterResumeUsesPreparedNativeIdentity(t *testing.T) {
	t.Setenv("AGM_STATE_DIR", t.TempDir())
	t.Setenv("TMUX", "/tmp/test-tmux")
	restoreClaudeAdapterRuntime(t)
	claudeHasSession = func(string) (bool, error) { return true, nil }
	claudeIsRunning = func(string) (bool, error) { return false, nil }
	var sent string
	claudeSendCommand = func(_ string, command string) error {
		sent = command
		return nil
	}
	claudeWaitForReady = func(string, time.Duration) error { return nil }

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agm-session": {TmuxName: "claude-resume", WorkingDir: "/tmp/resume", UUID: "native-claude-id"},
	}}
	adapter := &ClaudeAdapter{sessionStore: store}
	if err := adapter.ResumeSession("agm-session"); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	for _, want := range []string{
		"__exec-claude", "--handoff", "--resume-id 'native-claude-id'", "--workdir '/tmp/resume'",
	} {
		if !strings.Contains(sent, want) {
			t.Errorf("prepared Claude resume %q missing %q", sent, want)
		}
	}
}

func TestClaudeAdapterResumePreservesHandoffAfterUncertainSubmission(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	t.Setenv("TMUX", "/tmp/test-tmux")
	restoreClaudeAdapterRuntime(t)
	claudeHasSession = func(string) (bool, error) { return false, nil }
	claudeNewSession = func(string, string) error { return nil }
	var sent []string
	claudeSendCommand = func(_ string, command string) error {
		sent = append(sent, command)
		if strings.Contains(command, "__exec-claude") {
			return tmux.MarkPromptSubmissionUncertain(errors.New("lost acknowledgement"))
		}
		return nil
	}
	claudeWaitForReady = func(string, time.Duration) error { return nil }

	store := &MockSessionStore{sessions: map[SessionID]*SessionMetadata{
		"agm-session": {TmuxName: "claude-resume-uncertain", WorkingDir: "/tmp/resume", UUID: "native-claude-id"},
	}}
	adapter := &ClaudeAdapter{sessionStore: store}
	if err := adapter.ResumeSession("agm-session"); err != nil {
		t.Fatalf("ResumeSession returned uncertain submission as failure: %v", err)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "__exec-claude") {
		t.Fatalf("commands = %q, want one private resume and no compensating exit", sent)
	}
	assertOnePrivateHandoff(t, stateDir)
}

func assertOnePrivateHandoff(t *testing.T, stateDir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(stateDir, "private-launch"))
	if err != nil {
		t.Fatalf("read private handoff directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("private handoffs = %v, want exactly one preserved handoff", entries)
	}
}

func restoreClaudeAdapterRuntime(t *testing.T) {
	t.Helper()
	originalHasSession := claudeHasSession
	originalNewSession := claudeNewSession
	originalSendCommand := claudeSendCommand
	originalWaitForReady := claudeWaitForReady
	originalIsRunning := claudeIsRunning
	originalAttachSession := claudeAttachSession
	t.Cleanup(func() {
		claudeHasSession = originalHasSession
		claudeNewSession = originalNewSession
		claudeSendCommand = originalSendCommand
		claudeWaitForReady = originalWaitForReady
		claudeIsRunning = originalIsRunning
		claudeAttachSession = originalAttachSession
	})
}

// MockSessionStore is a mock implementation of SessionStore for testing.
type MockSessionStore struct {
	sessions map[SessionID]*SessionMetadata
}

func (m *MockSessionStore) Get(sessionID SessionID) (*SessionMetadata, error) {
	metadata, exists := m.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}
	return metadata, nil
}

func (m *MockSessionStore) Set(sessionID SessionID, metadata *SessionMetadata) error {
	m.sessions[sessionID] = metadata
	return nil
}

func (m *MockSessionStore) Delete(sessionID SessionID) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *MockSessionStore) List() (map[SessionID]*SessionMetadata, error) {
	result := make(map[SessionID]*SessionMetadata)
	for k, v := range m.sessions {
		result[k] = v
	}
	return result, nil
}
