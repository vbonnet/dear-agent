package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type apiDeliveryTestAgent struct {
	status     agent.Status
	statusErr  error
	sendErr    error
	statusCtx  context.Context
	sendCtx    context.Context
	statusID   agent.SessionID
	sendID     agent.SessionID
	sent       agent.Message
	legacySend int
}

func (a *apiDeliveryTestAgent) Name() string { return "api-delivery-test" }

func (a *apiDeliveryTestAgent) Version() string { return "test" }

func (a *apiDeliveryTestAgent) CreateSession(agent.SessionContext) (agent.SessionID, error) {
	return "", nil
}

func (a *apiDeliveryTestAgent) ResumeSession(agent.SessionID) error { return nil }

func (a *apiDeliveryTestAgent) TerminateSession(agent.SessionID) error { return nil }

func (a *apiDeliveryTestAgent) GetSessionStatus(agent.SessionID) (agent.Status, error) {
	return a.status, a.statusErr
}

func (a *apiDeliveryTestAgent) SendMessage(agent.SessionID, agent.Message) error {
	a.legacySend++
	return nil
}

func (a *apiDeliveryTestAgent) GetHistory(agent.SessionID) ([]agent.Message, error) {
	return nil, nil
}

func (a *apiDeliveryTestAgent) ExportConversation(agent.SessionID, agent.ConversationFormat) ([]byte, error) {
	return nil, nil
}

func (a *apiDeliveryTestAgent) ImportConversation([]byte, agent.ConversationFormat) (agent.SessionID, error) {
	return "", nil
}

func (a *apiDeliveryTestAgent) Capabilities() agent.Capabilities { return agent.Capabilities{} }

func (a *apiDeliveryTestAgent) ExecuteCommand(agent.Command) error { return nil }

type readyAPIDeliveryTestAgent struct {
	*apiDeliveryTestAgent
}

func (a *readyAPIDeliveryTestAgent) GetSessionStatusContext(ctx context.Context, sessionID agent.SessionID) (agent.Status, error) {
	a.statusCtx = ctx
	a.statusID = sessionID
	return a.status, a.statusErr
}

type sendableAPIDeliveryTestAgent struct {
	*readyAPIDeliveryTestAgent
}

func (a *sendableAPIDeliveryTestAgent) SendMessageContext(ctx context.Context, sessionID agent.SessionID, message agent.Message) error {
	a.sendCtx = ctx
	a.sendID = sessionID
	a.sent = message
	return a.sendErr
}

func newAPIDeliveryTestSession(t *testing.T, lifecycle string) (*dolt.MockAdapter, *manifest.Manifest) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	storage := dolt.NewMockAdapter()
	current := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "api-delivery-session-id",
		Name:          "current-api-session",
		Harness:       "openai",
		Lifecycle:     lifecycle,
	}
	if err := storage.CreateSession(current); err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	return storage, current
}

func TestNewAPISessionAgentRejectsMissingOrNonAPISession(t *testing.T) {
	if _, err := NewAPISessionAgent(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "manifest is required") {
		t.Fatalf("NewAPISessionAgent(nil) error = %v, want required-manifest error", err)
	}
	if _, err := NewAPISessionAgent(t.Context(), &manifest.Manifest{Harness: "codex-cli"}); err == nil || !strings.Contains(err.Error(), "not a pure API session") {
		t.Fatalf("NewAPISessionAgent(codex-cli) error = %v, want pure-API error", err)
	}
}

