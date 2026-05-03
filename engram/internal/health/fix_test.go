package health

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFixHookExtensionMismatches tests removing .py extensions when binaries exist
func TestFixHookExtensionMismatches(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "engram-fix-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock hook binary (without .py extension)
	hookDir := filepath.Join(tmpDir, ".claude", "hooks")
	if err := os.MkdirAll(hookDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks dir: %v", err)
	}

	// Create binary file (no .py extension)
	binaryPath := filepath.Join(hookDir, "posttool-auto-commit-beads")
	if err := os.WriteFile(binaryPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create binary: %v", err)
	}

	// Load broken settings fixture
	fixtureData, err := os.ReadFile("testdata/settings-broken-extensions.json")
	if err != nil {
		t.Fatalf("Failed to read fixture: %v", err)
	}

	// Create test settings file with broken extension
	settingsPath := filepath.Join(tmpDir, ".claude", "settings.json")
	// Update paths in fixture to point to our temp directory
	settingsContent := strings.ReplaceAll(string(fixtureData), "~/.claude/hooks/", tmpDir+"/.claude/hooks/")
	if err := os.WriteFile(settingsPath, []byte(settingsContent), 0644); err != nil {
		t.Fatalf("Failed to create test settings: %v", err)
	}

	// Run the fix
	fixer := NewTier1Fixer(tmpDir)
	settingsData, _ := os.ReadFile(settingsPath)
	if err := fixer.fixExtensionsInFile(settingsPath, settingsData); err != nil {
		t.Fatalf("fixExtensionsInFile failed: %v", err)
	}

	// Verify backup was created
	backupPath := settingsPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Read fixed settings
	fixedData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read fixed settings: %v", err)
	}

	// Verify .py extensions were removed
	fixedContent := string(fixedData)
	if strings.Contains(fixedContent, "posttool-auto-commit-beads.py") {
		t.Error("Extension .py was not removed from posttool-auto-commit-beads")
	}

	// Verify the fixed path exists
	if !strings.Contains(fixedContent, "posttool-auto-commit-beads") {
		t.Error("Fixed command path not found in settings")
	}

	// Verify JSON is still valid
	var settings map[string]interface{}
	if err := json.Unmarshal(fixedData, &settings); err != nil {
		t.Errorf("Fixed settings.json is not valid JSON: %v", err)
	}
}

// TestFixHookPaths tests correcting known wrong paths
func TestFixHookPaths(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "engram-fix-paths-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create correct hook directory structure
	correctHookDir := filepath.Join(tmpDir, "src/ws/oss/repos/engram/hooks")
	if err := os.MkdirAll(correctHookDir, 0755); err != nil {
		t.Fatalf("Failed to create correct hooks dir: %v", err)
	}

	// Create hook file at correct location
	hookPath := filepath.Join(correctHookDir, "token-tracker-init")
	if err := os.WriteFile(hookPath, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create hook: %v", err)
	}

	// Create settings with simplified wrong path that uses our known correction
	settingsDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("Failed to create settings dir: %v", err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Use absolute path with /main/hooks/ that should be corrected to /hooks/
	wrongPath := filepath.Join(tmpDir, "src/ws/oss/repos/engram/main/hooks/token-tracker-init")
	settingsContent := `{"hooks":{"SessionStart":[{"hooks":[{"command":"` + wrongPath + `"}],"matcher":".*"}]}}`
	if err := os.WriteFile(settingsPath, []byte(settingsContent), 0644); err != nil {
		t.Fatalf("Failed to create test settings: %v", err)
	}

	// Run the fix
	fixer := NewTier1Fixer(tmpDir)
	settingsData, _ := os.ReadFile(settingsPath)
	if err := fixer.fixPathsInFile(settingsPath, settingsData); err != nil {
		t.Fatalf("fixPathsInFile failed: %v", err)
	}

	// Read fixed settings
	fixedData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read fixed settings: %v", err)
	}

	fixedContent := string(fixedData)

	// Verify wrong path was corrected
	if strings.Contains(fixedContent, "/main/hooks/") {
		t.Errorf("Wrong path '/main/hooks/' was not corrected. Content: %s", fixedContent)
	}

	// Verify correct path exists
	if !strings.Contains(fixedContent, "/hooks/token-tracker-init") {
		t.Errorf("Correct path not found after fix. Content: %s", fixedContent)
	}

	// Verify backup was created
	backupPath := settingsPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}
}

