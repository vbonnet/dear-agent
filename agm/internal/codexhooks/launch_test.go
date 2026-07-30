package codexhooks

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	if err := hardenHookCommands(hooks, nil); err != nil {
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

func TestAttestedHookPathContainsOnlyOperatorOwnedLocations(t *testing.T) {
	const want = "/usr/local/libexec:/usr/bin:/bin:/usr/sbin:/sbin"
	if attestedHookPath != want {
		t.Fatalf("attested hook PATH = %q, want %q", attestedHookPath, want)
	}
	if strings.Contains(attestedHookPath, "/opt/homebrew") ||
		strings.Contains(attestedHookPath, "/usr/local/bin") ||
		strings.Contains(attestedHookPath, "/usr/local/go/bin") {
		t.Fatalf("attested hook PATH includes a commonly user-writable location: %q", attestedHookPath)
	}
	if trustedHookJSONPath != "/usr/local/libexec/dear-agent-codex-hook-json" {
		t.Fatalf("trusted hook JSON path = %q", trustedHookJSONPath)
	}
	// Do not assert host/container ownership here. GitHub's Ubuntu container
	// maps system paths such as /usr to the runner UID even though production
	// must continue to require uid 0. The rejection fixture below exercises the
	// validator without weakening that runtime boundary.
}

func TestTrustedExecutableSearchPathRejectsWritableDirectory(t *testing.T) {
	writable := t.TempDir()
	if err := os.Chmod(writable, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := validateTrustedExecutableSearchPath(writable); err == nil ||
		!strings.Contains(err.Error(), "operator-owned") {
		t.Fatalf("validateTrustedExecutableSearchPath() error = %v, want ownership rejection", err)
	}
}

func TestValidateMaterializedSystemExecutablesChecksExactAllowlistedFiles(t *testing.T) {
	previous := validateTrustedSystemExecutable
	var got []string
	validateTrustedSystemExecutable = func(path string) error {
		got = append(got, path)
		return nil
	}
	t.Cleanup(func() { validateTrustedSystemExecutable = previous })

	assets := map[string]asset{
		".codex/hooks/fixture": {
			executable: true,
			content: []byte(
				"#!/bin/sh\n" +
					"/usr/local/libexec/dear-agent-codex-hook-json -r .\n" +
					"/usr/local/libexec/dear-agent-bead-close-guard --help\n",
			),
		},
	}
	if err := validateMaterializedSystemExecutables(assets); err != nil {
		t.Fatalf("validateMaterializedSystemExecutables() error = %v", err)
	}
	want := []string{
		"/usr/local/libexec/dear-agent-bead-close-guard",
		"/usr/local/libexec/dear-agent-codex-hook-json",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("validated exact executables = %v, want %v", got, want)
	}

	assets[".codex/hooks/fixture"] = asset{
		executable: true,
		content:    []byte("#!/bin/sh\n/usr/local/libexec/user-tools/helper\n"),
	}
	if err := validateMaterializedSystemExecutables(assets); err == nil ||
		!strings.Contains(err.Error(), "unapproved system executable") {
		t.Fatalf("validateMaterializedSystemExecutables() error = %v, want unapproved-path rejection", err)
	}
}

func TestNeutralizeWorkspaceExecutingHooksPreservesHandlerIndexes(t *testing.T) {
	for eventName := range neutralizedAttestedHookEvents {
		hooks := map[string]any{
			eventName: []any{map[string]any{
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": "mutable-workspace-command",
				}},
			}},
		}
		if err := neutralizeWorkspaceExecutingHooks(hooks); err != nil {
			t.Fatalf("neutralizeWorkspaceExecutingHooks(%s): %v", eventName, err)
		}
		groups := hooks[eventName].([]any)
		group := groups[0].(map[string]any)
		handlers := group["hooks"].([]any)
		if len(handlers) != 1 {
			t.Fatalf("%s handler count = %d, want 1", eventName, len(handlers))
		}
		handler := handlers[0].(map[string]any)
		if handler["command"] != "/bin/true" {
			t.Fatalf("%s command = %q, want /bin/true", eventName, handler["command"])
		}
	}
}

