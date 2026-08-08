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
				arguments := strings.Fields(adapters[0])
				if argumentValue(arguments, "--provider") != config.provider || argumentValue(arguments, "--event") != event || argumentValue(arguments, "--root") == "" {
					t.Fatalf("%s %s adapter does not bind the provider protocol, root, and event: %q", harness, event, adapters[0])
				}
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
	command.Env = append(os.Environ(), "DEAR_AGENT_HOOK_STATE_DIR="+t.TempDir())
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

func TestOpenCodeSPECContractPluginUsesIdleEventAndConservativeTransport(t *testing.T) {
	root := repoRoot(t)
	tombstoneData, err := os.ReadFile(filepath.Join(root, ".opencode", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tombstone struct {
		DearAgent struct {
			Status       string `json:"status"`
			Replacement  string `json:"replacement"`
			RuntimeClaim string `json:"runtime_claim"`
		} `json:"_dear_agent"`
		Hooks json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(tombstoneData, &tombstone); err != nil {
		t.Fatal(err)
	}
	if len(tombstone.Hooks) != 0 || tombstone.DearAgent.Status != "retired" ||
		tombstone.DearAgent.Replacement != ".opencode/plugins/spec-contract-guard.mjs" || tombstone.DearAgent.RuntimeClaim != "none" {
		t.Fatalf("OpenCode hooks.json must be an inert retirement tombstone, got %#v", tombstone)
	}
	owner, err := os.ReadFile(filepath.Join(root, ".opencode", "plugins", "SPEC.owner"))
	if err != nil {
		t.Fatal(err)
	}
	if string(owner) != "internal/hookparity/SPEC.md\n" {
		t.Fatalf("OpenCode plugin SPEC owner = %q, want neutral hook parity contract", owner)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the OpenCode plugin transport contract")
	}
	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.mjs")
	script := `
import {pathToFileURL} from "node:url";
const calls = [];
const logs = [];
const prompts = [];
let failNextLog = false;
let failNextPrompt = false;
let reenterPrompt = false;
let reenterLogSession = "";
const encoded = (value) => new TextEncoder().encode(value);
const results = [];
globalThis.Bun = {
  spawnSync(args, options) {
    calls.push([args, options]);
    return results.shift() || {exitCode: 0, stdout: encoded('{"decision":"block","systemMessage":"repair the contract"}'), stderr: new Uint8Array()};
  },
};
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {
    app: {async log(entry) { if (failNextLog) { failNextLog = false; throw new Error("log unavailable"); } logs.push(entry); if (reenterLogSession) { const sessionID = reenterLogSession; reenterLogSession = ""; await hooks.event({event: {type: "session.idle", properties: {sessionID}}}); } }},
    session: {async promptAsync(entry) { prompts.push(entry); if (reenterPrompt) { reenterPrompt = false; await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: entry.path.id, id: entry.body.messageID}}}}); } if (failNextPrompt) { failNextPrompt = false; throw new Error("prompt unavailable"); } }},
};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: "real-user-message-1"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (calls.length !== 1) throw new Error("session.idle did not run exactly one adapter: " + calls.length);
const [args, options] = calls[0];
if (args.join("|") !== ["go", "run", "./cmd/spec-contract-hook", "--root", process.argv[2], "--provider", "opencode", "--event", "Stop"].join("|")) throw new Error("adapter argv: " + JSON.stringify(args));
if (options.cwd !== process.argv[2] || options.stdin !== "ignore" || options.timeout !== 60000 || options.maxBuffer !== 65536) throw new Error("adapter options: " + JSON.stringify(options));
if (logs.length !== 1 || logs[0].body.level !== "warn" || !logs[0].body.message.includes("bounded follow-up")) throw new Error("block log: " + JSON.stringify(logs));
if (prompts.length !== 1 || prompts[0].path.id !== "session-1" || !prompts[0].body.messageID || !prompts[0].body.parts[0].text.includes("repair the contract")) throw new Error("bounded prompt: " + JSON.stringify(prompts));
if (prompts[0].throwOnError !== true) throw new Error("prompt transport did not require SDK errors to throw");

await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (calls.length !== 1 || prompts.length !== 1) throw new Error("same idle turn repeated its continuation");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: prompts[0].body.messageID}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (calls.length !== 1) throw new Error("synthetic prompt reset its own budget");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: "real-user-message-1"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (calls.length !== 1) throw new Error("delayed update of the current real user message reset the budget");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: "real-user-message-2"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (calls.length !== 2 || prompts.length !== 2) throw new Error("real user turn did not reset budget");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: prompts[0].body.messageID}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (calls.length !== 2) throw new Error("delayed update of an older injected message reset the budget");

results.push({exitCode: 0, stdout: encoded("{}"), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "clean-session"}}});
if (calls.length !== 3 || prompts.length !== 2 || logs.length !== 2) throw new Error("clean result was not a no-op");

results.push({exitCode: 0, stdout: encoded("not json"), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "malformed-session"}}});
if (logs.length !== 3 || prompts.length !== 3 || !logs[2].body.message.includes("invalid JSON")) throw new Error("invalid adapter output was not surfaced conservatively: " + JSON.stringify(logs));

results.push({exitCode: null, exitedDueToTimeout: true, stdout: new Uint8Array(), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "timeout-session"}}});
if (logs.length !== 4 || prompts.length !== 4 || !logs[3].body.message.includes("timeout")) throw new Error("timeout was not surfaced conservatively: " + JSON.stringify(logs));

results.push({exitCode: null, signalCode: "SIGKILL", stdout: new Uint8Array(), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "signal-session"}}});
if (logs.length !== 5 || prompts.length !== 5 || !logs[4].body.message.includes("SIGKILL")) throw new Error("signal was not surfaced conservatively: " + JSON.stringify(logs));

results.push({exitCode: 0, stdout: encoded("x".repeat(70000)), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "oversize-session"}}});
if (logs.length !== 6 || prompts.length !== 6 || !logs[5].body.message.includes("exceeded")) throw new Error("oversized adapter output was not surfaced conservatively: " + JSON.stringify(logs));

await hooks.event({event: {type: "session.deleted", properties: {info: {id: "session-1"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (calls.length !== 8 || prompts.length !== 7) throw new Error("session deletion did not clear bounded state");

failNextLog = true;
await hooks.event({event: {type: "session.idle", properties: {sessionID: "log-failure-session"}}});
if (calls.length !== 9 || prompts.length !== 8) throw new Error("diagnostic failure suppressed the model-visible follow-up");

failNextPrompt = true;
await hooks.event({event: {type: "session.idle", properties: {sessionID: "prompt-failure-session"}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "prompt-failure-session"}}});
if (calls.length !== 10 || prompts.length !== 9) throw new Error("failed prompt attempt was not bounded for the real turn");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "prompt-failure-session", id: "new-real-user"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "prompt-failure-session"}}});
if (calls.length !== 11 || prompts.length !== 10) throw new Error("real user turn did not reset a failed prompt attempt");

reenterPrompt = true;
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-session"}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-session"}}});
if (calls.length !== 12 || prompts.length !== 11) throw new Error("reentrant synthetic message reset the bounded attempt");

reenterLogSession = "reentrant-log-session";
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-log-session"}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-log-session"}}});
if (calls.length !== 13 || prompts.length !== 12) throw new Error("reentrant diagnostic idle duplicated the bounded attempt");

const callsBeforeExhaustion = calls.length;
const promptsBeforeExhaustion = prompts.length;
const logsBeforeExhaustion = logs.length;
for (let index = 0; index <= 4096; index++) {
  await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "exhausted-session", id: "real-exhaustion-" + index}}}});
}
await hooks.event({event: {type: "session.idle", properties: {sessionID: "exhausted-session"}}});
if (calls.length !== callsBeforeExhaustion || prompts.length !== promptsBeforeExhaustion) throw new Error("capacity exhaustion executed another follow-up");
if (logs.length !== logsBeforeExhaustion + 1 || !logs.at(-1).body.message.includes("bounded message-identity limit")) throw new Error("capacity exhaustion was not disclosed once: " + JSON.stringify(logs.slice(logsBeforeExhaustion)));
await hooks.event({event: {type: "session.deleted", properties: {info: {id: "exhausted-session"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "exhausted-session"}}});
if (calls.length !== callsBeforeExhaustion + 1 || prompts.length !== promptsBeforeExhaustion + 1) throw new Error("deleted exhausted session identifier did not begin fresh");

const cappedHooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
for (let index = 0; index < 256; index++) {
  await cappedHooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "capacity-session-" + index, id: "capacity-message-" + index}}}});
}
const callsBeforeSessionCapacity = calls.length;
const promptsBeforeSessionCapacity = prompts.length;
const logsBeforeSessionCapacity = logs.length;
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
if (calls.length !== callsBeforeSessionCapacity || prompts.length !== promptsBeforeSessionCapacity) throw new Error("untracked over-capacity session executed a follow-up");
if (logs.length !== logsBeforeSessionCapacity + 1 || !logs.at(-1).body.message.includes("bounded session limit")) throw new Error("session capacity yield was not disclosed once: " + JSON.stringify(logs.slice(logsBeforeSessionCapacity)));
await cappedHooks.event({event: {type: "session.deleted", properties: {sessionID: "capacity-overflow"}}});
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
if (calls.length !== callsBeforeSessionCapacity || prompts.length !== promptsBeforeSessionCapacity) throw new Error("deleting an untracked session evicted tracked state");
await cappedHooks.event({event: {type: "session.deleted", properties: {sessionID: "capacity-session-0"}}});
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
if (calls.length !== callsBeforeSessionCapacity + 1 || prompts.length !== promptsBeforeSessionCapacity + 1) throw new Error("tracked deletion did not deterministically admit the yielded session");
`
	command := exec.Command(node, "--input-type=module", "-e", script, plugin, root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode plugin transport: %v\n%s", err, output)
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

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
