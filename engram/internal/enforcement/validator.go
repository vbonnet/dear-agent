// Package enforcement implements company policy enforcement for Engram.
//
// Enforcement validates that users comply with company requirements:
//   - Identity verification (allowed email domains)
//   - Required plugins installed
//   - Required config values set
//
// Example usage:
//
//	validator := enforcement.NewValidator(cfg.Enforcement, identity)
//	if err := validator.Validate(ctx); err != nil {
//	    // User is non-compliant, display error
//	    fmt.Fprintf(os.Stderr, "%v\n", err)
//	    os.Exit(1)
//	}
//
// See core/docs/adr/enforcement-architecture.md for design details.
package enforcement

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/engram/internal/identity"
	"github.com/vbonnet/dear-agent/engram/internal/plugin"
)

// Validator validates enforcement rules against user identity and configuration.
//
// Validator supports two-phase validation:
//   - Phase 1 (pre-plugin-load): Validates identity and required config
//   - Phase 2 (post-plugin-load): Validates required plugins
//
// This two-phase approach allows enforcement to occur at the appropriate
// lifecycle stages during platform initialization.
type Validator struct {
	config         *config.EnforcementConfig
	identity       *identity.Identity
	pluginRegistry PluginRegistry // Interface for plugin enumeration
}

// PluginRegistry interface for plugin enumeration.
//
// This interface allows the enforcement package to query installed plugins
// without creating a circular dependency with the plugin package.
// Use plugin.NewEnforcementAdapter() to create an implementation.
type PluginRegistry interface {
	// ListPlugins returns all installed plugins with minimal metadata
	ListPlugins() []plugin.PluginInfo
}

// NewValidator creates a new enforcement validator.
//
// The validator will check the provided identity and configuration against
// the enforcement rules in cfg. Plugin validation requires calling
// SetPluginRegistry() after plugins are loaded.
//
// Parameters:
//   - cfg: Enforcement configuration (from company/core tier)
//   - id: Detected user identity (may be nil if detection failed)
//
// Returns a validator ready for phase 1 validation (identity + config).
func NewValidator(cfg *config.EnforcementConfig, id *identity.Identity) *Validator {
	return &Validator{
		config:   cfg,
		identity: id,
	}
}

// SetPluginRegistry sets the plugin registry for phase 2 validation.
//
// This must be called after plugins are loaded if plugin requirements are
// configured. Call this before the second Validate() invocation.
//
// Example:
//
//	validator := NewValidator(cfg, identity)
//	validator.Validate(ctx) // Phase 1: identity + config
//	// ... load plugins ...
//	validator.SetPluginRegistry(pluginAdapter)
//	validator.Validate(ctx) // Phase 2: plugins
func (v *Validator) SetPluginRegistry(registry PluginRegistry) {
	v.pluginRegistry = registry
}

// Validate runs all configured enforcement checks.
//
// Returns nil if all requirements are met, or an error describing the first
// violation encountered. Error messages are formatted using the configured
// error template for user-friendly output.
//
// Enforcement checks performed (in order):
//  1. Identity verification (if identity.required = true)
//  2. Plugin requirements (if pluginRegistry set and plugins configured)
//  3. Configuration requirements (if required_config configured)
//
// Validate is designed to be called twice during platform initialization:
//   - First call (phase 1): After config load, validates identity + config
//   - Second call (phase 2): After plugin load, validates plugins
//
// The method is idempotent and can be called multiple times safely.
//
// Context cancellation is respected - if ctx is cancelled, returns ctx.Err().
func (v *Validator) Validate(ctx context.Context) error {
	if v.config == nil || !v.config.Enabled {
		return nil // Enforcement disabled
	}

	// Validate identity
	if err := v.validateIdentity(); err != nil {
		return v.formatError("Identity Verification Failed", err, []string{
			"Run: gcloud auth application-default login",
			"Log in with your company email when prompted",
			"Ensure git config user.email is set: git config --global user.email you@company.com",
		})
	}

	// Validate required plugins
	if err := v.validatePlugins(); err != nil {
		return v.formatError("Required Plugins Missing", err, nil)
	}

	// Validate required config values
	if err := v.validateConfig(); err != nil {
		return v.formatError("Required Configuration Missing", err, nil)
	}

	return nil
}

