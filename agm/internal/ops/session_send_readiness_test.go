package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

type postRecoveryReadyTmux struct {
	*mockTmux
	result session.InputReadiness
}

func (t *postRecoveryReadyTmux) SendKeysIfInputReady(ctx context.Context, sessionName, harness, keys string, options session.InputDeliveryOptions) (session.InputReadiness, error) {
	t.atomicChecks = append(t.atomicChecks, sessionName+":"+harness)
	t.atomicOptions = append(t.atomicOptions, options)
	t.paneSendCtx = ctx
	if t.result.Ready {
		t.sent = append(t.sent, sentKey{session: t.result.PaneID, keys: keys})
	}
	return t.result, nil
}

func TestSendMessage_AtomicReadinessAndDeliveryIsTheLocalRuntimePath(t *testing.T) {
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "hello"})
	if err != nil || result == nil || !result.Delivered {
		t.Fatalf("SendMessage() = (%#v, %v), want exact-ready delivery", result, err)
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

func TestSendMessage_AcceptsPostRecoveryReadyState(t *testing.T) {
	t.Parallel()

	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	tmuxClient := &postRecoveryReadyTmux{
		mockTmux: ctx.Tmux.(*mockTmux),
		result:   session.InputReadiness{Ready: true, State: "YES", PaneID: "%7", Forced: true},
	}
	ctx.Tmux = tmuxClient

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "replacement", Force: true})
	if err != nil || result == nil || !result.Delivered {
		t.Fatalf("SendMessage(post-recovery YES) = (%#v, %v), want delivered", result, err)
	}
	if len(tmuxClient.atomicOptions) != 1 || !tmuxClient.atomicOptions[0].AllowQueuedAGM {
		t.Fatalf("atomic options = %#v, want queued-AGM recovery", tmuxClient.atomicOptions)
	}
	if len(tmuxClient.sent) != 1 || tmuxClient.sent[0] != (sentKey{session: "%7", keys: "replacement"}) {
		t.Fatalf("post-recovery sends = %#v, want one exact-pane replacement", tmuxClient.sent)
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
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	ctx.Context = wantCtx
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
}

func TestSendMessage_CancelledRequestNeverChecksOrSends(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := testCtx([]*manifest.Manifest{newManifest("id-1", "my-session", "~/project")}, "my-session")
	ctx.Context = cancelled
	tmuxMock := ctx.Tmux.(*mockTmux)

	result, err := SendMessage(ctx, &SendMessageRequest{Recipient: "id-1", Message: "must not send"})
	if err == nil || result == nil || result.Delivered {
		t.Fatalf("SendMessage() = (%#v, %v), want cancelled non-delivery", result, err)
	}
	if len(tmuxMock.readinessChecks) != 0 || len(tmuxMock.sent) != 0 {
		t.Fatalf("cancelled send performed I/O: tmux checks=%v tmux sends=%v",
			tmuxMock.readinessChecks, tmuxMock.sent)
	}
}
