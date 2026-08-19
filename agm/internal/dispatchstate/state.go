// Package dispatchstate holds the host-local state AGM needs to route
// worker completions: which Dispatch session receives them, and the
// provider quota snapshot routing decisions consult. Both live as plain
// files under the user's home directory rather than in the session store,
// so they stay readable when that store is unreachable, which is exactly
// when completion routing matters most. See SPEC.md.
package dispatchstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
)

const (
	relayTargetEnv  = "AGM_COMPLETION_RELAY_TARGET"
	relayTargetFile = "completion-relay-target"
	quotaStateFile  = ".local/state/dear-agent/quota/latest.json"

	// relayDirPerm and relayFilePerm are the modes SPEC.md promises for the
	// relay state. They are enforced on every write, including when the
	// directory or file already exists with broader modes, because a target
	// another user can rewrite redirects every completion on the host.
	relayDirPerm  os.FileMode = 0o700
	relayFilePerm os.FileMode = 0o600

	// quotaStaleAfter is how long a quota snapshot stays authoritative.
	quotaStaleAfter = 30 * time.Minute
)

// DefaultRelayFallback is the session the installed watch-stalled schedule
// relays to when nothing overrides it. It lives here, next to the
// resolution rule, so every surface that reports the effective target
// (CLI getter, MCP getter, the schedule installer) answers the same thing
// instead of each keeping a copy that can drift.
const DefaultRelayFallback = "vroom-orchestrator"