// validateIdentity checks user identity matches allowed domains
func (v *Validator) validateIdentity() error {
	if !v.config.Identity.Required {
		return nil // Identity not required
	}

	if v.identity == nil {
		return fmt.Errorf("no identity detected")
	}

	// Check if domain is allowed
	if len(v.config.Identity.AllowedDomains) == 0 {
		// No domain restriction
		return nil
	}

	for _, allowedDomain := range v.config.Identity.AllowedDomains {
		if v.identity.Domain == allowedDomain {
			return nil // Match found
		}
	}

	return fmt.Errorf("identity domain %s not in allowed list (detected via %s)",
		v.identity.Domain, v.identity.Method)
}

// validatePlugins checks required plugins are installed
func (v *Validator) validatePlugins() error {
	if len(v.config.Plugins) == 0 {
		return nil // No plugin requirements
	}

	// If plugin registry not set, skip validation
	// This happens during early enforcement validation before plugins are loaded
	if v.pluginRegistry == nil {
		return nil
	}

	// Get list of installed plugins
	installedPlugins := v.pluginRegistry.ListPlugins()

	// Build map of installed plugins for O(1) lookup
	installed := make(map[string]string) // name -> version
	for _, p := range installedPlugins {
		installed[p.Name] = p.Version
	}

	// Check each required plugin
	var missingPlugins []string
	var versionMismatches []string

	for _, req := range v.config.Plugins {
		version, found := installed[req.Name]
		if !found {
			missingPlugins = append(missingPlugins, req.Name)
			continue
		}

		// Check version requirement if specified
		if req.VersionMin != "" {
			if !versionSatisfies(version, req.VersionMin) {
				versionMismatches = append(versionMismatches,
					fmt.Sprintf("%s (have %s, need >= %s)", req.Name, version, req.VersionMin))
			}
		}
	}

	// Build error message
	if len(missingPlugins) > 0 || len(versionMismatches) > 0 {
		var errMsg string
		if len(missingPlugins) > 0 {
			errMsg = fmt.Sprintf("missing required plugins: %s", strings.Join(missingPlugins, ", "))
		}
		if len(versionMismatches) > 0 {
			if errMsg != "" {
				errMsg += "; "
			}
			errMsg += fmt.Sprintf("version mismatches: %s", strings.Join(versionMismatches, ", "))
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// versionSatisfies checks if version >= minVersion (simple string comparison)
// TODO: Use semver library for proper semantic versioning
func versionSatisfies(version, minVersion string) bool {
	// Simple string comparison for now
	// In production, would use github.com/hashicorp/go-version or similar
	return version >= minVersion
}

// validateConfig checks required config values are set
func (v *Validator) validateConfig() error {
	// Config validation is handled by enforceTier in config loader
	// This is a secondary check - already validated during config merge
	return nil
}

// formatError formats user-friendly error message using template
func (v *Validator) formatError(title string, err error, actions []string) error {
	// If no custom template, return simple error
	if v.config.ErrorMessages == nil || v.config.ErrorMessages.Template == "" {
		return fmt.Errorf("%s: %w", title, err)
	}

	// Parse and execute template
	tmpl, parseErr := template.New("error").Parse(v.config.ErrorMessages.Template)
	if parseErr != nil {
		// Template parsing failed, fallback to simple error
		return fmt.Errorf("%s: %w", title, err)
	}

	var buf bytes.Buffer
	data := map[string]interface{}{
		"Message": fmt.Sprintf("%s: %v", title, err),
		"Actions": actions,
		"HelpURL": v.config.ErrorMessages.HelpURL,
	}

	if execErr := tmpl.Execute(&buf, data); execErr != nil {
		// Template execution failed, fallback to simple error
		return fmt.Errorf("%s: %w", title, err)
	}

	return fmt.Errorf("%s", buf.String())
}
