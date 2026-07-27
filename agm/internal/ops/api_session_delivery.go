package ops

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// APISessionDeliveryAdapter is the complete capability set required by the
// pure API message transaction. It deliberately excludes unrelated lifecycle,
// history, import/export, and command behavior.
type APISessionDeliveryAdapter interface {
	agent.ContextSessionStatusGetter
	agent.ContextMessageSender
}

// APISessionDeliveryFactory reconstructs a pure API delivery adapter from a
// current AGM manifest. Credentials are intentionally resolved at call time
// rather than persisted in the manifest.
type APISessionDeliveryFactory func(context.Context, *manifest.Manifest) (APISessionDeliveryAdapter, error)

// NewAPISessionDeliveryAdapter reconstructs the production adapter for a pure
// API session using its persisted non-secret runtime locator.
func NewAPISessionDeliveryAdapter(ctx context.Context, m *manifest.Manifest) (APISessionDeliveryAdapter, error) {
	if m == nil {
		return nil, fmt.Errorf("API session manifest is required")
	}
	switch m.Harness {
	case "openai", "gpt":
		apiConfig := &agent.OpenAIConfig{Model: m.Model}
		if m.OpenAI != nil {
			apiConfig.SessionsDir = m.OpenAI.SessionsDir
			apiConfig.BaseURL = m.OpenAI.BaseURL
			apiConfig.IsAzure = m.OpenAI.IsAzure
			apiConfig.AzureAPIVersion = m.OpenAI.AzureAPIVersion
			apiConfig.Temperature = m.OpenAI.Temperature
			apiConfig.MaxTokens = m.OpenAI.MaxTokens
		}
		return agent.NewOpenAIAdapterForSession(ctx, agent.SessionID(m.SessionID), apiConfig)
	default:
		return nil, fmt.Errorf("harness %q is not a pure API session", m.Harness)
	}
}

// DeliverAPISessionMessage owns the lifecycle/readiness/provider transaction
// for every AGM surface. The stable session-ID lock is shared with archive;
// the locked reload therefore decides whether provider work may begin.
func DeliverAPISessionMessage(ctx context.Context, storage dolt.Storage, stale *manifest.Manifest, message agent.Message, newAdapter APISessionDeliveryFactory) (*manifest.Manifest, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if storage == nil {
		return nil, fmt.Errorf("verified API delivery requires session storage")
	}
	if stale == nil || stale.SessionID == "" {
		return nil, fmt.Errorf("verified API delivery requires a stable session ID")
	}
	if newAdapter == nil {
		newAdapter = NewAPISessionDeliveryAdapter
	}
	recipient := stale.Name
	if recipient == "" {
		recipient = stale.SessionID
	}

	preflightCtx, cancelPreflight := context.WithTimeout(ctx, agent.OpenAIPreflightTimeout)
	defer cancelPreflight()
	var delivered *manifest.Manifest
	err := WithAPISessionLockContext(preflightCtx, stale.SessionID, func() error {
		current, err := storage.GetSession(stale.SessionID)
		if err != nil {
			return fmt.Errorf("reload API session %q under mutation lock: %w", recipient, err)
		}
		if current == nil {
			return ErrSessionNotFound(stale.SessionID)
		}
		if err := requireActiveDeliverySession(current, recipient); err != nil {
			return err
		}
		if err := deliverThroughAPIAdapter(preflightCtx, ctx, current, recipient, message, newAdapter); err != nil {
			return err
		}
		delivered = current
		return nil
	})
	return delivered, err
}

func deliverThroughAPIAdapter(preflightCtx, deliveryCtx context.Context, current *manifest.Manifest, recipient string, message agent.Message, newAdapter APISessionDeliveryFactory) error {
	adapter, err := newAdapter(preflightCtx, current)
	if err != nil {
		return fmt.Errorf("create API harness adapter for %q: %w", recipient, err)
	}
	sessionID := agent.SessionID(current.SessionID)
	status, err := adapter.GetSessionStatusContext(preflightCtx, sessionID)
	if err != nil {
		return fmt.Errorf("check API session %q readiness: %w", recipient, err)
	}
	if status != agent.StatusActive && status != agent.StatusIdle {
		return fmt.Errorf("API session %q is not ready for direct delivery (status %s)", recipient, status)
	}
	if err := adapter.SendMessageContext(deliveryCtx, sessionID, message); err != nil {
		return fmt.Errorf("failed to send message via harness: %w", err)
	}
	return nil
}