// TestFixMarketplaceConfig tests fixing source="directory" entries
func TestFixMarketplaceConfig(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "engram-fix-marketplace-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create marketplace config directory
	pluginsDir := filepath.Join(tmpDir, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		t.Fatalf("Failed to create plugins dir: %v", err)
	}

	// Load invalid marketplace fixture
	fixtureData, err := os.ReadFile("testdata/marketplace-invalid-source.json")
	if err != nil {
		t.Fatalf("Failed to read fixture: %v", err)
	}

	// Create test marketplace config
	mktPath := filepath.Join(pluginsDir, "known_marketplaces.json")
	if err := os.WriteFile(mktPath, fixtureData, 0644); err != nil {
		t.Fatalf("Failed to create test marketplace config: %v", err)
	}

	// Set HOME to tmpDir for the fixer
	t.Setenv("HOME", tmpDir)

	// Run the fix
	fixer := NewTier1Fixer(tmpDir)
	if err := fixer.fixMarketplaceConfig(); err != nil {
		t.Fatalf("fixMarketplaceConfig failed: %v", err)
	}

	// Verify backup was created
	backupPath := mktPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Read fixed marketplace config
	fixedData, err := os.ReadFile(mktPath)
	if err != nil {
		t.Fatalf("Failed to read fixed marketplace config: %v", err)
	}

	// Parse fixed JSON
	var marketplaces map[string]interface{}
	if err := json.Unmarshal(fixedData, &marketplaces); err != nil {
		t.Fatalf("Fixed marketplace config is not valid JSON: %v", err)
	}

	// Verify engram entry was fixed
	if engram, ok := marketplaces["engram"]; ok {
		engramMap := engram.(map[string]interface{})
		source := engramMap["source"]

		// Source should now be a string (path), not an object
		if sourceStr, ok := source.(string); ok {
			if sourceStr != "/opt/workspace/repos/engram" {
				t.Errorf("Expected source to be path string, got: %v", sourceStr)
			}
		} else {
			t.Errorf("Source is not a string after fix, type: %T", source)
		}
	} else {
		t.Error("engram entry not found in fixed marketplace config")
	}
}

// TestRemoveNonExistentHooks tests removing missing hook references
func TestRemoveNonExistentHooks(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "engram-remove-hooks-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create settings directory
	settingsDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatalf("Failed to create settings dir: %v", err)
	}

	// Create settings with mix of existing and non-existent hooks
	hooksDir := filepath.Join(settingsDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("Failed to create hooks dir: %v", err)
	}

	// Create one hook that exists
	existingHook := filepath.Join(hooksDir, "existing-hook")
	if err := os.WriteFile(existingHook, []byte("#!/bin/bash\necho test"), 0755); err != nil {
		t.Fatalf("Failed to create existing hook: %v", err)
	}

	// Create settings with both existing and missing hooks
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"SessionStart": []interface{}{
				map[string]interface{}{
					"hooks": []interface{}{
						map[string]interface{}{
							"command": filepath.Join(hooksDir, "existing-hook"),
							"timeout": 10,
						},
						map[string]interface{}{
							"command": filepath.Join(hooksDir, "missing-hook"),
							"timeout": 10,
						},
					},
					"matcher": ".*",
				},
			},
		},
	}

	settingsPath := filepath.Join(settingsDir, "settings.json")
	settingsJSON, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(settingsPath, settingsJSON, 0644); err != nil {
		t.Fatalf("Failed to create test settings: %v", err)
	}

	// Set HOME to tmpDir
	t.Setenv("HOME", tmpDir)

	// Run the fix
	fixer := NewTier1Fixer(tmpDir)
	if err := fixer.removeNonExistentHooks(); err != nil {
		t.Fatalf("removeNonExistentHooks failed: %v", err)
	}

	// Read fixed settings
	fixedData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read fixed settings: %v", err)
	}

	// Parse fixed JSON
	var fixedSettings map[string]interface{}
	if err := json.Unmarshal(fixedData, &fixedSettings); err != nil {
		t.Fatalf("Fixed settings is not valid JSON: %v", err)
	}

	// Verify missing hook was removed
	fixedContent := string(fixedData)
	if strings.Contains(fixedContent, "missing-hook") {
		t.Error("Missing hook reference was not removed")
	}

	// Verify existing hook was kept
	if !strings.Contains(fixedContent, "existing-hook") {
		t.Error("Existing hook was incorrectly removed")
	}

	// Verify backup was created
	backupPath := settingsPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}
}

// TestFixExtensionsNoChanges verifies no modification when hooks already correct
func TestFixExtensionsNoChanges(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "engram-no-change-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Load valid settings (no issues)
	fixtureData, err := os.ReadFile("testdata/settings-valid.json")
	if err != nil {
		t.Fatalf("Failed to read fixture: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(settingsPath, fixtureData, 0644); err != nil {
		t.Fatalf("Failed to create test settings: %v", err)
	}

	// Run the fix
	fixer := NewTier1Fixer(tmpDir)
	if err := fixer.fixExtensionsInFile(settingsPath, fixtureData); err != nil {
		t.Fatalf("fixExtensionsInFile failed: %v", err)
	}

	// Verify no backup was created (no changes made)
	backupPath := settingsPath + ".bak"
	if _, err := os.Stat(backupPath); err == nil {
		t.Error("Backup file should not be created when no changes needed")
	}

	// Verify content unchanged
	afterData, _ := os.ReadFile(settingsPath)
	if string(afterData) != string(fixtureData) {
		t.Error("Settings file was modified when no changes were needed")
	}
}
