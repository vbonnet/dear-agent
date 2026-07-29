// Package marketplaceparity defines executable coverage for neutral plugin
// marketplace publication across harnesses.
package marketplaceparity

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/pkg/skilllint"
	"gopkg.in/yaml.v3"
)

const (
	// NeutralCatalogPath is the harness-neutral marketplace catalog path.
	NeutralCatalogPath = ".dear-agent/marketplace.json"
	// ClaudeCatalogPath is the native Claude marketplace mirror path.
	ClaudeCatalogPath = ".claude-plugin/marketplace.json"
)

var requiredPluginNames = []string{"agm", "wayfinder", "youtube", "research-pipeline"}
var requiredPluginCapabilities = map[string][]string{
	"agm":               {"commands", "skills"},
	"wayfinder":         {"skills"},
	"youtube":           {"commands"},
	"research-pipeline": {"skills"},
}

// Owner describes the marketplace owner metadata.
type Owner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// PluginEntry describes one published plugin in a marketplace catalog.
type PluginEntry struct {
	Name         string   `json:"name"`
	Source       string   `json:"source"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// HarnessSurface describes a harness marketplace discovery surface.
type HarnessSurface struct {
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Catalog string `json:"catalog"`
}

// Catalog describes the harness-neutral marketplace catalog.
type Catalog struct {
	SchemaVersion string           `json:"schema_version"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Owner         Owner            `json:"owner"`
	Plugins       []PluginEntry    `json:"plugins"`
	Harnesses     []HarnessSurface `json:"harnesses"`
}

type claudeCatalog struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Owner       Owner         `json:"owner"`
	Plugins     []PluginEntry `json:"plugins"`
}

type claudePluginManifest struct {
	Name   string   `json:"name"`
	Skills []string `json:"skills"`
}

type piSettings struct {
	Skills []string `json:"skills"`
}

type exportedSkill struct {
	Name          string
	CanonicalPath string
}

// LoadCatalog reads the harness-neutral marketplace catalog from root.
func LoadCatalog(root string) (Catalog, error) {
	var catalog Catalog
	if err := readJSON(filepath.Join(root, NeutralCatalogPath), &catalog); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// ValidateCatalog verifies the harness-neutral marketplace catalog is complete
// enough for active harness parity.
func ValidateCatalog(root string) error {
	catalog, err := LoadCatalog(root)
	if err != nil {
		return err
	}
	if catalog.SchemaVersion != "dear-agent.marketplace/v1" {
		return fmt.Errorf("marketplace schema_version = %q, want dear-agent.marketplace/v1", catalog.SchemaVersion)
	}
	if catalog.Name == "" || catalog.Owner.Name == "" {
		return fmt.Errorf("marketplace catalog missing name or owner")
	}
	if len(catalog.Plugins) == 0 {
		return fmt.Errorf("marketplace catalog has no plugins")
	}
	if err := validateRequiredPlugins(catalog); err != nil {
		return err
	}
	for _, plugin := range catalog.Plugins {
		if err := validatePlugin(root, plugin); err != nil {
			return err
		}
	}
	return ValidateHarnessSurfaces(root)
}

func validateRequiredPlugins(catalog Catalog) error {
	present := make(map[string]PluginEntry, len(catalog.Plugins))
	for _, plugin := range catalog.Plugins {
		present[plugin.Name] = plugin
	}
	for _, name := range requiredPluginNames {
		plugin, ok := present[name]
		if !ok {
			return fmt.Errorf("marketplace catalog missing required plugin %q", name)
		}
		for _, capability := range requiredPluginCapabilities[name] {
			if !slices.Contains(plugin.Capabilities, capability) {
				return fmt.Errorf("marketplace catalog required plugin %q missing capability %q", name, capability)
			}
		}
	}
	return nil
}

// ValidateHarnessSurfaces verifies every active harness has a declared
// marketplace discovery surface.
func ValidateHarnessSurfaces(root string) error {
	catalog, err := LoadCatalog(root)
	if err != nil {
		return err
	}
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(catalog, harness)
		if !ok {
			return fmt.Errorf("active harness %q has no marketplace surface", harness)
		}
		if surface.Mode == "" || surface.Catalog == "" {
			return fmt.Errorf("active harness %q has incomplete marketplace surface: %+v", harness, surface)
		}
		wantMode := ExpectedMarketplaceMode(harness)
		if surface.Mode != wantMode {
			return fmt.Errorf("%s marketplace mode = %q, want %s", harness, surface.Mode, wantMode)
		}
		if _, err := os.Stat(filepath.Join(root, surface.Catalog)); err != nil {
			return fmt.Errorf("%s marketplace catalog %q: %w", harness, surface.Catalog, err)
		}
		if err := validateNativeSkillCoverage(root, catalog, surface); err != nil {
			return fmt.Errorf("%s marketplace catalog %q: %w", harness, surface.Catalog, err)
		}
	}
	return nil
}

