package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manager"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
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

func TestSendMessage_AtomicReadinessAndDeliveryPrecedesGenericManagerCheck(t *testing.T) {
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
	if len(backend.sent) != 0 {
		t.Fatalf("session-targeted manager delivered after exact pane proof: %v", backend.sent)
	}
	tmuxMock := ctx.Tmux.(*mockTmux)
	if len(tmuxMock.atomicChecks) != 1 || tmuxMock.atomicChecks[0] != "my-session:claude-code" {
		t.Fatalf("atomic input checks = %v, want [my-session:claude-code]", tmuxMock.atomicChecks)
	}
	if len(tmuxMock.sent) != 1 || tmuxMock.sent[0].session != "%1" {
		t.Fatalf("exact pane sends = %v, want %%1", tmuxMock.sent)
	}
}

func TestSendMessage_PiPermissionPromptBlocksAtomicDelivery(t *testing.T) {
	m := newManifest("id-1", "pi-session", "~/project")
	m.Harness = "pi-cli"
	ctx := testCtx([]*manifest.Manifest{m}, "pi-session")
	tmuxMock := ctx.Tmux.(*mockTmux)
	tmuxMock.readiness = session.InputReadiness{State: "PERMISSION", PaneID: "%1"}

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not authorize"})
	if result == nil || result.Delivered {
		t.Fatalf("result = %#v, want non-delivery", result)
	}
	opErr := &OpError{}
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	retryAdvice := strings.Join(opErr.Suggestions, "\n")
	if !strings.Contains(retryAdvice, "agm send msg pi-session --prompt <text>") {
		t.Fatalf("retry advice does not use the registered --prompt flag: %q", retryAdvice)
	}
	if strings.Contains(retryAdvice, "--message") {
		t.Fatalf("retry advice uses unregistered --message flag: %q", retryAdvice)
	}
	if len(tmuxMock.atomicChecks) != 1 || tmuxMock.atomicChecks[0] != "pi-session:pi-cli" {
		t.Fatalf("atomic input checks = %v, want [pi-session:pi-cli]", tmuxMock.atomicChecks)
	}
	if len(tmuxMock.sent) != 0 {
		t.Fatalf("Pi permission prompt received input: %v", tmuxMock.sent)
	}
}

func TestSendMessage_QueuedAGMRecoveryPolicies(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		request SendMessageRequest
	}{
		{name: "force", request: SendMessageRequest{Force: true}},
		{name: "autonomous", request: SendMessageRequest{Autonomous: true}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
			tmuxMock := ctx.Tmux.(*mockTmux)
			tmuxMock.readiness = session.InputReadiness{State: "QUEUED_AGM", PaneID: "%7"}
			testCase.request.Recipient = "id-1"
			testCase.request.Message = "recovery message"

			result, err := SendMessage(ctx, &testCase.request)
			if err != nil || result == nil || !result.Delivered {
				t.Fatalf("SendMessage(%s queued AGM) = (%#v, %v), want exact-pane delivery", testCase.name, result, err)
			}
			if len(tmuxMock.atomicOptions) != 1 || !tmuxMock.atomicOptions[0].AllowQueuedAGM {
				t.Fatalf("atomic delivery options = %#v, want %s queued-AGM recovery", tmuxMock.atomicOptions, testCase.name)
			}
			if len(tmuxMock.sent) != 1 || tmuxMock.sent[0].session != "%7" || tmuxMock.sent[0].keys != "recovery message" {
				t.Fatalf("%s exact-pane sends = %#v, want %%7 recovery message", testCase.name, tmuxMock.sent)
			}
		})
	}
}

func TestSendMessage_ForceDoesNotBypassProtectedInputStates(t *testing.T) {
	t.Parallel()

	for _, readinessState := range []string{"QUEUE", "PERMISSION", "OVERLAY", "ONBOARDING", "WRONG_HARNESS", "NOT_FOUND"} {
		t.Run(readinessState, func(t *testing.T) {
			t.Parallel()
			ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
			tmuxMock := ctx.Tmux.(*mockTmux)
			tmuxMock.readiness = session.InputReadiness{State: readinessState, PaneID: "%7"}

			result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send", Force: true})
			if result == nil || result.Delivered || err == nil {
				t.Fatalf("SendMessage(force %s) = (%#v, %v), want non-delivery", readinessState, result, err)
			}
			if len(tmuxMock.sent) != 0 {
				t.Fatalf("force bypassed %s: %#v", readinessState, tmuxMock.sent)
			}
		})
	}
}

