package codexhooks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var sessionFlagsHookSource = func() string {
	if runtime.GOOS == "windows" {
		return `C:\<session-flags>\config.toml`
	}
	return "/<session-flags>/config.toml"
}()

const (
	attestedHookPath          = "/usr/local/libexec:/usr/bin:/bin:/usr/sbin:/sbin"
	attestedHookCommandPrefix = "/usr/bin/env PATH=" + attestedHookPath + " /bin/sh -c "
)

var (
	trustedHookJSONPath            = "/usr/local/libexec/dear-agent-codex-hook-json"
	validateAttestedExecutablePath = validateTrustedExecutableSearchPath
)

var neutralizedAttestedHookEvents = map[string]struct{}{
	"PostCompact":      {},
	"PreCompact":       {},
	"SessionStart":     {},
	"Stop":             {},
	"SubagentStop":     {},
	"UserPromptSubmit": {},
}

// LaunchConfigOverrides returns a Codex CLI config override that loads the
// attested hooks from the immutable materialization, pins their exact trust
// hashes, and disables the corresponding mutable project-layer entries.
//
// The manifest and state are encoded into argv before exec. AGM deliberately
// does not ask Codex to bypass hook trust globally: an entry appended to a
// writable project hooks.json after attestation therefore has neither a pinned
// session hash nor a trusted project key and cannot run. Project trust itself
// remains unchanged so the interactive TUI can start.
func LaunchConfigOverrides(hookRoot, workDir string) ([]string, error) {
	if !filepath.IsAbs(hookRoot) || filepath.Clean(hookRoot) != hookRoot {
		return nil, fmt.Errorf("materialized hook root must be a clean absolute path")
	}
	if !filepath.IsAbs(workDir) || filepath.Clean(workDir) != workDir {
		return nil, fmt.Errorf("codex working directory must be a clean absolute path")
	}

	hooks, err := readMaterializedHookManifest(hookRoot)
	if err != nil {
		return nil, err
	}
	if err := validateAttestedExecutablePath(attestedHookPath); err != nil {
		return nil, err
	}
	if err := validateTrustedHookExecutable(trustedHookJSONPath); err != nil {
		return nil, fmt.Errorf(
			"validate trusted Codex hook JSON helper (install with make install-codex-hook-json): %w",
			err,
		)
	}
	if err := neutralizeWorkspaceExecutingHooks(hooks); err != nil {
		return nil, err
	}
	if err := hardenHookCommands(hooks); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projectRoot, err := gitRoot(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Codex project root for immutable hooks: %w", err)
	}
	projectHookRoots, err := projectHookSourceRoots(ctx, workDir, projectRoot)
	if err != nil {
		return nil, err
	}
	hooks, err = hooksWithPinnedTrustState(hooks, projectHookRoots)
	if err != nil {
		return nil, err
	}
	encodedHooks, err := inlineTOML(hooks)
	if err != nil {
		return nil, fmt.Errorf("encode materialized Codex hooks for launch: %w", err)
	}
	return []string{"hooks=" + encodedHooks}, nil
}

