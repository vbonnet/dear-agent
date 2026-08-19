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
	"slices"
	"strings"
	"testing"
	"time"
)

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
		tombstone.DearAgent.Replacement != ".opencode/plugins/spec-contract-guard.js" || tombstone.DearAgent.RuntimeClaim != "none" {
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
	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {pathToFileURL} from "node:url";
const calls = [];
const logs = [];
const prompts = [];
const groupSignals = [];
const directKills = [];
const processes = [];
const statusStreams = new Map();
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
  return {
    stream,
    close() { try { controller.close(); } catch {} },
    closeWith(value) { try { if (value?.byteLength) controller.enqueue(value); controller.close(); } catch {} },
  };
};
process.kill = (pid, signal) => {
  groupSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
globalThis.Bun = {
  file(fd) {
    const stream = statusStreams.get(fd);
    if (!stream) throw new Error("unknown supervisor status descriptor " + fd);
    return {stream() { return stream; }};
  },
  spawn(args, options) {
    calls.push([args, options]);
    const result = results.shift() || {exitCode: 0, stdout: encoded('{"decision":"block","systemMessage":"repair the contract"}'), stderr: new Uint8Array()};
    const stdout = controlledStream(result.stdout, Boolean(result.pending));
    const stderr = controlledStream(result.stderr, Boolean(result.pending));
    const status = controlledStream(result.pending ? new Uint8Array() : encoded(String(result.exitCode ?? 0) + "\n"), Boolean(result.pending));
    const statusFD = 2000 + calls.length;
    statusStreams.set(statusFD, status.stream);
    let exitCode = null;
    let signalCode = null;
    let resolveExit;
    const exited = new Promise((resolve) => { resolveExit = resolve; });
    const proc = {
      pid: 1000 + calls.length,
      stdout: stdout.stream,
      stderr: stderr.stream,
      stdio: [null, null, null, statusFD],
      exited,
      get exitCode() { return exitCode; },
      get signalCode() { return signalCode; },
      cleanupRequests: 0,
      stdin: {
        write(value) { if (value !== "cleanup\n") throw new Error("invalid cleanup frame"); proc.cleanupRequests++; if (result.controlFailure) throw new Error("injected cleanup channel loss"); return value.length; },
        end() { if (result.controlFailure) throw new Error("injected cleanup EOF loss"); if (result.cleanupExitCode !== undefined) proc.exitLeader(result.cleanupExitCode); else if (!result.ignoreCleanup) proc.selfCleanup(); return 0; },
      },
      selfCleanup() { signalCode = "SIGKILL"; exitCode = null; resolveExit?.(1); },
      kill(signal) { directKills.push({signal, pid: this.pid}); this.selfCleanup(); },
      finish(code = 0) { status.closeWith?.(encoded(String(code) + "\n")); stdout.close(); stderr.close(); },
      exitLeader(code = 1) { exitCode = code; resolveExit?.(code); },
    };
    processes.push(proc);
    if (result.leaderExit) proc.exitLeader(result.exitCode ?? 1);
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
if (args.slice(0, 4).join("|") !== ["/bin/sh", "-c", args[2], "dear-agent-opencode-spec-supervisor"].join("|")) throw new Error("supervisor argv: " + JSON.stringify(args));
if (args.slice(4).join("|") !== ["go", "run", "./cmd/spec-contract-hook", "--root", process.argv[2], "--provider", "opencode", "--event", "Stop"].join("|")) throw new Error("adapter argv: " + JSON.stringify(args));
if (!args[2].includes('trap - HUP INT QUIT TERM PIPE') || !args[2].includes('exec 3>&-') || !args[2].includes('exec 0<&4') || !args[2].includes('exec "$@"') || !args[2].includes('kill -KILL 0') || args[2].includes("go run ./cmd/spec-contract-hook")) throw new Error("supervisor did not preserve its structural argv, signal, private-fd, and identity-relative cleanup contract");
if (options.cwd !== process.argv[2] || options.detached !== true || JSON.stringify(options.stdio.slice(0, 4)) !== JSON.stringify(["pipe", "pipe", "pipe", "pipe"]) || options.stdio.length !== 5 || !Number.isInteger(options.stdio[4]) || "stdin" in options || "stdout" in options || "stderr" in options || "timeout" in options || "maxBuffer" in options) throw new Error("adapter options: " + JSON.stringify(options));
if (timers.length !== 1 || timers[0].milliseconds !== 60000 || !timers[0].cleared) throw new Error("adapter timeout was not explicit and cleared: " + JSON.stringify(timers));
if (groupSignals.length !== 0 || directKills.length !== 0) throw new Error("successful adapter used parent-side numeric process signalling: " + JSON.stringify({groupSignals, directKills}));
if (processes[0].cleanupRequests !== 1) throw new Error("successful adapter did not issue exactly one supervisor-owned cleanup frame");
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
const signalsBeforeTimeout = groupSignals.length;
const timeoutRun = hooks.event({event: {type: "session.idle", properties: {sessionID: "timeout-session"}}});
while (timers.length === timeoutTimerCount) await Promise.resolve();
timers.at(-1).callback();
await timeoutRun;
if (logs.length !== 4 || prompts.length !== 4 || !logs[3].body.message.includes("timeout")) throw new Error("timeout was not surfaced conservatively: " + JSON.stringify(logs));
if (groupSignals.length !== signalsBeforeTimeout || directKills.length !== 0) throw new Error("timeout used parent-side numeric process signalling: " + JSON.stringify({groupSignals, directKills}));

results.push({exitCode: 7, stdout: encoded("ignored nonzero output"), stderr: encoded("exact adapter failure")});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "signal-session"}}});
if (logs.length !== 5 || prompts.length !== 5 || !logs[4].body.message.includes("exact adapter failure")) throw new Error("nonzero adapter status and stderr were not surfaced conservatively: " + JSON.stringify(logs));

