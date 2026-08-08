package hookparity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
const groupSignals = [];
const directKills = [];
const processes = [];
const timers = [];
let failNextLog = false;
let failNextPrompt = false;
let reenterPrompt = false;
let reenterLogSession = "";
const encoded = (value) => new TextEncoder().encode(value);
const results = [];
globalThis.setTimeout = (callback, milliseconds) => { const timer = {callback, milliseconds, cleared: false}; timers.push(timer); return timer; };
globalThis.clearTimeout = (timer) => { timer.cleared = true; };
const controlledStream = (value, holdOpen) => {
  const chunks = Array.isArray(value) ? value : value ? [value] : [];
  let controller;
  const stream = new ReadableStream({
    start(next) { controller = next; for (const chunk of chunks) next.enqueue(chunk); if (!holdOpen) next.close(); },
  });
  return {stream, close() { try { controller.close(); } catch {} }};
};
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  if (pid >= 0) return originalProcessKill(pid, signal);
  const proc = processes.find((candidate) => candidate.pid === -pid);
  if (!proc) throw new Error("unknown process group " + pid);
  groupSignals.push({pid, signal});
  proc.groupKill(signal);
  return true;
};
globalThis.Bun = {
  spawn(args, options) {
    calls.push([args, options]);
    const result = results.shift() || {exitCode: 0, stdout: encoded('{"decision":"block","systemMessage":"repair the contract"}'), stderr: new Uint8Array()};
    const stdout = controlledStream(result.stdout, Boolean(result.pending));
    const stderr = controlledStream(result.stderr, Boolean(result.pending));
    let exitCode = result.pending ? null : (result.exitCode ?? 0);
    let signalCode = result.signalCode || null;
    let resolveExit;
    const exited = result.pending ? new Promise((resolve) => { resolveExit = resolve; }) : Promise.resolve(exitCode ?? 1);
    const proc = {
      pid: 1000 + calls.length,
      stdout: stdout.stream,
      stderr: stderr.stream,
      exited,
      get exitCode() { return exitCode; },
      get signalCode() { return signalCode; },
      groupKill(signal) { signalCode = signal; exitCode = null; resolveExit?.(1); },
      kill(signal) { directKills.push({signal, pid: this.pid}); this.groupKill(signal); },
      finish(code = 0) { exitCode = code; stdout.close(); stderr.close(); resolveExit?.(code); },
    };
    processes.push(proc);
    return proc;
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
if (options.cwd !== process.argv[2] || options.detached !== true || options.stdin !== "ignore" || options.stdout !== "pipe" || options.stderr !== "pipe" || "timeout" in options || "maxBuffer" in options) throw new Error("adapter options: " + JSON.stringify(options));
if (timers.length !== 1 || timers[0].milliseconds !== 60000 || !timers[0].cleared) throw new Error("adapter timeout was not explicit and cleared: " + JSON.stringify(timers));
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

results.push({pending: true, stdout: new Uint8Array(), stderr: new Uint8Array()});
const timeoutTimerCount = timers.length;
const timeoutRun = hooks.event({event: {type: "session.idle", properties: {sessionID: "timeout-session"}}});
while (timers.length === timeoutTimerCount) await Promise.resolve();
timers.at(-1).callback();
await timeoutRun;
if (logs.length !== 4 || prompts.length !== 4 || !logs[3].body.message.includes("timeout")) throw new Error("timeout was not surfaced conservatively: " + JSON.stringify(logs));
if (groupSignals.length !== 1 || groupSignals[0].signal !== "SIGKILL" || directKills.length !== 0) throw new Error("timeout did not terminate and await the adapter process group: " + JSON.stringify({groupSignals, directKills}));

results.push({exitCode: null, signalCode: "SIGKILL", stdout: new Uint8Array(), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "signal-session"}}});
if (logs.length !== 5 || prompts.length !== 5 || !logs[4].body.message.includes("SIGKILL")) throw new Error("signal was not surfaced conservatively: " + JSON.stringify(logs));

results.push({exitCode: 0, stdout: [encoded("x".repeat(40000)), encoded("x".repeat(30000))], stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "oversize-session"}}});
if (logs.length !== 6 || prompts.length !== 6 || !logs[5].body.message.includes("exceeded")) throw new Error("oversized adapter output was not surfaced conservatively: " + JSON.stringify(logs));
if (groupSignals.length !== 2 || groupSignals[1].signal !== "SIGKILL" || directKills.length !== 0) throw new Error("streamed overflow did not terminate and await the adapter process group: " + JSON.stringify({groupSignals, directKills}));

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

