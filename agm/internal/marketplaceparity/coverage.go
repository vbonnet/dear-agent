// Package marketplaceparity defines executable coverage for neutral plugin
// marketplace publication across harnesses.
package marketplaceparity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/internal/specgovernance"
)

const (
	// NeutralCatalogPath is the harness-neutral marketplace catalog path.
	NeutralCatalogPath = ".dear-agent/marketplace.json"
	// ClaudeCatalogPath is the native Claude marketplace mirror path.
	ClaudeCatalogPath = ".claude-plugin/marketplace.json"
)

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
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Owner       Owner               `json:"owner"`
	Plugins     []claudePluginEntry `json:"plugins"`
}

type claudePluginEntry struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Strict      *bool    `json:"strict,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

type nativeSpecGovernanceManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	Skills []string `json:"skills"`
}

type distributionEntryKind uint8

const (
	distributionDirectory distributionEntryKind = iota
	distributionRegularFile
)

type distributionEntry struct {
	path       string
	kind       distributionEntryKind
	executable bool
}

// specGovernanceDistributionManifest is the minimum validated closure
// required to load both skills and execute specaudit without consulting the
// active repository or a network dependency.
var specGovernanceDistributionManifest = []distributionEntry{
	{path: ".claude-plugin", kind: distributionDirectory},
	{path: ".claude-plugin/plugin.json", kind: distributionRegularFile},
	{path: "go.mod", kind: distributionRegularFile},
	{path: "internal", kind: distributionDirectory},
	{path: "internal/gittest", kind: distributionDirectory},
	// This test helper is shipped as declared non-runtime support; the launcher
	// independently inventories only the packages reachable by its go run target.
	{path: "internal/gittest/gittest.go", kind: distributionRegularFile},
	{path: "earslint", kind: distributionDirectory},
	{path: "earslint/config.go", kind: distributionRegularFile},
	{path: "earslint/earslint.go", kind: distributionRegularFile},
	{path: "scripts", kind: distributionDirectory},
	{path: "scripts/specaudit", kind: distributionRegularFile, executable: true},
	{path: "skills", kind: distributionDirectory},
	{path: "skills/audit-specs", kind: distributionDirectory},
	{path: "skills/audit-specs/SKILL.md", kind: distributionRegularFile},
	{path: "skills/audit-specs/references", kind: distributionDirectory},
	{path: "skills/audit-specs/references/audit-verdicts.md", kind: distributionRegularFile},
	{path: "skills/audit-specs/references/report-schema.md", kind: distributionRegularFile},
	{path: "skills/audit-specs/scripts", kind: distributionDirectory},
	{path: "skills/audit-specs/scripts/specaudit", kind: distributionDirectory},
	{path: "skills/audit-specs/scripts/specaudit/main.go", kind: distributionRegularFile},
	{path: "skills/write-spec", kind: distributionDirectory},
	{path: "skills/write-spec/SKILL.md", kind: distributionRegularFile},
	{path: "skills/write-spec/references", kind: distributionDirectory},
	{path: "skills/write-spec/references/contract-model.md", kind: distributionRegularFile},
	{path: "skills/write-spec/references/ears-and-bdd.md", kind: distributionRegularFile},
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
	for _, plugin := range catalog.Plugins {
		if err := validatePlugin(root, plugin); err != nil {
			return err
		}
	}
	return ValidateHarnessSurfaces(root)
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
		if harness == "claude-code" && surface.Mode != "native-claude-plugin-marketplace" {
			return fmt.Errorf("claude-code marketplace mode = %q, want native-claude-plugin-marketplace", surface.Mode)
		}
		if harness != "claude-code" && surface.Mode != "agents-md-skill-fallback" {
			return fmt.Errorf("%s marketplace mode = %q, want agents-md-skill-fallback", harness, surface.Mode)
		}
		if _, err := os.Stat(filepath.Join(root, surface.Catalog)); err != nil {
			return fmt.Errorf("%s marketplace catalog %q: %w", harness, surface.Catalog, err)
		}
	}
	return nil
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
	neutralNames, err := sortedUniquePluginNames("neutral", neutral.Plugins, func(plugin PluginEntry) string {
		return plugin.Name
	})
	if err != nil {
		return err
	}
	claudeNames, err := sortedUniquePluginNames("claude", claude.Plugins, func(plugin claudePluginEntry) string {
		return plugin.Name
	})
	if err != nil {
		return err
	}
	byName := make(map[string]claudePluginEntry, len(claude.Plugins))
	for _, plugin := range claude.Plugins {
		byName[plugin.Name] = plugin
	}
	if !slices.Equal(claudeNames, neutralNames) {
		return fmt.Errorf("claude marketplace plugins = %v, want exact neutral set %v", claudeNames, neutralNames)
	}
	for _, plugin := range neutral.Plugins {
		if err := validateClaudePluginMirror(root, plugin, byName[plugin.Name]); err != nil {
			return err
		}
	}
	return nil
}

func sortedUniquePluginNames[T any](catalog string, plugins []T, name func(T) string) ([]string, error) {
	names := make([]string, 0, len(plugins))
	seen := make(map[string]struct{}, len(plugins))
	for _, plugin := range plugins {
		pluginName := name(plugin)
		if pluginName == "" {
			return nil, fmt.Errorf("%s marketplace has a plugin with an empty name", catalog)
		}
		if _, exists := seen[pluginName]; exists {
			return nil, fmt.Errorf("%s marketplace has duplicate plugin %q", catalog, pluginName)
		}
		seen[pluginName] = struct{}{}
		names = append(names, pluginName)
	}
	sort.Strings(names)
	return names, nil
}

func validateClaudePluginMirror(root string, neutral PluginEntry, claude claudePluginEntry) error {
	if claude.Source != neutral.Source {
		if err := validateClaudeSkillBundleAdapter(root, neutral, claude); err != nil {
			return fmt.Errorf("claude marketplace plugin %q source = %q, want %q or a valid native skill-bundle adapter: %w", neutral.Name, claude.Source, neutral.Source, err)
		}
	}
	if claude.Version != neutral.Version {
		return fmt.Errorf("claude marketplace plugin %q version = %q, want %q", neutral.Name, claude.Version, neutral.Version)
	}
	if neutral.Name != "spec-governance" {
		return nil
	}
	if err := validateNativeSpecGovernanceSurface(root, neutral, claude); err != nil {
		return fmt.Errorf("claude marketplace SPEC governance plugin: %w", err)
	}
	return nil
}

func validateNativeSpecGovernanceSurface(root string, neutral PluginEntry, claude claudePluginEntry) error {
	if claude.Source != "./spec-governance" || neutral.Source != "./spec-governance" {
		return errors.New("must use the isolated spec-governance plugin root")
	}
	pluginRoot := filepath.Join(root, "spec-governance")
	if err := validateDistributionClosure(root, "spec-governance"); err != nil {
		return fmt.Errorf("isolated plugin root: %w", err)
	}
	want, err := nativeSkillExports(pluginRoot)
	if err != nil {
		return err
	}
	if claude.Strict != nil || len(claude.Skills) != 0 {
		return errors.New("must delegate its native surface to the isolated plugin manifest")
	}
	var manifest nativeSpecGovernanceManifest
	if err := readStrictJSON(filepath.Join(pluginRoot, ".claude-plugin", "plugin.json"), &manifest); err != nil {
		return err
	}
	if manifest.Name != "spec-governance" || manifest.Version != neutral.Version || manifest.Description == "" || manifest.Author.Name == "" {
		return errors.New("isolated plugin manifest has incomplete identity")
	}
	if !slices.Equal(manifest.Skills, want) {
		return fmt.Errorf("isolated plugin skills = %v, want exact canonical set %v", manifest.Skills, want)
	}
	return rejectForbiddenSpecGovernanceSurfaces(pluginRoot)
}

func rejectForbiddenSpecGovernanceSurfaces(pluginRoot string) error {
	for _, forbidden := range []string{".mcp.json", "mcp.json", ".lsp.json", "lsp.json", "hooks", "agents", "commands"} {
		if _, err := os.Lstat(filepath.Join(pluginRoot, forbidden)); err == nil {
			return fmt.Errorf("isolated plugin must not expose %s", forbidden)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect isolated plugin %s: %w", forbidden, err)
		}
	}
	return nil
}

func nativeSkillExports(pluginRoot string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(pluginRoot, "skills"))
	if err != nil {
		return nil, fmt.Errorf("read isolated plugin skills: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	wantNames := specgovernance.Names()
	if !slices.Equal(names, wantNames) {
		return nil, fmt.Errorf("isolated plugin canonical skill directories = %v, want fixed set %v", names, wantNames)
	}
	for _, name := range wantNames {
		path := filepath.Join(pluginRoot, "skills", name, "SKILL.md")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			if err != nil {
				return nil, fmt.Errorf("isolated plugin skill %q: %w", name, err)
			}
			return nil, fmt.Errorf("isolated plugin skill %q has no regular SKILL.md", name)
		}
	}
	return specgovernance.NativeExports(), nil
}

func validateDistributionClosure(repositoryRoot, relativeRoot string) error {
	pluginRoot, err := requireContainedRelativePath(repositoryRoot, relativeRoot)
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(pluginRoot)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("distribution root must be a real directory")
	}
	for _, entry := range specGovernanceDistributionManifest {
		if err := validateDistributionEntry(pluginRoot, entry); err != nil {
			return fmt.Errorf("distribution closure %q: %w", filepath.ToSlash(entry.path), err)
		}
	}
	declaredGoSources := make(map[string]bool)
	for _, entry := range specGovernanceDistributionManifest {
		if strings.HasSuffix(entry.path, ".go") {
			declaredGoSources[filepath.ToSlash(entry.path)] = true
		}
	}
	return filepath.WalkDir(pluginRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		return validateDistributionTreeEntry(pluginRoot, path, entry, declaredGoSources, walkErr)
	})
}

func validateDistributionEntry(pluginRoot string, entry distributionEntry) error {
	path, err := requireContainedRelativePath(pluginRoot, entry.path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(pluginRoot, path)
	if err != nil {
		return err
	}
	info, err := lstatDistributionPath(pluginRoot, relative)
	if err != nil {
		return err
	}
	return validateDistributionLeaf(info, entry)
}

func lstatDistributionPath(pluginRoot, relative string) (fs.FileInfo, error) {
	current := pluginRoot
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("path or ancestor must not be a symlink")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return nil, errors.New("ancestor must be a real directory")
		}
		if index == len(parts)-1 {
			return info, nil
		}
	}
	return nil, errors.New("distribution path has no entries")
}

func validateDistributionLeaf(info fs.FileInfo, entry distributionEntry) error {
	switch entry.kind {
	case distributionDirectory:
		if !info.IsDir() {
			return errors.New("must be a real directory")
		}
	case distributionRegularFile:
		if !info.Mode().IsRegular() {
			return errors.New("must be a regular file")
		}
		if !hasSingleHardLink(info) {
			return errors.New("must have exactly one hard link")
		}
		if entry.executable && info.Mode().Perm()&0o111 == 0 {
			return errors.New("must be executable")
		}
	default:
		return errors.New("has an invalid manifest kind")
	}
	return nil
}

func validateDistributionTreeEntry(pluginRoot, path string, _ fs.DirEntry, declaredGoSources map[string]bool, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	relative, err := filepath.Rel(pluginRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("distribution entry escapes its root: %s", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("distribution entry %q must not be a symlink", filepath.ToSlash(relative))
	}
	if info.IsDir() {
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("distribution entry %q must be a regular file", filepath.ToSlash(relative))
	}
	if !hasSingleHardLink(info) {
		return fmt.Errorf("distribution entry %q must have exactly one hard link", filepath.ToSlash(relative))
	}
	if strings.HasSuffix(relative, ".go") && !strings.HasSuffix(relative, "_test.go") && !declaredGoSources[filepath.ToSlash(relative)] {
		return fmt.Errorf("distribution entry %q is undeclared executable Go source", filepath.ToSlash(relative))
	}
	return nil
}

func requireContainedRelativePath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", errors.New("path must be relative")
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path must remain inside the distribution root")
	}
	joined := filepath.Join(root, clean)
	resolvedRelative, err := filepath.Rel(root, joined)
	if err != nil || resolvedRelative != clean {
		return "", errors.New("path must remain inside the distribution root")
	}
	return joined, nil
}

func hasSingleHardLink(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}

func validateClaudeSkillBundleAdapter(root string, neutral PluginEntry, claude claudePluginEntry) error {
	if claude.Source != "." {
		return fmt.Errorf("expanded source must be the authenticated marketplace root")
	}
	if claude.Strict == nil || *claude.Strict {
		return fmt.Errorf("expanded source must explicitly set strict to false")
	}
	if len(neutral.Capabilities) != 1 || neutral.Capabilities[0] != "skills" {
		return fmt.Errorf("expanded source is permitted only for an exact skills capability")
	}
	want, err := canonicalSkillExports(root, neutral.Source)
	if err != nil {
		return err
	}
	got := append([]string(nil), claude.Skills...)
	sort.Strings(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("skill exports = %v, want exact canonical set %v", claude.Skills, want)
	}
	return nil
}

func canonicalSkillExports(root, source string) ([]string, error) {
	canonicalSource, err := catalogSubtree(source)
	if err != nil {
		return nil, err
	}
	skillsRoot := filepath.Join(root, canonicalSource, "skills")
	if err := requireRealDirectoryTree(root, filepath.Join(canonicalSource, "skills")); err != nil {
		return nil, fmt.Errorf("canonical skill directory: %w", err)
	}
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return nil, fmt.Errorf("read canonical skill directory: %w", err)
	}
	want := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		skillFile := filepath.Join(skillsRoot, entry.Name(), "SKILL.md")
		info, statErr := os.Lstat(skillFile)
		if statErr != nil || !info.Mode().IsRegular() {
			if statErr != nil {
				return nil, fmt.Errorf("canonical skill %q: %w", entry.Name(), statErr)
			}
			return nil, fmt.Errorf("canonical skill %q has no regular SKILL.md", entry.Name())
		}
		want = append(want, "./"+canonicalSource+"/skills/"+entry.Name())
	}
	sort.Strings(want)
	return want, nil
}

func catalogSubtree(source string) (string, error) {
	if filepath.IsAbs(source) {
		return "", fmt.Errorf("neutral plugin source must be repository-relative")
	}
	cleaned := filepath.Clean(source)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("neutral plugin source must name a repository subtree")
	}
	return filepath.ToSlash(cleaned), nil
}

func requireRealDirectoryTree(root, relative string) error {
	current := root
	for part := range strings.SplitSeq(filepath.Clean(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%s must be a real directory", filepath.ToSlash(relative))
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

func readStrictJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("parse %s: contains multiple JSON values", path)
		}
		return fmt.Errorf("parse trailing %s: %w", path, err)
	}
	return nil
}
