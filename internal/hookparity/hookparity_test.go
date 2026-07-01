package hookparity_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type hookSettings struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Command string `json:"command"`
	Type    string `json:"type"`
}

func TestHookHarnessesExposeRequiredGuardrails(t *testing.T) {
	root := repoRoot(t)
	harnesses := map[string]string{
		"claude-code":  filepath.Join(root, ".claude", "settings.json"),
		"codex-cli":    filepath.Join(root, ".codex", "hooks.json"),
		"agy":          filepath.Join(root, ".agents", "hooks.json"),
		"opencode-cli": filepath.Join(root, ".opencode", "hooks.json"),
	}

	for harness, manifestPath := range harnesses {
		t.Run(harness, func(t *testing.T) {
			settings := readHookSettings(t, manifestPath)
			commands := allCommands(settings)

			for _, required := range []string{
				"pretool-spawn-routing",
				"pretool-bead-close-guard",
				"pretool-bypass-guard",
				"pretool-pr-guard",
			} {
				if !hasCommandContaining(commands, required) {
					t.Fatalf("%s missing PreToolUse guardrail %q in %s", harness, required, manifestPath)
				}
			}

			for _, event := range []string{"Stop", "SubagentStop"} {
				if !eventHasCommandContaining(settings, event, "stop-guardrail-feedback") {
					t.Fatalf("%s missing %s stop guardrail feedback hook", harness, event)
				}
			}
		})
	}
}

func TestNonClaudeHookHarnessesExposeBeadsLifecycleHooks(t *testing.T) {
	root := repoRoot(t)
	harnesses := map[string]struct {
		path   string
		prefix string
	}{
		"codex-cli":    {path: filepath.Join(root, ".codex", "hooks.json"), prefix: "codex"},
		"agy":          {path: filepath.Join(root, ".agents", "hooks.json"), prefix: "antigravity"},
		"opencode-cli": {path: filepath.Join(root, ".opencode", "hooks.json"), prefix: "opencode"},
	}

	for harness, cfg := range harnesses {
		t.Run(harness, func(t *testing.T) {
			settings := readHookSettings(t, cfg.path)
			for event, suffix := range map[string]string{
				"SessionStart":     "SessionStart",
				"UserPromptSubmit": "UserPromptSubmit",
				"PreCompact":       "PreCompact",
				"PostCompact":      "PostCompact",
			} {
				want := "bd " + cfg.prefix + "-hook " + suffix
				if !eventHasCommandContaining(settings, event, want) {
					t.Fatalf("%s missing %s Beads hook command %q", harness, event, want)
				}
			}
		})
	}
}

func TestHookManifestLocalScriptsExistAndAreExecutable(t *testing.T) {
	root := repoRoot(t)
	for _, manifestPath := range []string{
		filepath.Join(root, ".claude", "settings.json"),
		filepath.Join(root, ".codex", "hooks.json"),
		filepath.Join(root, ".agents", "hooks.json"),
		filepath.Join(root, ".opencode", "hooks.json"),
	} {
		t.Run(filepath.ToSlash(strings.TrimPrefix(manifestPath, root+string(os.PathSeparator))), func(t *testing.T) {
			settings := readHookSettings(t, manifestPath)
			for _, command := range allCommands(settings) {
				if !strings.Contains(command, "${CLAUDE_PROJECT_DIR}/.") {
					continue
				}
				localPath := strings.Replace(command, "${CLAUDE_PROJECT_DIR}", root, 1)
				info, err := os.Stat(localPath)
				if err != nil {
					t.Fatalf("hook command %q points to missing script %s: %v", command, localPath, err)
				}
				if info.Mode()&0o111 == 0 {
					t.Fatalf("hook command %q script %s is not executable: mode %v", command, localPath, info.Mode())
				}
			}
		})
	}
}

func TestOpenCodeHookParserRegressions(t *testing.T) {
	root := repoRoot(t)
	tests := []struct {
		name       string
		script     string
		command    string
		wantCode   int
		wantOutput string
	}{
		{
			name:     "bead close guard ignores close in comments",
			script:   ".opencode/hooks/pretool-bead-close-guard",
			command:  `bd comment ce-rpet --text "please close this after merge"`,
			wantCode: 0,
		},
		{
			name:       "bypass guard blocks short force push",
			script:     ".opencode/hooks/pretool-bypass-guard",
			command:    "git push -f origin feat/harness-model-parity",
			wantCode:   2,
			wantOutput: "git push --force",
		},
		{
			name:       "pr guard catches repo flag before create",
			script:     ".opencode/hooks/pretool-pr-guard",
			command:    "gh pr -R vbonnet/dear-agent create --title test --body test",
			wantCode:   2,
			wantOutput: "gh pr create",
		},
		{
			name:       "spawn routing skips launcher prefix",
			script:     ".opencode/hooks/pretool-spawn-routing",
			command:    "env claude-code --continue",
			wantCode:   0,
			wantOutput: "outside AGM/VROOM",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, output := runHookScript(t, filepath.Join(root, tc.script), tc.command)
			if code != tc.wantCode {
				t.Fatalf("exit code = %d, want %d; output: %s", code, tc.wantCode, output)
			}
			if tc.wantOutput != "" && !strings.Contains(output, tc.wantOutput) {
				t.Fatalf("output missing %q: %s", tc.wantOutput, output)
			}
		})
	}
}

func readHookSettings(t *testing.T, path string) hookSettings {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var settings hookSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(settings.Hooks) == 0 {
		t.Fatalf("%s has no hooks", path)
	}
	return settings
}

func runHookScript(t *testing.T, script, command string) (int, string) {
	t.Helper()
	input, err := json.Marshal(map[string]any{
		"tool_name": "Bash",
		"tool_input": map[string]string{
			"command": command,
		},
	})
	if err != nil {
		t.Fatalf("marshal hook input: %v", err)
	}

	cmd := exec.Command(script)
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(output)
	}
	t.Fatalf("run hook script %s: %v; output: %s", script, err, output)
	return -1, string(output)
}

func allCommands(settings hookSettings) []string {
	var commands []string
	for _, groups := range settings.Hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				commands = append(commands, hook.Command)
			}
		}
	}
	return commands
}

func eventHasCommandContaining(settings hookSettings, event, substr string) bool {
	for _, group := range settings.Hooks[event] {
		for _, hook := range group.Hooks {
			if strings.Contains(hook.Command, substr) {
				return true
			}
		}
	}
	return false
}

func hasCommandContaining(commands []string, substr string) bool {
	for _, command := range commands {
		if strings.Contains(command, substr) {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
