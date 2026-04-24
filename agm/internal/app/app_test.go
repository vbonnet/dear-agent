package app

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/terminal"
)

// mockAgent implements agent.Agent interface for testing
type mockAgent struct{}

func (m *mockAgent) Name() string    { return "mock" }
func (m *mockAgent) Version() string { return "1.0" }
func (m *mockAgent) CreateSession(ctx agent.SessionContext) (agent.SessionID, error) {
	return agent.SessionID("mock-session-id"), nil
}
func (m *mockAgent) ResumeSession(sessionID agent.SessionID) error { return nil }
func (m *mockAgent) TerminateSession(sessionID agent.SessionID) error {
	return nil
}
func (m *mockAgent) GetSessionStatus(sessionID agent.SessionID) (agent.Status, error) {
	return agent.StatusActive, nil
}
func (m *mockAgent) SendMessage(sessionID agent.SessionID, message agent.Message) error {
	return nil
}
func (m *mockAgent) GetHistory(sessionID agent.SessionID) ([]agent.Message, error) {
	return []agent.Message{}, nil
}
func (m *mockAgent) ExportConversation(sessionID agent.SessionID, format agent.ConversationFormat) ([]byte, error) {
	return []byte{}, nil
}
func (m *mockAgent) ImportConversation(data []byte, format agent.ConversationFormat) (agent.SessionID, error) {
	return agent.SessionID("mock-import-id"), nil
}
func (m *mockAgent) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		SupportsSlashCommands: true,
		SupportsTools:         true,
		MaxContextWindow:      200000,
		ModelName:             "mock-1.0",
	}
}
func (m *mockAgent) ExecuteCommand(cmd agent.Command) error { return nil }

// mockFilesystem implements FilesystemInterface for testing
type mockFilesystem struct{}

func TestNewApp(t *testing.T) {
	pty := terminal.NewMockPTY()
	tmux := session.NewMockTmux()
	agent := &mockAgent{}
	fs := &mockFilesystem{}

	app := NewApp(pty, tmux, agent, fs)

	if app == nil {
		t.Fatal("NewApp() returned nil")
	}
	if app.PTY == nil {
		t.Error("App.PTY is nil")
	}
	if app.Tmux == nil {
		t.Error("App.Tmux is nil")
	}
	if app.Harness == nil {
		t.Error("App.Harness is nil")
	}
	if app.FS == nil {
		t.Error("App.FS is nil")
	}
}

func TestApp_DependencyInjection(t *testing.T) {
	// Test that dependencies can be injected and accessed
	pty := terminal.NewMockPTY()
	tmux := session.NewMockTmux()
	agent := &mockAgent{}
	fs := &mockFilesystem{}

	app := NewApp(pty, tmux, agent, fs)

	// Verify we can access injected dependencies
	if app.PTY == nil {
		t.Error("PTY dependency is nil")
	}
	if app.Tmux == nil {
		t.Error("Tmux dependency is nil")
	}
	if app.Harness == nil {
		t.Error("Agent dependency is nil")
	}
	if app.FS == nil {
		t.Error("FS dependency is nil")
	}

	// Verify correct types are injected
	if _, ok := app.PTY.(*terminal.MockPTY); !ok {
		t.Error("PTY is not of expected mock type")
	}
	if _, ok := app.Tmux.(*session.MockTmux); !ok {
		t.Error("Tmux is not of expected mock type")
	}
}

func TestApp_Run_Placeholder(t *testing.T) {
	// Test that Run() executes without error (placeholder test)
	pty := terminal.NewMockPTY()
	tmux := session.NewMockTmux()
	agent := &mockAgent{}
	fs := &mockFilesystem{}

	app := NewApp(pty, tmux, agent, fs)

	err := app.Run([]string{})
	if err != nil {
		t.Errorf("Run() returned unexpected error: %v", err)
	}
}
