package backend

import (
	"errors"
	"testing"
)

// fakeBackend is a controllable mock for testing the adapter layer
type fakeBackend struct {
	hasSessionResult  bool
	hasSessionErr     error
	listResult        []string
	listErr           error
	listInfoResult    []SessionInfo
	listInfoErr       error
	listClientsResult []ClientInfo
	listClientsErr    error
	createErr         error
	attachErr         error
	sendKeysErr       error
}

func (f *fakeBackend) HasSession(name string) (bool, error) {
	return f.hasSessionResult, f.hasSessionErr
}
func (f *fakeBackend) ListSessions() ([]string, error) {
	return f.listResult, f.listErr
}
func (f *fakeBackend) ListSessionsWithInfo() ([]SessionInfo, error) {
	return f.listInfoResult, f.listInfoErr
}
func (f *fakeBackend) ListClients(sessionName string) ([]ClientInfo, error) {
	return f.listClientsResult, f.listClientsErr
}
func (f *fakeBackend) CreateSession(name, workdir string) error {
	return f.createErr
}
func (f *fakeBackend) AttachSession(name string) error {
	return f.attachErr
}
func (f *fakeBackend) SendKeys(session, keys string) error {
	return f.sendKeysErr
}

func TestNewBackendAdapter(t *testing.T) {
	fb := &fakeBackend{}
	adapter := NewBackendAdapter(fb)
	if adapter == nil {
		t.Fatal("NewBackendAdapter returned nil")
	}
	if adapter.backend != fb {
		t.Error("adapter.backend should be the wrapped backend")
	}
}

func TestBackendAdapter_HasSession(t *testing.T) {
	tests := []struct {
		name   string
		result bool
		err    error
	}{
		{"session exists", true, nil},
		{"session not found", false, nil},
		{"error", false, errors.New("connection failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &fakeBackend{hasSessionResult: tt.result, hasSessionErr: tt.err}
			adapter := NewBackendAdapter(fb)

			got, err := adapter.HasSession("test")
			if got != tt.result {
				t.Errorf("HasSession() = %v, want %v", got, tt.result)
			}
			if (err != nil) != (tt.err != nil) {
				t.Errorf("HasSession() error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestBackendAdapter_ListSessions(t *testing.T) {
	fb := &fakeBackend{listResult: []string{"s1", "s2"}}
	adapter := NewBackendAdapter(fb)

	sessions, err := adapter.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("ListSessions() returned %d sessions, want 2", len(sessions))
	}
}

func TestBackendAdapter_ListSessionsWithInfo(t *testing.T) {
	fb := &fakeBackend{
		listInfoResult: []SessionInfo{
			{Name: "s1", AttachedClients: 1, AttachedList: "tty1"},
			{Name: "s2", AttachedClients: 0, AttachedList: ""},
		},
	}
	adapter := NewBackendAdapter(fb)

	infos, err := adapter.ListSessionsWithInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("got %d infos, want 2", len(infos))
	}
	if infos[0].Name != "s1" {
		t.Errorf("infos[0].Name = %q, want %q", infos[0].Name, "s1")
	}
	if infos[0].AttachedClients != 1 {
		t.Errorf("infos[0].AttachedClients = %d, want 1", infos[0].AttachedClients)
	}
}

func TestBackendAdapter_ListSessionsWithInfo_Error(t *testing.T) {
	fb := &fakeBackend{listInfoErr: errors.New("fail")}
	adapter := NewBackendAdapter(fb)

	_, err := adapter.ListSessionsWithInfo()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestBackendAdapter_ListClients(t *testing.T) {
	fb := &fakeBackend{
		listClientsResult: []ClientInfo{
			{SessionName: "s1", TTY: "/dev/pts/0", PID: 1234},
		},
	}
	adapter := NewBackendAdapter(fb)

	clients, err := adapter.ListClients("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("got %d clients, want 1", len(clients))
	}
	if clients[0].SessionName != "s1" {
		t.Errorf("clients[0].SessionName = %q, want %q", clients[0].SessionName, "s1")
	}
	if clients[0].TTY != "/dev/pts/0" {
		t.Errorf("clients[0].TTY = %q, want %q", clients[0].TTY, "/dev/pts/0")
	}
	if clients[0].PID != 1234 {
		t.Errorf("clients[0].PID = %d, want 1234", clients[0].PID)
	}
}

func TestBackendAdapter_ListClients_Error(t *testing.T) {
	fb := &fakeBackend{listClientsErr: errors.New("fail")}
	adapter := NewBackendAdapter(fb)

	_, err := adapter.ListClients("s1")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestBackendAdapter_CreateSession(t *testing.T) {
	fb := &fakeBackend{}
	adapter := NewBackendAdapter(fb)

	err := adapter.CreateSession("new-session", "~/code")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBackendAdapter_CreateSession_Error(t *testing.T) {
	fb := &fakeBackend{createErr: errors.New("create failed")}
	adapter := NewBackendAdapter(fb)

	err := adapter.CreateSession("new-session", "~/code")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestBackendAdapter_AttachSession(t *testing.T) {
	fb := &fakeBackend{}
	adapter := NewBackendAdapter(fb)

	err := adapter.AttachSession("test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBackendAdapter_SendKeys(t *testing.T) {
	fb := &fakeBackend{}
	adapter := NewBackendAdapter(fb)

	err := adapter.SendKeys("test", "echo hello")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBackendAdapter_SendKeys_Error(t *testing.T) {
	fb := &fakeBackend{sendKeysErr: errors.New("send failed")}
	adapter := NewBackendAdapter(fb)

	err := adapter.SendKeys("test", "echo hello")
	if err == nil {
		t.Error("expected error, got nil")
	}
}
