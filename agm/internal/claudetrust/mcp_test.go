package claudetrust

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readSettingsFile(t *testing.T, workDir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workDir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return settings
}

// Seeding trust alone only moves the stall one dialog later: a project shipping
// a .mcp.json makes Claude ask which servers to enable, and that prompt owns
// input just as the trust dialog does.
func TestApproveProjectMcpServersWritesTheSetting(t *testing.T) {
	workDir := t.TempDir()
	if err := ApproveProjectMcpServers(workDir); err != nil {
		t.Fatalf("ApproveProjectMcpServers() error = %v", err)
	}
	if got := readSettingsFile(t, workDir)[mcpApprovalKey]; got != true {
		t.Errorf("%s = %v, want true", mcpApprovalKey, got)
	}
}

// AGM's permission pass writes this same file first, so the MCP answer has to
// merge into it rather than replace it.
func TestApproveProjectMcpServersPreservesExistingSettings(t *testing.T) {
	workDir := t.TempDir()
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"permissions":{"allow":["Bash(git status)"]},"model":"opus"}`
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ApproveProjectMcpServers(workDir); err != nil {
		t.Fatalf("ApproveProjectMcpServers() error = %v", err)
	}
	settings := readSettingsFile(t, workDir)
	if settings[mcpApprovalKey] != true {
		t.Errorf("%s = %v, want true", mcpApprovalKey, settings[mcpApprovalKey])
	}
	if settings["model"] != "opus" {
		t.Errorf("model = %v, want preserved", settings["model"])
	}
	permissions, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions = %v, want preserved", settings["permissions"])
	}
	allow, ok := permissions["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash(git status)" {
		t.Errorf("permissions.allow = %v, want preserved", permissions["allow"])
	}
}

func TestApproveProjectMcpServersIsIdempotent(t *testing.T) {
	workDir := t.TempDir()
	if err := ApproveProjectMcpServers(workDir); err != nil {
		t.Fatalf("first call error = %v", err)
	}
	if err := ApproveProjectMcpServers(workDir); err != nil {
		t.Fatalf("second call error = %v", err)
	}
	if got := len(readSettingsFile(t, workDir)); got != 1 {
		t.Errorf("settings keys = %d, want 1", got)
	}
}

// Malformed settings belong to whoever wrote them; replacing the file would
// discard the permission allowlist the session depends on.
func TestApproveProjectMcpServersRefusesCorruptSettings(t *testing.T) {
	workDir := t.TempDir()
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	if err := os.WriteFile(settingsPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApproveProjectMcpServers(workDir); err == nil {
		t.Fatal("ApproveProjectMcpServers() accepted corrupt settings")
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{not json" {
		t.Errorf("corrupt settings were rewritten as %q", data)
	}
}