func validateNativeSkillCoverage(root string, catalog Catalog, surface HarnessSurface) error {
	if surface.Mode != "native-codex-skill" &&
		surface.Mode != "native-opencode-skill" &&
		surface.Mode != "native-pi-skill-path" &&
		surface.Mode != "agents-md-skill-fallback" {
		return nil
	}
	for _, plugin := range catalog.Plugins {
		if !slices.Contains(plugin.Capabilities, "skills") {
			continue
		}
		exported, err := loadExportedSkills(root, plugin)
		if err != nil {
			return err
		}
		for _, skill := range exported {
			entrypoint, err := nativeSkillEntrypoint(root, surface, plugin.Name, skill.Name)
			if err != nil {
				return err
			}
			if err := validateNativeSkillEntrypoint(root, entrypoint, plugin.Name, skill); err != nil {
				return err
			}
		}
	}
	return nil
}

func nativeSkillEntrypoint(root string, surface HarnessSurface, pluginName, skillName string) (string, error) {
	if surface.Mode != "native-pi-skill-path" {
		entrypointRoot := surface.Catalog
		if surface.Mode == "agents-md-skill-fallback" {
			entrypointRoot = ".agents/skills"
		}
		declaredRoot := filepath.Join(root, filepath.FromSlash(entrypointRoot))
		if _, err := resolvedPathWithin(root, declaredRoot); err != nil {
			return "", fmt.Errorf("native skill root %q: %w", entrypointRoot, err)
		}
		return requireNativeSkillEntrypoint(filepath.Join(declaredRoot, skillName, "SKILL.md"), pluginName, skillName)
	}

	declaredSettingsPath := filepath.Join(root, filepath.FromSlash(surface.Catalog))
	settingsPath, err := resolvedPathWithin(root, declaredSettingsPath)
	if err != nil {
		return "", fmt.Errorf("pi skill settings %q: %w", surface.Catalog, err)
	}
	var settings piSettings
	if err := readJSON(settingsPath, &settings); err != nil {
		return "", fmt.Errorf("read Pi skill settings: %w", err)
	}
	if len(settings.Skills) == 0 {
		return "", fmt.Errorf("pi skill settings %q declare no skill roots", surface.Catalog)
	}
	for _, declared := range settings.Skills {
		skillRoot := filepath.Clean(filepath.Join(filepath.Dir(settingsPath), filepath.FromSlash(declared)))
		resolvedSkillRoot, err := resolvedPathWithin(root, skillRoot)
		if err != nil {
			return "", fmt.Errorf("pi skill root %q: %w", declared, err)
		}
		entrypoint := filepath.Join(resolvedSkillRoot, skillName, "SKILL.md")
		if _, err := os.Lstat(entrypoint); os.IsNotExist(err) {
			continue
		}
		return requireNativeSkillEntrypoint(entrypoint, pluginName, skillName)
	}
	return "", fmt.Errorf("plugin %q exported skill %q missing from Pi configured skill roots", pluginName, skillName)
}

