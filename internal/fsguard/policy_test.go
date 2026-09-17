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
	wantSandboxWorkspace := filepath.Join(filepath.Dir(cfg.Policy.WorktreesDir), ".agm", "sandboxes")
	if got := cfg.Policy.SandboxWorkspace; got != wantSandboxWorkspace {
		t.Errorf("SandboxWorkspace=%q, want compatibility default %q", got, wantSandboxWorkspace)
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
	t.Setenv(EnvSandboxWorkspace, "/srv/agm/sandboxes/session-a")
	t.Setenv(EnvLog, "/var/log/fsguard.jsonl")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Policy.WorktreesDir != "/work" {
		t.Errorf("WorktreesDir=%q, want /work", cfg.Policy.WorktreesDir)
	}
	if cfg.Policy.SandboxWorkspace != "/srv/agm/sandboxes/session-a" {
		t.Errorf("SandboxWorkspace=%q, want /srv/agm/sandboxes/session-a", cfg.Policy.SandboxWorkspace)
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

func TestConfiguredSandboxWorkspaceReplacesParentWideDefault(t *testing.T) {
	clearFSGuardEnv(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvSandboxWorkspace, "/srv/agm/sandboxes/session-a")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	g := &Guard{Home: home, policy: &cfg.Policy}

	tests := []struct {
		path string
		want bool
	}{
		{"/srv/agm/sandboxes/session-a", true},
		{"/srv/agm/sandboxes/session-a/upper/file", true},
		{"/srv/agm/sandboxes/session-b/file", false},
		{"/srv/agm/sandboxes/session-a-sibling/file", false},
		{"/srv/agm/sandboxes/session-a/../session-b/file", false},
		{filepath.Join(home, ".agm", "sandboxes", "legacy", "file"), false},
	}
	for _, tt := range tests {
		allowed, _ := g.Classify(tt.path, home)
		if allowed != tt.want {
			t.Errorf("Classify(%q) allowed=%v, want %v", tt.path, allowed, tt.want)
		}
	}
}

func TestExactSandboxWorkspaceShadowsGenericWritableParents(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tempRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, parent := range []string{
		filepath.Join(home, "worktrees", "centralized-sandboxes"),
		filepath.Join(tempRoot, "centralized-sandboxes"),
	} {
		t.Run(parent, func(t *testing.T) {
			workspace := filepath.Join(parent, "session-a")
			sibling := filepath.Join(parent, "session-b")
			for _, dir := range []string{workspace, sibling} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			policy := DefaultPolicy(home)
			policy.SandboxWorkspace = workspace
			guard := &Guard{Home: home, policy: &policy, resolveSymlinks: true}

			if allowed, message := guard.Classify(filepath.Join(workspace, "file"), home); !allowed {
				t.Fatalf("exact workspace under generic writable parent was blocked: %s", message)
			}
			if allowed, _ := guard.Classify(filepath.Join(sibling, "file"), home); allowed {
				t.Fatalf("sibling workspace under generic writable parent %q was allowed", parent)
			}
		})
	}
}

func TestInvalidConfiguredSandboxWorkspaceFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		root string
	}{
		{"empty", ""},
		{"relative", "relative/sandboxes"},
		{"unclean parent", "/srv/agm/../sandboxes"},
		{"unclean trailing separator", "/srv/agm/sandboxes/"},
		{"filesystem root", string(filepath.Separator)},
		{"control character", "/srv/agm/sandboxes\nother"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearFSGuardEnv(t)
			t.Setenv(EnvSandboxWorkspace, tt.root)

			cfg, err := LoadConfig()
			if err == nil {
				t.Fatal("LoadConfig returned nil error for invalid sandbox workspace")
			}
			if cfg.Policy.SandboxWorkspace != "" {
				t.Fatalf("SandboxWorkspace=%q, want empty fail-closed allowance", cfg.Policy.SandboxWorkspace)
			}
			g := &Guard{Home: "/home/tester", policy: &cfg.Policy}
			if allowed, _ := g.Classify("/home/tester/.agm/sandboxes/session/file", "/home/tester"); allowed {
				t.Fatal("invalid configured sandbox workspace retained the default sandbox allowance")
			}
		})
	}
}

func TestInvalidSandboxWorkspaceDoesNotRemoveExplicitWritablePaths(t *testing.T) {
	clearFSGuardEnv(t)
	t.Setenv(EnvSandboxWorkspace, "/")
	t.Setenv(EnvWritable, "/explicit/scratch")

	cfg, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig returned nil error for invalid sandbox workspace")
	}
	g := &Guard{Home: "/home/tester", policy: &cfg.Policy}
	if allowed, msg := g.Classify("/explicit/scratch/file", "/home/tester"); !allowed {
		t.Fatalf("explicit writable path was removed by invalid sandbox workspace: %s", msg)
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

func TestExpandHomeEmpty(t *testing.T) {
	t.Parallel()
	// A blank entry must not resolve to "." (the cwd); it stays empty so it can
	// never accidentally protect or allow the working directory.
	if got := expandHome("", "/home/tester"); got != "" {
		t.Errorf("expandHome(\"\") = %q, want empty", got)
	}
	if got := expandHome("~/x", "/home/tester"); got != "/home/tester/x" {
		t.Errorf("expandHome(~/x) = %q", got)
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
		EnvConfig, EnvProtected, EnvWritable, EnvWorktreesDir, EnvSandboxWorkspace, EnvLog, EnvDisableLog,
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}
