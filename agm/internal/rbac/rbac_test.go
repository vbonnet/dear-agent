package rbac

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllRolesHaveProfiles(t *testing.T) {
	for _, role := range allRoles {
		if _, ok := profiles[role]; !ok {
			t.Errorf("role %q has no profile defined", role)
		}
	}
}

func TestLookupProfile_Valid(t *testing.T) {
	for _, role := range allRoles {
		p, err := LookupProfile(string(role))
		if err != nil {
			t.Errorf("LookupProfile(%q): %v", role, err)
			continue
		}
		if p.Name != role {
			t.Errorf("profile.Name = %q, want %q", p.Name, role)
		}
		if len(p.AllowedTools) == 0 {
			t.Errorf("profile %q has empty AllowedTools", role)
		}
		if p.Description == "" {
			t.Errorf("profile %q has empty Description", role)
		}
		if p.TrustLevel < TrustSandboxed || p.TrustLevel > TrustTrusted {
			t.Errorf("profile %q has invalid TrustLevel %d", role, p.TrustLevel)
		}
	}
}

func TestLookupProfile_Invalid(t *testing.T) {
	_, err := LookupProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestValidRole(t *testing.T) {
	if !ValidRole("worker") {
		t.Error("worker should be valid")
	}
	if ValidRole("nonexistent") {
		t.Error("nonexistent should not be valid")
	}
}

func TestAllRoleNames(t *testing.T) {
	names := AllRoleNames()
	if len(names) != len(allRoles) {
		t.Errorf("AllRoleNames() returned %d names, want %d", len(names), len(allRoles))
	}
}

func TestProfileNames(t *testing.T) {
	names := ProfileNames()
	if len(names) == 0 {
		t.Fatal("ProfileNames() returned empty")
	}
	// Every profile name should be a valid role
	for _, name := range names {
		if !ValidRole(name) {
			t.Errorf("ProfileNames() returned %q which is not a valid role", name)
		}
	}
}

func TestResolvePermissions_DefaultsOnly(t *testing.T) {
	result, err := ResolvePermissions(ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != len(DefaultPermissions) {
		t.Errorf("got %d permissions, want %d", len(result), len(DefaultPermissions))
	}
}

func TestResolvePermissions_WithProfile(t *testing.T) {
	result, err := ResolvePermissions(ResolveOptions{ProfileName: "worker"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) <= len(DefaultPermissions) {
		t.Error("expected more permissions than defaults when using worker profile")
	}
	// Check that worker-specific tools are present
	found := make(map[string]bool)
	for _, p := range result {
		found[p] = true
	}
	if !found["Edit(~/src/**)"] {
		t.Error("worker profile should include Edit(~/src/**)")
	}
}

func TestResolvePermissions_Deduplication(t *testing.T) {
	result, err := ResolvePermissions(ResolveOptions{
		Explicit: []string{"Bash(git status)", "Bash(dup:*)", "Bash(dup:*)"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	for _, p := range result {
		if p == "Bash(dup:*)" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of Bash(dup:*), got %d", count)
	}
}

func TestResolvePermissions_InvalidProfile(t *testing.T) {
	_, err := ResolvePermissions(ResolveOptions{ProfileName: "bogus"})
	if err == nil {
		t.Fatal("expected error for invalid profile")
	}
}

func TestConfigureProjectPermissions_Creates(t *testing.T) {
	tmpDir := t.TempDir()
	err := ConfigureProjectPermissions(tmpDir, []string{"Bash(test:*)"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("settings.local.json not created: %v", err)
	}

	var settings map[string]interface{}
	json.Unmarshal(data, &settings)
	perms := settings["permissions"].(map[string]interface{})
	allow := perms["allow"].([]interface{})

	if len(allow) != 1 || allow[0].(string) != "Bash(test:*)" {
		t.Errorf("unexpected allow list: %v", allow)
	}
}

func TestConfigureProjectPermissions_Merges(t *testing.T) {
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

	err := ConfigureProjectPermissions(tmpDir, []string{"Bash(new:*)"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, _ := os.ReadFile(filepath.Join(claudeDir, "settings.local.json"))
	var settings map[string]interface{}
	json.Unmarshal(result, &settings)
	perms := settings["permissions"].(map[string]interface{})
	allow := perms["allow"].([]interface{})

	if len(allow) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(allow))
	}
}

func TestConfigureProjectPermissions_EmptyNoOp(t *testing.T) {
	tmpDir := t.TempDir()
	err := ConfigureProjectPermissions(tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, ".claude")); err == nil {
		t.Error(".claude dir should not be created for empty list")
	}
}

func TestTrustLevels(t *testing.T) {
	// Orchestrator and overseer should be trusted (level 4)
	p, _ := LookupProfile("orchestrator")
	if p.TrustLevel != TrustTrusted {
		t.Errorf("orchestrator trust level = %d, want %d", p.TrustLevel, TrustTrusted)
	}

	// Verifier should be sandboxed (level 1)
	p, _ = LookupProfile("verifier")
	if p.TrustLevel != TrustSandboxed {
		t.Errorf("verifier trust level = %d, want %d", p.TrustLevel, TrustSandboxed)
	}

	// Worker should be standard (level 2)
	p, _ = LookupProfile("worker")
	if p.TrustLevel != TrustStandard {
		t.Errorf("worker trust level = %d, want %d", p.TrustLevel, TrustStandard)
	}
}

func TestFlatten(t *testing.T) {
	result := flatten([]string{"a", "b"}, []string{"c"}, nil, []string{"d"})
	if len(result) != 4 {
		t.Errorf("flatten returned %d items, want 4", len(result))
	}
}

func TestRoleConstants(t *testing.T) {
	// Sanity check that constants match expected strings
	if string(RoleWorker) != "worker" {
		t.Errorf("RoleWorker = %q", RoleWorker)
	}
	if string(RoleOrchestrator) != "orchestrator" {
		t.Errorf("RoleOrchestrator = %q", RoleOrchestrator)
	}
	if string(RoleMetaOrchestrator) != "meta-orchestrator" {
		t.Errorf("RoleMetaOrchestrator = %q", RoleMetaOrchestrator)
	}
}

func TestPermissionProfiles_Required(t *testing.T) {
	tests := map[string]struct {
		role     string
		required []string
	}{
		"worker": {
			role: "worker",
			required: []string{
				"Edit(~/src/**)",     // Code write access
				"Bash(git:*)",        // Git access
				"Bash(go:*)",         // Go build tools
			},
		},
		"researcher": {
			role: "researcher",
			required: []string{
				"WebSearch(*)",  // Web search
				"WebFetch(*)",   // Web fetch
				"Bash(git:*)",   // Git access
				"Edit(~/src/**)", // Code write access
			},
		},
		"orchestrator": {
			role: "orchestrator",
			required: []string{
				"Bash(tmux:*)",                      // Tmux wildcard
				"Bash(agm:*)",                       // AGM wildcard
				"Bash(git:*)",                       // Git wildcard
				"Bash(agm session list *)",           // Session listing
				"Bash(agm session archive *)",        // Session archival
				"Bash(agm session gc *)",             // Session garbage collection
				"Bash(agm session health *)",         // Session health
				"Bash(agm session summary *)",        // Session summary
				"Bash(agm session tag *)",            // Session tagging
				"Bash(agm session select-option *)",  // Interactive selection
				"Bash(agm send *)",                   // Messaging
				"Bash(agm send msg *)",               // Explicit msg subcommand
				"Bash(agm verify *)",                 // Verification
				"Bash(agm trust score *)",            // Trust scoring
				"Bash(agm trust record *)",           // Trust recording
				"Bash(agm metrics *)",                // Metrics
				"Bash(agm dashboard *)",              // Dashboard
				"Bash(agm scan *)",                   // Scanning
				"Bash(agm escape-ui *)",              // UI escape
				"Bash(tmux capture-pane *)",           // Pane monitoring
				"Bash(tmux list-sessions *)",          // Session listing
				"Bash(tmux send-keys *)",              // Key sending
				"Bash(git -C *)",                     // Cross-repo git
			},
		},
		"supervisor": {
			role: "supervisor",
			required: []string{
				"Bash(git:*)",                       // Git wildcard
				"Bash(tmux:*)",                      // Tmux wildcard
				"Bash(agm:*)",                       // AGM wildcard
				"Read(~/.agm/**)",                   // .agm/ read
				"Write(~/.agm/**)",                  // .agm/ write
				"Read(docs/**)",                     // docs/ read
				"Bash(tmux capture-pane *)",          // Pane monitoring
				"Bash(agm session list *)",           // Session listing
			},
		},
		"auditor": {
			role: "auditor",
			required: []string{
				"Bash(git log *)",   // Git log
				"Bash(git diff *)",  // Git diff
			},
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			p, err := LookupProfile(tc.role)
			if err != nil {
				t.Fatalf("failed to lookup role %q: %v", tc.role, err)
			}

			// Convert allowed tools to a set for quick lookup
			allowed := make(map[string]bool)
			for _, tool := range p.AllowedTools {
				allowed[tool] = true
			}

			// Check that all required permissions are present
			for _, req := range tc.required {
				if !allowed[req] {
					t.Errorf("role %q missing required permission %q", tc.role, req)
				}
			}
		})
	}
}

func TestOrchestratorTierRoles_ShareAllowlists(t *testing.T) {
	// All orchestrator-tier roles must include the shared orchestratorAGM,
	// orchestratorTmux, and orchestratorGit patterns.
	orchestratorTierRoles := []string{"meta-orchestrator", "orchestrator", "overseer", "supervisor"}

	sharedPatterns := flatten(orchestratorAGM, orchestratorTmux, orchestratorGit)

	for _, roleName := range orchestratorTierRoles {
		t.Run(roleName, func(t *testing.T) {
			p, err := LookupProfile(roleName)
			if err != nil {
				t.Fatalf("LookupProfile(%q): %v", roleName, err)
			}

			allowed := make(map[string]bool)
			for _, tool := range p.AllowedTools {
				allowed[tool] = true
			}

			for _, pattern := range sharedPatterns {
				if !allowed[pattern] {
					t.Errorf("role %q missing shared orchestrator pattern %q", roleName, pattern)
				}
			}
		})
	}
}

func TestOrchestratorProfile_NoCodeWriteAccess(t *testing.T) {
	// Orchestrator should have read-only source code access, not write.
	// Write/Edit to ~/.agm/ is allowed for session management.
	p, _ := LookupProfile("orchestrator")

	for _, tool := range p.AllowedTools {
		if strings.HasPrefix(tool, "Write(~/src/") {
			t.Errorf("orchestrator should not have source code Write permissions, got %q", tool)
		}
		if strings.HasPrefix(tool, "Edit(~/src/") {
			t.Errorf("orchestrator should not have source code Edit permissions, got %q", tool)
		}
	}
}

func TestSupervisorProfile_HasRequiredPermissions(t *testing.T) {
	p, err := LookupProfile("supervisor")
	if err != nil {
		t.Fatalf("failed to lookup supervisor profile: %v", err)
	}

	allowed := make(map[string]bool)
	for _, tool := range p.AllowedTools {
		allowed[tool] = true
	}

	required := []string{
		// Git operations
		"Bash(git:*)",
		"Bash(git -C *)",
		"Bash(git -C * log *)",
		"Bash(git -C * diff *)",
		"Bash(git -C * status *)",
		"Bash(git log:*)",
		"Bash(git diff:*)",
		"Bash(git status:*)",
		// docs/ read
		"Read(docs/**)",
		"Read(*/docs/**)",
		"Glob(docs/**)",
		"Grep(docs/**)",
		// .agm/ read/write
		"Read(~/.agm/**)",
		"Write(~/.agm/**)",
		"Edit(~/.agm/**)",
		"Glob(~/.agm/**)",
		"Grep(~/.agm/**)",
		// tmux commands
		"Bash(tmux:*)",
		"Bash(tmux capture-pane *)",
		"Bash(tmux list-sessions *)",
		"Bash(tmux send-keys *)",
		// agm CLI commands
		"Bash(agm:*)",
		"Skill(agm:*)",
		"Bash(agm session list *)",
		"Bash(agm send *)",
		"Bash(agm verify *)",
	}

	for _, req := range required {
		if !allowed[req] {
			t.Errorf("supervisor profile missing required permission %q", req)
		}
	}
}

func TestSupervisorProfile_TrustLevel(t *testing.T) {
	p, err := LookupProfile("supervisor")
	if err != nil {
		t.Fatalf("failed to lookup supervisor profile: %v", err)
	}
	if p.TrustLevel != TrustTrusted {
		t.Errorf("supervisor trust level = %d, want %d (TrustTrusted)", p.TrustLevel, TrustTrusted)
	}
}

func TestIsSupervisorRole(t *testing.T) {
	supervisorRoles := []string{"meta-orchestrator", "orchestrator", "overseer", "supervisor"}
	for _, role := range supervisorRoles {
		if !IsSupervisorRole(role) {
			t.Errorf("IsSupervisorRole(%q) = false, want true", role)
		}
	}

	nonSupervisor := []string{"worker", "implementer", "researcher", "verifier", "auditor", "monitor"}
	for _, role := range nonSupervisor {
		if IsSupervisorRole(role) {
			t.Errorf("IsSupervisorRole(%q) = true, want false", role)
		}
	}
}

func TestSupervisorTierRoles_HaveAGMFileAccess(t *testing.T) {
	supervisorRoles := []string{"meta-orchestrator", "orchestrator", "overseer", "supervisor"}

	for _, roleName := range supervisorRoles {
		t.Run(roleName, func(t *testing.T) {
			p, err := LookupProfile(roleName)
			if err != nil {
				t.Fatalf("LookupProfile(%q): %v", roleName, err)
			}

			allowed := make(map[string]bool)
			for _, tool := range p.AllowedTools {
				allowed[tool] = true
			}

			for _, pattern := range agmFileAccess {
				if !allowed[pattern] {
					t.Errorf("role %q missing .agm/ file access pattern %q", roleName, pattern)
				}
			}
			for _, pattern := range docsReadAccess {
				if !allowed[pattern] {
					t.Errorf("role %q missing docs/ read access pattern %q", roleName, pattern)
				}
			}
		})
	}
}

func TestPermissionProfiles_Auditor_ReadOnly(t *testing.T) {
	p, _ := LookupProfile("auditor")

	// Auditor should NOT have Write or Edit tools
	hasWrite := false
	hasEdit := false
	for _, tool := range p.AllowedTools {
		if strings.HasPrefix(tool, "Write(") {
			hasWrite = true
		}
		if strings.HasPrefix(tool, "Edit(") {
			hasEdit = true
		}
	}

	if hasWrite {
		t.Error("auditor should not have Write permissions")
	}
	if hasEdit {
		t.Error("auditor should not have Edit permissions")
	}
}
