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
	"codex-cli":    "neutral marketplace plus AGENTS.md/SKILL fallback",
	"agy":          "neutral marketplace plus AGENTS.md/SKILL fallback",
	"opencode-cli": "neutral marketplace plus AGENTS.md/SKILL fallback",
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

// ValidateAssets verifies shared Wayfinder files and plugin assets exist as
// regular files contained in the repository.
func ValidateAssets(root string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root for Wayfinder assets: %w", err)
	}
	for _, rel := range []string{
		"wayfinder/SPEC.md",
		"wayfinder/SKILL.md",
		"wayfinder/.claude-plugin/plugin.json",
		"wayfinder/ARCHITECTURE.md",
		"wayfinder/cmd/wayfinder-session/SPEC.md",
	} {
		if err := requireContainedRegularFile(resolvedRoot, root, rel); err != nil {
			return err
		}
	}
	return ValidatePiSkillDiscovery(root)
}

// requireContainedRegularFile rejects a Wayfinder asset that a clean clone or
// package would not carry. os.Stat alone follows links, so a repository-local
// symlink to an external regular file — or a real file reached through an
// intermediate directory symlink — would otherwise report the asset present.
func requireContainedRegularFile(resolvedRoot, root, rel string) error {
	path := filepath.Join(root, rel)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("wayfinder asset %s: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("wayfinder asset %s is not a regular file", rel)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve wayfinder asset %s: %w", rel, err)
	}
	if !containedWithin(resolvedRoot, resolved) {
		return fmt.Errorf("wayfinder asset %s escapes the repository", rel)
	}
	return nil
}

// ValidatePiSkillDiscovery verifies Pi reads the living skill trees instead of
// relying on copied or harness-specific skill definitions.
func ValidatePiSkillDiscovery(root string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root for Pi settings: %w", err)
	}
	settingsPath := filepath.Join(root, ".pi", "settings.json")
	resolvedSettings, err := filepath.EvalSymlinks(settingsPath)
	if err != nil {
		return fmt.Errorf("resolve Pi settings: %w", err)
	}
	if !containedWithin(resolvedRoot, resolvedSettings) {
		return fmt.Errorf("pi settings escape the repository")
	}
	info, err := os.Stat(resolvedSettings)
	if err != nil {
		return fmt.Errorf("stat Pi settings: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("pi settings are not a regular file")
	}
	data, err := os.ReadFile(resolvedSettings)
	if err != nil {
		return fmt.Errorf("read Pi settings: %w", err)
	}
	var settings struct {
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse Pi settings: %w", err)
	}
	for _, required := range []string{"../.agents/skills", "../agm/plugins"} {
		if !slices.Contains(settings.Skills, required) {
			return fmt.Errorf("pi settings missing required native skill discovery path %q", required)
		}
	}
	for _, declared := range settings.Skills {
		if err := validatePiSkillRoot(root, declared); err != nil {
			return err
		}
	}
	return nil
}

// containedWithin reports whether path lies inside root. Both arguments must
// already be resolved when the caller needs symlink-proof containment.
func containedWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validatePiSkillRoot(root, declared string) error {
	skillRoot := filepath.Clean(filepath.Join(root, ".pi", filepath.FromSlash(declared)))
	if !containedWithin(root, skillRoot) {
		return fmt.Errorf("pi skill discovery path %q escapes the repository", declared)
	}
	// The lexical check above cannot see through an intermediate symlink, so
	// resolve both sides before trusting containment and before loading any
	// entrypoint. Otherwise a declared root that links outside the repository
	// lets Wayfinder parity pass on assets absent from a clone.
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root for pi skill discovery path %q: %w", declared, err)
	}
	resolvedSkillRoot, err := filepath.EvalSymlinks(skillRoot)
	if err != nil {
		return fmt.Errorf("pi skill discovery path %q: %w", declared, err)
	}
	if !containedWithin(resolvedRoot, resolvedSkillRoot) {
		return fmt.Errorf("pi skill discovery path %q escapes the repository", declared)
	}
	info, err := os.Stat(resolvedSkillRoot)
	if err != nil {
		return fmt.Errorf("pi skill discovery path %q: %w", declared, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("pi skill discovery path %q is not a directory", declared)
	}
	entries, err := os.ReadDir(resolvedSkillRoot)
	if err != nil {
		return fmt.Errorf("read Pi skill discovery path %q: %w", declared, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		entrypoint := filepath.Join(resolvedSkillRoot, entry.Name(), "SKILL.md")
		resolvedEntrypoint, resolveErr := filepath.EvalSymlinks(entrypoint)
		if resolveErr != nil || !containedWithin(resolvedRoot, resolvedEntrypoint) {
			continue
		}
		if info, statErr := os.Stat(resolvedEntrypoint); statErr == nil && info.Mode().IsRegular() {
			return nil
		}
	}
	return fmt.Errorf("pi skill discovery path %q contains no skill entrypoint", declared)
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
