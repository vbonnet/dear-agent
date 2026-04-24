package enforcement

import (
	"context"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// Telemetry event types for enforcement validation
const (
	EventEnforcementValidation = "enforcement.validation"
	EventIdentityDetection     = "enforcement.identity_detection"
	EventCacheAccess           = "enforcement.cache_access"
	EventViolation             = "enforcement.violation"
	EventPluginValidation      = "enforcement.plugin_validation"
	EventConfigValidation      = "enforcement.config_validation"
)

// ValidationEvent contains telemetry data for enforcement validation
type ValidationEvent struct {
	Phase    string        `json:"phase"`            // "identity", "plugins", "config", "enforcement"
	Result   string        `json:"result"`           // "success", "failure"
	Duration time.Duration `json:"duration"`         // Time spent in validation
	Error    string        `json:"error,omitempty"`  // Error message if failure
	Method   string        `json:"method,omitempty"` // Detection method for identity (gcp_adc, git, env)
}

// IdentityDetectionEvent contains telemetry for identity detection
type IdentityDetectionEvent struct {
	Method   string        `json:"method"` // "gcp_adc", "git_config", "env"
	Result   string        `json:"result"` // "success", "failure", "not_found"
	Duration time.Duration `json:"duration"`
	Cached   bool          `json:"cached"` // Whether result came from cache
}

// CacheAccessEvent contains telemetry for cache access
type CacheAccessEvent struct {
	Type     string        `json:"type"`   // "disk", "memory", "miss"
	Result   string        `json:"result"` // "hit", "miss", "error"
	Duration time.Duration `json:"duration"`
}

// ViolationEvent contains telemetry for enforcement violations
type ViolationEvent struct {
	Type    string `json:"type"`              // "domain", "plugin_missing", "plugin_version", "config"
	Detail  string `json:"detail"`            // Specific violation details
	Domain  string `json:"domain,omitempty"`  // For domain violations
	Plugin  string `json:"plugin,omitempty"`  // For plugin violations
	Version string `json:"version,omitempty"` // For version violations
}

// ValidateWithTelemetry runs enforcement validation and emits telemetry events.
//
// This is the instrumented version of Validate() that should be used when
// eventbus is available. Falls back to standard Validate() if bus is nil.
//
// Telemetry events emitted:
//   - EventEnforcementValidation: Overall validation result
//   - EventViolation: For each violation encountered
func (v *Validator) ValidateWithTelemetry(ctx context.Context, bus *eventbus.LocalBus) error {
	// Fall back to standard validation if no telemetry bus
	if bus == nil {
		return v.Validate(ctx)
	}

	start := time.Now()

	err := v.Validate(ctx)

	eventData := map[string]interface{}{
		"phase":    "enforcement",
		"result":   "success",
		"duration": time.Since(start).Milliseconds(),
	}

	if err != nil {
		eventData["result"] = "failure"
		eventData["error"] = err.Error()

		// Emit specific violation event
		v.emitViolationEvent(ctx, bus, err)
	}

	event := eventbus.NewEvent(EventEnforcementValidation, "enforcement", eventData)
	bus.Publish(ctx, event)

	return err
}

// emitViolationEvent analyzes the error and emits a specific violation event
func (v *Validator) emitViolationEvent(ctx context.Context, bus *eventbus.LocalBus, err error) {
	errMsg := err.Error()

	violationData := map[string]interface{}{
		"detail": errMsg,
	}

	// Determine violation type from error message
	if containsStr(errMsg, "domain") && containsStr(errMsg, "not in allowed list") {
		violationData["type"] = "domain"
		if v.identity != nil {
			violationData["domain"] = v.identity.Domain
		}
	} else if containsStr(errMsg, "missing required plugins") {
		violationData["type"] = "plugin_missing"
		// Extract plugin name from error if possible
		if parts := extractPluginNames(errMsg); len(parts) > 0 {
			violationData["plugin"] = parts[0] // First missing plugin
		}
	} else if containsStr(errMsg, "version mismatches") {
		violationData["type"] = "plugin_version"
		// Extract plugin and version info
		if parts := extractVersionMismatch(errMsg); len(parts) >= 2 {
			violationData["plugin"] = parts[0]
			violationData["version"] = parts[1]
		}
	} else if containsStr(errMsg, "no identity detected") {
		violationData["type"] = "identity_missing"
	} else {
		violationData["type"] = "config"
	}

	event := eventbus.NewEvent(EventViolation, "enforcement", violationData)
	bus.Publish(ctx, event)
}

// Helper functions

func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && strIndex(s, substr) >= 0)
}

func strIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// extractPluginNames extracts plugin names from error message
// Example: "missing required plugins: foo, bar" -> ["foo", "bar"]
func extractPluginNames(errMsg string) []string {
	// Find "missing required plugins: " prefix
	prefix := "missing required plugins: "
	idx := strIndex(errMsg, prefix)
	if idx < 0 {
		return nil
	}

	// Extract plugin list after prefix
	pluginList := errMsg[idx+len(prefix):]

	// Stop at semicolon (marks end of plugin list)
	semiIdx := strIndex(pluginList, ";")
	if semiIdx >= 0 {
		pluginList = pluginList[:semiIdx]
	}

	// Split by comma
	var plugins []string
	current := ""
	for i := 0; i < len(pluginList); i++ {
		ch := pluginList[i]
		if ch == ',' {
			if len(current) > 0 {
				plugins = append(plugins, trimSpace(current))
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if len(current) > 0 {
		plugins = append(plugins, trimSpace(current))
	}

	return plugins
}

// extractVersionMismatch extracts plugin and version from version mismatch error
// Example: "foo (have 1.0.0, need >= 2.0.0)" -> ["foo", "1.0.0", "2.0.0"]
func extractVersionMismatch(errMsg string) []string {
	// Find first "(" for plugin name
	start := strIndex(errMsg, "(have ")
	if start < 0 {
		return nil
	}

	// Extract plugin name before " (have"
	// First skip backwards over any whitespace before the "("
	nameEnd := start
	for nameEnd > 0 && errMsg[nameEnd-1] == ' ' {
		nameEnd--
	}

	// Now find the start of the plugin name (first space/colon before it, or beginning)
	nameStart := 0
	for i := nameEnd - 1; i >= 0; i-- {
		ch := errMsg[i]
		if ch == ' ' || ch == ':' {
			nameStart = i + 1
			break
		}
	}

	pluginName := trimSpace(errMsg[nameStart:nameEnd])

	// Extract version info
	versionPart := errMsg[start:]
	// "have X.X.X, need >= Y.Y.Y)"

	// Extract "have" version
	haveStart := strIndex(versionPart, "have ")
	haveEnd := strIndex(versionPart, ",")
	haveVersion := ""
	if haveStart >= 0 && haveEnd > haveStart {
		haveVersion = trimSpace(versionPart[haveStart+5 : haveEnd])
	}

	// Extract "need" version
	needStart := strIndex(versionPart, "need >= ")
	needEnd := strIndex(versionPart, ")")
	needVersion := ""
	if needStart >= 0 && needEnd > needStart {
		needVersion = trimSpace(versionPart[needStart+8 : needEnd])
	}

	return []string{pluginName, haveVersion, needVersion}
}

func trimSpace(s string) string {
	// Trim leading and trailing whitespace
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
		end--
	}

	return s[start:end]
}
