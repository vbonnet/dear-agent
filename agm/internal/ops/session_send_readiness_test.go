package ops

import (
	"context"
	"errors"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type sendReadinessBackend struct {
	readiness    manager.CanReceive
	readinessErr error
	sendErr      error
	sent         []string
	checks       int
	checkCtx     context.Context
	sendCtx      context.Context
}

func (b *sendReadinessBackend) CreateSession(context.Context, manager.SessionConfig) (manager.SessionID, error) {
	return "", nil
}
func (b *sendReadinessBackend) TerminateSession(context.Context, manager.SessionID) error {
	return nil
}
func (b *sendReadinessBackend) ListSessions(context.Context, manager.SessionFilter) ([]manager.SessionInfo, error) {
	return nil, nil
}
func (b *sendReadinessBackend) GetSession(context.Context, manager.SessionID) (manager.SessionInfo, error) {
	return manager.SessionInfo{}, nil
}
func (b *sendReadinessBackend) RenameSession(context.Context, manager.SessionID, string) error {
	return nil
}
func (b *sendReadinessBackend) SendMessage(ctx context.Context, _ manager.SessionID, message string) (manager.SendResult, error) {
	b.sendCtx = ctx
	if b.sendErr != nil {
		return manager.SendResult{}, b.sendErr
	}
	b.sent = append(b.sent, message)
	return manager.SendResult{Delivered: true}, nil
}
func (b *sendReadinessBackend) ReadOutput(context.Context, manager.SessionID, int) (string, error) {
	return "", nil
}
func (b *sendReadinessBackend) Interrupt(context.Context, manager.SessionID) error { return nil }
func (b *sendReadinessBackend) GetState(context.Context, manager.SessionID) (manager.StateResult, error) {
	return manager.StateResult{}, nil
}
func (b *sendReadinessBackend) CheckDelivery(ctx context.Context, _ manager.SessionID) (manager.CanReceive, error) {
	b.checks++
	b.checkCtx = ctx
	return b.readiness, b.readinessErr
}
func (b *sendReadinessBackend) HealthCheck(context.Context) error { return nil }
func (b *sendReadinessBackend) Name() string                      { return "readiness-test" }
func (b *sendReadinessBackend) Capabilities() manager.BackendCapabilities {
	return manager.BackendCapabilities{}
}

func TestSendMessage_ManagerChecksReadinessBeforeDelivery(t *testing.T) {
	for _, readiness := range []manager.CanReceive{
		manager.CanReceiveNo,
		manager.CanReceiveQueue,
		manager.CanReceiveNotFound,
	} {
		t.Run(managerReadinessName(readiness), func(t *testing.T) {
			backend := &sendReadinessBackend{readiness: readiness}
			ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
			ctx.Tmux = nil
			ctx.Manager = backend

			result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
			if result == nil || result.Delivered {
				t.Fatalf("result = %#v, want non-delivery", result)
			}
			opErr := &OpError{}
			if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
				t.Fatalf("error = %v, want %s", err, ErrCodeSessionNotReady)
			}
			if len(backend.sent) != 0 {
				t.Fatalf("manager sent before readiness: %v", backend.sent)
			}
		})
	}
}

func TestSendMessage_ManagerReadyDelivers(t *testing.T) {
	backend := &sendReadinessBackend{readiness: manager.CanReceiveYes}
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	ctx.Tmux = nil
	ctx.Manager = backend

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "hello"})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if !result.Delivered || len(backend.sent) != 1 || backend.sent[0] != "hello" {
		t.Fatalf("result = %#v, sent = %v", result, backend.sent)
	}
}

func TestSendMessage_ManagerReadinessErrorDoesNotSend(t *testing.T) {
	wantErr := errors.New("state probe failed")
	backend := &sendReadinessBackend{readinessErr: wantErr}
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	ctx.Tmux = nil
	ctx.Manager = backend

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
	if result == nil || result.Delivered || err == nil {
		t.Fatalf("result = %#v, error = %v, want failed non-delivery", result, err)
	}
	if len(backend.sent) != 0 {
		t.Fatalf("manager sent after readiness error: %v", backend.sent)
	}
}

func TestSendMessage_ExactTmuxReadinessPrecedesGenericManagerCheck(t *testing.T) {
	backend := &sendReadinessBackend{readiness: manager.CanReceiveNo}
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	ctx.Manager = backend

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "hello"})
	if err != nil || result == nil || !result.Delivered {
		t.Fatalf("SendMessage() = (%#v, %v), want exact-ready delivery", result, err)
	}
	if backend.checks != 0 {
		t.Fatalf("generic manager readiness ran %d times after exact tmux proof", backend.checks)
	}
}

func TestSendMessage_PropagatesRequestContextThroughReadinessAndDelivery(t *testing.T) {
	type contextKey struct{}
	wantCtx := context.WithValue(context.Background(), contextKey{}, "request")
	backend := &sendReadinessBackend{readiness: manager.CanReceiveYes}
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	ctx.Context = wantCtx
	ctx.Manager = backend
	tmuxMock := ctx.Tmux.(*mockTmux)

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "hello"})
	if err != nil || result == nil || !result.Delivered {
		t.Fatalf("SendMessage() = (%#v, %v), want delivery", result, err)
	}
	if tmuxMock.inputCtx != wantCtx {
		t.Fatal("exact tmux readiness did not receive the operation request context")
	}
	if backend.sendCtx != wantCtx {
		t.Fatal("manager delivery did not receive the operation request context")
	}
}

func TestSendMessage_CancelledRequestNeverChecksOrSends(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	backend := &sendReadinessBackend{readiness: manager.CanReceiveYes}
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	ctx.Context = cancelled
	ctx.Manager = backend
	tmuxMock := ctx.Tmux.(*mockTmux)

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
	if err == nil || result == nil || result.Delivered {
		t.Fatalf("SendMessage() = (%#v, %v), want cancelled non-delivery", result, err)
	}
	if len(tmuxMock.readinessChecks) != 0 || len(tmuxMock.sent) != 0 || backend.checks != 0 || len(backend.sent) != 0 {
		t.Fatalf("cancelled send performed I/O: tmux checks=%v tmux sends=%v manager checks=%d manager sends=%v",
			tmuxMock.readinessChecks, tmuxMock.sent, backend.checks, backend.sent)
	}
}
