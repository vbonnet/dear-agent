// Package ops provides a shared operations layer for AGM.
//
// All three API surfaces (CLI, MCP, Skills) call these functions
// instead of implementing business logic directly. This ensures
// consistent behavior, error handling, and output formatting.
package ops

import (
	"encoding/json"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manager"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// OpContext holds dependencies for all operations.
// CLI, MCP, and Skills each construct an OpContext and pass it to ops.
type OpContext struct {
	Storage    dolt.Storage
	Tmux       session.TmuxInterface
	Manager    manager.Backend // New abstraction layer (optional, nil = legacy path)
	Config     *config.Config
	DryRun     bool
	Fields     []string // field mask: if non-empty, only include these fields in output
	OutputMode string   // "json", "text" (default: "json" for programmatic consumers)
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
