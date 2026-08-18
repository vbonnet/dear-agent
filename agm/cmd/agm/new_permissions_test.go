package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/session"
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
		return
	}
	if flag.DefValue != "" {
		t.Errorf("--permission-profile default should be empty string, got %q", flag.DefValue)
	}
}

func TestPermissionsAllowFlagRegistered(t *testing.T) {
	flag := newCmd.Flags().Lookup("permissions-allow")
	if flag == nil {
		t.Fatal("--permissions-allow flag not registered on newCmd")
		return
	}
	if flag.Value.Type() != "stringSlice" {
		t.Errorf("--permissions-allow should be stringSlice type, got %q", flag.Value.Type())
	}
	if !strings.Contains(flag.Usage, "shared policy") || strings.Contains(flag.Usage, "written to project .claude") {
		t.Errorf("--permissions-allow usage is not harness-neutral: %q", flag.Usage)
	}
}

func TestInheritPermissionsFlagRegistered(t *testing.T) {
	flag := newCmd.Flags().Lookup("inherit-permissions")
	if flag == nil {
		t.Fatal("--inherit-permissions flag not registered on newCmd")
		return
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

// TestVROOMRoleAutoDerivesProfile validates the invariant that every VROOM role
// name is also a valid permission profile — the precondition for the auto-derive
// logic in configureProjectPermissions: when --role names a known role and
// --permission-profile is empty, the role is used as the profile.
func TestVROOMRoleAutoDerivesProfile(t *testing.T) {
	t.Parallel()
	vroomRoles := []string{
		"meta-orchestrator",
		"orchestrator",
		"overseer",
		"worker",
		"implementer",
		"researcher",
		"verifier",
		"requester",
		"auditor",
		"monitor",
	}
	for _, role := range vroomRoles {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			if !rbac.ValidRole(role) {
				t.Errorf("role %q is not a valid profile — auto-derive would silently skip it", role)
			}
			result, err := rbac.ResolvePermissions(rbac.ResolveOptions{ProfileName: role})
			if err != nil {
				t.Errorf("ResolvePermissions(role=%q): %v", role, err)
			}
			if len(result) == 0 {
				t.Errorf("role %q resolved to empty permission list", role)
			}
		})
	}
}

func TestResolveSessionPermissionPolicyRecordsProfileAndTargets(t *testing.T) {
	origAllow := permissionsAllow
	origProfile := permissionProfile
	origInherit := inheritPermissions
	origRole := roleName
	defer func() {
		permissionsAllow = origAllow
		permissionProfile = origProfile
		inheritPermissions = origInherit
		roleName = origRole
	}()

	permissionsAllow = []string{"Bash(custom *)"}
	permissionProfile = ""
	inheritPermissions = false
	roleName = "worker"

	policy, allowList, err := resolveSessionPermissionPolicy()
	if err != nil {
		t.Fatalf("resolveSessionPermissionPolicy: %v", err)
	}
	if policy.Profile != "worker" {
		t.Fatalf("policy profile = %q, want worker", policy.Profile)
	}
	if policy.ProfileSource != "role" {
		t.Fatalf("policy profile source = %q, want role", policy.ProfileSource)
	}
	for _, source := range []string{"defaults", "explicit", "profile"} {
		if !slices.Contains(policy.Sources, source) {
			t.Errorf("policy sources missing %q: %v", source, policy.Sources)
		}
	}
	if !slices.Contains(policy.Explicit, "Bash(custom *)") {
		t.Errorf("policy explicit entries missing custom permission: %v", policy.Explicit)
	}
	if !slices.Contains(allowList, "Bash(custom *)") {
		t.Errorf("allow list missing custom permission: %v", allowList)
	}
	if len(policy.Targets) != len(agent.ActiveHarnesses()) {
		t.Fatalf("policy targets = %d, want %d: %v", len(policy.Targets), len(agent.ActiveHarnesses()), policy.Targets)
	}
	targets := map[string]bool{}
	for _, target := range policy.Targets {
		targets[target.Harness] = target.PolicySurface != "" &&
			target.StartupSurface != "" &&
			target.RuntimeSurface != "" &&
			target.NativeEnforcement != ""
	}
	for _, harness := range agent.ActiveHarnesses() {
		if !targets[harness] {
			t.Errorf("policy target for %q missing or incomplete: %v", harness, policy.Targets)
		}
	}
}