func TestLaunchConfigOverridesDisablesMutableWorkspaceExecutingHooks(t *testing.T) {
	useTrustedHookJSONFixture(t)
	hookRoot := t.TempDir()
	writeFile(t, filepath.Join(hookRoot, ".codex", "hooks.json"), `{
		"hooks":{
			"Stop":[{"hooks":[{"type":"command","command":"scripts/guardrail-bundle.sh"}]}],
			"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"bd codex-hook SessionStart"}]}]
		}
	}`, 0o444)
	hookRoot, digest := sealMaterializedFixture(t, hookRoot)
	projectRoot := gittest.NewRepo(t)
	overrides, err := LaunchConfigOverrides(hookRoot, digest, projectRoot)
	if err != nil {
		t.Fatalf("LaunchConfigOverrides() error: %v", err)
	}
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(strings.Join(overrides, "\n")), &parsed); err != nil {
		t.Fatalf("generated overrides are not valid TOML: %v", err)
	}
	hooks := parsed["hooks"].(map[string]any)
	state := hooks["state"].(map[string]any)
	canonicalProjectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	for eventName, eventKey := range map[string]string{"Stop": "stop", "SessionStart": "session_start"} {
		groups := hooks[eventName].([]any)
		group := groups[0].(map[string]any)
		handlers := group["hooks"].([]any)
		handler := handlers[0].(map[string]any)
		command := handler["command"].(string)
		if !strings.Contains(command, "/bin/true") ||
			strings.Contains(command, "guardrail-bundle") ||
			strings.Contains(command, "bd codex-hook") {
			t.Fatalf("%s bypass command = %q, want trusted no-op", eventName, command)
		}
		projectKey := filepath.Join(canonicalProjectRoot, ".codex", "hooks.json") + ":" + eventKey + ":0:0"
		projectState := state[projectKey].(map[string]any)
		if projectState["enabled"] != false {
			t.Fatalf("%s project hook state = %#v, want disabled", eventName, projectState)
		}
		sessionKey := sessionFlagsHookSource + ":" + eventKey + ":0:0"
		if _, ok := state[sessionKey].(map[string]any)["trusted_hash"]; !ok {
			t.Fatalf("%s session hook state = %#v, want trusted hash", eventName, state[sessionKey])
		}
	}
}

