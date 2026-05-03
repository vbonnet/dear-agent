package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/rbac"
)

func TestPermissionProfilesExist(t *testing.T) {
	expectedProfiles := []string{"worker", "monitor", "auditor", "researcher", "verifier", "requester", "orchestrator", "overseer", "implementer", "meta-orchestrator"}
	for _, name := range expectedProfiles {
		if !rbac.ValidRole(name) {
			t.Errorf("expected profile %q to exist", name)
		}
	}
}

func TestPermissionProfilesNonEmpty(t *testing.T) {
	for _, name := range rbac.ProfileNames() {
		profile, err := rbac.LookupProfile(name)
		if err != nil {
			t.Errorf("LookupProfile(%q): %v", name, err)
			continue
		}
		if len(profile.AllowedTools) == 0 {
			t.Errorf("profile %q should have at least one permission entry", name)
		}
	}
}

func TestResolvePermissions_ExplicitOnly(t *testing.T) {
	explicit := []string{"Bash(tmux:*)", "Read(~/src/**)"}
	result, err := rbac.ResolvePermissions(rbac.ResolveOptions{Explicit: explicit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) < len(rbac.DefaultPermissions)+2 {
		t.Fatalf("expected at least %d permissions (defaults + explicit), got %d",
			len(rbac.DefaultPermissions)+2, len(result))
	}
	found := make(map[string]bool)
	for _, p := range result {
		found[p] = true
	}
	if !found["Bash(tmux:*)"] || !found["Read(~/src/**)"] {
		t.Errorf("explicit entries missing from result: %v", result)
	}
}

func TestResolvePermissions_ProfileOnly(t *testing.T) {
	result, err := rbac.ResolvePermissions(rbac.ResolveOptions{ProfileName: "auditor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	profile, _ := rbac.LookupProfile("auditor")
	found := make(map[string]bool)
	for _, p := range result {
		found[p] = true
	}
	for _, p := range profile.AllowedTools {
		if !found[p] {
			t.Errorf("auditor profile entry %q missing from result", p)
		}
	}
}

func TestResolvePermissions_InvalidProfile(t *testing.T) {
	_, err := rbac.ResolvePermissions(rbac.ResolveOptions{ProfileName: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for invalid profile, got nil")
	}
}

func TestResolvePermissions_Deduplication(t *testing.T) {
	explicit := []string{"Bash(git status)", "Bash(custom:*)", "Bash(custom:*)"}
	result, err := rbac.ResolvePermissions(rbac.ResolveOptions{Explicit: explicit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	for _, p := range result {
		if p == "Bash(custom:*)" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of Bash(custom:*) after dedup, got %d", count)
	}
	count = 0
	for _, p := range result {
		if p == "Bash(git status)" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of Bash(git status) after dedup, got %d", count)
	}
}

func TestResolvePermissions_ExplicitPlusProfile(t *testing.T) {
	explicit := []string{"Bash(custom:*)"}
	result, err := rbac.ResolvePermissions(rbac.ResolveOptions{
		Explicit:    explicit,
		ProfileName: "auditor",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := make(map[string]bool)
	for _, p := range result {
		found[p] = true
	}
	if !found["Bash(custom:*)"] {
		t.Error("explicit entry Bash(custom:*) missing from result")
	}
	profile, _ := rbac.LookupProfile("auditor")
	for _, p := range profile.AllowedTools {
		if !found[p] {
			t.Errorf("auditor profile entry %q missing from result", p)
		}
	}
}

func TestResolvePermissions_EmptyInputsGetsDefaults(t *testing.T) {
	result, err := rbac.ResolvePermissions(rbac.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != len(rbac.DefaultPermissions) {
		t.Errorf("expected %d default permissions, got %d: %v",
			len(rbac.DefaultPermissions), len(result), result)
	}
}

func TestConfigureProjectPermissions_CreatesSettingsFile(t *testing.T) {
	tmpDir := t.TempDir()
	allowList := []string{"Bash(tmux:*)", "Read(~/src/**)"}

	err := rbac.ConfigureProjectPermissions(tmpDir, allowList)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	settingsPath := filepath.Join(tmpDir, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings.local.json: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("failed to parse settings.local.json: %v", err)
	}

	perms, ok := settings["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("permissions key missing or wrong type")
	}

	allow, ok := perms["allow"].([]interface{})
	if !ok {
		t.Fatal("permissions.allow missing or wrong type")
	}

	if len(allow) != 2 {
		t.Fatalf("expected 2 allow entries, got %d", len(allow))
	}
	if allow[0].(string) != "Bash(tmux:*)" {
		t.Errorf("expected first entry to be 'Bash(tmux:*)', got %q", allow[0])
	}
	if allow[1].(string) != "Read(~/src/**)" {
		t.Errorf("expected second entry to be 'Read(~/src/**)', got %q", allow[1])
	}
}

func TestConfigureProjectPermissions_MergesWithExisting(t *testing.T) {
	tmpDir := t.TempDir()

	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	existing := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"Bash(existing:*)"},
		},
	}
	data, _ := json.Marshal(existing)
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), data, 0644)

	err := rbac.ConfigureProjectPermissions(tmpDir, []string{"Bash(new:*)"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := os.ReadFile(filepath.Join(claudeDir, "settings.local.json"))
	if err != nil {
		t.Fatalf("failed to read settings.local.json: %v", err)
	}

	var settings map[string]interface{}
	json.Unmarshal(result, &settings)
	perms := settings["permissions"].(map[string]interface{})
	allow := perms["allow"].([]interface{})

	if len(allow) != 2 {
		t.Fatalf("expected 2 allow entries (existing + new), got %d: %v", len(allow), allow)
	}
	if allow[0].(string) != "Bash(existing:*)" {
		t.Errorf("existing entry should be preserved, got %q", allow[0])
	}
	if allow[1].(string) != "Bash(new:*)" {
		t.Errorf("new entry should be appended, got %q", allow[1])
	}
}

func TestConfigureProjectPermissions_DeduplicatesWithExisting(t *testing.T) {
	tmpDir := t.TempDir()

	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	existing := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"Bash(tmux:*)"},
		},
	}
	data, _ := json.Marshal(existing)
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), data, 0644)

	err := rbac.ConfigureProjectPermissions(tmpDir, []string{"Bash(tmux:*)"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(claudeDir, "settings.local.json"))
	var settings map[string]interface{}
	json.Unmarshal(result, &settings)
	perms := settings["permissions"].(map[string]interface{})
	allow := perms["allow"].([]interface{})

	if len(allow) != 1 {
		t.Errorf("duplicate should be deduplicated, got %d entries: %v", len(allow), allow)
	}
}

func TestConfigureProjectPermissions_EmptyList(t *testing.T) {
	tmpDir := t.TempDir()

	err := rbac.ConfigureProjectPermissions(tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claudeDir := filepath.Join(tmpDir, ".claude")
	if _, err := os.Stat(claudeDir); err == nil {
		t.Error(".claude directory should not be created for empty allow list")
	}
}

func TestPermissionProfileFlagRegistered(t *testing.T) {
	flag := newCmd.Flags().Lookup("permission-profile")
	if flag == nil {
		t.Fatal("--permission-profile flag not registered on newCmd")
	}
	if flag.DefValue != "" {
		t.Errorf("--permission-profile default should be empty string, got %q", flag.DefValue)
	}
}

func TestPermissionsAllowFlagRegistered(t *testing.T) {
	flag := newCmd.Flags().Lookup("permissions-allow")
	if flag == nil {
		t.Fatal("--permissions-allow flag not registered on newCmd")
	}
	if flag.Value.Type() != "stringSlice" {
		t.Errorf("--permissions-allow should be stringSlice type, got %q", flag.Value.Type())
	}
}

func TestInheritPermissionsFlagRegistered(t *testing.T) {
	flag := newCmd.Flags().Lookup("inherit-permissions")
	if flag == nil {
		t.Fatal("--inherit-permissions flag not registered on newCmd")
	}
	if flag.DefValue != "false" {
		t.Errorf("--inherit-permissions default should be false, got %q", flag.DefValue)
	}
}

func TestReadParentPermissions_NoFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	result, err := rbac.ReadParentPermissions()
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for missing settings, got %v", result)
	}
}

func TestReadParentPermissions_WithPermissions(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	os.MkdirAll(claudeDir, 0755)
	settings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"Bash(git:*)", "Read(~/src/**)"},
		},
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0600)

	result, err := rbac.ReadParentPermissions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 parent permissions, got %d: %v", len(result), result)
	}
	if result[0] != "Bash(git:*)" || result[1] != "Read(~/src/**)" {
		t.Errorf("unexpected parent permissions: %v", result)
	}
}