func readMaterializedHookManifest(hookRoot string) (map[string]any, error) {
	manifestPath := filepath.Join(hookRoot, filepath.FromSlash(hooksManifestPath))
	info, err := os.Lstat(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("inspect materialized Codex hook manifest: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		return nil, fmt.Errorf("materialized Codex hook manifest must be a read-only regular file")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read materialized Codex hook manifest: %w", err)
	}

	var manifest struct {
		Description string         `json:"description,omitempty"`
		Hooks       map[string]any `json:"hooks"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse materialized Codex hook manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("parse materialized Codex hook manifest: %w", err)
	}
	if manifest.Hooks == nil {
		return nil, fmt.Errorf("materialized Codex hook manifest has no hooks object")
	}
	return manifest.Hooks, nil
}

// neutralizeWorkspaceExecutingHooks preserves each project-layer handler's
// index so hooksWithPinnedTrustState can disable the mutable copy, but replaces
// its session handler with an OS-owned no-op. Context-refresh and stop-time
// guardrail hooks intentionally execute workspace tools or code; they may run
// after ordinary Codex path review, but never under AGM's unattended bypass.
func neutralizeWorkspaceExecutingHooks(hooks map[string]any) error {
	for eventName := range neutralizedAttestedHookEvents {
		rawGroups, exists := hooks[eventName]
		if !exists {
			continue
		}
		groups, ok := rawGroups.([]any)
		if !ok {
			return fmt.Errorf("materialized Codex hook event %s must be an array", eventName)
		}
		for groupIndex, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok {
				return fmt.Errorf("materialized Codex hook event %s group %d must be an object", eventName, groupIndex)
			}
			rawHandlers, ok := group["hooks"]
			if !ok {
				continue
			}
			handlers, ok := rawHandlers.([]any)
			if !ok {
				return fmt.Errorf("materialized Codex hook event %s group %d handlers must be an array", eventName, groupIndex)
			}
			for handlerIndex, rawHandler := range handlers {
				handler, ok := rawHandler.(map[string]any)
				if !ok {
					return fmt.Errorf(
						"materialized Codex hook event %s group %d handler %d must be an object",
						eventName, groupIndex, handlerIndex,
					)
				}
				handler["command"] = "/bin/true"
				handler["statusMessage"] = "Workspace-executing hook disabled for attested bypass"
			}
		}
	}
	return nil
}

func hardenHookCommands(hooks map[string]any) error {
	for eventName := range hookEventKeyLabels {
		rawGroups, exists := hooks[eventName]
		if !exists {
			continue
		}
		groups, ok := rawGroups.([]any)
		if !ok {
			return fmt.Errorf("materialized Codex hook event %s must be an array", eventName)
		}
		for groupIndex, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok {
				return fmt.Errorf("materialized Codex hook event %s group %d must be an object", eventName, groupIndex)
			}
			rawHandlers, ok := group["hooks"]
			if !ok {
				continue
			}
			handlers, ok := rawHandlers.([]any)
			if !ok {
				return fmt.Errorf("materialized Codex hook event %s group %d handlers must be an array", eventName, groupIndex)
			}
			for handlerIndex, rawHandler := range handlers {
				handler, ok := rawHandler.(map[string]any)
				if !ok {
					return fmt.Errorf(
						"materialized Codex hook event %s group %d handler %d must be an object",
						eventName, groupIndex, handlerIndex,
					)
				}
				field := "command"
				if runtime.GOOS == "windows" {
					return fmt.Errorf("attested Codex command hooks require a POSIX runtime")
				}
				command, ok := handler[field].(string)
				if !ok || strings.TrimSpace(command) == "" {
					return fmt.Errorf(
						"materialized Codex hook event %s group %d handler %d command must be a non-empty string",
						eventName, groupIndex, handlerIndex,
					)
				}
				handler[field] = attestedHookCommandPrefix + shellSingleQuote(command)
			}
		}
	}
	return nil
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

var hookEventKeyLabels = map[string]string{
	"PreToolUse":        "pre_tool_use",
	"PermissionRequest": "permission_request",
	"PostToolUse":       "post_tool_use",
	"PreCompact":        "pre_compact",
	"PostCompact":       "post_compact",
	"SessionStart":      "session_start",
	"SessionEnd":        "session_end",
	"UserPromptSubmit":  "user_prompt_submit",
	"SubagentStart":     "subagent_start",
	"SubagentStop":      "subagent_stop",
	"Stop":              "stop",
}

func projectHookSourceRoots(ctx context.Context, workDir, projectRoot string) ([]string, error) {
	roots := map[string]struct{}{projectRoot: {}}
	commonDir, err := gitOutput(ctx, workDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("resolve Codex root checkout for immutable hooks: %w", err)
	}
	commonDir = filepath.Clean(strings.TrimSpace(commonDir))
	if filepath.IsAbs(commonDir) && filepath.Base(commonDir) == ".git" {
		root, err := filepath.EvalSymlinks(filepath.Dir(commonDir))
		if err != nil {
			return nil, fmt.Errorf("resolve Codex root checkout for immutable hooks: %w", err)
		}
		roots[filepath.Clean(root)] = struct{}{}
	}
	sorted := make([]string, 0, len(roots))
	for root := range roots {
		sorted = append(sorted, root)
	}
	sort.Strings(sorted)
	return sorted, nil
}

func hooksWithPinnedTrustState(hooks map[string]any, projectRoots []string) (map[string]any, error) {
	withState := make(map[string]any, len(hooks)+1)
	maps.Copy(withState, hooks)
	state := make(map[string]any)
	for eventName, eventKey := range hookEventKeyLabels {
		rawGroups, exists := hooks[eventName]
		if !exists {
			continue
		}
		groups, ok := rawGroups.([]any)
		if !ok {
			return nil, fmt.Errorf("materialized Codex hook event %s must be an array", eventName)
		}
		for groupIndex, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("materialized Codex hook event %s group %d must be an object", eventName, groupIndex)
			}
			rawHandlers, ok := group["hooks"]
			if !ok {
				continue
			}
			handlers, ok := rawHandlers.([]any)
			if !ok {
				return nil, fmt.Errorf("materialized Codex hook event %s group %d handlers must be an array", eventName, groupIndex)
			}
			for handlerIndex, rawHandler := range handlers {
				trustedHash, err := commandHookTrustedHash(eventName, eventKey, group, rawHandler)
				if err != nil {
					return nil, fmt.Errorf(
						"materialized Codex hook event %s group %d handler %d: %w",
						eventName, groupIndex, handlerIndex, err,
					)
				}
				sessionKey := fmt.Sprintf(
					"%s:%s:%d:%d",
					sessionFlagsHookSource, eventKey, groupIndex, handlerIndex,
				)
				state[sessionKey] = map[string]any{"trusted_hash": trustedHash}
				for _, root := range projectRoots {
					source := filepath.Join(root, filepath.FromSlash(hooksManifestPath))
					key := fmt.Sprintf("%s:%s:%d:%d", source, eventKey, groupIndex, handlerIndex)
					state[key] = map[string]any{"enabled": false}
				}
			}
		}
	}
	withState["state"] = state
	return withState, nil
}

func commandHookTrustedHash(
	eventName, eventKey string,
	group map[string]any,
	rawHandler any,
) (string, error) {
	normalizedHandler, err := normalizedCommandHookHandler(eventName, rawHandler)
	if err != nil {
		return "", err
	}
	matcher, err := normalizedHookMatcher(eventName, group)
	if err != nil {
		return "", err
	}
	identity := map[string]any{
		"event_name": eventKey,
		"hooks":      []any{normalizedHandler},
	}
	if matcher != nil {
		identity["matcher"] = *matcher
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(identity); err != nil {
		return "", fmt.Errorf("encode normalized hook identity: %w", err)
	}
	encoded := bytes.TrimSuffix(canonical.Bytes(), []byte("\n"))
	return fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)), nil
}

func normalizedCommandHookHandler(eventName string, rawHandler any) (map[string]any, error) {
	handler, ok := rawHandler.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object")
	}
	handlerType, ok := handler["type"].(string)
	if !ok || handlerType != "command" {
		return nil, fmt.Errorf("type must be command")
	}
	command, err := normalizedHookCommand(handler)
	if err != nil {
		return nil, err
	}
	timeout, err := normalizedHookTimeout(eventName, handler["timeout"])
	if err != nil {
		return nil, err
	}
	async, err := optionalBool(handler, "async")
	if err != nil {
		return nil, err
	}
	normalizedHandler := map[string]any{
		"type":    "command",
		"command": command,
		"timeout": timeout,
		"async":   async,
	}
	if err := copyOptionalString(normalizedHandler, handler, "statusMessage"); err != nil {
		return nil, err
	}
	if err := addNormalizedAdditionalContextLimit(normalizedHandler, handler, eventName); err != nil {
		return nil, err
	}
	return normalizedHandler, nil
}

func normalizedHookCommand(handler map[string]any) (string, error) {
	field := "command"
	if runtime.GOOS == "windows" {
		switch {
		case handler["commandWindows"] != nil:
			field = "commandWindows"
		case handler["command_windows"] != nil:
			field = "command_windows"
		}
	}
	command, ok := handler[field].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", field)
	}
	return command, nil
}

func normalizedHookMatcher(eventName string, group map[string]any) (*string, error) {
	if eventName == "UserPromptSubmit" || eventName == "Stop" || group["matcher"] == nil {
		return nil, nil
	}
	matcher, ok := group["matcher"].(string)
	if !ok {
		return nil, fmt.Errorf("matcher must be a string")
	}
	return &matcher, nil
}

func copyOptionalString(destination, source map[string]any, field string) error {
	raw, exists := source[field]
	if !exists || raw == nil {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return fmt.Errorf("%s must be a string", field)
	}
	destination[field] = value
	return nil
}

func addNormalizedAdditionalContextLimit(
	destination, source map[string]any,
	eventName string,
) error {
	raw, exists := source["additionalContextLimit"]
	if !exists || raw == nil || !hookEventSupportsAdditionalContext(eventName) {
		return nil
	}
	limit, err := jsonUint(raw, "additionalContextLimit")
	if err != nil {
		return err
	}
	if limit != 2500 {
		destination["additionalContextLimit"] = limit
	}
	return nil
}

func normalizedHookTimeout(eventName string, raw any) (uint64, error) {
	defaultTimeout := uint64(600)
	if eventName == "SessionEnd" {
		defaultTimeout = 1
	}
	timeout := defaultTimeout
	if raw != nil {
		var err error
		timeout, err = jsonUint(raw, "timeout")
		if err != nil {
			return 0, err
		}
	}
	if timeout < 1 {
		timeout = 1
	}
	if eventName == "SessionEnd" && timeout > 3 {
		timeout = 3
	}
	return timeout, nil
}

func jsonUint(raw any, field string) (uint64, error) {
	number, ok := raw.(json.Number)
	if !ok {
		return 0, fmt.Errorf("%s must be an unsigned integer", field)
	}
	value, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", field)
	}
	return value, nil
}

func optionalBool(object map[string]any, field string) (bool, error) {
	raw, exists := object[field]
	if !exists {
		return false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", field)
	}
	return value, nil
}

func hookEventSupportsAdditionalContext(eventName string) bool {
	switch eventName {
	case "PreToolUse", "PostToolUse", "SessionStart", "UserPromptSubmit", "SubagentStart":
		return true
	default:
		return false
	}
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func inlineTOML(value any) (string, error) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make([]string, 0, len(keys))
		for _, key := range keys {
			encodedKey, err := inlineTOML(key)
			if err != nil {
				return "", err
			}
			encodedValue, err := inlineTOML(typed[key])
			if err != nil {
				return "", fmt.Errorf("%s: %w", key, err)
			}
			entries = append(entries, encodedKey+"="+encodedValue)
		}
		return "{" + strings.Join(entries, ",") + "}", nil
	case []any:
		items := make([]string, 0, len(typed))
		for index, item := range typed {
			encoded, err := inlineTOML(item)
			if err != nil {
				return "", fmt.Errorf("item %d: %w", index, err)
			}
			items = append(items, encoded)
		}
		return "[" + strings.Join(items, ",") + "]", nil
	case string:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("encode string: %w", err)
		}
		return string(encoded), nil
	case json.Number:
		return typed.String(), nil
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	case nil:
		return "", fmt.Errorf("null is not a TOML value")
	default:
		return "", fmt.Errorf("unsupported JSON value %T", value)
	}
}