results.push({exitCode: 0, stdout: [encoded("x".repeat(40000)), encoded("x".repeat(30000))], stderr: new Uint8Array()});
const signalsBeforeOverflow = groupSignals.length;
await hooks.event({event: {type: "session.idle", properties: {sessionID: "oversize-session"}}});
if (logs.length !== 6 || prompts.length !== 6 || !logs[5].body.message.includes("exceeded")) throw new Error("oversized adapter output was not surfaced conservatively: " + JSON.stringify(logs));
if (groupSignals.length !== signalsBeforeOverflow || directKills.length !== 0) throw new Error("streamed overflow used parent-side numeric process signalling: " + JSON.stringify({groupSignals, directKills}));

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
if (groupSignals.length !== signalsBeforeStderrLimit || directKills.length !== 0) throw new Error("stderr overflow used parent-side numeric process signalling");
if (logs.length !== logsBeforeStderrLimit + 1 || !logs.at(-1).body.message.includes("stderr exceeded")) throw new Error("stderr overflow was not surfaced conservatively: " + JSON.stringify(logs.at(-1)));

const signalsBeforeLostLeader = groupSignals.length;
const logsBeforeLostLeader = logs.length;
const promptsBeforeLostLeader = prompts.length;
results.push({leaderExit: true, exitCode: 23, pending: true, stdout: new Uint8Array(), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "lost-supervisor-session"}}});
if (groupSignals.length !== signalsBeforeLostLeader || directKills.length !== 0) throw new Error("lost supervisor identity was signalled by stale numeric PID: " + JSON.stringify({groupSignals, directKills}));
if (processes.at(-1).cleanupRequests !== 1) throw new Error("lost supervisor did not close its private control channel exactly once");
if (logs.length !== logsBeforeLostLeader + 1 || prompts.length !== promptsBeforeLostLeader + 1 || !logs.at(-1).body.message.includes("exited before reporting status")) throw new Error("lost supervisor identity was not surfaced conservatively: " + JSON.stringify(logs.at(-1)));

const logsBeforeReapFailure = logs.length;
const promptsBeforeReapFailure = prompts.length;
const reapFailureStarted = Date.now();
results.push({ignoreCleanup: true, exitCode: 0, stdout: encoded("{}"), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reap-failure-session"}}});
if (Date.now() - reapFailureStarted > 3000) throw new Error("lost supervisor reap exceeded its fixed bound");
if (processes.at(-1).cleanupRequests !== 1 || groupSignals.length !== signalsBeforeLostLeader || directKills.length !== 0) throw new Error("reap failure retried cleanup or used numeric signalling");
if (logs.length !== logsBeforeReapFailure + 1 || prompts.length !== promptsBeforeReapFailure + 1 || !logs.at(-1).body.message.includes("did not exit after supervisor-owned cleanup")) throw new Error("reap failure was masked by the adapter result: " + JSON.stringify(logs.at(-1)));

const logsBeforeUnexpectedCleanupExit = logs.length;
const promptsBeforeUnexpectedCleanupExit = prompts.length;
results.push({cleanupExitCode: 0, exitCode: 0, stdout: encoded("{}"), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "unexpected-cleanup-exit-session"}}});
if (processes.at(-1).cleanupRequests !== 1 || groupSignals.length !== signalsBeforeLostLeader || directKills.length !== 0) throw new Error("unexpected cleanup exit retried or used numeric signalling");
if (logs.length !== logsBeforeUnexpectedCleanupExit + 1 || prompts.length !== promptsBeforeUnexpectedCleanupExit + 1 || !logs.at(-1).body.message.includes("did not exit with SIGKILL")) throw new Error("unexpected supervisor cleanup exit was accepted: " + JSON.stringify(logs.at(-1)));

