package claudetrust

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readProjects(t *testing.T, configPath string) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}
	var config struct {
		Projects map[string]map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse %s: %v", configPath, err)
	}
	return config.Projects
}

func TestSeedWorkspaceTrustCreatesEntry(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude.json")
	workDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	seeded, err := seedWorkspaceTrustAt(configPath, workDir)
	if err != nil {
		t.Fatalf("seedWorkspaceTrustAt() error = %v", err)
	}
	projects := readProjects(t, configPath)
	entry, ok := projects[seeded]
	if !ok {
		t.Fatalf("no project entry for %q; got %v", seeded, projects)
	}
	if entry["hasTrustDialogAccepted"] != true {
		t.Errorf("hasTrustDialogAccepted = %v, want true", entry["hasTrustDialogAccepted"])
	}
}

// This is the whole bug. AGM hands the harness a path under the sandbox's
// "merged" symlink, but Claude resolves its cwd before keying trust, so it looks
// up ".../upper/repo0". Seeding the symlinked path leaves the resolved one
// untrusted and the dialog appears on every spawn.
func TestSeedWorkspaceTrustKeysTheResolvedPath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude.json")
	upper := filepath.Join(dir, "upper", "repo0")
	if err := os.MkdirAll(upper, 0o755); err != nil {
		t.Fatal(err)
	}
	merged := filepath.Join(dir, "merged")
	if err := os.Symlink(filepath.Join(dir, "upper"), merged); err != nil {
		t.Fatal(err)
	}

	seeded, err := seedWorkspaceTrustAt(configPath, filepath.Join(merged, "repo0"))
	if err != nil {
		t.Fatalf("seedWorkspaceTrustAt() error = %v", err)
	}
	resolvedUpper, err := filepath.EvalSymlinks(upper)
	if err != nil {
		t.Fatal(err)
	}
	if seeded != resolvedUpper {
		t.Errorf("seeded %q, want the resolved path %q", seeded, resolvedUpper)
	}
	if _, ok := readProjects(t, configPath)[resolvedUpper]; !ok {
		t.Errorf("resolved path %q was not trusted", resolvedUpper)
	}
}

// ~/.claude.json holds every project's history and Claude's own account state.
// Seeding one flag must not drop any of it.
func TestSeedWorkspaceTrustPreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude.json")
	workDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"userID":"abc","numStartups":42,"projects":{"/other":{"hasTrustDialogAccepted":true,"lastCost":1.5}}}`
	if err := os.WriteFile(configPath, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := seedWorkspaceTrustAt(configPath, workDir); err != nil {
		t.Fatalf("seedWorkspaceTrustAt() error = %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["userID"] != "abc" {
		t.Errorf("userID = %v, want preserved", config["userID"])
	}
	if config["numStartups"] != float64(42) {
		t.Errorf("numStartups = %v, want preserved", config["numStartups"])
	}
	projects := readProjects(t, configPath)
	if projects["/other"]["lastCost"] != 1.5 {
		t.Errorf("unrelated project entry was not preserved: %v", projects["/other"])
	}
	if len(projects) != 2 {
		t.Errorf("projects = %v, want the existing one plus the seeded one", projects)
	}
}

// Re-seeding an already-trusted workspace is how every resume and respawn hits
// this path, so it has to be a no-op rather than an error.
func TestSeedWorkspaceTrustIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude.json")
	workDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := seedWorkspaceTrustAt(configPath, workDir)
	if err != nil {
		t.Fatalf("first seed error = %v", err)
	}
	second, err := seedWorkspaceTrustAt(configPath, workDir)
	if err != nil {
		t.Fatalf("second seed error = %v", err)
	}
	if first != second {
		t.Errorf("seeded paths differ across calls: %q then %q", first, second)
	}
	if got := len(readProjects(t, configPath)); got != 1 {
		t.Errorf("projects = %d, want 1 after re-seeding the same workspace", got)
	}
}

// Existing per-project state (permissions, MCP servers, history) belongs to the
// user; flipping trust must not reset it.
func TestSeedWorkspaceTrustPreservesExistingProjectFields(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude.json")
	workDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatal(err)
	}
	existing := map[string]any{"projects": map[string]any{
		resolved: map[string]any{"hasTrustDialogAccepted": false, "allowedTools": []any{"Read"}},
	}}
	data, err := json.Marshal(existing)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := seedWorkspaceTrustAt(configPath, workDir); err != nil {
		t.Fatalf("seedWorkspaceTrustAt() error = %v", err)
	}
	entry := readProjects(t, configPath)[resolved]
	if entry["hasTrustDialogAccepted"] != true {
		t.Errorf("hasTrustDialogAccepted = %v, want true", entry["hasTrustDialogAccepted"])
	}
	tools, ok := entry["allowedTools"].([]any)
	if !ok || len(tools) != 1 || tools[0] != "Read" {
		t.Errorf("allowedTools = %v, want preserved", entry["allowedTools"])
	}
}

// A workspace that does not exist cannot be the harness cwd, and seeding an
// unresolvable path would trust the wrong key. Fail loudly instead.
func TestSeedWorkspaceTrustRejectsMissingWorkspace(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude.json")
	if _, err := seedWorkspaceTrustAt(configPath, filepath.Join(dir, "absent")); err == nil {
		t.Fatal("seedWorkspaceTrustAt() accepted a workspace that does not exist")
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("a rejected seed still wrote the config file")
	}
}

// A corrupt config is the user's to repair; silently replacing it would discard
// their account state and every project's history.
func TestSeedWorkspaceTrustRefusesToOverwriteCorruptConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".claude.json")
	workDir := filepath.Join(dir, "repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := seedWorkspaceTrustAt(configPath, workDir); err == nil {
		t.Fatal("seedWorkspaceTrustAt() accepted a corrupt config")
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{not json" {
		t.Errorf("corrupt config was rewritten as %q", data)
	}
}
