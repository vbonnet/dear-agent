package hookparity_test

import (
	"bytes"
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
	Timeout int    `json:"timeout"`
}

func TestNativeSPECContractTransportsUseTheirProviderSchemas(t *testing.T) {
	root := repoRoot(t)
	harnesses := map[string]struct {
		path     string
		provider string
	}{
		"claude-code": {path: filepath.Join(root, ".claude", "settings.json"), provider: "claude"},
		"codex-cli":   {path: filepath.Join(root, ".codex", "hooks.json"), provider: "codex"},
		"pi-cli":      {path: filepath.Join(root, ".pi", "hooks.json"), provider: "pi"},
	}
	for harness, config := range harnesses {
		t.Run(harness, func(t *testing.T) {
			settings := readHookSettings(t, config.path)
			for _, event := range []string{"Stop", "SubagentStop"} {
				commands := eventCommands(settings, event)
				adapters := matchingCommands(commands, "cmd/spec-contract-hook")
				if len(adapters) != 1 {
					t.Fatalf("%s must expose exactly one cooperative native SPEC guard adapter for %s, got %v", harness, event, adapters)
				}
				siblings := matchingCommands(commands, "stop-guardrail-feedback")
				if len(commands) != 2 || len(siblings) != 1 {
					t.Fatalf("%s %s terminal chain = %v, want one SPEC adapter and one sibling guardrail independent of order", harness, event, commands)
				}
				arguments := strings.Fields(adapters[0])
				if argumentValue(arguments, "--provider") != config.provider || argumentValue(arguments, "--event") != event || argumentValue(arguments, "--root") == "" {
					t.Fatalf("%s %s adapter does not bind the provider protocol, root, and event: %q", harness, event, adapters[0])
				}
			}
		})
	}

	claude := readHookSettings(t, filepath.Join(root, ".claude", "settings.json"))
	resets := eventCommands(claude, "UserPromptSubmit")
	if len(resets) != 1 || len(matchingCommands(resets, "cmd/spec-contract-hook")) != 1 {
		t.Fatalf("Claude UserPromptSubmit chain = %v, want exactly one SPEC feedback reset adapter", resets)
	}
	arguments := strings.Fields(resets[0])
	if argumentValue(arguments, "--provider") != "claude" || argumentValue(arguments, "--event") != "UserPromptSubmit" || argumentValue(arguments, "--root") == "" {
		t.Fatalf("Claude user-turn reset does not bind the provider protocol, root, and native event: %q", resets[0])
	}
}

func TestCodexSourceSPECContractTransportResolvesRootFromNestedCWD(t *testing.T) {
	root := repoRoot(t)
	settings := readHookSettings(t, filepath.Join(root, ".codex", "hooks.json"))
	for _, event := range []string{"Stop", "SubagentStop"} {
		t.Run(event, func(t *testing.T) {
			adapters := matchingCommands(eventCommands(settings, event), "cmd/spec-contract-hook")
			if len(adapters) != 1 {
				t.Fatalf("Codex %s adapters = %v, want exactly one", event, adapters)
			}
			command := exec.Command("/bin/sh", "-c", adapters[0])
			command.Dir = filepath.Join(root, "agm")
			command.Env = append(environmentWithout(os.Environ(), "AGM_CODEX_HOOK_ROOT", "CLAUDE_PROJECT_DIR"), "DEAR_AGENT_HOOK_STATE_DIR="+t.TempDir())
			command.Stdin = strings.NewReader(`{"hook_event_name":"` + event + `","session_id":"nested-codex","turn_id":"nested-codex-turn","stop_hook_active":false}`)
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("run Codex %s adapter from nested cwd: %v\nstdout=%s\nstderr=%s", event, err, stdout.String(), stderr.String())
			}
			var response map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
				t.Fatalf("decode Codex %s response %q: %v; stderr=%s", event, stdout.String(), err, stderr.String())
			}
		})
	}
}