const callsBeforePendingIdle = calls.length;
const promptsBeforePendingIdle = prompts.length;
results.push({pending: true, stdout: encoded('{"decision":"block","systemMessage":"async repair"}'), stderr: new Uint8Array()});
const pendingIdle = hooks.event({event: {type: "session.idle", properties: {sessionID: "pending-session"}}});
while (calls.length === callsBeforePendingIdle) await Promise.resolve();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "pending-session"}}});
if (calls.length !== callsBeforePendingIdle + 1) throw new Error("pending streamed adapter admitted a duplicate idle run");
processes.at(-1).finish();
await pendingIdle;
if (prompts.length !== promptsBeforePendingIdle + 1 || !prompts.at(-1).body.parts[0].text.includes("async repair")) throw new Error("pending streamed adapter did not complete one bounded follow-up");

const signalsBeforeStderrLimit = groupSignals.length;
const logsBeforeStderrLimit = logs.length;
results.push({exitCode: 0, stdout: new Uint8Array(), stderr: [encoded("e".repeat(40000)), encoded("e".repeat(30000))]});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "stderr-limit-session"}}});
if (groupSignals.length !== signalsBeforeStderrLimit + 1 || groupSignals.at(-1).signal !== "SIGKILL" || directKills.length !== 0) throw new Error("stderr overflow did not terminate the adapter process group");
if (logs.length !== logsBeforeStderrLimit + 1 || !logs.at(-1).body.message.includes("stderr exceeded")) throw new Error("stderr overflow was not surfaced conservatively: " + JSON.stringify(logs.at(-1)));
`
	command := exec.Command(node, "--input-type=module", "-e", script, plugin, root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode plugin transport: %v\n%s", err, output)
	}
}

func TestOpenCodeSPECContractPluginTerminatesProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenCode source transport deliberately requires POSIX process groups")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to exercise the OpenCode process-group contract")
	}
	root := repoRoot(t)
	fixtureDir := t.TempDir()
	pidFile := filepath.Join(fixtureDir, "processes")
	fakeGo := filepath.Join(fixtureDir, "go")
	const fakeGoScript = `#!/bin/sh
set -eu
/bin/sh -c 'trap "" TERM; i=0; while test "$i" -lt 8000; do printf 0123456789; i=$((i + 1)); done; while :; do /bin/sleep 60; done' &
child=$!
printf '%s %s\n' "$$" "$child" > "$OPENCODE_DESCENDANT_PID_FILE"
wait "$child"
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		data, readErr := os.ReadFile(pidFile)
		if readErr != nil {
			return
		}
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			_ = exec.Command("/bin/kill", "-KILL", "-"+fields[0]).Run()
		}
	})

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.mjs")
	script := `
import {readFileSync} from "node:fs";
import {spawn} from "node:child_process";
import {Readable} from "node:stream";
import {pathToFileURL} from "node:url";
const logs = [];
const prompts = [];
globalThis.Bun = {spawn(args, options) {
  if (options.detached !== true) throw new Error("adapter was not isolated in a process group");
  const child = spawn(args[0], args.slice(1), {cwd: options.cwd, detached: true, env: process.env, stdio: ["ignore", "pipe", "pipe"]});
  let signalCode = null;
  const exited = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => { signalCode = signal; resolve(code ?? 1); });
  });
  return {pid: child.pid, stdout: Readable.toWeb(child.stdout), stderr: Readable.toWeb(child.stderr), exited, get signalCode() { return signalCode; }, kill(signal) { child.kill(signal); }};
}};
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log(entry) { logs.push(entry); }}, session: {async promptAsync(entry) { prompts.push(entry); }}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const started = Date.now();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "process-group"}}});
if (Date.now() - started > 5000) throw new Error("overflow termination exceeded five seconds");
if (logs.length !== 1 || !logs[0].body.message.includes("exceeded") || prompts.length !== 1) throw new Error("overflow was not surfaced: " + JSON.stringify({logs, prompts}));
const [leader] = readFileSync(process.env.OPENCODE_DESCENDANT_PID_FILE, "utf8").trim().split(/\s+/).map(Number);
let alive = true;
for (let attempt = 0; attempt < 100; attempt++) {
  try { process.kill(-leader, 0); } catch { alive = false; break; }
  await new Promise((resolve) => setTimeout(resolve, 10));
}
if (alive) throw new Error("adapter process group survived overflow termination");
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, root)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PATH=") && !strings.HasPrefix(entry, "OPENCODE_DESCENDANT_PID_FILE=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env,
		"PATH="+fixtureDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCODE_DESCENDANT_PID_FILE="+pidFile,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode process-group transport: %v\n%s", err, output)
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