func TestSendMessage_AutonomousDoesNotBypassProtectedInputStates(t *testing.T) {
	t.Parallel()

	for _, readinessState := range []string{"QUEUE", "PERMISSION", "OVERLAY", "ONBOARDING", "WRONG_HARNESS", "NOT_FOUND"} {
		t.Run(readinessState, func(t *testing.T) {
			t.Parallel()
			ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
			tmuxMock := ctx.Tmux.(*mockTmux)
			tmuxMock.readiness = session.InputReadiness{State: readinessState, PaneID: "%8"}

			result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send", Autonomous: true})
			if result == nil || result.Delivered || err == nil {
				t.Fatalf("SendMessage(autonomous %s) = (%#v, %v), want non-delivery", readinessState, result, err)
			}
			if len(tmuxMock.sent) != 0 {
				t.Fatalf("autonomous mode bypassed %s: %#v", readinessState, tmuxMock.sent)
			}
		})
	}
}

func TestSendMessage_ReadyWithoutVerifiedPaneFailsClosed(t *testing.T) {
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	tmuxMock := ctx.Tmux.(*mockTmux)
	tmuxMock.readiness = session.InputReadiness{Ready: true, State: "YES"}

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
	if result == nil || result.Delivered {
		t.Fatalf("result = %#v, want non-delivery", result)
	}
	opErr := &OpError{}
	if !errors.As(err, &opErr) || opErr.Code != ErrCodeSessionNotReady {
		t.Fatalf("error = %v, want %s", err, ErrCodeSessionNotReady)
	}
	if len(tmuxMock.sent) != 0 {
		t.Fatalf("delivery occurred without a verified pane: %v", tmuxMock.sent)
	}
}

func TestSendMessage_NormalizesLegacyAgyHarnessBeforeReadiness(t *testing.T) {
	t.Parallel()

	for _, legacyHarness := range []string{"agy-cli", "antigravity"} {
		t.Run(legacyHarness, func(t *testing.T) {
			t.Parallel()

			m := newManifest("id-1", "agy-session", "~/project")
			m.Harness = legacyHarness
			ctx := testCtx([]*manifest.Manifest{m}, "agy-session")
			tmuxMock := ctx.Tmux.(*mockTmux)

			result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "hello"})
			if err != nil || result == nil || !result.Delivered {
				t.Fatalf("SendMessage() = (%#v, %v), want canonical AGY delivery", result, err)
			}
			if len(tmuxMock.readinessChecks) != 1 || tmuxMock.readinessChecks[0] != "agy-session:agy" {
				t.Fatalf("readiness checks = %v, want [agy-session:agy]", tmuxMock.readinessChecks)
			}
		})
	}
}

func TestSendMessage_NormalizesPiHarnessAliasBeforeReadiness(t *testing.T) {
	t.Parallel()

	m := newManifest("id-1", "pi-session", "~/project")
	m.Harness = "pi"
	ctx := testCtx([]*manifest.Manifest{m}, "pi-session")
	tmuxMock := ctx.Tmux.(*mockTmux)

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "hello"})
	if err != nil || result == nil || !result.Delivered {
		t.Fatalf("SendMessage() = (%#v, %v), want canonical Pi delivery", result, err)
	}
	if len(tmuxMock.readinessChecks) != 1 || tmuxMock.readinessChecks[0] != "pi-session:pi-cli" {
		t.Fatalf("readiness checks = %v, want [pi-session:pi-cli]", tmuxMock.readinessChecks)
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
		t.Fatal("atomic tmux readiness did not receive the operation request context")
	}
	if tmuxMock.paneSendCtx != wantCtx {
		t.Fatal("exact pane delivery did not receive the operation request context")
	}
	if backend.sendCtx != nil {
		t.Fatal("session-targeted manager ran after atomic exact-pane delivery")
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