func TestResolvePermissions_InheritFromParent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	claudeDir := filepath.Join(tmpHome, ".claude")
	os.MkdirAll(claudeDir, 0755)
	settings := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"Bash(parent:*)"},
		},
	}
	data, _ := json.Marshal(settings)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0600)

	result, err := rbac.ResolvePermissions(rbac.ResolveOptions{
		Explicit:      []string{"Bash(child:*)"},
		InheritParent: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := make(map[string]bool)
	for _, p := range result {
		found[p] = true
	}
	if !found["Bash(child:*)"] {
		t.Error("child entry missing from result")
	}
	if !found["Bash(parent:*)"] {
		t.Error("parent entry missing from result")
	}
}

func TestPermissionProfileValidation(t *testing.T) {
	invalidProfiles := []string{"invalid", "root", "admin"}
	for _, profile := range invalidProfiles {
		if rbac.ValidRole(profile) {
			t.Errorf("profile %q should not exist", profile)
		}
	}
}

func TestConfigureProjectPermissions_PreservesOtherSettings(t *testing.T) {
	tmpDir := t.TempDir()

	claudeDir := filepath.Join(tmpDir, ".claude")
	os.MkdirAll(claudeDir, 0755)
	existing := map[string]interface{}{
		"model": "sonnet",
		"permissions": map[string]interface{}{
			"deny": []interface{}{"Bash(rm:*)"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), data, 0644)

	err := rbac.ConfigureProjectPermissions(tmpDir, []string{"Bash(git:*)"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(claudeDir, "settings.local.json"))
	var settings map[string]interface{}
	json.Unmarshal(result, &settings)

	if settings["model"] != "sonnet" {
		t.Errorf("model field should be preserved, got %v", settings["model"])
	}

	perms := settings["permissions"].(map[string]interface{})
	deny, ok := perms["deny"].([]interface{})
	if !ok || len(deny) != 1 {
		t.Errorf("deny list should be preserved, got %v", perms["deny"])
	}

	allow := perms["allow"].([]interface{})
	if len(allow) != 1 || allow[0].(string) != "Bash(git:*)" {
		t.Errorf("allow list should contain new entry, got %v", allow)
	}
}

func TestDefaultPermissionsIncludesSafeGitCommands(t *testing.T) {
	requiredPatterns := []string{
		"Bash(git status)",
		"Bash(git status *)",
		"Bash(git -C * status *)",
		"Bash(git log *)",
		"Bash(git -C * log *)",
		"Bash(git branch *)",
		"Bash(git -C * branch *)",
		"Bash(git diff *)",
		"Bash(git -C * diff *)",
		"Bash(git show *)",
		"Bash(git -C * show *)",
		"Bash(git rev-parse *)",
		"Bash(git -C * rev-parse *)",
		"Bash(git worktree list *)",
		"Bash(git -C * worktree list *)",
	}
	defaultSet := make(map[string]bool)
	for _, p := range defaultPermissions {
		defaultSet[p] = true
	}
	for _, req := range requiredPatterns {
		if !defaultSet[req] {
			t.Errorf("defaultPermissions missing required pattern: %q", req)
		}
	}
}

func TestDefaultPermissionsIncludesAgmAndTooling(t *testing.T) {
	requiredPatterns := []string{
		"Bash(agm *)",
		"Bash(agm session *)",
		"Bash(go version *)",
		"Bash(go env *)",
		"Bash(chmod +x /tmp/*)",
	}
	defaultSet := make(map[string]bool)
	for _, p := range defaultPermissions {
		defaultSet[p] = true
	}
	for _, req := range requiredPatterns {
		if !defaultSet[req] {
			t.Errorf("defaultPermissions missing required pattern: %q", req)
		}
	}
}

func TestResolvePermissions_DefaultsAlwaysPresent(t *testing.T) {
	result, err := rbac.ResolvePermissions(rbac.ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resultSet := make(map[string]bool)
	for _, p := range result {
		resultSet[p] = true
	}
	for _, dp := range defaultPermissions {
		if !resultSet[dp] {
			t.Errorf("default permission %q missing from resolve result with no flags", dp)
		}
	}
}
