// Package ops provides a shared operations layer for AGM.
//
// All three API surfaces (CLI, MCP, Skills) call these functions
// instead of implementing business logic directly. This ensures
// consistent behavior, error handling, and output formatting.
package ops

import (
	"context"
	"encoding/json"

	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// OpContext holds dependencies for all operations.
// CLI, MCP, and Skills each construct an OpContext and pass it to ops.
type OpContext struct {
	// Context carries request cancellation into external I/O performed by an
	// operation. Nil preserves compatibility for non-request-scoped callers.
	Context    context.Context
	Storage    dolt.Storage
	Tmux       session.TmuxInterface
	Config     *config.Config
	DryRun     bool
	Fields     []string // field mask: if non-empty, only include these fields in output
	OutputMode string   // "json", "text" (default: "json" for programmatic consumers)
	Detailed   bool     // --detailed: re-enable IDs, full paths, and verbose hints in agent-mode output
	// ExternalSessionArchiver synchronizes a successfully archived AGM session
	// with its harness-specific desktop or remote representation. Nil selects
	// the production dispatcher; tests inject a deterministic implementation.
	ExternalSessionArchiver ExternalSessionArchiver
	// CreationRuntime adapts harness-specific interactive startup and
	// post-registration completion without exposing lifecycle phase hooks.
	CreationRuntime CreateSessionRuntime
	// OpenSessionStorage lazily constructs surface-specific session storage.
	// Nil uses Storage directly.
	OpenSessionStorage SessionStorageOpener
	// CodexThreadCreator adapts the external Codex remote-control dependency.
	// Nil selects the production implementation.
	CodexThreadCreator CodexThreadCreator
	// AgyWorkspaceCreateLocker serializes the provider-global identity window
	// across every AGY creation surface. Nil selects the production lock.
	AgyWorkspaceCreateLocker AgyWorkspaceCreateLocker
	// AgyCreateIdentityTracker snapshots and correlates the provider-native AGY
	// conversation while the shared workspace lock is held. Nil selects the
	// production tracker.
	AgyCreateIdentityTracker agysession.CreateIdentityTracker
	// APIDeliveryFactory reconstructs a pure API adapter from the session's
	// persisted non-secret runtime configuration. Nil selects the production
	// factory. Tests and non-CLI surfaces may inject a deterministic adapter.
	APIDeliveryFactory APISessionDeliveryFactory
	// archiveSandboxCleaner is an internal archive-boundary seam. Production
	// uses the host sandbox safety checker; package tests inject the same
	// allowlist behavior without consulting the real process or mount tables.
	archiveSandboxCleaner sandboxCleanupFunc
}

func requestContext(opCtx *OpContext) context.Context {
	if opCtx != nil && opCtx.Context != nil {
		return opCtx.Context
	}
	return context.Background()
}

// Result is the base type for all operation results.
// Every op returns a concrete result type that embeds or follows this pattern.
type Result struct {
	Operation string `json:"operation"`
	Success   bool   `json:"success"`
}

// ApplyFieldMask filters a JSON-serializable result to only include specified fields.
// Returns the original value if fields is empty.
func ApplyFieldMask(v any, fields []string) (json.RawMessage, error) {
	if len(fields) == 0 {
		return json.Marshal(v)
	}

	// Marshal to map, then filter
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		// Not an object — return as-is
		return data, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	filtered := make(map[string]json.RawMessage, len(fields))
	for _, f := range fields {
		if val, ok := m[f]; ok {
			filtered[f] = val
		}
	}

	return json.Marshal(filtered)
}
