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

type apiDeliveryTestAdapter struct {
	status    agent.Status
	statusErr error
	sendErr   error
	statusCtx context.Context
	sendCtx   context.Context
	statusID  agent.SessionID
	sendID    agent.SessionID
	sent      agent.Message
}

func (a *apiDeliveryTestAdapter) GetSessionStatusContext(ctx context.Context, sessionID agent.SessionID) (agent.Status, error) {
	a.statusCtx = ctx
	a.statusID = sessionID
	return a.status, a.statusErr
}

func (a *apiDeliveryTestAdapter) SendMessageContext(ctx context.Context, sessionID agent.SessionID, message agent.Message) error {
	a.sendCtx = ctx
	a.sendID = sessionID
	a.sent = message
	return a.sendErr
}

var _ APISessionDeliveryAdapter = (*apiDeliveryTestAdapter)(nil)

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

func TestNewAPISessionDeliveryAdapterRejectsMissingOrNonAPISession(t *testing.T) {
	if _, err := NewAPISessionDeliveryAdapter(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "manifest is required") {
		t.Fatalf("NewAPISessionDeliveryAdapter(nil) error = %v, want required-manifest error", err)
	}
	if _, err := NewAPISessionDeliveryAdapter(t.Context(), &manifest.Manifest{Harness: "codex-cli"}); err == nil || !strings.Contains(err.Error(), "not a pure API session") {
		t.Fatalf("NewAPISessionDeliveryAdapter(codex-cli) error = %v, want pure-API error", err)
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
			adapter := &apiDeliveryTestAdapter{status: status}
			var factoryManifest *manifest.Manifest
			message := agent.Message{Role: agent.RoleUser, Content: "verified delivery"}

			delivered, err := DeliverAPISessionMessage(t.Context(), storage, stale, message, func(_ context.Context, got *manifest.Manifest) (APISessionDeliveryAdapter, error) {
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
		})
	}
}

func TestSendMessageAPIResultIncludesStableSessionID(t *testing.T) {
	storage, current := newAPIDeliveryTestSession(t, "")
	adapter := &apiDeliveryTestAdapter{status: agent.StatusActive}

	result, err := SendMessage(&OpContext{
		Context: t.Context(),
		Storage: storage,
		APIDeliveryFactory: func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
			return adapter, nil
		},
	}, &SendMessageRequest{
		Recipient: current.Name,
		Message:   "shared API delivery",
	})
	if err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}
	if result == nil || !result.Delivered || result.SessionID != current.SessionID {
		t.Fatalf("SendMessage() result = %#v, want delivered stable ID %q", result, current.SessionID)
	}
}

func TestDeliverAPISessionMessageRejectsReloadAndLifecycleChanges(t *testing.T) {
	t.Run("reload failure", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		storage := dolt.NewMockAdapter()
		_, err := DeliverAPISessionMessage(t.Context(), storage, &manifest.Manifest{
			SessionID: "missing-api-session",
			Name:      "missing",
		}, agent.Message{}, func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
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
			}, agent.Message{}, func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
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

func TestDeliverAPISessionMessageRejectsFailedAdapter(t *testing.T) {
	factoryErr := errors.New("factory failed")
	readinessErr := errors.New("readiness failed")
	sendErr := errors.New("send failed")

	for _, test := range []struct {
		name    string
		factory APISessionDeliveryFactory
		want    string
	}{
		{
			name: "factory failure",
			factory: func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
				return nil, factoryErr
			},
			want: factoryErr.Error(),
		},
		{
			name: "nil adapter",
			factory: func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
				return nil, nil
			},
			want: "factory returned nil adapter",
		},
		{
			name: "readiness failure",
			factory: func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
				return &apiDeliveryTestAdapter{statusErr: readinessErr}, nil
			},
			want: readinessErr.Error(),
		},
		{
			name: "not ready",
			factory: func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
				return &apiDeliveryTestAdapter{status: agent.StatusSuspended}, nil
			},
			want: "is not ready for direct delivery",
		},
		{
			name: "delivery failure",
			factory: func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error) {
				return &apiDeliveryTestAdapter{status: agent.StatusIdle, sendErr: sendErr}, nil
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