func TestAntigravityUsesNamedStopHookSchema(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".agents", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]map[string][]hookEntry
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if _, legacy := manifest["hooks"]; legacy || len(manifest) != 1 {
		t.Fatalf("Antigravity manifest must be a named-hook map, got keys %#v", mapKeys(manifest))
	}
	guard := manifest["spec-contract-guard"]
	if len(guard) != 1 || len(guard["Stop"]) != 1 || guard["SubagentStop"] != nil {
		t.Fatalf("Antigravity guard event map = %#v, want one Stop handler and no SubagentStop", guard)
	}
	handler := guard["Stop"][0]
	const wantCommand = "/usr/local/libexec/dear-agent-spec-contract-hook --root-from-workspace-stdin --provider antigravity --event Stop"
	if handler.Type != "command" || handler.Timeout != 60 || handler.Command != wantCommand {
		t.Fatalf("Antigravity Stop handler = %#v", handler)
	}
	if strings.Contains(handler.Command, "go run") || strings.Contains(handler.Command, "./cmd/") || strings.Contains(handler.Command, "--root .") {
		t.Fatalf("Antigravity Stop handler must not depend on checkout source or CWD: %q", handler.Command)
	}

	// Exercise an independently built absolute artifact from a CWD outside the
	// repository. The real installation is intentionally operator-owned and is
	// not required for repository tests.
	helper := filepath.Join(t.TempDir(), "dear-agent-spec-contract-hook")
	build := exec.Command("go", "build", "-o", helper, "./cmd/spec-contract-hook")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Antigravity helper: %v\n%s", err, output)
	}
	payload, err := json.Marshal(map[string]any{
		"conversationId": "external-cwd",
		"executionNum":   1,
		"workspacePaths": []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(helper, "--root-from-workspace-stdin", "--provider", "antigravity", "--event", "Stop")
	command.Dir = t.TempDir()
	stateDirectory := t.TempDir()
	if err := os.Chmod(stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	command.Env = append(os.Environ(), "DEAR_AGENT_HOOK_STATE_DIR="+stateDirectory)
	command.Stdin = strings.NewReader(string(payload))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Antigravity Stop helper from external CWD: %v\n%s", err, output)
	}
	var response struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("decode Antigravity Stop response %q: %v", output, err)
	}
	if response.Decision != "allow" && response.Decision != "continue" {
		t.Fatalf("Antigravity Stop response = %#v, want native response after workspace resolution", response)
	}
	if strings.Contains(response.Reason, "must supply exactly one valid absolute Git workspace root") {
		t.Fatalf("Antigravity Stop helper did not use its native workspace root: %#v", response)
	}
}

func TestSpawnRoutingCreatesWorkersDetachedWithInitialPrompt(t *testing.T) {
	root := repoRoot(t)
	for _, script := range []string{
		".claude/hooks/pretool-spawn-routing",
		".codex/hooks/pretool-spawn-routing",
		".agents/hooks/pretool-spawn-routing",
		".opencode/hooks/pretool-spawn-routing",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(script)))
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		guidance := string(data)
		for _, required := range []string{"agm session new", "--detached", "--prompt-file <path>"} {
			if !strings.Contains(guidance, required) {
				t.Errorf("%s spawn guidance missing %q", script, required)
			}
		}
		if strings.Contains(guidance, "agm send msg <name>") {
			t.Errorf("%s retains the separate send sequence", script)
		}
	}
}

func TestPRGuardEscalationWorksOutsideAGM(t *testing.T) {
	root := repoRoot(t)
	for _, script := range []string{
		".claude/hooks/pretool-pr-guard",
		".codex/hooks/pretool-pr-guard",
		".agents/hooks/pretool-pr-guard",
		".opencode/hooks/pretool-pr-guard",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(script)))
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		guidance := string(data)
		for _, required := range []string{"agm escalate ask --session <registered-session>", "ask the current user directly"} {
			if !strings.Contains(guidance, required) {
				t.Errorf("%s escalation guidance missing %q", script, required)
			}
		}
	}
}

func TestBypassGuardEscalationIsRunnableAndTruthful(t *testing.T) {
	root := repoRoot(t)
	for _, script := range []string{
		".claude/hooks/pretool-bypass-guard",
		".codex/hooks/pretool-bypass-guard",
		".agents/hooks/pretool-bypass-guard",
		".opencode/hooks/pretool-bypass-guard",
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(script)))
		if err != nil {
			t.Fatalf("read %s: %v", script, err)
		}
		guidance := string(data)
		for _, required := range []string{
			"agm escalate ask --kind blocked-action --context",
			"--session <registered-session>",
			"ask the current user directly",
			"does not create or update a Bead",
		} {
			if !strings.Contains(guidance, required) {
				t.Errorf("%s escalation guidance missing %q", script, required)
			}
		}
		if strings.Contains(guidance, "Every escalation creates a bead") {
			t.Errorf("%s promises unimplemented Beads creation", script)
		}
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

func eventCommands(settings hookSettings, event string) []string {
	commands := []string{}
	for _, group := range settings.Hooks[event] {
		for _, hook := range group.Hooks {
			commands = append(commands, hook.Command)
		}
	}
	return commands
}

func matchingCommands(commands []string, needle string) []string {
	result := []string{}
	for _, command := range commands {
		if strings.Contains(command, needle) {
			result = append(result, command)
		}
	}
	return result
}

func argumentValue(arguments []string, flag string) string {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == flag {
			return arguments[index+1]
		}
	}
	return ""
}

func mapKeys(values map[string]map[string][]hookEntry) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func hasCommandContaining(commands []string, substr string) bool {
	for _, command := range commands {
		if strings.Contains(command, substr) {
			return true
		}
	}
	return false
}

func environmentWithout(environment []string, names ...string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		keep := true
		for _, name := range names {
			if strings.HasPrefix(entry, name+"=") {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
