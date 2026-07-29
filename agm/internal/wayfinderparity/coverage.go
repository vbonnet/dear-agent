// Package wayfinderparity defines executable coverage for harness-neutral
// Wayfinder surfaces.
package wayfinderparity

import (
	"encoding/json"
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

var expectedDiscoverySurfaces = map[string]string{
	"claude-code":  "native Claude plugin root skill",
	"codex-cli":    ".agents/skills/wayfinder/SKILL.md native skill discovery",
	"agy":          "neutral marketplace plus AGENTS.md fallback",
	"opencode-cli": ".opencode/skills/wayfinder/SKILL.md native skill discovery",
	"pi-cli":       ".pi/settings.json native skill discovery plus AGENTS.md",
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
			DiscoverySurface: expectedDiscoverySurfaces["codex-cli"],
			ExecutionSurface: "wayfinder session CLI",
			StatusSurface:    "MCP Wayfinder tools and WAYFINDER-STATUS.md",
		}, true
	case "agy":
		return HarnessSurface{
			Harness:          "agy",
			DiscoverySurface: expectedDiscoverySurfaces["agy"],
			ExecutionSurface: "wayfinder session CLI",
			StatusSurface:    "MCP Wayfinder tools and WAYFINDER-STATUS.md",
		}, true
	case "opencode-cli":
		return HarnessSurface{
			Harness:          "opencode-cli",
			DiscoverySurface: expectedDiscoverySurfaces["opencode-cli"],
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
		if expected := expectedDiscoverySurfaces[harness]; surface.DiscoverySurface != expected {
			return fmt.Errorf("active harness %q discovery surface = %q, want %q", harness, surface.DiscoverySurface, expected)
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
		".agents/skills/wayfinder/SKILL.md",
		".opencode/skills/wayfinder/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("wayfinder asset %s: %w", rel, err)
		}
	}
	return ValidatePiSkillDiscovery(root)
}

// ValidatePiSkillDiscovery verifies Pi reads the living skill trees instead of
// relying on copied or harness-specific skill definitions.
func ValidatePiSkillDiscovery(root string) error {
	data, err := os.ReadFile(filepath.Join(root, ".pi", "settings.json"))
	if err != nil {
		return fmt.Errorf("read Pi settings: %w", err)
	}
	var settings struct {
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse Pi settings: %w", err)
	}
	const sharedSkillRoot = "../.agents/skills"
	if !slices.Contains(settings.Skills, sharedSkillRoot) {
		return fmt.Errorf("pi settings missing shared native skill discovery path %q", sharedSkillRoot)
	}
	for _, declared := range settings.Skills {
		skillRoot := filepath.Clean(filepath.Join(root, ".pi", filepath.FromSlash(declared)))
		relative, relErr := filepath.Rel(root, skillRoot)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("pi skill discovery path %q escapes the repository", declared)
		}
		info, err := os.Stat(skillRoot)
		if err != nil {
			return fmt.Errorf("pi skill discovery path %q: %w", declared, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("pi skill discovery path %q is not a directory", declared)
		}
		entrypoints, err := filepath.Glob(filepath.Join(skillRoot, "*", "SKILL.md"))
		if err != nil {
			return fmt.Errorf("glob Pi skill entrypoints under %q: %w", declared, err)
		}
		found := false
		for _, entrypoint := range entrypoints {
			if info, statErr := os.Stat(entrypoint); statErr == nil && info.Mode().IsRegular() {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("pi skill discovery path %q contains no skill entrypoint", declared)
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
