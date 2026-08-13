package dispatchstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	relayTargetEnv  = "AGM_COMPLETION_RELAY_TARGET"
	relayTargetFile = "completion-relay-target"
	quotaStateFile  = ".local/state/dear-agent/quota/latest.json"
)

type RelayTargetResult struct {
	Operation string `json:"operation"`
	Target    string `json:"target,omitempty"`
	Source    string `json:"source"`
	Path      string `json:"path,omitempty"`
}

func RelayTargetPath(homeDir string) string {
	return filepath.Join(homeDir, ".agm", relayTargetFile)
}

func ResolveRelayTarget(homeDir, fallback string, getenv func(string) string) RelayTargetResult {
	if getenv == nil {
		getenv = os.Getenv
	}
	if target := strings.TrimSpace(getenv(relayTargetEnv)); target != "" {
		return RelayTargetResult{Operation: "completion_relay_target", Target: target, Source: "env:" + relayTargetEnv}
	}
	path := RelayTargetPath(homeDir)
	if data, err := os.ReadFile(path); err == nil {
		if target := strings.TrimSpace(string(data)); target != "" {
			return RelayTargetResult{Operation: "completion_relay_target", Target: target, Source: "file", Path: path}
		}
	}
	return RelayTargetResult{Operation: "completion_relay_target", Target: strings.TrimSpace(fallback), Source: "fallback"}
}

func SetRelayTarget(homeDir, target string) (RelayTargetResult, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return RelayTargetResult{}, fmt.Errorf("relay target is required")
	}
	path := RelayTargetPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return RelayTargetResult{}, fmt.Errorf("create relay target dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(target+"\n"), 0o600); err != nil {
		return RelayTargetResult{}, fmt.Errorf("write relay target: %w", err)
	}
	return RelayTargetResult{Operation: "completion_relay_target", Target: target, Source: "file", Path: path}, nil
}

type QuotaStatus struct {
	Operation string         `json:"operation"`
	Available bool           `json:"available"`
	Provider  string         `json:"provider,omitempty"`
	Path      string         `json:"path"`
	UpdatedAt string         `json:"updated_at,omitempty"`
	Stale     bool           `json:"stale"`
	Warning   bool           `json:"warning"`
	Reason    string         `json:"reason,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

func QuotaPath(homeDir string) string {
	return filepath.Join(homeDir, quotaStateFile)
}

func ReadQuotaStatus(homeDir, provider string, now time.Time) QuotaStatus {
	path := QuotaPath(homeDir)
	result := QuotaStatus{Operation: "quota_status", Provider: provider, Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		result.Reason = err.Error()
		if errors.Is(err, os.ErrNotExist) {
			result.Reason = "quota state not found"
		}
		return result
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		result.Reason = "parse quota state: " + err.Error()
		return result
	}
	selected := selectProvider(payload, provider)
	result.Available = true
	result.Data = selected
	result.UpdatedAt = firstString(payload, "updated_at", "updatedAt", "timestamp", "captured_at", "capturedAt", "generated_at", "generatedAt", "writtenAt")
	if result.UpdatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, result.UpdatedAt); err == nil {
			result.Stale = now.Sub(ts) > 30*time.Minute
		}
	}
	result.Warning = result.Stale || boolValue(selected, "throttled", "warning", "overspending") ||
		strings.EqualFold(firstString(selected, "breaker_state", "breakerState"), "throttled") ||
		lowRemaining(selected)
	return result
}

func selectProvider(payload map[string]any, provider string) map[string]any {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return payload
	}
	if providers, ok := payload["providers"].([]any); ok {
		for _, item := range providers {
			candidate, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"provider", "source_id", "sourceId", "family"} {
				if strings.EqualFold(firstString(candidate, key), provider) {
					return candidate
				}
			}
		}
	}
	if nested, ok := payload[provider].(map[string]any); ok {
		return nested
	}
	return payload
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func boolValue(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, _ := m[key].(bool)
		if value {
			return true
		}
	}
	return false
}

func lowRemaining(m map[string]any) bool {
	for _, key := range []string{"remaining_percent", "remainingPercent", "remaining_pct", "percent_remaining"} {
		switch value := m[key].(type) {
		case float64:
			return value <= 20
		case int:
			return value <= 20
		}
	}
	return false
}
