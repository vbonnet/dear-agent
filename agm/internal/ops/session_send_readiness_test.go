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
func (b *sendReadinessBackend) SendMessage(_ context.Context, _ manager.SessionID, message string) (manager.SendResult, error) {
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
func (b *sendReadinessBackend) CheckDelivery(context.Context, manager.SessionID) (manager.CanReceive, error) {
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
	ctx.Manager = backend

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
	if result == nil || result.Delivered || err == nil {
		t.Fatalf("result = %#v, error = %v, want failed non-delivery", result, err)
	}
	if len(backend.sent) != 0 {
		t.Fatalf("manager sent after readiness error: %v", backend.sent)
	}
}
