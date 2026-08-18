// Package wayfinderparity defines executable coverage for harness-neutral
// Wayfinder surfaces.
package wayfinderparity

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/pkg/phaseengram"
)

// HarnessSurface describes how a harness discovers, executes, and reports
// Wayfinder sessions.
type HarnessSurface struct {
	Harness          string
	DiscoverySurface string
	ExecutionSurface string
	StatusSurface    string
}

// SurfaceForHarness returns the Wayfinder surface for a harness.
func SurfaceForHarness(harness string) (HarnessSurface, bool) {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return HarnessSurface{
			Harness:          "claude-code",
			DiscoverySurface: "native Claude plugin root skill",
			ExecutionSurface: "wayfinder session CLI",
			StatusSurface:    "MCP Wayfinder tools and WAYFINDER-STATUS.md",
		}, true
	case "codex-cli":
		return HarnessSurface{
			Harness:          "codex-cli",
			DiscoverySurface: "neutral marketplace plus AGENTS.md/SKILL fallback",
			ExecutionSurface: "wayfinder session CLI",
			StatusSurface:    "MCP Wayfinder tools and WAYFINDER-STATUS.md",
		}, true
	case "agy":
		return HarnessSurface{
			Harness:          "agy",
			DiscoverySurface: "neutral marketplace plus AGENTS.md/SKILL fallback",
			ExecutionSurface: "wayfinder session CLI",
			StatusSurface:    "MCP Wayfinder tools and WAYFINDER-STATUS.md",
		}, true
	case "opencode-cli":
		return HarnessSurface{
			Harness:          "opencode-cli",
			DiscoverySurface: "neutral marketplace plus AGENTS.md/SKILL fallback",
			ExecutionSurface: "wayfinder session CLI",
			StatusSurface:    "MCP Wayfinder tools and WAYFINDER-STATUS.md",
		}, true
	case "pi-cli":
		return HarnessSurface{
			Harness:          "pi-cli",
			DiscoverySurface: ".pi/settings.json native skill discovery plus AGENTS.md",
			ExecutionSurface: "Wayfinder skill and wayfinder-session CLI",
			StatusSurface:    "MCP Wayfinder tools and WAYFINDER-STATUS.md",
		}, true
	default:
		return HarnessSurface{}, false
	}
}

// ActiveHarnessSurfaces returns Wayfinder surfaces for all active harnesses.
func ActiveHarnessSurfaces() []HarnessSurface {
	active := agent.ActiveHarnesses()
	out := make([]HarnessSurface, 0, len(active))
	for _, harness := range active {
		surface, ok := SurfaceForHarness(harness)
		if ok {
			out = append(out, surface)
		}
	}
	return out
}

// ValidateActiveHarnessSurfaces verifies all active harnesses have complete
// Wayfinder surfaces.
func ValidateActiveHarnessSurfaces() error {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no Wayfinder surface", harness)
		}
		if surface.DiscoverySurface == "" || surface.ExecutionSurface == "" || surface.StatusSurface == "" {
			return fmt.Errorf("active harness %q has incomplete Wayfinder surface: %+v", harness, surface)
		}
	}
	return nil
}

// ValidateAssets verifies shared Wayfinder files and plugin assets exist.
func ValidateAssets(root string) error {
	for _, rel := range []string{
		"wayfinder/SPEC.md",
		"wayfinder/SKILL.md",
		"wayfinder/.claude-plugin/plugin.json",
		"wayfinder/ARCHITECTURE.md",
		"wayfinder/cmd/wayfinder-session/SPEC.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("wayfinder asset %s: %w", rel, err)
		}
	}
	return nil
}

// ValidateMCPOperations verifies Wayfinder status operations are exposed via
// the MCP operation registry.
func ValidateMCPOperations() error {
	required := map[string]bool{
		"list_wayfinder_sessions": false,
		"get_wayfinder_session":   false,
	}
	for _, op := range ops.ListOps().Operations {
		if _, ok := required[op.Name]; ok && strings.Contains(op.Surface, "mcp") {
			required[op.Name] = true
		}
	}
	for name, ok := range required {
		if !ok {
			return fmt.Errorf("wayfinder mcp operation %q missing from ops registry", name)
		}
	}
	return nil
}

// ValidatePhaseEngramCoverage verifies core Wayfinder phases have Engram
// registry coverage.
func ValidatePhaseEngramCoverage() error {
	for _, phase := range []string{"CHARTER", "RESEARCH", "SPEC", "BUILD", "RETRO"} {
		if !slices.Contains(phaseengram.KnownPhases(), phase) {
			return fmt.Errorf("phase engram registry missing %s", phase)
		}
	}
	return nil
}
