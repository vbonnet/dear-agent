package plugin

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// APIVersionV1 is the only manifest API version Phase 1 accepts.
// Adding a field that older binaries can ignore is additive and stays
// on v1; a breaking change bumps to v2 and requires an ADR amendment
// (ADR-014 §Open question 4).
const APIVersionV1 = "dear-agent.io/v1"

// KindPlugin is the only Manifest.Kind value in Phase 1. Reserved here
// so future kinds (KindBundle, KindOverride) can be added without
// silently widening the valid set.
const KindPlugin = "Plugin"

// Capability names a single extensibility surface a plugin participates
// in. Capabilities are declared in the manifest *and* implied by the
// Go interface the plugin satisfies; the two must agree, or
// Registry.Register reports the discrepancy. Declaring a capability
// the plugin does not implement is the more common bug, so the
// registry rejects that case loudly.
type Capability string

// Capability values defined in Phase 1 (ADR-014 §D7). Reserved names
// (EventSubscriber, NodeKindProvider, SourceAdapter) are not yet
// validators; a manifest declaring them today will fail
// Capability.IsValid().
const (
	CapabilityHooks  Capability = "hooks"
	CapabilityChecks Capability = "checks"
)

// IsValid reports whether c names a capability Phase 1 understands.
// Reserved-but-unimplemented capabilities return false on purpose:
// a manifest that declares them should fail validation today rather
// than load a plugin whose declared surface is partially nonexistent.
func (c Capability) IsValid() bool {
	switch c {
	case CapabilityHooks, CapabilityChecks:
		return true
	}
	return false
}

// Permissions is the manifest's bounded-permissions declaration. The
// fields piggy-back on ADR-010 §D5: when a plugin participates in node
// execution (e.g. via OnEnforce), its declared permissions are unioned
// into the node's permissions for the duration of the call. The
// existing PermissionEnforcer in pkg/workflow remains the enforcer;
// pkg/plugin is just the declaration site.
//
// All fields are optional. An empty Permissions value declares no
// expectations (the conservative default); operators can still
// override via the workflow node's own permissions block.
type Permissions struct {
	// FSRead is the list of glob patterns the plugin expects to read.
	// Patterns are repository-relative; absolute paths are an
	// authoring error caught by Validate.
	FSRead []string `yaml:"fs_read,omitempty"`
	// FSWrite is the list of glob patterns the plugin expects to write.
	FSWrite []string `yaml:"fs_write,omitempty"`
	// Network indicates whether the plugin expects to reach the network.
	// True is informational at the plugin layer; the workflow runner is
	// the actual gate (see ADR-010 §D5).
	Network bool `yaml:"network,omitempty"`
	// Tools is the list of tool ids (audit.* check ids, MCP tool names,
	// etc.) the plugin expects to invoke. Empty means "no tools".
	Tools []string `yaml:"tools,omitempty"`
}

// Manifest is the discoverable contract every plugin returns from
// Plugin.Manifest(). It is also the on-disk format of plugin.yaml
// files loaded by FilesystemLoader. The two paths share one type so
// the in-memory and on-disk representations cannot drift.
//
// Fields are documented in ADR-014 §D2. Validate enforces the rules
// every consumer is allowed to assume:
//
//   - APIVersion is APIVersionV1.
//   - Kind is KindPlugin.
//   - Name is non-empty, contains no control characters, and matches
//     the lowercase-dot-separated namespace convention loosely (the
//     regex check is the same permissive shape audit.CheckMeta.Validate
//     uses, so dear-agent.audit.license-header validates the same way
//     audit.lint.license-header does).
//   - Version is non-empty (we do not enforce strict semver — a free
//     identifier is allowed for development builds).
//   - Every Capability listed is IsValid.
type Manifest struct {
	APIVersion   string         `yaml:"api_version"`
	Kind         string         `yaml:"kind"`
	Name         string         `yaml:"name"`
	Version      string         `yaml:"version"`
	Description  string         `yaml:"description,omitempty"`
	Author       string         `yaml:"author,omitempty"`
	Capabilities []Capability   `yaml:"capabilities,omitempty"`
	Permissions  Permissions    `yaml:"permissions,omitempty"`
	Config       map[string]any `yaml:"config,omitempty"`
}

// Validate returns a non-nil error if the manifest is malformed.
// Validation is intentionally permissive on identifier characters
// (lowercase, dots, dashes, digits) to match audit.CheckMeta.Validate;
// it rejects the obvious-bad cases (empty fields, control characters,
// unknown capability values) and lets downstream tooling layer on
// stricter checks.
func (m Manifest) Validate() error {
	if m.APIVersion != APIVersionV1 {
		return fmt.Errorf("plugin: Manifest.APIVersion %q: only %q is supported", m.APIVersion, APIVersionV1)
	}
	if m.Kind != KindPlugin {
		return fmt.Errorf("plugin: Manifest.Kind %q: only %q is supported", m.Kind, KindPlugin)
	}
	if m.Name == "" {
		return fmt.Errorf("plugin: Manifest.Name is empty")
	}
	for _, r := range m.Name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("plugin: Manifest.Name %q contains control char", m.Name)
		}
	}
	if strings.TrimSpace(m.Name) != m.Name {
		return fmt.Errorf("plugin: Manifest.Name %q has leading/trailing whitespace", m.Name)
	}
	if m.Version == "" {
		return fmt.Errorf("plugin: Manifest.Version is empty")
	}
	for i, cap := range m.Capabilities {
		if !cap.IsValid() {
			return fmt.Errorf("plugin: Manifest.Capabilities[%d] = %q: not a known capability", i, cap)
		}
	}
	for i, p := range m.Permissions.FSRead {
		if strings.HasPrefix(p, "/") {
			return fmt.Errorf("plugin: Manifest.Permissions.FSRead[%d] = %q: must be repo-relative, not absolute", i, p)
		}
	}
	for i, p := range m.Permissions.FSWrite {
		if strings.HasPrefix(p, "/") {
			return fmt.Errorf("plugin: Manifest.Permissions.FSWrite[%d] = %q: must be repo-relative, not absolute", i, p)
		}
	}
	return nil
}

// HasCapability reports whether m declares c. It does not consult the
// Go interfaces the plugin implements — see Registry.Register for the
// cross-check that flags a manifest declaring a capability the code
// does not satisfy.
func (m Manifest) HasCapability(c Capability) bool {
	for _, mc := range m.Capabilities {
		if mc == c {
			return true
		}
	}
	return false
}

// LoadManifest reads and validates a plugin.yaml file at path.
// Validation errors are returned with the path threaded in so a loader
// scanning a directory can attribute failures to a specific file.
//
// The returned Manifest is the in-memory representation; callers
// match it against compiled-in plugins by Manifest.Name. LoadManifest
// does not register or activate anything — that is FilesystemLoader's
// and Registry's job.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin: read %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("plugin: parse %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("plugin: validate %s: %w", path, err)
	}
	return m, nil
}
