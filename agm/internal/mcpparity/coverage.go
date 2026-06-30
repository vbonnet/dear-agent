package mcpparity

import (
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// CreateSessionSurface describes the MCP session-create contract for a harness.
type CreateSessionSurface struct {
	Harness      string
	DefaultModel string
	Deprecated   bool
	ModelPolicy  string
}

// CreateSessionSurfaceFor returns the MCP session-create contract for a known
// harness.
func CreateSessionSurfaceFor(harness string) (CreateSessionSurface, bool) {
	normalized := agent.NormalizeHarnessName(harness)
	if err := agent.ValidateHarnessName(normalized); err != nil {
		return CreateSessionSurface{}, false
	}
	model, ok := agent.DefaultModelForHarness(normalized)
	if !ok {
		model = "sonnet"
	}
	if err := agent.ValidateModel(normalized, model); err != nil {
		return CreateSessionSurface{}, false
	}
	return CreateSessionSurface{
		Harness:      normalized,
		DefaultModel: model,
		Deprecated:   agent.IsDeprecatedHarness(normalized),
		ModelPolicy:  "shared-agent-registry",
	}, true
}

// ActiveCreateSessionSurfaces returns MCP create-session coverage for all
// active harnesses.
func ActiveCreateSessionSurfaces() []CreateSessionSurface {
	active := agent.ActiveHarnesses()
	out := make([]CreateSessionSurface, 0, len(active))
	for _, harness := range active {
		surface, ok := CreateSessionSurfaceFor(harness)
		if ok {
			out = append(out, surface)
		}
	}
	return out
}

// ValidateActiveCreateSessionSurfaces verifies active harnesses are accepted by
// the MCP create-session validation contract.
func ValidateActiveCreateSessionSurfaces() error {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := CreateSessionSurfaceFor(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no MCP create-session surface", harness)
		}
		if surface.DefaultModel == "" || surface.ModelPolicy == "" {
			return fmt.Errorf("active harness %q has incomplete MCP create-session surface: %+v", harness, surface)
		}
	}
	return nil
}

// ValidateModelIdentifier verifies MCP accepts model identifiers through the
// same shared model validator used by CLI entrypoints.
func ValidateModelIdentifier(harness, model string) error {
	normalized := agent.NormalizeHarnessName(harness)
	if err := agent.ValidateHarnessName(normalized); err != nil {
		return err
	}
	return agent.ValidateModel(normalized, model)
}

// HasMCPOperation reports whether the ops discovery registry exposes the named
// operation on the MCP surface.
func HasMCPOperation(name string) bool {
	for _, op := range ops.ListOps().Operations {
		if op.Name == name && strings.Contains(op.Surface, "mcp") {
			return true
		}
	}
	return false
}

// ValidateLifecycleOperations verifies MCP discovery includes lifecycle
// mutations required for parity.
func ValidateLifecycleOperations() error {
	for _, name := range []string{"create_session", "send_message"} {
		if !HasMCPOperation(name) {
			return fmt.Errorf("MCP operation %q missing from ops registry", name)
		}
	}
	return nil
}
