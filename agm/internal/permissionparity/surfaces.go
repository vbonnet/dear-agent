// Package permissionparity defines executable coverage for permission policy
// surfaces across supported harnesses.
package permissionparity

import (
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// Surface describes how AGM carries a resolved permission policy through a
// harness. Some harnesses expose native allowlists; others only expose startup
// sandbox or mode controls, so the manifest remains the shared source of truth.
type Surface struct {
	Harness           string
	PolicySurface     string
	StartupSurface    string
	RuntimeSurface    string
	NativeEnforcement string
}

// ActiveHarnessSurfaces returns the permission-policy surfaces for every active
// parity harness.
func ActiveHarnessSurfaces() []Surface {
	active := agent.ActiveHarnesses()
	out := make([]Surface, 0, len(active))
	for _, harness := range active {
		surface, ok := SurfaceForHarness(harness)
		if ok {
			out = append(out, surface)
		}
	}
	return out
}

// SurfaceForHarness returns the permission-policy surface for a harness.
func SurfaceForHarness(harness string) (Surface, bool) {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return Surface{
			Harness:           "claude-code",
			PolicySurface:     ".claude/settings.local.json permissions.allow",
			StartupSurface:    "claude --permission-mode",
			RuntimeSurface:    "Shift+Tab and /plan",
			NativeEnforcement: "Claude Code allowlist and permission modes",
		}, true
	case "codex-cli":
		return Surface{
			Harness:           "codex-cli",
			PolicySurface:     "AGM manifest permission_policy",
			StartupSurface:    "codex -s workspace-write",
			RuntimeSurface:    "manifest-only mode record",
			NativeEnforcement: "Codex sandbox and approval prompts",
		}, true
	case "agy":
		return Surface{
			Harness:           "agy",
			PolicySurface:     "AGM manifest permission_policy",
			StartupSurface:    "agy --dangerously-skip-permissions for auto mode",
			RuntimeSurface:    "manifest-only mode record",
			NativeEnforcement: "AGY native auto bypass when requested",
		}, true
	case "opencode-cli":
		return Surface{
			Harness:           "opencode-cli",
			PolicySurface:     "AGM manifest permission_policy",
			StartupSurface:    "opencode attach",
			RuntimeSurface:    "Tab plan/default where supported",
			NativeEnforcement: "OpenCode server policy plus AGM manifest",
		}, true
	case "pi-cli":
		return Surface{
			Harness:           "pi-cli",
			PolicySurface:     "AGM manifest permission_policy and managed Pi extension allowlist",
			StartupSurface:    "pi --tools plus AGM authorization extension",
			RuntimeSurface:    "/agm-mode plan|default|auto",
			NativeEnforcement: "Pi tool_call blocking, interactive confirmation, and active-tool restriction",
		}, true
	default:
		return Surface{}, false
	}
}

// ValidateActiveHarnessSurfaces verifies that every active harness has a
// non-empty permission surface declaration.
func ValidateActiveHarnessSurfaces() error {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no permission parity surface", harness)
		}
		if surface.PolicySurface == "" || surface.StartupSurface == "" || surface.RuntimeSurface == "" || surface.NativeEnforcement == "" {
			return fmt.Errorf("active harness %q has incomplete permission parity surface: %+v", harness, surface)
		}
	}
	return nil
}
