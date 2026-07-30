package codexhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LaunchConfigOverrides returns Codex CLI config overrides that load the
// attested hooks from the immutable materialization and disable project-local
// hook discovery. The manifest is encoded into argv before exec, so a process
// with write access to the sandbox cannot replace .codex/hooks.json between
// verification and Codex startup.
func LaunchConfigOverrides(hookRoot, workDir string) ([]string, error) {
	if !filepath.IsAbs(hookRoot) || filepath.Clean(hookRoot) != hookRoot {
		return nil, fmt.Errorf("materialized hook root must be a clean absolute path")
	}
	if !filepath.IsAbs(workDir) || filepath.Clean(workDir) != workDir {
		return nil, fmt.Errorf("codex working directory must be a clean absolute path")
	}

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

	hooks, err := inlineTOML(manifest.Hooks)
	if err != nil {
		return nil, fmt.Errorf("encode materialized Codex hooks for launch: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projectRoot, err := gitRoot(ctx, workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve Codex project root for immutable hooks: %w", err)
	}
	projects, err := inlineTOML(map[string]any{
		workDir:     map[string]any{"trust_level": "untrusted"},
		projectRoot: map[string]any{"trust_level": "untrusted"},
	})
	if err != nil {
		return nil, fmt.Errorf("encode disabled project hook discovery: %w", err)
	}
	return []string{
		"projects=" + projects,
		"hooks=" + hooks,
	}, nil
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
