package fsguard

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDefaultPolicyAllowsBeads(t *testing.T) {
	t.Parallel()
	g := &Guard{Home: "/home/tester"}
	cwd := "/home/tester"

	// ~/beads is the canonical Beads tracker store; bd/dolt must be able to
	// write there (ce-05r). It is a default writable carve-out.
	for _, p := range []string{
		"~/beads/context-engine/.beads/issues.db",
		"/home/tester/beads/context-engine/x",
	} {
		if allowed, msg := g.Classify(p, cwd); !allowed {
			t.Errorf("Classify(%q) blocked, want allowed (msg=%q)", p, msg)
		}
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	// Not parallel: mutates process environment.
	clearFSGuardEnv(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Policy.Protected) == 0 {
		t.Fatal("default policy has no protected paths")
	}
	if filepath.Base(cfg.Policy.WorktreesDir) != "worktrees" {
		t.Errorf("WorktreesDir=%q, want .../worktrees", cfg.Policy.WorktreesDir)
	}
	if filepath.Base(cfg.LogPath) != "violations.jsonl" {
		t.Errorf("LogPath=%q, want .../violations.jsonl", cfg.LogPath)
	}
}

func TestLoadConfigEnvOverrides(t *testing.T) {
	clearFSGuardEnv(t)
	home, _ := os.UserHomeDir()

	t.Setenv(EnvProtected, "~/golden:/opt/ref")
	t.Setenv(EnvWritable, "/scratch,/cache")
	t.Setenv(EnvWorktreesDir, "/work")
	t.Setenv(EnvLog, "/var/log/fsguard.jsonl")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Policy.WorktreesDir != "/work" {
		t.Errorf("WorktreesDir=%q, want /work", cfg.Policy.WorktreesDir)
	}
	if cfg.LogPath != "/var/log/fsguard.jsonl" {
		t.Errorf("LogPath=%q", cfg.LogPath)
	}
	wantProtected := filepath.Join(home, "golden")
	if !contains(cfg.Policy.Protected, wantProtected) {
		t.Errorf("Protected=%v, want to contain %q", cfg.Policy.Protected, wantProtected)
	}
	if !contains(cfg.Policy.Protected, "/opt/ref") {
		t.Errorf("Protected=%v, want to contain /opt/ref", cfg.Policy.Protected)
	}
	if !contains(cfg.Policy.Writable, "/scratch") || !contains(cfg.Policy.Writable, "/cache") {
		t.Errorf("Writable=%v, want /scratch and /cache", cfg.Policy.Writable)
	}
}

func TestLoadConfigDisableLog(t *testing.T) {
	clearFSGuardEnv(t)
	t.Setenv(EnvDisableLog, "1")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LogPath != "" {
		t.Errorf("LogPath=%q, want empty when logging disabled", cfg.LogPath)
	}
}

func TestLoadConfigFile(t *testing.T) {
	clearFSGuardEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "fsguard.json")
	body := `{
		"worktrees_dir": "/work",
		"protected_paths": ["/opt/golden"],
		"writable_paths": ["/scratch"],
		"log_path": "/var/log/v.jsonl"
	}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfig, cfgPath)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Policy.WorktreesDir != "/work" {
		t.Errorf("WorktreesDir=%q", cfg.Policy.WorktreesDir)
	}
	if cfg.LogPath != "/var/log/v.jsonl" {
		t.Errorf("LogPath=%q", cfg.LogPath)
	}
	if !contains(cfg.Policy.Protected, "/opt/golden") {
		t.Errorf("Protected=%v missing /opt/golden", cfg.Policy.Protected)
	}
	if !contains(cfg.Policy.Writable, "/scratch") {
		t.Errorf("Writable=%v missing /scratch", cfg.Policy.Writable)
	}
}

func TestLoadConfigBadFileDegradesToDefaults(t *testing.T) {
	clearFSGuardEnv(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(cfgPath, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvConfig, cfgPath)

	cfg, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error from malformed config file")
	}
	// Critically, enforcement still has safe defaults — not an empty policy.
	if len(cfg.Policy.Protected) == 0 {
		t.Fatal("malformed config left an unguarded (empty) policy")
	}
}

// TestConfiguredProtectedPathBlocks proves a path added via config is enforced.
func TestConfiguredProtectedPathBlocks(t *testing.T) {
	t.Parallel()
	pol := DefaultPolicy("/home/tester")
	pol.Protected = append(pol.Protected, "/opt/golden")
	g := &Guard{Home: "/home/tester", policy: &pol}

	allowed, msg := g.Classify("/opt/golden/lib/x.go", "/home/tester")
	if allowed {
		t.Fatal("write under configured protected path was allowed")
	}
	if msg == "" {
		t.Fatal("blocked write returned empty message")
	}
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

// clearFSGuardEnv unsets every FSGUARD_* variable for the duration of a test so
// host configuration cannot leak into assertions. t.Setenv restores originals.
func clearFSGuardEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvConfig, EnvProtected, EnvWritable, EnvWorktreesDir, EnvLog, EnvDisableLog,
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}
