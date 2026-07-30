package codexhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LaunchConfigOverrides returns a Codex CLI config override that loads the
// attested hooks from the immutable materialization and disables their mutable
// project-layer copies through hooks.state. The manifest and disablement state
// are encoded into argv before exec, so a process with write access to the
// sandbox cannot replace .codex/hooks.json between verification and Codex
// startup. Project trust remains unchanged so the interactive TUI can start.
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
	hooks, err = hooksWithProjectCopiesDisabled(hooks, projectHookRoots)
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

func hooksWithProjectCopiesDisabled(hooks map[string]any, projectRoots []string) (map[string]any, error) {
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
			for handlerIndex := range handlers {
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
