package codexhooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestLaunchConfigOverridesPinsManifestAndDisablesProjectHooks(t *testing.T) {
	hookRoot := t.TempDir()
	manifestPath := filepath.Join(hookRoot, ".codex", "hooks.json")
	writeFile(t, manifestPath, `{
		"description":"reviewed hooks",
		"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard --check","timeout":10}]}]}
	}`, 0o444)

	projectRoot := gittest.NewRepo(t)
	workDir := filepath.Join(projectRoot, "nested", "sandbox")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	overrides, err := LaunchConfigOverrides(hookRoot, workDir)
	if err != nil {
		t.Fatalf("LaunchConfigOverrides() error: %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("LaunchConfigOverrides() = %q, want two overrides", overrides)
	}

	var parsed map[string]any
	if err := toml.Unmarshal([]byte(strings.Join(overrides, "\n")), &parsed); err != nil {
		t.Fatalf("generated overrides are not valid TOML: %v\n%s", err, strings.Join(overrides, "\n"))
	}
	projects, ok := parsed["projects"].(map[string]any)
	if !ok {
		t.Fatalf("projects override = %#v", parsed["projects"])
	}
	project, ok := projects[workDir].(map[string]any)
	if !ok || project["trust_level"] != "untrusted" {
		t.Fatalf("project trust override = %#v", projects[workDir])
	}
	canonicalProjectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	rootProject, ok := projects[canonicalProjectRoot].(map[string]any)
	if !ok || rootProject["trust_level"] != "untrusted" {
		t.Fatalf("root project trust override = %#v", projects[canonicalProjectRoot])
	}
	hooks, ok := parsed["hooks"].(map[string]any)
	if !ok || hooks["PreToolUse"] == nil {
		t.Fatalf("hooks override = %#v", parsed["hooks"])
	}
}

func TestLaunchConfigOverridesRejectsMutableOrAmbiguousManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		mode     os.FileMode
		want     string
	}{
		{name: "writable", manifest: `{"hooks":{}}`, mode: 0o644, want: "read-only regular file"},
		{name: "missing hooks", manifest: `{"description":"none"}`, mode: 0o444, want: "no hooks object"},
		{name: "trailing value", manifest: `{"hooks":{}} {}`, mode: 0o444, want: "trailing JSON value"},
		{name: "null hook value", manifest: `{"hooks":{"Stop":null}}`, mode: 0o444, want: "null is not a TOML value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hookRoot := t.TempDir()
			writeFile(t, filepath.Join(hookRoot, ".codex", "hooks.json"), tt.manifest, tt.mode)
			_, err := LaunchConfigOverrides(hookRoot, gittest.NewRepo(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LaunchConfigOverrides() error = %v, want %q", err, tt.want)
			}
		})
	}
}
