package backend

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/session"
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
	killErr           error
	killed            []string
}

type readinessBackend struct {
	*fakeBackend
	waitCtx   context.Context
	checkCtx  context.Context
	atomicCtx context.Context
}

type adapterContextKey struct{}

func (b *readinessBackend) WaitForHarnessReady(ctx context.Context, _, _ string, _ time.Duration) error {
	b.waitCtx = ctx
	return nil
}

func (b *readinessBackend) CheckInputReadiness(ctx context.Context, _, _ string) (session.InputReadiness, error) {
	b.checkCtx = ctx
	return session.InputReadiness{Ready: true, State: "YES", PaneID: "%1"}, nil
}

func (b *readinessBackend) SendKeysIfInputReady(ctx context.Context, _, _, _ string, _ session.InputDeliveryOptions) (session.InputReadiness, error) {
	b.atomicCtx = ctx
	return session.InputReadiness{Ready: true, State: "YES", PaneID: "%1"}, nil
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
func (f *fakeBackend) KillSession(name string) error {
	f.killed = append(f.killed, name)
	return f.killErr
}

func TestBackendAdapter_KillSessionForwardsProductionCapability(t *testing.T) {
	wantErr := errors.New("kill denied")
	fb := &fakeBackend{killErr: wantErr}
	adapter := NewBackendAdapter(fb)

	err := adapter.KillSession("exact-target")
	if !errors.Is(err, wantErr) {
		t.Fatalf("KillSession() error = %v, want %v", err, wantErr)
	}
	if len(fb.killed) != 1 || fb.killed[0] != "exact-target" {
		t.Fatalf("killed = %v, want [exact-target]", fb.killed)
	}
}

type strictMockTmux struct {
	*session.MockTmux
	strictCalls int
}

func (m *strictMockTmux) HasSessionStrict(_ context.Context, name string) (bool, error) {
	m.strictCalls++
	return m.HasSession(name)
}

func TestBackendAdapter_PreservesTmuxKillAndStrictProbeThroughProductionChain(t *testing.T) {
	inner := &strictMockTmux{MockTmux: session.NewMockTmux()}
	inner.Sessions["exact-target"] = true
	adapter := NewBackendAdapter(NewTmuxBackendWithClient(inner))

	exists, err := adapter.HasSessionStrict(context.Background(), "exact-target")
	if err != nil || !exists || inner.strictCalls != 1 {
		t.Fatalf("strict probe: exists=%v calls=%d error=%v", exists, inner.strictCalls, err)
	}
	if err := adapter.KillSession("exact-target"); err != nil {
		t.Fatalf("KillSession(): %v", err)
	}
	if inner.Sessions["exact-target"] {
		t.Fatal("production adapter chain left exact target running")
	}
}

func TestNewBackendAdapter(t *testing.T) {
	fb := &fakeBackend{}
	adapter := NewBackendAdapter(fb)
	if adapter == nil {
		t.Fatal("NewBackendAdapter returned nil")
		return
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

func TestBackendAdapter_ForwardsReadinessCapabilities(t *testing.T) {
	wantCtx := context.WithValue(context.Background(), adapterContextKey{}, "request")
	backend := &readinessBackend{fakeBackend: &fakeBackend{}}
	adapter := NewBackendAdapter(backend)

	if err := adapter.WaitForHarnessReady(wantCtx, "worker", "codex-cli", time.Second); err != nil {
		t.Fatalf("WaitForHarnessReady() error = %v", err)
	}
	readiness, err := adapter.CheckInputReadiness(wantCtx, "worker", "codex-cli")
	if err != nil || !readiness.Ready {
		t.Fatalf("CheckInputReadiness() = (%#v, %v)", readiness, err)
	}
	atomic, err := adapter.SendKeysIfInputReady(wantCtx, "worker", "codex-cli", "hello", session.InputDeliveryOptions{})
	if err != nil || !atomic.Ready || atomic.PaneID != "%1" {
		t.Fatalf("SendKeysIfInputReady() = (%#v, %v)", atomic, err)
	}
	if backend.waitCtx != wantCtx || backend.checkCtx != wantCtx || backend.atomicCtx != wantCtx {
		t.Fatal("adapter did not preserve request context through readiness capabilities")
	}
}

func TestBackendAdapter_ReadinessFailsClosedWhenCapabilityMissing(t *testing.T) {
	adapter := NewBackendAdapter(&fakeBackend{})
	if err := adapter.WaitForHarnessReady(context.Background(), "worker", "codex-cli", time.Second); err == nil {
		t.Fatal("WaitForHarnessReady() succeeded without backend capability")
	}
	if _, err := adapter.CheckInputReadiness(context.Background(), "worker", "codex-cli"); err == nil {
		t.Fatal("CheckInputReadiness() succeeded without backend capability")
	}
	if _, err := adapter.SendKeysIfInputReady(context.Background(), "worker", "codex-cli", "hello", session.InputDeliveryOptions{}); err == nil {
		t.Fatal("SendKeysIfInputReady() succeeded without backend capability")
	}
}

func TestTmuxBackend_ForwardsReadinessCapabilities(t *testing.T) {
	tmuxMock := session.NewMockTmux()
	backend := NewTmuxBackendWithClient(tmuxMock)
	wantCtx := context.WithValue(context.Background(), adapterContextKey{}, "request")

	if err := backend.WaitForHarnessReady(wantCtx, "worker", "codex-cli", time.Second); err != nil {
		t.Fatalf("WaitForHarnessReady() error = %v", err)
	}
	if _, err := backend.CheckInputReadiness(wantCtx, "worker", "codex-cli"); err != nil {
		t.Fatalf("CheckInputReadiness() error = %v", err)
	}
	if _, err := backend.SendKeysIfInputReady(wantCtx, "worker", "codex-cli", "hello", session.InputDeliveryOptions{}); err != nil {
		t.Fatalf("SendKeysIfInputReady() error = %v", err)
	}
	if tmuxMock.WaitContext != wantCtx || tmuxMock.InputContext != wantCtx {
		t.Fatal("tmux backend did not preserve request context")
	}
	if len(tmuxMock.AtomicInputChecks) != 1 || len(tmuxMock.ExactPaneDeliveries) != 1 {
		t.Fatalf("atomic backend calls = %v/%v, want one exact delivery", tmuxMock.AtomicInputChecks, tmuxMock.ExactPaneDeliveries)
	}
}
