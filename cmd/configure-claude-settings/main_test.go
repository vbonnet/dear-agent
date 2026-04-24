package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestSettings(t *testing.T, dir string, settings map[string]interface{}) string {
	t.Helper()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(claudeDir, "settings.json")
	if settings != nil {
		data, _ := json.MarshalIndent(settings, "", "  ")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func readTestSettings(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]interface{}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGetNestedValue(t *testing.T) {
	m := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{"a", "b"},
		},
		"model": "opus",
	}

	if v := getNestedValue(m, "model"); v != "opus" {
		t.Errorf("got %v, want opus", v)
	}
	if v := getNestedValue(m, "hooks.PreToolUse"); v == nil {
		t.Error("expected non-nil for hooks.PreToolUse")
	}
	if v := getNestedValue(m, "nonexistent"); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestSetNestedValue(t *testing.T) {
	m := map[string]interface{}{}
	setNestedValue(m, "hooks.PreToolUse", "test")
	hooks := m["hooks"].(map[string]interface{})
	if hooks["PreToolUse"] != "test" {
		t.Errorf("got %v, want test", hooks["PreToolUse"])
	}
}

func TestRemoveNestedKey(t *testing.T) {
	m := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse":  "a",
			"PostToolUse": "b",
		},
		"model": "opus",
	}
	removeNestedKey(m, "hooks.PreToolUse")
	hooks := m["hooks"].(map[string]interface{})
	if _, ok := hooks["PreToolUse"]; ok {
		t.Error("PreToolUse should have been removed")
	}
	if hooks["PostToolUse"] != "b" {
		t.Error("PostToolUse should be preserved")
	}
}

func TestHookExists(t *testing.T) {
	hooksMap := map[string]interface{}{
		"PreToolUse": []interface{}{
			map[string]interface{}{
				"hooks": []interface{}{
					map[string]interface{}{
						"command": "~/.claude/hooks/my-hook",
						"type":    "command",
					},
				},
			},
		},
	}

	if !hookExists(hooksMap, "PreToolUse", "~/.claude/hooks/my-hook") {
		t.Error("expected hook to exist")
	}
	if hookExists(hooksMap, "PreToolUse", "~/.claude/hooks/other-hook") {
		t.Error("expected hook to not exist")
	}
	if hookExists(hooksMap, "PostToolUse", "~/.claude/hooks/my-hook") {
		t.Error("expected hook to not exist for wrong event")
	}
}

func TestRemoveHookByCommand(t *testing.T) {
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"command": "~/.claude/hooks/keep-me",
							"type":    "command",
						},
					},
				},
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"command": "~/.claude/hooks/remove-me",
							"type":    "command",
						},
					},
				},
			},
		},
	}

	removed := removeHookByCommand(settings, "hooks.PreToolUse", "~/.claude/hooks/remove-me")
	if !removed {
		t.Error("expected removal to succeed")
	}

	hooksMap := settings["hooks"].(map[string]interface{})
	arr := hooksMap["PreToolUse"].([]interface{})
	if len(arr) != 1 {
		t.Errorf("expected 1 remaining group, got %d", len(arr))
	}

	// Try removing non-existent
	removed = removeHookByCommand(settings, "hooks.PreToolUse", "~/.claude/hooks/nonexistent")
	if removed {
		t.Error("expected removal of nonexistent to return false")
	}
}

func TestReadWriteSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")

	// Read non-existent file returns empty map
	s, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 0 {
		t.Errorf("expected empty map, got %v", s)
	}

	// Write and read back
	s["model"] = "opus"
	if err := writeSettings(path, s, false); err != nil {
		t.Fatal(err)
	}

	s2, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if s2["model"] != "opus" {
		t.Errorf("expected opus, got %v", s2["model"])
	}

	// Dry run should not modify
	s2["model"] = "sonnet"
	if err := writeSettings(path, s2, true); err != nil {
		t.Fatal(err)
	}
	s3, _ := readSettings(path)
	if s3["model"] != "opus" {
		t.Error("dry run should not have modified the file")
	}
}

func TestCleanupDirs(t *testing.T) {
	dir := t.TempDir()

	// Create one real directory and one that doesn't exist
	realDir := filepath.Join(dir, "exists")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	staleDir := filepath.Join(dir, "gone")

	settings := map[string]interface{}{
		"additionalDirectories": []interface{}{realDir, staleDir},
		"model":                 "opus",
	}
	path := writeTestSettings(t, dir, settings)

	result, err := cleanupDirs(path, false)
	if err != nil {
		t.Fatal(err)
	}

	if result.Before != 2 {
		t.Errorf("expected Before=2, got %d", result.Before)
	}
	if result.After != 1 {
		t.Errorf("expected After=1, got %d", result.After)
	}
	if len(result.Removed) != 1 || result.Removed[0] != staleDir {
		t.Errorf("expected Removed=[%s], got %v", staleDir, result.Removed)
	}

	// Verify file was written correctly
	s := readTestSettings(t, path)
	dirs, ok := s["additionalDirectories"].([]interface{})
	if !ok {
		t.Fatal("additionalDirectories not found after cleanup")
	}
	if len(dirs) != 1 || dirs[0] != realDir {
		t.Errorf("expected [%s], got %v", realDir, dirs)
	}
	// Other keys preserved
	if s["model"] != "opus" {
		t.Error("model key should be preserved")
	}
}

func TestCleanupDirs_NoStale(t *testing.T) {
	dir := t.TempDir()

	realDir := filepath.Join(dir, "exists")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}

	settings := map[string]interface{}{
		"additionalDirectories": []interface{}{realDir},
	}
	path := writeTestSettings(t, dir, settings)

	result, err := cleanupDirs(path, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Removed) != 0 {
		t.Errorf("expected no removals, got %v", result.Removed)
	}
}

func TestCleanupDirs_DryRun(t *testing.T) {
	dir := t.TempDir()

	staleDir := filepath.Join(dir, "gone")
	settings := map[string]interface{}{
		"additionalDirectories": []interface{}{staleDir},
	}
	path := writeTestSettings(t, dir, settings)

	result, err := cleanupDirs(path, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Removed) != 1 {
		t.Errorf("expected 1 removal in dry run, got %d", len(result.Removed))
	}

	// File should NOT be modified in dry run
	s := readTestSettings(t, path)
	dirs := s["additionalDirectories"].([]interface{})
	if len(dirs) != 1 {
		t.Error("dry run should not have modified the file")
	}
}