func TestBuildSessionManifestPersistsPermissionPolicy(t *testing.T) {
	policy := &manifest.PermissionPolicy{
		Profile: "auditor",
		Sources: []string{"defaults", "profile"},
		Allow:   []string{"Bash(git status)"},
		Targets: []manifest.PermissionPolicyTarget{{
			Harness:           "claude-code",
			PolicySurface:     ".claude/settings.local.json permissions.allow",
			StartupSurface:    "claude --permission-mode",
			RuntimeSurface:    "Shift+Tab and /plan",
			NativeEnforcement: "Claude Code allowlist and permission modes",
		}},
	}
	m := createPermissionManifest(t, "claude-code", "sonnet", "", policy)
	if m.PermissionPolicy == nil {
		t.Fatal("manifest missing permission policy")
	}
	if m.PermissionPolicy.Profile != "auditor" {
		t.Fatalf("manifest permission profile = %q, want auditor", m.PermissionPolicy.Profile)
	}

	policy.Allow[0] = "Bash(mutated)"
	if m.PermissionPolicy.Allow[0] != "Bash(git status)" {
		t.Fatalf("manifest permission policy was not cloned: %v", m.PermissionPolicy.Allow)
	}
}

func TestBuildSessionManifestPersistsStartupPermissionMode(t *testing.T) {
	m := createPermissionManifest(t, "agy", "3.5-flash", "auto", nil)
	if m.Agy == nil || m.Agy.ConversationID != "test-native-id" {
		t.Fatalf("AGY identity = %+v, want test-native-id", m.Agy)
	}
	if m.PermissionMode != "auto" {
		t.Fatalf("permission mode = %q, want auto", m.PermissionMode)
	}
	if m.PermissionModeSource != "startup" {
		t.Fatalf("permission mode source = %q, want startup", m.PermissionModeSource)
	}
	if m.PermissionModeUpdatedAt == nil {
		t.Fatal("permission mode updated timestamp was not set")
	}
}

func createPermissionManifest(t *testing.T, harness, model, permissionMode string, policy *manifest.PermissionPolicy) *manifest.Manifest {
	t.Helper()
	var got *manifest.Manifest
	runtime := &cliCreateSessionRuntime{
		launch: func(context.Context, ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
			return ops.CreateSessionLaunchResult{}, nil
		},
		bootstrapAgyIdentity: func(context.Context, ops.AgyCreateIdentityBootstrap) error { return nil },
		complete: func(_ context.Context, completion ops.CreateSessionCompletion) error {
			got = completion.Manifest
			return nil
		},
	}
	opCtx := &ops.OpContext{
		Tmux: session.NewMockTmux(), Storage: dolt.NewMockAdapter(), CreationRuntime: runtime,
	}
	if agent.NormalizeHarnessName(harness) == "agy" {
		opCtx.AgyCreateIdentityTracker = permissionManifestAgyIdentityTracker{}
	}
	_, err := ops.CreateSessionWithContext(context.Background(), opCtx, &ops.CreateSessionRequest{
		Cwd:                    t.TempDir(),
		Prompt:                 "fixture startup prompt",
		Title:                  "session-name",
		Model:                  model,
		Harness:                harness,
		SessionID:              "session-id",
		Caller:                 ops.CreateSessionCaller{Surface: ops.CreateSurfaceCLI},
		PermissionMode:         permissionMode,
		AllowEmptyPrompt:       true,
		SkipCodexRemoteControl: true,
		Metadata: ops.CreateSessionMetadata{
			Workspace:        "test",
			PermissionPolicy: policy,
			PermissionMode:   permissionMode,
		},
	})
	if err != nil {
		t.Fatalf("CreateSessionWithContext: %v", err)
	}
	if got == nil {
		t.Fatal("creation lifecycle did not expose a manifest")
	}
	return got
}

type permissionManifestAgyIdentityTracker struct{}

func (permissionManifestAgyIdentityTracker) Snapshot(context.Context, string) (string, error) {
	return "", nil
}

func (permissionManifestAgyIdentityTracker) Discover(_ context.Context, workDir, _ string) (*agysession.Metadata, error) {
	return &agysession.Metadata{
		ConversationID: "test-native-id",
		WorkspacePath:  workDir,
	}, nil
}
