package codexhooks

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestHardenHookCommandsReplacesCallerPath(t *testing.T) {
	script := filepath.Join(t.TempDir(), "guard")
	writeFile(t, script, "#!/bin/bash\nprintf '%s' \"$PATH\"\n", 0o755)
	hooks := map[string]any{
		"Stop": []any{map[string]any{
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": script,
			}},
		}},
	}
	if err := hardenHookCommands(hooks); err != nil {
		t.Fatalf("hardenHookCommands() error: %v", err)
	}
	groups := hooks["Stop"].([]any)
	group := groups[0].(map[string]any)
	handlers := group["hooks"].([]any)
	handler := handlers[0].(map[string]any)
	command := handler["command"].(string)
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env = []string{"PATH=" + t.TempDir()}
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("run hardened hook command: %v", err)
	}
	if got := string(output); got != attestedHookPath {
		t.Fatalf("hook PATH = %q, want %q", got, attestedHookPath)
	}
}

func TestLaunchConfigOverridesPinsManifestAndDisablesProjectHookCopies(t *testing.T) {
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
	if len(overrides) != 1 {
		t.Fatalf("LaunchConfigOverrides() = %q, want one override", overrides)
	}

	var parsed map[string]any
	if err := toml.Unmarshal([]byte(strings.Join(overrides, "\n")), &parsed); err != nil {
		t.Fatalf("generated overrides are not valid TOML: %v\n%s", err, strings.Join(overrides, "\n"))
	}
	canonicalProjectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	hooks, ok := parsed["hooks"].(map[string]any)
	if !ok || hooks["PreToolUse"] == nil {
		t.Fatalf("hooks override = %#v", parsed["hooks"])
	}
	groups := hooks["PreToolUse"].([]any)
	group := groups[0].(map[string]any)
	handlers := group["hooks"].([]any)
	handler := handlers[0].(map[string]any)
	command := handler["command"].(string)
	if !strings.HasPrefix(command, attestedHookCommandPrefix) ||
		!strings.Contains(command, "guard --check") {
		t.Fatalf("hardened hook command = %q", command)
	}
	state, ok := hooks["state"].(map[string]any)
	if !ok {
		t.Fatalf("hooks state override = %#v", hooks["state"])
	}
	key := filepath.Join(canonicalProjectRoot, ".codex", "hooks.json") + ":pre_tool_use:0:0"
	disabled, ok := state[key].(map[string]any)
	if !ok || disabled["enabled"] != false {
		t.Fatalf("project hook state = %#v", state[key])
	}
	sessionKey := sessionFlagsHookSource + ":pre_tool_use:0:0"
	trusted, ok := state[sessionKey].(map[string]any)
	handler["timeout"] = json.Number("10")
	wantTrustedHash, err := commandHookTrustedHash("PreToolUse", "pre_tool_use", group, handler)
	if err != nil {
		t.Fatalf("commandHookTrustedHash() error: %v", err)
	}
	if !ok || trusted["trusted_hash"] != wantTrustedHash {
		t.Fatalf("session hook state = %#v", state[sessionKey])
	}
	if _, exists := parsed["projects"]; exists {
		t.Fatalf("launch overrides project trust: %#v", parsed["projects"])
	}
}

func TestProjectHookSourceRootsIncludesLinkedRootCheckout(t *testing.T) {
	rootCheckout := gittest.NewRepo(t)
	linkedCheckout := filepath.Join(t.TempDir(), "linked")
	gittest.Run(t, rootCheckout, "worktree", "add", "-b", "linked", linkedCheckout)

	projectRoot, err := gitRoot(context.Background(), linkedCheckout)
	if err != nil {
		t.Fatalf("gitRoot() error: %v", err)
	}
	roots, err := projectHookSourceRoots(context.Background(), linkedCheckout, projectRoot)
	if err != nil {
		t.Fatalf("projectHookSourceRoots() error: %v", err)
	}
	got := make(map[string]bool, len(roots))
	for _, root := range roots {
		got[root] = true
	}
	canonicalRootCheckout, err := filepath.EvalSymlinks(rootCheckout)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{projectRoot, canonicalRootCheckout} {
		if !got[want] {
			t.Fatalf("project hook roots = %q, missing %q", roots, want)
		}
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
		{name: "null hook value", manifest: `{"hooks":{"Stop":null}}`, mode: 0o444, want: "event Stop must be an array"},
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
