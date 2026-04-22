package delegation

import (
	"fmt"
	"os"
	"os/exec"
)

// NewDelegationStrategy creates the best available delegation strategy based on:
//  1. Provider override (if user specifies cross-provider execution)
//  2. CLI availability (for headless mode)
//  3. Fallback to external API
//
// Priority order:
//   - Headless (if CLI available)
//   - ExternalAPI (always available as fallback)
//
// Parameters:
//   - providerOverride: Optional provider family override (e.g., "gemini", "anthropic")
//     If empty, uses environment detection for best strategy.
//     If specified and doesn't match harness, tries headless mode first.
//
// Returns:
//   - DelegationStrategy: The best available strategy
//   - error: If no strategy is available (should never happen - ExternalAPI is always fallback)
func NewDelegationStrategy(providerOverride string) (DelegationStrategy, error) {
	// Normalize provider override
	provider := normalizeProvider(providerOverride)

	// If provider override specified, try headless mode
	if provider != "" {
		strategy := NewHeadlessStrategy(provider)
		if strategy.Available() {
			return strategy, nil
		}
	}

	// Fallback to external API (always available)
	if provider == "" {
		provider = "anthropic" // Default provider
	}
	return NewExternalAPIStrategy(provider), nil
}

// detectHarnessProvider detects which harness we're running in.
//
// Returns:
//   - "anthropic": Running in Claude Code (CLAUDE_SESSION_ID detected)
//   - "gemini": Running in Gemini CLI (GEMINI_SESSION_ID detected)
//   - "": Not in a harness
func detectHarnessProvider() string {
	if os.Getenv("CLAUDE_SESSION_ID") != "" {
		return "anthropic"
	}
	if os.Getenv("GEMINI_SESSION_ID") != "" {
		return "gemini"
	}
	return ""
}

// normalizeProvider normalizes provider family names.
//
// Supports aliases:
//   - "claude" → "anthropic"
//   - "google" → "gemini"
//
// Returns lowercase normalized name.
func normalizeProvider(provider string) string {
	switch provider {
	case "claude":
		return "anthropic"
	case "google":
		return "gemini"
	default:
		return provider
	}
}

// CanUseHeadless checks if headless CLI mode is available for a provider.
//
// Checks for CLI binary availability:
//   - gemini/google: Requires `gemini` binary
//   - anthropic/claude: Requires `claude` binary
//   - codex: Requires `codex` binary
//
// Returns true if the CLI binary exists in PATH.
func CanUseHeadless(provider string) bool {
	provider = normalizeProvider(provider)

	var binary string
	switch provider {
	case "gemini":
		binary = "gemini"
	case "anthropic":
		binary = "claude"
	case "codex":
		binary = "codex"
	default:
		return false
	}

	_, err := exec.LookPath(binary)
	return err == nil
}

// GetAvailableStrategies returns all available strategies for a provider.
//
// Useful for diagnostics and testing.
//
// Parameters:
//   - provider: Provider family (e.g., "anthropic", "gemini")
//
// Returns:
//   - []string: Names of available strategies (e.g., ["SubAgent", "Headless", "ExternalAPI"])
func GetAvailableStrategies(provider string) []string {
	var available []string

	// Check headless
	if CanUseHeadless(provider) {
		available = append(available, "Headless")
	}

	// External API always available
	available = append(available, "ExternalAPI")

	return available
}

// SelectStrategyWithFallback attempts to create a strategy with fallback on errors.
//
// Unlike NewDelegationStrategy which always succeeds (ExternalAPI fallback),
// this function tries each strategy in priority order and returns the first
// that successfully initializes.
//
// Priority:
//  1. Headless (if CLI available)
//  2. ExternalAPI (always succeeds)
//
// Parameters:
//   - provider: Provider family
//   - allowFallback: If false, returns error instead of falling back to ExternalAPI
//
// Returns:
//   - DelegationStrategy: The selected strategy
//   - error: Only if allowFallback is false and preferred strategies unavailable
func SelectStrategyWithFallback(provider string, allowFallback bool) (DelegationStrategy, error) {
	provider = normalizeProvider(provider)

	// Try headless
	if CanUseHeadless(provider) {
		headless := NewHeadlessStrategy(provider)
		if headless.Available() {
			return headless, nil
		}
	}

	// Fallback to external API
	if !allowFallback {
		return nil, fmt.Errorf("no suitable strategy available for provider %s (fallback disabled)", provider)
	}

	if provider == "" {
		provider = "anthropic"
	}
	return NewExternalAPIStrategy(provider), nil
}