const logsBeforeControlLoss = logs.length;
const promptsBeforeControlLoss = prompts.length;
results.push({controlFailure: true, exitCode: 0, stdout: encoded("{}"), stderr: new Uint8Array()});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "cleanup-control-loss-session"}}});
if (processes.at(-1).cleanupRequests !== 1 || groupSignals.length !== signalsBeforeLostLeader || directKills.length !== 0) throw new Error("cleanup channel loss retried or used numeric signalling");
if (logs.length !== logsBeforeControlLoss + 1 || prompts.length !== promptsBeforeControlLoss + 1 || !logs.at(-1).body.message.includes("injected cleanup channel loss") || !logs.at(-1).body.message.includes("did not exit")) throw new Error("cleanup channel loss was not bounded and fail closed: " + JSON.stringify(logs.at(-1)));
`
	command := exec.Command(node, "--input-type=module", "-e", script, plugin, root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode plugin transport: %v\n%s", err, output)
	}
}

func TestOpenCodeSPECContractPluginUsesNativeDiscoveryExtension(t *testing.T) {
	root := repoRoot(t)
	want := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	var discovered []string
	for _, directory := range []string{"plugin", "plugins"} {
		entries, err := os.ReadDir(filepath.Join(root, ".opencode", directory))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			extension := filepath.Ext(entry.Name())
			if extension == ".js" || extension == ".ts" {
				discovered = append(discovered, filepath.Join(root, ".opencode", directory, entry.Name()))
			}
		}
	}
	if !slices.Contains(discovered, want) {
		t.Fatalf("OpenCode native project-plugin scan did not discover %s; discovered %v", want, discovered)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.mjs")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired unsupported .mjs plugin remains present: %v", err)
	}
}

func TestOpenCodeSPECContractPluginProjectsNativePreToolGuards(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenCode repository guard scripts require a POSIX shell")
	}
	root := repoRoot(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the OpenCode native pre-tool contract")
	}
	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {readFileSync} from "node:fs";
import {pathToFileURL} from "node:url";
const encoded = (value) => new TextEncoder().encode(value);
const allow = () => ({exitCode: 0, stdout: new Uint8Array(), stderr: new Uint8Array()});
const calls = [];
const results = [];
const toasts = [];
const logs = [];
const statusStreams = new Map();
const timers = [];
let holdToast = false;
globalThis.setTimeout = (callback, milliseconds) => { const timer = {callback, milliseconds, cleared: false}; timers.push(timer); return timer; };
globalThis.clearTimeout = (timer) => { timer.cleared = true; };
const controlledStream = (value, holdOpen) => {
  let controller;
  const stream = new ReadableStream({
    start(next) { controller = next; if (value?.byteLength) next.enqueue(value); if (!holdOpen) next.close(); },
  });
  return {stream, close() { try { controller.close(); } catch {} }};
};
globalThis.Bun = {
  file(fd) {
    const stream = statusStreams.get(fd);
    if (!stream) throw new Error("unknown supervisor status descriptor " + fd);
    return {stream() { return stream; }};
  },
  spawn(args, options) {
    const result = results.shift();
    if (!result) throw new Error("missing native guard result");
    const payload = readFileSync(options.stdio[4], "utf8");
    calls.push({args, options, payload});
    const stdout = controlledStream(result.stdout || new Uint8Array(), Boolean(result.pending));
    const stderr = controlledStream(result.stderr || new Uint8Array(), Boolean(result.pending));
    const status = controlledStream(result.pending ? new Uint8Array() : encoded(String(result.exitCode ?? 0) + "\n"), Boolean(result.pending));
    const statusFD = 3000 + calls.length;
    statusStreams.set(statusFD, status.stream);
    let signalCode = null;
    let resolveExit;
    const exited = new Promise((resolve) => { resolveExit = resolve; });
    const proc = {
      stdout: stdout.stream,
      stderr: stderr.stream,
      stdio: [null, null, null, statusFD],
      exited,
      get signalCode() { return signalCode; },
      cleanupRequests: 0,
      stdin: {
        write(value) { if (value !== "cleanup\n") throw new Error("invalid cleanup frame"); proc.cleanupRequests++; return value.length; },
        end() { signalCode = "SIGKILL"; resolveExit?.(1); return 0; },
      },
    };
    return proc;
  },
};
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {
  app: {async log(entry) { logs.push(entry); }},
  tui: {async showToast(entry) { toasts.push(entry); if (holdToast) await new Promise(() => {}); }},
  session: {async promptAsync() { throw new Error("pre-tool guard must not prompt the model"); }},
};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const before = hooks["tool.execute.before"];
if (typeof before !== "function") throw new Error("native tool.execute.before hook is absent");

results.push(allow(), allow(), allow(), allow());
await before({tool: "bash", sessionID: "safe-session", callID: "safe-call"}, {args: {command: "go test ./..."}});
if (calls.length !== 4) throw new Error("safe Bash call did not traverse four guards: " + calls.length);
const expectedPaths = [
  ".opencode/hooks/pretool-spawn-routing",
  ".opencode/hooks/pretool-bead-close-guard",
  ".opencode/hooks/pretool-bypass-guard",
  ".opencode/hooks/pretool-pr-guard",
];
const expectedTimeouts = [5000, 30000, 5000, 5000];
for (let index = 0; index < 4; index++) {
  const call = calls[index];
  if (call.args.slice(4).join("|") !== [process.argv[2] + "/" + expectedPaths[index]].join("|")) throw new Error("guard argv: " + JSON.stringify(call.args));
  if (call.options.cwd !== process.argv[2] || call.options.detached !== true || JSON.stringify(call.options.stdio.slice(0, 4)) !== JSON.stringify(["pipe", "pipe", "pipe", "pipe"]) || call.options.stdio.length !== 5 || !Number.isInteger(call.options.stdio[4]) || "timeout" in call.options || "maxBuffer" in call.options) throw new Error("guard supervisor bounds: " + JSON.stringify(call.options));
  if (!call.args[2].includes('exec 0<&4') || !call.args[2].includes('kill -KILL 0')) throw new Error("guard did not use the trusted payload and cleanup supervisor");
  const payload = JSON.parse(call.payload);
  if (payload.tool_name !== "Bash" || payload.tool_input.command !== "go test ./...") throw new Error("legacy guard envelope: " + JSON.stringify(payload));
  if (timers[index].milliseconds !== expectedTimeouts[index] || !timers[index].cleared) throw new Error("guard deadline was not explicit and cleared: " + JSON.stringify(timers[index]));
}
if (toasts.length !== 0 || logs.length !== 0) throw new Error("safe guards produced feedback");

results.push({exitCode: 0, stdout: encoded(JSON.stringify({hookSpecificOutput: {additionalContext: "route this launch through AGM"}})), stderr: new Uint8Array()});
holdToast = true;
await before({tool: "scheduled-tasks_create_scheduled_task", sessionID: "route-session", callID: "route-call"}, {args: {name: "nightly"}});
if (calls.length !== 5 || !calls.at(-1).args.at(-1).endsWith(expectedPaths[0])) throw new Error("scheduled-task routing used the wrong guard set");
if (toasts.length !== 1 || toasts[0].body.variant !== "warning" || !toasts[0].body.message.includes("through AGM")) throw new Error("routing reminder did not preserve non-blocking native feedback: " + JSON.stringify(toasts));
const scheduledPayload = JSON.parse(calls.at(-1).payload);
if (scheduledPayload.tool_name !== "mcp__scheduled-tasks__create_scheduled_task" || scheduledPayload.tool_input.name !== "nightly") throw new Error("scheduled-task guard envelope: " + JSON.stringify(scheduledPayload));
holdToast = false;

results.push(
  allow(),
  allow(),
  {exitCode: 2, stdout: encoded(JSON.stringify({hookSpecificOutput: {permissionDecision: "deny", additionalContext: "use the approved wrapper"}})), stderr: new Uint8Array()},
);
let denied = "";
try {
  await before({tool: "bash", sessionID: "deny-session", callID: "deny-call"}, {args: {command: "git commit --no-verify"}});
} catch (error) { denied = String(error?.message || error); }
if (!denied.includes("approved wrapper")) throw new Error("native denial was not preserved: " + denied);
if (calls.length !== 8) throw new Error("denied guard allowed later scripts to run: " + calls.length);

results.push(
  allow(),
  allow(),
  allow(),
  {exitCode: 2, stdout: new Uint8Array(), stderr: encoded("route raw gh lifecycle through safe-pr")},
);
let stderrDenied = "";
try {
  await before({tool: "bash", sessionID: "pr-deny-session", callID: "pr-deny-call"}, {args: {command: "gh pr create"}});
} catch (error) { stderrDenied = String(error?.message || error); }
if (!stderrDenied.includes("safe-pr")) throw new Error("stderr-only native denial was not preserved: " + stderrDenied);
if (calls.length !== 12) throw new Error("stderr-denied guard used the wrong script sequence: " + calls.length);

results.push(
  allow(),
  {exitCode: 0, stdout: encoded(JSON.stringify({hookSpecificOutput: {additionalContext: "bead-close-guard error (failing open): dependency unavailable"}})), stderr: new Uint8Array()},
  allow(),
  allow(),
);
await before({tool: "bash", sessionID: "advisory-session", callID: "advisory-call"}, {args: {command: "bd close ce-test"}});
if (calls.length !== 16 || toasts.length !== 2 || !toasts.at(-1).body.message.includes("failing open")) throw new Error("legacy advisory was upgraded to a block or lost: " + JSON.stringify(toasts));

results.push({exitCode: 0, stdout: encoded("null"), stderr: new Uint8Array()});
let malformed = "";
try {
  await before({tool: "bash", sessionID: "malformed-session", callID: "malformed-call"}, {args: {command: "echo safe"}});
} catch (error) { malformed = String(error?.message || error); }
if (!malformed.includes("non-object JSON response")) throw new Error("malformed guard output did not fail closed: " + malformed);
if (calls.length !== 17) throw new Error("malformed guard output allowed later scripts to run: " + calls.length);

results.push({exitCode: 0, stdout: encoded(JSON.stringify({hookSpecificOutput: {permissionDecision: "allow"}, permissionDecision: "deny"})), stderr: new Uint8Array()});
let conflicting = "";
try {
  await before({tool: "bash", sessionID: "conflicting-session", callID: "conflicting-call"}, {args: {command: "echo safe"}});
} catch (error) { conflicting = String(error?.message || error); }
if (!conflicting.includes("conflicting permission decisions")) throw new Error("conflicting guard decisions did not fail closed: " + conflicting);
if (calls.length !== 18) throw new Error("conflicting guard decisions allowed later scripts to run: " + calls.length);

results.push({pending: true});
const timerCount = timers.length;
let timedOut = "";
const timeoutRun = before({tool: "bash", sessionID: "timeout-session", callID: "timeout-call"}, {args: {command: "echo safe"}}).catch((error) => { timedOut = String(error?.message || error); });
while (timers.length === timerCount) await Promise.resolve();
timers.at(-1).callback();
await timeoutRun;
if (!timedOut.includes("failed (timeout)")) throw new Error("guard timeout did not fail closed: " + timedOut);

const beforeOversize = calls.length;
let oversize = "";
try {
  await before({tool: "bash", sessionID: "oversize-session", callID: "oversize-call"}, {args: {command: "x".repeat(1024 * 1024)}});
} catch (error) { oversize = String(error?.message || error); }
if (!oversize.includes("input exceeds 1048576 bytes") || calls.length !== beforeOversize) throw new Error("oversized guard input did not fail before launch: " + oversize);

const beforeUnknown = calls.length;
await before({tool: "read", sessionID: "read-session", callID: "read-call"}, {args: {filePath: "README.md"}});
if (calls.length !== beforeUnknown) throw new Error("unrelated tool executed Bash-only guards");
`
	command := exec.Command(node, "--input-type=module", "-e", script, plugin, root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode native pre-tool guard projection: %v\n%s", err, output)
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
	cleanupFile := filepath.Join(fixtureDir, "cleanup")
	fakeGo := filepath.Join(fixtureDir, "go")
	const fakeGoScript = `#!/bin/sh
set -eu
(
  while test ! -f "$OPENCODE_TEST_CLEANUP_FILE"; do /bin/sleep 0.05; done
  kill -KILL 0
) </dev/null >/dev/null 2>&1 &
/bin/sh -c 'trap "" TERM; i=0; while test "$i" -lt 8000; do printf 0123456789; i=$((i + 1)); done; while :; do /bin/sleep 60; done' &
child=$!
printf '%s %s %s\n' "$PPID" "$$" "$child" > "$OPENCODE_DESCENDANT_PID_FILE"
wait "$child"
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(cleanupFile, []byte("cleanup\n"), 0o600)
		time.Sleep(100 * time.Millisecond)
	})

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {readFileSync} from "node:fs";
import {spawn} from "node:child_process";
import {Readable} from "node:stream";
import {pathToFileURL} from "node:url";
const logs = [];
const prompts = [];
const cleanupSignals = [];
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  if (signal !== 0) cleanupSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
let supervisorPID = 0;
const statusStreams = new Map();
globalThis.Bun = {
file(fd) { return {stream() { const stream = statusStreams.get(fd); if (!stream) throw new Error("unknown status fd"); return stream; }}; },
spawn(args, options) {
  if (options.detached !== true) throw new Error("adapter was not isolated in a process group");
  if (JSON.stringify(options.stdio.slice(0, 4)) !== JSON.stringify(["pipe", "pipe", "pipe", "pipe"]) || options.stdio.length !== 5 || !Number.isInteger(options.stdio[4])) throw new Error("adapter did not request private control/status/payload descriptors");
  const child = spawn(args[0], args.slice(1), {cwd: options.cwd, detached: true, env: process.env, stdio: ["pipe", "pipe", "pipe", "pipe", options.stdio[4]]});
  supervisorPID = child.pid;
  const statusFD = child.pid + 100000;
  statusStreams.set(statusFD, Readable.toWeb(child.stdio[3]));
  let signalCode = null;
  const exited = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => { signalCode = signal; resolve(code ?? 1); });
  });
  return {pid: child.pid, stdin: child.stdin, stdout: Readable.toWeb(child.stdout), stderr: Readable.toWeb(child.stderr), stdio: [null, null, null, statusFD], exited, get signalCode() { return signalCode; }};
}};
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log(entry) { logs.push(entry); }}, session: {async promptAsync(entry) { prompts.push(entry); }}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const started = Date.now();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "process-group"}}});
if (Date.now() - started > 5000) throw new Error("overflow termination exceeded five seconds");
if (logs.length !== 1 || !logs[0].body.message.includes("exceeded") || prompts.length !== 1) throw new Error("overflow was not surfaced: " + JSON.stringify({logs, prompts}));
const [recordedSupervisor, adapter, descendant] = readFileSync(process.env.OPENCODE_DESCENDANT_PID_FILE, "utf8").trim().split(/\s+/).map(Number);
if (recordedSupervisor !== supervisorPID) throw new Error("fixture did not run beneath the pinned supervisor: " + JSON.stringify({recordedSupervisor, supervisorPID}));
if (cleanupSignals.length !== 0) throw new Error("cleanup used parent-side numeric process signalling: " + JSON.stringify(cleanupSignals));
for (const pid of [supervisorPID, adapter, descendant]) {
  let alive = true;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { originalProcessKill(pid, 0); } catch { alive = false; break; }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  if (alive) throw new Error("contained adapter process survived overflow termination: " + pid);
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, root)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PATH=") && !strings.HasPrefix(entry, "OPENCODE_DESCENDANT_PID_FILE=") &&
			!strings.HasPrefix(entry, "OPENCODE_TEST_CLEANUP_FILE=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env,
		"PATH="+fixtureDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCODE_DESCENDANT_PID_FILE="+pidFile,
		"OPENCODE_TEST_CLEANUP_FILE="+cleanupFile,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode process-group transport: %v\n%s", err, output)
	}
}

func TestOpenCodeSPECContractPluginBoundsEscapedOutputWithoutStaleGroupSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenCode source transport deliberately requires POSIX process groups")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to exercise the OpenCode supervisor contract")
	}
	root := repoRoot(t)
	fixtureDir := t.TempDir()
	pidFile := filepath.Join(fixtureDir, "processes")
	cleanupFile := filepath.Join(fixtureDir, "cleanup")
	fixture := filepath.Join(fixtureDir, "escaped-pipe.cjs")
	fakeGo := filepath.Join(fixtureDir, "go")
	const fixtureScript = `
const {writeFileSync, writeSync} = require("node:fs");
const {spawn} = require("node:child_process");
const escaped = spawn("/bin/sh", ["-c", "trap '' TERM; while test ! -f \"$OPENCODE_ESCAPE_CLEANUP_FILE\"; do /bin/sleep 0.05; done"], {
  detached: true,
  stdio: ["ignore", 1, 2],
});
writeFileSync(process.env.OPENCODE_ESCAPE_PID_FILE, process.ppid + " " + process.pid + " " + escaped.pid + "\n");
writeSync(1, "ignored nonzero stdout\n");
writeSync(2, "exact escaped-pipe failure\n");
escaped.unref();
process.exit(7);
`
	if err := os.WriteFile(fixture, []byte(fixtureScript), 0o600); err != nil {
		t.Fatal(err)
	}
	const fakeGoScript = `#!/bin/sh
exec "$OPENCODE_NODE" "$OPENCODE_ESCAPE_FIXTURE"
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(cleanupFile, []byte("cleanup\n"), 0o600)
		time.Sleep(100 * time.Millisecond)
	})

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {readFileSync, writeFileSync} from "node:fs";
import {spawn} from "node:child_process";
import {Readable} from "node:stream";
import {pathToFileURL} from "node:url";
const logs = [];
const prompts = [];
const cleanupSignals = [];
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  if (signal !== 0) cleanupSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
let supervisorPID = 0;
const statusStreams = new Map();
globalThis.Bun = {
file(fd) { return {stream() { const stream = statusStreams.get(fd); if (!stream) throw new Error("unknown status fd"); return stream; }}; },
spawn(args, options) {
  if (JSON.stringify(options.stdio.slice(0, 4)) !== JSON.stringify(["pipe", "pipe", "pipe", "pipe"]) || options.stdio.length !== 5 || !Number.isInteger(options.stdio[4])) throw new Error("adapter did not request private control/status/payload descriptors");
  const child = spawn(args[0], args.slice(1), {cwd: options.cwd, detached: options.detached, env: process.env, stdio: ["pipe", "pipe", "pipe", "pipe", options.stdio[4]]});
  supervisorPID = child.pid;
  const statusFD = child.pid + 100000;
  statusStreams.set(statusFD, Readable.toWeb(child.stdio[3]));
  let signalCode = null;
  const exited = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => { signalCode = signal; resolve(code ?? 1); });
  });
  return {pid: child.pid, stdin: child.stdin, stdout: Readable.toWeb(child.stdout), stderr: Readable.toWeb(child.stderr), stdio: [null, null, null, statusFD], exited, get signalCode() { return signalCode; }};
}};
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log(entry) { logs.push(entry); }}, session: {async promptAsync(entry) { prompts.push(entry); }}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const started = Date.now();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "escaped-pipe"}}});
const elapsed = Date.now() - started;
if (elapsed > 5000) throw new Error("escaped output pipe exceeded bounded settlement: " + elapsed);
if (logs.length !== 1 || prompts.length !== 1 || !logs[0].body.message.includes("exact escaped-pipe failure")) throw new Error("real adapter status/stderr were not preserved: " + JSON.stringify({logs, prompts}));
const [recordedSupervisor, adapter, escaped] = readFileSync(process.env.OPENCODE_ESCAPE_PID_FILE, "utf8").trim().split(/\s+/).map(Number);
if (recordedSupervisor !== supervisorPID) throw new Error("real adapter was not owned by the pinned supervisor: " + JSON.stringify({recordedSupervisor, supervisorPID}));
if (cleanupSignals.length !== 0) throw new Error("cleanup used a stale adapter or other parent-side numeric PID: " + JSON.stringify({cleanupSignals, supervisorPID, adapter}));
for (const pid of [supervisorPID, adapter]) {
  let alive = true;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { originalProcessKill(pid, 0); } catch { alive = false; break; }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  if (alive) throw new Error("contained process survived supervisor cleanup: " + pid);
}
originalProcessKill(-escaped, 0);
writeFileSync(process.env.OPENCODE_ESCAPE_CLEANUP_FILE, "cleanup\n", {mode: 0o600});
let escapedAlive = true;
for (let attempt = 0; attempt < 100; attempt++) {
  try { originalProcessKill(escaped, 0); } catch { escapedAlive = false; break; }
  await new Promise((resolve) => setTimeout(resolve, 10));
}
if (escapedAlive) throw new Error("escaped fixture ignored its identity-free cleanup request");
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, root)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PATH=") && !strings.HasPrefix(entry, "OPENCODE_NODE=") &&
			!strings.HasPrefix(entry, "OPENCODE_ESCAPE_FIXTURE=") && !strings.HasPrefix(entry, "OPENCODE_ESCAPE_PID_FILE=") &&
			!strings.HasPrefix(entry, "OPENCODE_ESCAPE_CLEANUP_FILE=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env,
		"PATH="+fixtureDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCODE_NODE="+node,
		"OPENCODE_ESCAPE_FIXTURE="+fixture,
		"OPENCODE_ESCAPE_PID_FILE="+pidFile,
		"OPENCODE_ESCAPE_CLEANUP_FILE="+cleanupFile,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode escaped-pipe supervisor transport: %v\n%s", err, output)
	}
}