func TestDeliverAPISessionMessageValidatesStableInputs(t *testing.T) {
	stale := &manifest.Manifest{SessionID: "stable-id"}
	for _, test := range []struct {
		name    string
		storage dolt.Storage
		stale   *manifest.Manifest
		want    string
	}{
		{name: "missing storage", stale: stale, want: "requires session storage"},
		{name: "missing manifest", storage: dolt.NewMockAdapter(), want: "requires a stable session ID"},
		{name: "missing session ID", storage: dolt.NewMockAdapter(), stale: &manifest.Manifest{}, want: "requires a stable session ID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DeliverAPISessionMessage(t.Context(), test.storage, test.stale, agent.Message{}, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DeliverAPISessionMessage() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDeliverAPISessionMessageReloadsCurrentManifestAndUsesContextContracts(t *testing.T) {
	for _, status := range []agent.Status{agent.StatusActive, agent.StatusIdle} {
		t.Run(string(status), func(t *testing.T) {
			storage, current := newAPIDeliveryTestSession(t, "")
			stale := &manifest.Manifest{
				SessionID: current.SessionID,
				Name:      "stale-api-session",
				Harness:   current.Harness,
			}
			base := &apiDeliveryTestAgent{status: status}
			adapter := &sendableAPIDeliveryTestAgent{
				readyAPIDeliveryTestAgent: &readyAPIDeliveryTestAgent{apiDeliveryTestAgent: base},
			}
			var factoryManifest *manifest.Manifest
			message := agent.Message{Role: agent.RoleUser, Content: "verified delivery"}

			delivered, err := DeliverAPISessionMessage(t.Context(), storage, stale, message, func(_ context.Context, got *manifest.Manifest) (agent.Agent, error) {
				factoryManifest = got
				return adapter, nil
			})
			if err != nil {
				t.Fatalf("DeliverAPISessionMessage() error: %v", err)
			}
			if delivered == nil || delivered.Name != current.Name {
				t.Fatalf("delivered manifest = %#v, want current stored manifest", delivered)
			}
			if factoryManifest == nil || factoryManifest.Name != current.Name {
				t.Fatalf("factory manifest = %#v, want locked current manifest", factoryManifest)
			}
			if adapter.statusID != agent.SessionID(current.SessionID) || adapter.sendID != agent.SessionID(current.SessionID) {
				t.Fatalf("adapter session IDs = (%q, %q), want %q", adapter.statusID, adapter.sendID, current.SessionID)
			}
			if adapter.statusCtx == nil {
				t.Fatal("readiness context is nil")
			}
			if _, ok := adapter.statusCtx.Deadline(); !ok {
				t.Fatal("readiness context has no bounded deadline")
			}
			if adapter.sendCtx == nil {
				t.Fatal("delivery context is nil")
			}
			if adapter.sent.Content != message.Content || adapter.sent.Role != message.Role {
				t.Fatalf("sent message = %#v, want %#v", adapter.sent, message)
			}
			if base.legacySend != 0 {
				t.Fatalf("legacy SendMessage called %d times, want context-aware delivery", base.legacySend)
			}
		})
	}
}

func TestDeliverAPISessionMessageRejectsReloadAndLifecycleChanges(t *testing.T) {
	t.Run("reload failure", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		storage := dolt.NewMockAdapter()
		_, err := DeliverAPISessionMessage(t.Context(), storage, &manifest.Manifest{
			SessionID: "missing-api-session",
			Name:      "missing",
		}, agent.Message{}, func(context.Context, *manifest.Manifest) (agent.Agent, error) {
			t.Fatal("factory called after reload failure")
			return nil, nil
		})
		if err == nil || !strings.Contains(err.Error(), "reload API session") {
			t.Fatalf("DeliverAPISessionMessage() error = %v, want reload error", err)
		}
	})

	for _, test := range []struct {
		name      string
		lifecycle string
		code      string
	}{
		{name: "archived", lifecycle: manifest.LifecycleArchived, code: ErrCodeSessionArchived},
		{name: "reaping", lifecycle: manifest.LifecycleReaping, code: ErrCodeSessionNotReady},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage, current := newAPIDeliveryTestSession(t, test.lifecycle)
			_, err := DeliverAPISessionMessage(t.Context(), storage, &manifest.Manifest{
				SessionID: current.SessionID,
				Name:      current.Name,
			}, agent.Message{}, func(context.Context, *manifest.Manifest) (agent.Agent, error) {
				t.Fatal("factory called for non-active lifecycle")
				return nil, nil
			})
			opErr := &OpError{}
			if !errors.As(err, &opErr) || opErr.Code != test.code {
				t.Fatalf("DeliverAPISessionMessage() error = %v, want %s", err, test.code)
			}
		})
	}
}

func TestDeliverAPISessionMessageRejectsIncompleteOrFailedAdapter(t *testing.T) {
	factoryErr := errors.New("factory failed")
	readinessErr := errors.New("readiness failed")
	sendErr := errors.New("send failed")

	for _, test := range []struct {
		name    string
		factory APISessionAgentFactory
		want    string
	}{
		{
			name: "factory failure",
			factory: func(context.Context, *manifest.Manifest) (agent.Agent, error) {
				return nil, factoryErr
			},
			want: factoryErr.Error(),
		},
		{
			name: "missing readiness contract",
			factory: func(context.Context, *manifest.Manifest) (agent.Agent, error) {
				return &apiDeliveryTestAgent{}, nil
			},
			want: "does not support context-aware readiness",
		},
		{
			name: "readiness failure",
			factory: func(context.Context, *manifest.Manifest) (agent.Agent, error) {
				return &readyAPIDeliveryTestAgent{apiDeliveryTestAgent: &apiDeliveryTestAgent{statusErr: readinessErr}}, nil
			},
			want: readinessErr.Error(),
		},
		{
			name: "not ready",
			factory: func(context.Context, *manifest.Manifest) (agent.Agent, error) {
				return &readyAPIDeliveryTestAgent{apiDeliveryTestAgent: &apiDeliveryTestAgent{status: agent.StatusSuspended}}, nil
			},
			want: "is not ready for direct delivery",
		},
		{
			name: "missing delivery contract",
			factory: func(context.Context, *manifest.Manifest) (agent.Agent, error) {
				return &readyAPIDeliveryTestAgent{apiDeliveryTestAgent: &apiDeliveryTestAgent{status: agent.StatusIdle}}, nil
			},
			want: "does not support context-aware delivery",
		},
		{
			name: "delivery failure",
			factory: func(context.Context, *manifest.Manifest) (agent.Agent, error) {
				return &sendableAPIDeliveryTestAgent{
					readyAPIDeliveryTestAgent: &readyAPIDeliveryTestAgent{
						apiDeliveryTestAgent: &apiDeliveryTestAgent{status: agent.StatusIdle, sendErr: sendErr},
					},
				}, nil
			},
			want: sendErr.Error(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage, current := newAPIDeliveryTestSession(t, "")
			delivered, err := DeliverAPISessionMessage(t.Context(), storage, current, agent.Message{Content: "must not deliver"}, test.factory)
			if delivered != nil {
				t.Fatalf("delivered manifest = %#v, want nil on failure", delivered)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DeliverAPISessionMessage() error = %v, want %q", err, test.want)
			}
		})
	}
}