// RelayTargetResult reports a resolved completion relay target and which
// source it came from (file, environment, or fallback). Reason carries a
// state-read failure that the caller worked around, so a degraded resolve
// is visible rather than silently indistinguishable from "no override set".
type RelayTargetResult struct {
	Operation string `json:"operation"`
	Target    string `json:"target,omitempty"`
	Source    string `json:"source"`
	Path      string `json:"path,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// RelayTargetPath returns the path of the relay-target state file.
func RelayTargetPath(homeDir string) string {
	return filepath.Join(homeDir, ".agm", relayTargetFile)
}

// ResolveRelayTarget resolves the Dispatch session completions should be
// relayed to.
//
// Precedence is state file, then AGM_COMPLETION_RELAY_TARGET, then the
// caller's fallback. The file wins deliberately: it is the live control
// surface a running watcher re-reads, while the environment variable is
// only the value that watcher started with. If the environment won, then
// on every host that exports it SetRelayTarget would report success while
// the watcher kept relaying to the old session, which is precisely the
// silent misrouting this package exists to prevent.
//
// A state file that exists but cannot be read is reported through Reason
// rather than hidden: resolution still falls through so the completion is
// delivered somewhere recoverable, but the caller can see that the live
// target was unreadable instead of concluding none was set.
func ResolveRelayTarget(homeDir, fallback string, getenv func(string) string) RelayTargetResult {
	if getenv == nil {
		getenv = os.Getenv
	}
	result := RelayTargetResult{Operation: "completion_relay_target"}
	path := RelayTargetPath(homeDir)

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if target := strings.TrimSpace(string(data)); target != "" {
			result.Target = target
			result.Source = "file"
			result.Path = path
			return result
		}
		// An empty file is a truncated or half-written state, not a
		// request to disable relaying: SetRelayTarget rejects blank
		// targets, so this value cannot have been set deliberately.
		result.Reason = "relay target state file is empty"
	case errors.Is(err, os.ErrNotExist):
		// No override configured. Not a failure.
	default:
		result.Reason = "read relay target state: " + err.Error()
	}

	if target := strings.TrimSpace(getenv(relayTargetEnv)); target != "" {
		result.Target = target
		result.Source = "env:" + relayTargetEnv
		return result
	}
	result.Target = strings.TrimSpace(fallback)
	result.Source = "fallback"
	return result
}

// SetRelayTarget persists target as the relay destination. A blank target
// is rejected so an empty write cannot silently disable relaying.
//
// The write is atomic (temp file plus rename) because a watcher resolving
// a completion can read this file at any moment: a truncating write would
// let it observe an empty target and route that completion to the stale
// fallback, and a crash mid-write would leave the file empty for good.
func SetRelayTarget(homeDir, target string) (RelayTargetResult, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return RelayTargetResult{}, fmt.Errorf("relay target is required")
	}
	path := RelayTargetPath(homeDir)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, relayDirPerm); err != nil {
		return RelayTargetResult{}, fmt.Errorf("create relay target dir: %w", err)
	}
	// MkdirAll leaves an existing directory's mode alone, so enforce it.
	if err := os.Chmod(dir, relayDirPerm); err != nil {
		return RelayTargetResult{}, fmt.Errorf("enforce relay target dir mode: %w", err)
	}
	// AtomicWrite chmods the temp file before renaming, so the replacement
	// inode carries relayFilePerm even if the previous file was broader.
	if err := fileutil.AtomicWrite(path, []byte(target+"\n"), relayFilePerm); err != nil {
		return RelayTargetResult{}, fmt.Errorf("write relay target: %w", err)
	}
	return RelayTargetResult{Operation: "completion_relay_target", Target: target, Source: "file", Path: path}, nil
}

// QuotaStatus is a read of the provider quota snapshot, including whether
// it is stale and whether callers should back off.
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

// QuotaPath returns the path of the quota snapshot written by the meter.
func QuotaPath(homeDir string) string {
	return filepath.Join(homeDir, quotaStateFile)
}

// ReadQuotaStatus reads the quota snapshot for provider. A missing or
// unparseable snapshot yields an unavailable result with a reason rather
// than an error, because callers consult this on completion and stall
// paths where failing hard would drop the output being routed.
//
// Two cases resolve pessimistically, because this status is used to pace
// work and a false "plenty of quota" is the expensive direction: a
// snapshot whose capture time is missing or not RFC3339 is treated as
// stale, and a provider absent from a structured provider collection is
// reported unavailable rather than falling back to the whole payload.
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
	selected, found := selectProvider(payload, provider)
	if !found {
		result.Reason = fmt.Sprintf("provider %q not found in quota state", provider)
		return result
	}
	result.Available = true
	result.Data = selected

	// Prefer the selected provider's own capture time, falling back to a
	// top-level one: in a multi-provider snapshot each block can carry its
	// own timestamp, and the envelope's time says nothing about how fresh
	// this provider's numbers are.
	result.UpdatedAt = firstString(selected, updatedAtKeys...)
	if result.UpdatedAt == "" {
		result.UpdatedAt = firstString(payload, updatedAtKeys...)
	}
	switch ts, err := time.Parse(time.RFC3339, result.UpdatedAt); {
	case result.UpdatedAt == "":
		result.Stale = true
		result.Reason = "quota state has no capture time; treating as stale"
	case err != nil:
		result.Stale = true
		result.Reason = "quota state capture time is not RFC3339; treating as stale"
	default:
		result.Stale = now.Sub(ts) > quotaStaleAfter
	}

	result.Warning = result.Stale || boolValue(selected, "throttled", "warning", "overspending") ||
		strings.EqualFold(firstString(selected, "breaker_state", "breakerState"), "throttled") ||
		lowRemaining(selected)
	return result
}

// updatedAtKeys are the capture-time field spellings emitted across the
// meters that write this snapshot.
var updatedAtKeys = []string{
	"updated_at", "updatedAt", "timestamp", "captured_at", "capturedAt",
	"generated_at", "generatedAt", "writtenAt",
}

// selectProvider picks provider's block out of a quota payload. The bool
// reports whether a block was actually identified: once the payload
// carries a structured provider collection, a miss must not degrade into
// "here is the whole file", which would report an absent provider as
// available and suppress its pacing warnings.
func selectProvider(payload map[string]any, provider string) (map[string]any, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return payload, true
	}
	if nested, ok := caseInsensitiveMap(payload, provider); ok {
		return nested, true
	}
	if providers, ok := payload["providers"].([]any); ok {
		for _, item := range providers {
			candidate, ok := item.(map[string]any)
			if !ok {
				continue
			}
			for _, key := range []string{"provider", "source_id", "sourceId", "family"} {
				if strings.EqualFold(firstString(candidate, key), provider) {
					return candidate, true
				}
			}
		}
		return nil, false
	}
	if providers, ok := payload["providers"].(map[string]any); ok {
		if nested, found := caseInsensitiveMap(providers, provider); found {
			return nested, true
		}
		return nil, false
	}
	// A flat, single-provider snapshot: no provider structure to
	// contradict the request, so the payload is that provider's data.
	return payload, true
}

// caseInsensitiveMap looks up key in m ignoring case, so a snapshot keyed
// "Codex" still answers a request for "codex".
func caseInsensitiveMap(m map[string]any, key string) (map[string]any, bool) {
	for candidate, value := range m {
		if !strings.EqualFold(candidate, key) {
			continue
		}
		nested, ok := value.(map[string]any)
		if ok {
			return nested, true
		}
	}
	return nil, false
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// boolValue reads a flag that snapshots spell either as a JSON bool or as
// a string ("true"/"yes"), since the meters disagree on the encoding.
func boolValue(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch value := m[key].(type) {
		case bool:
			if value {
				return true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "true", "yes":
				return true
			}
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