func TestOpenCodeSPECContractPluginUsesOnlySupervisorOwnedCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenCode source transport deliberately requires POSIX process groups")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to exercise the OpenCode supervisor contract")
	}
	root := repoRoot(t)
	fixtureDir := t.TempDir()
	cleanupFile := filepath.Join(fixtureDir, "cleanup")
	fakeGo := filepath.Join(fixtureDir, "go")
	const fakeGoScript = `#!/bin/sh
(
  while test ! -f "$OPENCODE_TEST_CLEANUP_FILE"; do /bin/sleep 0.05; done
  kill -KILL 0
) </dev/null >/dev/null 2>&1 &
printf '{}'
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(cleanupFile, []byte("cleanup\n"), 0o600)
		time.Sleep(100 * time.Millisecond)
	})

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {spawn} from "node:child_process";
import {Readable} from "node:stream";
import {pathToFileURL} from "node:url";
const logs = [];
const prompts = [];
const attemptedSignals = [];
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  if (signal !== 0) attemptedSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
let supervisorPID = 0;
const statusStreams = new Map();
globalThis.Bun = {
file(fd) { return {stream() { const stream = statusStreams.get(fd); if (!stream) throw new Error("unknown status fd"); return stream; }}; },
spawn(args, options) {
  if (JSON.stringify(options.stdio.slice(0, 4)) !== JSON.stringify(["pipe", "pipe", "pipe", "pipe"]) || options.stdio.length !== 5 || !Number.isInteger(options.stdio[4])) throw new Error("adapter did not request private control/status/payload descriptors");
  const child = spawn(args[0], args.slice(1), {cwd: options.cwd, detached: options.detached, env: process.env, stdio: ["pipe", "pipe", "pipe", "pipe", options.stdio[4]]});
  supervisorPID = child.pid;
  const statusFD = child.pid + 100000;
  statusStreams.set(statusFD, Readable.toWeb(child.stdio[3]));
  let signalCode = null;
  const exited = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => { signalCode = signal; resolve(code ?? 1); });
  });
  return {pid: child.pid, stdin: child.stdin, stdout: Readable.toWeb(child.stdout), stderr: Readable.toWeb(child.stderr), stdio: [null, null, null, statusFD], exited, get signalCode() { return signalCode; }};
}};
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log(entry) { logs.push(entry); }}, session: {async promptAsync(entry) { prompts.push(entry); }}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const started = Date.now();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "self-cleanup"}}});
if (Date.now() - started > 5000) throw new Error("supervisor self-cleanup exceeded bounded settlement");
if (logs.length !== 0 || prompts.length !== 0) throw new Error("clean adapter result was not preserved: " + JSON.stringify({logs, prompts}));
if (attemptedSignals.length !== 0) throw new Error("parent used numeric process signalling: " + JSON.stringify(attemptedSignals));
let alive = true;
for (let attempt = 0; attempt < 100; attempt++) {
  try { originalProcessKill(supervisorPID, 0); } catch { alive = false; break; }
  await new Promise((resolve) => setTimeout(resolve, 10));
}
if (alive) throw new Error("supervisor ignored its private self-cleanup request");
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, root)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PATH=") && !strings.HasPrefix(entry, "OPENCODE_TEST_CLEANUP_FILE=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env,
		"PATH="+fixtureDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCODE_TEST_CLEANUP_FILE="+cleanupFile,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode supervisor-owned cleanup: %v\n%s", err, output)
	}
}

func TestOpenCodeSPECContractPluginParentExitCleansDetachedSupervisorTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenCode source transport deliberately requires POSIX process groups")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to exercise the OpenCode parent-liveness contract")
	}
	root := repoRoot(t)
	fixtureDir := t.TempDir()
	pidFile := filepath.Join(fixtureDir, "processes")
	readyFile := filepath.Join(fixtureDir, "ready")
	cleanupFile := filepath.Join(fixtureDir, "cleanup")
	violationFile := filepath.Join(fixtureDir, "numeric-signal-violation")
	fakeGo := filepath.Join(fixtureDir, "go")
	const fakeGoScript = `#!/bin/sh
