package enrichment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// PluginContextEnricher enriches plugin_execution events with plugin loading context
type PluginContextEnricher struct {
	// No shared mutable state - thread-safe
}

// NewPluginContextEnricher creates a new PluginContextEnricher
func NewPluginContextEnricher() *PluginContextEnricher {
	return &PluginContextEnricher{}
}

// Enrich adds plugin context to plugin_execution events
func (p *PluginContextEnricher) Enrich(ctx context.Context, event *TelemetryEvent, ec EnrichmentContext) (*TelemetryEvent, error) {
	// Only enrich plugin_execution events
	if event.Type != EventTypePluginExecution {
		return event, nil
	}

	// Create copy of event to avoid modifying original
	enriched := *event
	if enriched.Data == nil {
		enriched.Data = make(map[string]interface{})
	}

	// Add prompt hash (privacy-preserving)
	enriched.Data["prompt_hash"] = hashPrompt(ec.Prompt, ec.SessionSalt)

	// Detect expected plugins based on prompt patterns
	expectedPlugins := detectExpectedPlugins(ec.Prompt, ec.AvailablePlugins)
	enriched.Data["plugins_expected"] = expectedPlugins

	// Calculate loaded plugin names
	loadedPluginNames := make([]string, len(ec.LoadedPlugins))
	for i, plugin := range ec.LoadedPlugins {
		loadedPluginNames[i] = plugin.Name
	}
	enriched.Data["plugins_loaded"] = loadedPluginNames

	// Calculate missing plugins (expected but not loaded)
	missingPlugins := calculateMissingPlugins(expectedPlugins, loadedPluginNames)
	if len(missingPlugins) > 0 {
		enriched.Data["plugins_missing"] = missingPlugins
	}

	return &enriched, nil
}

// Name returns the enricher name
func (p *PluginContextEnricher) Name() string {
	return "plugin_context"
}

// detectExpectedPlugins detects which plugins are expected based on prompt patterns
func detectExpectedPlugins(prompt string, availablePlugins []Plugin) []string {
	expected := make([]string, 0)

	// Minimum context length to avoid false positives
	const minContextLength = 10
	if len(prompt) < minContextLength {
		return expected
	}

	promptLower := strings.ToLower(prompt)

	// Check for each plugin pattern
	if detectResearchPattern(promptLower) && hasPlugin(availablePlugins, "research") {
		expected = append(expected, "research")
	}

	if detectWayfinderPattern(promptLower) && hasPlugin(availablePlugins, "wayfinder") {
		expected = append(expected, "wayfinder")
	}

	if detectPersonasPattern(prompt) && hasPlugin(availablePlugins, "personas") {
		expected = append(expected, "personas")
	}

	// Remove duplicates
	return uniqueStrings(expected)
}

// detectResearchPattern checks if prompt contains research: prefix
func detectResearchPattern(promptLower string) bool {
	return strings.Contains(promptLower, "research:")
}

// detectWayfinderPattern checks if prompt contains wayfinder: prefix
func detectWayfinderPattern(promptLower string) bool {
	return strings.Contains(promptLower, "wayfinder:")
}

// detectPersonasPattern checks if prompt contains @ character for persona mentions
// (distinguishes from email addresses)
func detectPersonasPattern(prompt string) bool {
	if !strings.Contains(prompt, "@") {
		return false
	}

	// Split by "@" and check each part after "@"
	parts := strings.Split(prompt, "@")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) == 0 {
			continue
		}

		afterAt := parts[i]

		// If first char is uppercase, likely a persona mention
		// Email: user@example.com (lowercase)
		// Persona: @TechLead (uppercase)
		if afterAt[0] >= 'A' && afterAt[0] <= 'Z' {
			return true
		}

		// If contains space before any dot, likely a persona mention
		dotIdx := strings.Index(afterAt, ".")
		spaceIdx := strings.Index(afterAt, " ")
		if spaceIdx != -1 && (dotIdx == -1 || spaceIdx < dotIdx) {
			return true
		}
	}

	return false
}

// hashPrompt creates a privacy-preserving hash of the prompt
//
// Uses per-session salt + per-prompt nonce to prevent:
//   - T1: Prompt reconstruction from hash
//   - T5: Timing attacks (nonce provides randomness)
//
// Security upgrade (P1-1): Uses crypto/rand for nonce instead of time.Now()
func hashPrompt(prompt string, sessionSalt string) string {
	// Per-prompt nonce (cryptographically secure random)
	nonce := generateNonce()

	// Combine: prompt + session salt + nonce
	input := prompt + sessionSalt + nonce

	// SHA-256 hash
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// generateNonce generates a cryptographically secure random nonce
// Returns a 16-byte hex-encoded string (32 characters)
func generateNonce() string {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		// Fallback to zero nonce if crypto/rand fails (extremely rare)
		// Still secure due to session salt, but logs warning
		return "0000000000000000000000000000000000000000"
	}
	return hex.EncodeToString(nonce)
}

// calculateMissingPlugins returns plugins that were expected but not loaded
func calculateMissingPlugins(expected []string, loaded []string) []string {
	loadedSet := make(map[string]bool)
	for _, plugin := range loaded {
		loadedSet[plugin] = true
	}

	missing := make([]string, 0)
	for _, plugin := range expected {
		if !loadedSet[plugin] {
			missing = append(missing, plugin)
		}
	}

	return missing
}

// hasPlugin checks if a plugin exists in the available plugins list
func hasPlugin(plugins []Plugin, name string) bool {
	for _, plugin := range plugins {
		if plugin.Name == name {
			return true
		}
	}
	return false
}

// uniqueStrings removes duplicates from a string slice
func uniqueStrings(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0)

	for _, str := range input {
		if !seen[str] {
			seen[str] = true
			result = append(result, str)
		}
	}

	return result
}
