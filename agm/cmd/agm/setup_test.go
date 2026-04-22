package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFindStaleHooks_NoSettings(t *testing.T) {
	tmpDir := t.TempDir()
	stale := findStaleHooks(tmpDir)
	if len(stale) != 0 {
		t.Errorf("expected 0 stale hooks when settings.json missing, got %d", len(stale))
	}
}

func TestFindStaleHooks_NoStale(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Create a hook binary
	hookPath := filepath.Join(claudeDir, "hooks", "test-hook")
	os.MkdirAll(filepath.Dir(hookPath), 0755)
	os.WriteFile(hookPath, []byte("#!/bin/sh"), 0755)

	// Create settings.json referencing it
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PostToolUse": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"command": filepath.Join("~", ".claude", "hooks", "test-hook"),
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(settings)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0600)

	stale := findStaleHooks(tmpDir)
	if len(stale) != 0 {
		t.Errorf("expected 0 stale hooks, got %d", len(stale))
	}
}

func TestFindStaleHooks_DetectsStale(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	// Create settings.json with a reference to non-existent binary
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PostToolUse": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"command": filepath.Join("~", ".claude", "hooks", "nonexistent-hook"),
						},
					},
				},
			},
		},
	}
	data, _ := json.Marshal(settings)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0600)

	stale := findStaleHooks(tmpDir)
	if len(stale) != 1 {
		t.Errorf("expected 1 stale hook, got %d", len(stale))
	}
}

func TestSetupCommand_Registered(t *testing.T) {
	// Verify the setup command is registered on rootCmd
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "setup" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'setup' command to be registered on rootCmd")
	}
}

func TestSetupCommand_CheckFlag(t *testing.T) {
	flag := setupCmd.Flags().Lookup("check")
	if flag == nil {
		t.Fatal("expected --check flag to be registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default false, got %s", flag.DefValue)
	}
}