func TestLaunchConfigOverridesPinsManifestAndDisablesProjectHookCopies(t *testing.T) {
	useTrustedHookJSONFixture(t)
	hookRoot := t.TempDir()
	manifestPath := filepath.Join(hookRoot, ".codex", "hooks.json")
	writeFile(t, manifestPath, `{
		"description":"reviewed hooks",
		"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard --check","timeout":10}]}]}
	}`, 0o444)
	hookRoot, digest := sealMaterializedFixture(t, hookRoot)

	projectRoot := gittest.NewRepo(t)
	workDir := filepath.Join(projectRoot, "nested", "sandbox")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	overrides, err := LaunchConfigOverrides(hookRoot, digest, workDir)
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

func TestLaunchConfigOverridesEmbedsVerifiedHookBytes(t *testing.T) {
	useTrustedHookJSONFixture(t)
	stage := t.TempDir()
	writeFile(t, filepath.Join(stage, ".codex", "hooks.json"), `{
		"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{
			"type":"command",
			"command":"${AGM_CODEX_HOOK_ROOT:-${CLAUDE_PROJECT_DIR:-.}}/.codex/hooks/guard"
		}]}]}
	}`, 0o444)
	guardPath := filepath.Join(stage, ".codex", "hooks", "guard")
	writeFile(t, guardPath, "#!/bin/sh\nprintf original\n", 0o555)
	hookRoot, digest := sealMaterializedFixture(t, stage)

	overrides, err := LaunchConfigOverrides(hookRoot, digest, gittest.NewRepo(t))
	if err != nil {
		t.Fatalf("LaunchConfigOverrides() error: %v", err)
	}
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(strings.Join(overrides, "\n")), &parsed); err != nil {
		t.Fatalf("generated overrides are not valid TOML: %v", err)
	}
	hooks := parsed["hooks"].(map[string]any)
	groups := hooks["PreToolUse"].([]any)
	group := groups[0].(map[string]any)
	handlers := group["hooks"].([]any)
	command := handlers[0].(map[string]any)["command"].(string)
	if strings.Contains(command, "AGM_CODEX_HOOK_ROOT") ||
		strings.Contains(command, filepath.Join(hookRoot, ".codex", "hooks", "guard")) ||
		!strings.Contains(command, "printf original") {
		t.Fatalf("session command did not embed verified hook bytes: %q", command)
	}

	for _, path := range []string{
		hookRoot,
		filepath.Join(hookRoot, ".codex"),
		filepath.Join(hookRoot, ".codex", "hooks"),
	} {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	guardPath = filepath.Join(hookRoot, ".codex", "hooks", "guard")
	if err := os.Chmod(guardPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guardPath, []byte("#!/bin/sh\nprintf substituted\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		t.Fatalf("run embedded hook command: %v", err)
	}
	if got := string(output); got != "original" {
		t.Fatalf("embedded hook output = %q, want original bytes", got)
	}
}

func TestLaunchConfigOverridesPreservesEmbeddedBashInterpreter(t *testing.T) {
	useTrustedHookJSONFixture(t)
	stage := t.TempDir()
	writeFile(t, filepath.Join(stage, ".codex", "hooks.json"), `{
		"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{
			"type":"command",
			"command":"${AGM_CODEX_HOOK_ROOT:-${CLAUDE_PROJECT_DIR:-.}}/.codex/hooks/guard"
		}]}]}
	}`, 0o444)
	writeFile(
		t,
		filepath.Join(stage, ".codex", "hooks", "guard"),
		"#!/bin/bash\nset -uo pipefail\nvalues=(original preserved)\n[[ ${values[1]} == preserved ]]\nprintf bash\n",
		0o555,
	)
	hookRoot, digest := sealMaterializedFixture(t, stage)

	overrides, err := LaunchConfigOverrides(hookRoot, digest, gittest.NewRepo(t))
	if err != nil {
		t.Fatalf("LaunchConfigOverrides() error: %v", err)
	}
	command := commandFromOverrides(t, overrides, "PreToolUse")
	if !strings.Contains(command, "/bin/bash -c") {
		t.Fatalf("embedded Bash hook command = %q, want absolute Bash interpreter", command)
	}
	output, err := exec.Command("/bin/sh", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("run embedded Bash hook command: %v\n%s", err, output)
	}
	if got := string(output); got != "bash" {
		t.Fatalf("embedded Bash hook output = %q, want bash", got)
	}
}

func TestLaunchConfigOverridesRejectsOversizedEmbeddedHookArgument(t *testing.T) {
	useTrustedHookJSONFixture(t)
	stage := t.TempDir()
	writeFile(t, filepath.Join(stage, ".codex", "hooks.json"), `{
		"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{
			"type":"command",
			"command":"${AGM_CODEX_HOOK_ROOT:-${CLAUDE_PROJECT_DIR:-.}}/.codex/hooks/guard"
		}]}]}
	}`, 0o444)
	writeFile(
		t,
		filepath.Join(stage, ".codex", "hooks", "guard"),
		"#!/bin/bash\n#"+strings.Repeat("x", maxHookConfigArgumentBytes)+"\n",
		0o555,
	)
	hookRoot, digest := sealMaterializedFixture(t, stage)

	_, err := LaunchConfigOverrides(hookRoot, digest, gittest.NewRepo(t))
	if err == nil || !strings.Contains(err.Error(), "safe single-argument limit") {
		t.Fatalf("LaunchConfigOverrides() error = %v, want oversized-argument rejection", err)
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
			useTrustedHookJSONFixture(t)
			hookRoot := t.TempDir()
			writeFile(t, filepath.Join(hookRoot, ".codex", "hooks.json"), tt.manifest, tt.mode)
			hookRoot, digest := sealMaterializedFixture(t, hookRoot)
			_, err := LaunchConfigOverrides(hookRoot, digest, gittest.NewRepo(t))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LaunchConfigOverrides() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func sealMaterializedFixture(t *testing.T, stage string) (string, string) {
	t.Helper()
	var assets []asset
	if err := filepath.WalkDir(stage, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(stage, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		executable := info.Mode().Perm()&0o111 != 0
		gitMode := "100644"
		if executable {
			gitMode = "100755"
		}
		assets = append(assets, asset{
			path: filepath.ToSlash(relative), gitMode: gitMode, content: content, executable: executable,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	digest := digestAssets(assets)
	root := filepath.Join(filepath.Dir(stage), digest)
	if err := os.Rename(stage, root); err != nil {
		t.Fatal(err)
	}
	if err := lockMaterializedDirectories(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && entry.IsDir() {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(root)
	})
	return root, digest
}

func commandFromOverrides(t *testing.T, overrides []string, event string) string {
	t.Helper()
	var parsed map[string]any
	if err := toml.Unmarshal([]byte(strings.Join(overrides, "\n")), &parsed); err != nil {
		t.Fatalf("generated overrides are not valid TOML: %v", err)
	}
	hooks := parsed["hooks"].(map[string]any)
	groups := hooks[event].([]any)
	group := groups[0].(map[string]any)
	handlers := group["hooks"].([]any)
	return handlers[0].(map[string]any)["command"].(string)
}

func useTrustedHookJSONFixture(t *testing.T) {
	t.Helper()
	previousJSONPath := trustedHookJSONPath
	previousPathValidator := validateAttestedExecutablePath
	previousJSONValidator := validateTrustedHookJSONExecutable
	previousSystemValidator := validateTrustedSystemExecutable
	trustedHookJSONPath = "/bin/sh"
	validateAttestedExecutablePath = func(got string) error {
		if got != attestedHookPath {
			t.Fatalf("validate attested executable PATH %q, want %q", got, attestedHookPath)
		}
		return nil
	}
	validateTrustedHookJSONExecutable = func(got string) error {
		if got != trustedHookJSONPath {
			t.Fatalf("validate trusted hook JSON executable %q, want %q", got, trustedHookJSONPath)
		}
		return nil
	}
	validateTrustedSystemExecutable = func(string) error { return nil }
	t.Cleanup(func() {
		trustedHookJSONPath = previousJSONPath
		validateAttestedExecutablePath = previousPathValidator
		validateTrustedHookJSONExecutable = previousJSONValidator
		validateTrustedSystemExecutable = previousSystemValidator
	})
}