set -eu
(
  while test ! -f "$OPENCODE_TEST_CLEANUP_FILE"; do /bin/sleep 0.05; done
  kill -KILL 0
) </dev/null >/dev/null 2>&1 &
/bin/sh -c 'while :; do /bin/sleep 60; done' &
child=$!
printf '%s %s %s\n' "$PPID" "$$" "$child" > "$OPENCODE_PARENT_EXIT_PID_FILE"
wait "$child"
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(cleanupFile, []byte("cleanup\n"), 0o600)
		time.Sleep(100 * time.Millisecond)
	})

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {existsSync, writeFileSync} from "node:fs";
import {spawn} from "node:child_process";
import {Readable} from "node:stream";
import {setTimeout as delay} from "node:timers/promises";
import {pathToFileURL} from "node:url";
process.kill = (pid, signal) => {
  writeFileSync(process.env.OPENCODE_NUMERIC_SIGNAL_VIOLATION, JSON.stringify({pid, signal}));
  throw new Error("parent numeric signalling is forbidden");
};
const statusStreams = new Map();
globalThis.Bun = {
  file(fd) { return {stream() { const stream = statusStreams.get(fd); if (!stream) throw new Error("unknown status fd"); return stream; }}; },
  spawn(args, options) {
    if (JSON.stringify(options.stdio.slice(0, 4)) !== JSON.stringify(["pipe", "pipe", "pipe", "pipe"]) || options.stdio.length !== 5 || !Number.isInteger(options.stdio[4])) throw new Error("adapter did not request private control/status/payload descriptors");
    const child = spawn(args[0], args.slice(1), {cwd: options.cwd, detached: options.detached, env: process.env, stdio: ["pipe", "pipe", "pipe", "pipe", options.stdio[4]]});
    const statusFD = child.pid + 100000;
    statusStreams.set(statusFD, Readable.toWeb(child.stdio[3]));
    let signalCode = null;
    const exited = new Promise((resolve, reject) => {
      child.once("error", reject);
      child.once("exit", (code, signal) => { signalCode = signal; resolve(code ?? 1); });
    });
    return {pid: child.pid, stdin: child.stdin, stdout: Readable.toWeb(child.stdout), stderr: Readable.toWeb(child.stderr), stdio: [null, null, null, statusFD], exited, get signalCode() { return signalCode; }};
  },
};
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log() {}}, session: {async promptAsync() {}}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
void hooks.event({event: {type: "session.idle", properties: {sessionID: "parent-exit"}}});
for (let attempt = 0; attempt < 500; attempt++) {
  if (existsSync(process.env.OPENCODE_PARENT_EXIT_PID_FILE)) {
    writeFileSync(process.env.OPENCODE_PARENT_EXIT_READY_FILE, "ready\n", {mode: 0o600});
    break;
  }
  await delay(10);
}
await new Promise(() => {});
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, root)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PATH=") && !strings.HasPrefix(entry, "OPENCODE_PARENT_EXIT_PID_FILE=") &&
			!strings.HasPrefix(entry, "OPENCODE_PARENT_EXIT_READY_FILE=") && !strings.HasPrefix(entry, "OPENCODE_TEST_CLEANUP_FILE=") &&
			!strings.HasPrefix(entry, "OPENCODE_NUMERIC_SIGNAL_VIOLATION=") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env,
		"PATH="+fixtureDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"OPENCODE_PARENT_EXIT_PID_FILE="+pidFile,
		"OPENCODE_PARENT_EXIT_READY_FILE="+readyFile,
		"OPENCODE_TEST_CLEANUP_FILE="+cleanupFile,
		"OPENCODE_NUMERIC_SIGNAL_VIOLATION="+violationFile,
	)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	ready := false
	for range 500 {
		if _, err := os.Stat(readyFile); err == nil {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("OpenCode parent did not reach detached supervisor fixture\n%s", output.String())
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	if data, err := os.ReadFile(violationFile); err == nil {
		t.Fatalf("plugin used parent-side numeric signalling before parent exit: %s", data)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(data))
	if len(fields) != 3 {
		t.Fatalf("parent-exit process fixture = %q, want supervisor adapter descendant", data)
	}
	for _, pid := range fields {
		alive := true
		for range 200 {
			if err := exec.Command("/bin/kill", "-0", pid).Run(); err != nil {
				alive = false
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if alive {
			t.Fatalf("detached OpenCode process survived parent control-pipe EOF: %s", pid)
		}
	}
}