func requireNativeSkillEntrypoint(entrypoint, pluginName, skillName string) (string, error) {
	info, err := os.Lstat(entrypoint)
	if err != nil {
		return "", fmt.Errorf("plugin %q exported skill %q missing native entrypoint: %w", pluginName, skillName, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("plugin %q exported skill %q native entrypoint %q is not a regular file", pluginName, skillName, entrypoint)
	}
	return entrypoint, nil
}

func loadExportedSkills(root string, plugin PluginEntry) ([]exportedSkill, error) {
	source := filepath.Join(root, filepath.FromSlash(plugin.Source))
	if _, err := resolvedPathWithin(root, source); err != nil {
		return nil, fmt.Errorf("plugin %q source %q: %w", plugin.Name, plugin.Source, err)
	}
	manifest, err := loadClaudePluginManifest(source, plugin.Name)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]exportedSkill)
	for _, declared := range manifest.Skills {
		skillFiles, err := exportedSkillFiles(source, plugin.Name, declared)
		if err != nil {
			return nil, err
		}
		for _, skillFile := range skillFiles {
			skill, err := exportedSkillFromFile(root, plugin.Name, skillFile)
			if err != nil {
				return nil, err
			}
			if previous, duplicate := byName[skill.Name]; duplicate {
				return nil, fmt.Errorf("skill-capable plugin %q exports duplicate skill name %q from %q and %q", plugin.Name, skill.Name, previous.CanonicalPath, skill.CanonicalPath)
			}
			byName[skill.Name] = skill
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	exported := make([]exportedSkill, 0, len(names))
	for _, name := range names {
		exported = append(exported, byName[name])
	}
	return exported, nil
}

func loadClaudePluginManifest(source, pluginName string) (claudePluginManifest, error) {
	var manifest claudePluginManifest
	manifestPath := filepath.Join(source, ".claude-plugin", "plugin.json")
	if err := readJSON(manifestPath, &manifest); err != nil {
		return claudePluginManifest{}, fmt.Errorf("skill-capable plugin %q manifest: %w", pluginName, err)
	}
	if manifest.Name != pluginName {
		return claudePluginManifest{}, fmt.Errorf("skill-capable plugin %q manifest names %q", pluginName, manifest.Name)
	}
	if len(manifest.Skills) == 0 {
		return claudePluginManifest{}, fmt.Errorf("skill-capable plugin %q manifest exports no skills", pluginName)
	}
	return manifest, nil
}

func exportedSkillFiles(source, pluginName, declared string) ([]string, error) {
	declaredPath := filepath.Clean(filepath.Join(source, filepath.FromSlash(declared)))
	relSource, err := filepath.Rel(source, declaredPath)
	if err != nil || relSource == ".." || strings.HasPrefix(relSource, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("skill-capable plugin %q skill export %q escapes its source", pluginName, declared)
	}
	if err := requireResolvedWithin(source, declaredPath); err != nil {
		return nil, fmt.Errorf("skill-capable plugin %q skill export %q: %w", pluginName, declared, err)
	}
	info, err := os.Stat(declaredPath)
	if err != nil {
		return nil, fmt.Errorf("skill-capable plugin %q skill export %q: %w", pluginName, declared, err)
	}
	if !info.IsDir() {
		if info.Mode().IsRegular() && filepath.Base(declaredPath) == "SKILL.md" {
			return []string{declaredPath}, nil
		}
		return nil, fmt.Errorf("skill-capable plugin %q skill export %q contains no SKILL.md", pluginName, declared)
	}

	var skillFiles []string
	if err := filepath.WalkDir(declaredPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "SKILL.md" {
			if err := requireResolvedWithin(source, path); err != nil {
				return err
			}
			skillFiles = append(skillFiles, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk skill-capable plugin %q export %q: %w", pluginName, declared, err)
	}
	if len(skillFiles) == 0 {
		return nil, fmt.Errorf("skill-capable plugin %q skill export %q contains no SKILL.md", pluginName, declared)
	}
	return skillFiles, nil
}

func requireResolvedWithin(root, path string) error {
	_, err := resolvedPathWithin(root, path)
	if err != nil {
		return fmt.Errorf("resolved exported skill path %q: %w", path, err)
	}
	return nil
}

func resolvedPathWithin(root, path string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve source: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("resolved path %q escapes its source", path)
	}
	return resolvedPath, nil
}

func exportedSkillFromFile(root, pluginName, skillFile string) (exportedSkill, error) {
	name, err := readSkillName(skillFile)
	if err != nil {
		return exportedSkill{}, fmt.Errorf("skill-capable plugin %q exported skill %q: %w", pluginName, skillFile, err)
	}
	canonical, err := filepath.Rel(root, skillFile)
	if err != nil {
		return exportedSkill{}, fmt.Errorf("resolve plugin %q exported skill %q: %w", pluginName, skillFile, err)
	}
	return exportedSkill{Name: name, CanonicalPath: filepath.ToSlash(canonical)}, nil
}

func readSkillName(path string) (string, error) {
	violations, err := skilllint.CheckFile(path)
	if err != nil {
		return "", err
	}
	if len(violations) > 0 {
		return "", fmt.Errorf("invalid skill: %s", violations[0].Reason)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var metadata struct {
		Name string `yaml:"name"`
	}
	parts := strings.SplitN(string(data), "\n---", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "---\n") {
		return "", fmt.Errorf("no parseable frontmatter")
	}
	if err := yaml.Unmarshal([]byte(strings.TrimPrefix(parts[0], "---\n")), &metadata); err != nil {
		return "", fmt.Errorf("parse metadata: %w", err)
	}
	if metadata.Name == "" {
		return "", fmt.Errorf("frontmatter has no name")
	}
	return metadata.Name, nil
}

func validateNativeSkillEntrypoint(root, entrypoint, pluginName string, skill exportedSkill) error {
	violations, err := skilllint.CheckFile(entrypoint)
	if err != nil {
		return fmt.Errorf("plugin %q exported skill %q native entrypoint: %w", pluginName, skill.Name, err)
	}
	if len(violations) > 0 {
		return fmt.Errorf("plugin %q exported skill %q native entrypoint invalid: %s", pluginName, skill.Name, violations[0].Reason)
	}
	name, err := readSkillName(entrypoint)
	if err != nil {
		return fmt.Errorf("plugin %q exported skill %q native entrypoint metadata: %w", pluginName, skill.Name, err)
	}
	if name != skill.Name {
		return fmt.Errorf("plugin %q exported skill %q native entrypoint names %q", pluginName, skill.Name, name)
	}
	canonicalPath := filepath.Join(root, filepath.FromSlash(skill.CanonicalPath))
	info, err := os.Stat(canonicalPath)
	if err != nil {
		return fmt.Errorf("plugin %q exported skill %q canonical entrypoint %q: %w", pluginName, skill.Name, skill.CanonicalPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("plugin %q exported skill %q canonical entrypoint %q is not a regular file", pluginName, skill.Name, skill.CanonicalPath)
	}
	if filepath.Clean(entrypoint) == filepath.Clean(canonicalPath) {
		return nil
	}
	wantReference, err := filepath.Rel(filepath.Dir(entrypoint), canonicalPath)
	if err != nil {
		return fmt.Errorf("plugin %q exported skill %q canonical reference: %w", pluginName, skill.Name, err)
	}
	wantReference = filepath.ToSlash(wantReference)
	data, err := os.ReadFile(entrypoint)
	if err != nil {
		return fmt.Errorf("read plugin %q exported skill %q native entrypoint: %w", pluginName, skill.Name, err)
	}
	if !hasActionableCanonicalWorkflow(string(data), wantReference) {
		return fmt.Errorf("plugin %q exported skill %q native entrypoint does not actionably load and follow canonical %q from its Workflow section", pluginName, skill.Name, skill.CanonicalPath)
	}
	return nil
}

func hasActionableCanonicalWorkflow(markdown, reference string) bool {
	const heading = "## Workflow"
	_, section, ok := strings.Cut(markdown, heading)
	if !ok {
		return false
	}
	if workflow, _, found := strings.Cut(section, "\n## "); found {
		section = workflow
	}
	normalized := strings.Join(strings.Fields(section), " ")
	lower := strings.ToLower(normalized)
	token := "`" + strings.ToLower(reference) + "`"
	before, after, ok := strings.Cut(lower, token)
	if !ok {
		return false
	}
	if len(before) > 120 {
		before = before[len(before)-120:]
	}
	if !strings.Contains(before, "read") && !strings.Contains(before, "load") {
		return false
	}
	return strings.Contains(after, "follow")
}

// ExpectedMarketplaceMode returns the executable skill-discovery mode owned by
// each active harness.
func ExpectedMarketplaceMode(harness string) string {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return "native-claude-plugin-marketplace"
	case "codex-cli":
		return "native-codex-skill"
	case "opencode-cli":
		return "native-opencode-skill"
	case "pi-cli":
		return "native-pi-skill-path"
	default:
		return "agents-md-skill-fallback"
	}
}

// SurfaceForHarness returns the marketplace surface for a harness.
func SurfaceForHarness(catalog Catalog, harness string) (HarnessSurface, bool) {
	normalized := agent.NormalizeHarnessName(harness)
	for _, surface := range catalog.Harnesses {
		if agent.NormalizeHarnessName(surface.Name) == normalized {
			return surface, true
		}
	}
	return HarnessSurface{}, false
}

// ValidateClaudeMarketplaceMirror verifies the native Claude marketplace is a
// projection of the harness-neutral catalog.
func ValidateClaudeMarketplaceMirror(root string) error {
	neutral, err := LoadCatalog(root)
	if err != nil {
		return err
	}
	var claude claudeCatalog
	if err := readJSON(filepath.Join(root, ClaudeCatalogPath), &claude); err != nil {
		return err
	}
	if neutral.Name != claude.Name {
		return fmt.Errorf("claude marketplace name = %q, want %q", claude.Name, neutral.Name)
	}
	byName := make(map[string]PluginEntry, len(claude.Plugins))
	for _, plugin := range claude.Plugins {
		byName[plugin.Name] = plugin
	}
	for _, plugin := range neutral.Plugins {
		claudePlugin, ok := byName[plugin.Name]
		if !ok {
			return fmt.Errorf("claude marketplace missing plugin %q", plugin.Name)
		}
		if claudePlugin.Source != plugin.Source {
			return fmt.Errorf("claude marketplace plugin %q source = %q, want %q", plugin.Name, claudePlugin.Source, plugin.Source)
		}
		if claudePlugin.Version != plugin.Version {
			return fmt.Errorf("claude marketplace plugin %q version = %q, want %q", plugin.Name, claudePlugin.Version, plugin.Version)
		}
	}
	return nil
}

func validatePlugin(root string, plugin PluginEntry) error {
	if plugin.Name == "" || plugin.Source == "" || plugin.Version == "" {
		return fmt.Errorf("marketplace plugin has empty required field: %+v", plugin)
	}
	source := filepath.Join(root, plugin.Source)
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("marketplace plugin %q source %q: %w", plugin.Name, plugin.Source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("marketplace plugin %q source %q is not a directory", plugin.Name, plugin.Source)
	}
	if slices.Contains(plugin.Capabilities, "commands") {
		if err := requireDir(filepath.Join(source, "commands")); err != nil {
			return fmt.Errorf("marketplace plugin %q commands capability: %w", plugin.Name, err)
		}
	}
	if slices.Contains(plugin.Capabilities, "skills") {
		if err := requireSkillSurface(source); err != nil {
			return fmt.Errorf("marketplace plugin %q skills capability: %w", plugin.Name, err)
		}
	}
	return nil
}

func requireSkillSurface(source string) error {
	if info, err := os.Stat(filepath.Join(source, "SKILL.md")); err == nil && !info.IsDir() {
		return nil
	}
	return requireDir(filepath.Join(source, "skills"))
}

func requireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func readJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
