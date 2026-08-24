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
	"strconv"
	"strings"
	"testing"
	"time"
)

// adapterFixtureScript is a real Node subprocess that stands in for the
// trusted Go SPEC contract adapter (or a native pre-tool guard script) in
// tests. The plugin now spawns adapters through node:child_process.spawn
// directly rather than through a mockable globalThis.Bun indirection, so
// these tests drive real OS processes instead of intercepting the spawn
// call. Each invocation reads a JSON "plan" describing what to emit, and
// appends one record of its own argv/cwd/stdin to a shared NDJSON log so
// the JS test driver can observe what actually ran.
const adapterFixtureScript = `import {existsSync, readFileSync, writeFileSync, appendFileSync} from "node:fs";
import {setTimeout as delay} from "node:timers/promises";

const planPath = process.env.OPENCODE_FIXTURE_PLAN;
const recordPath = process.env.OPENCODE_FIXTURE_RECORD;
const forceExitPath = process.env.OPENCODE_FIXTURE_FORCE_EXIT;

const plan = JSON.parse(readFileSync(planPath, "utf8"));
let stdin = "";
try {
  stdin = readFileSync(0, "utf8");
} catch {
  // No payload descriptor was inherited for this call.
}
if (recordPath) {
  appendFileSync(recordPath, JSON.stringify({label: plan.label, argv: process.argv.slice(2), cwd: process.cwd(), stdin, pid: process.pid, ppid: process.ppid}) + "\n");
}
if (plan.selfDestructGroup) {
  const {execSync} = await import("node:child_process");
  try {
    execSync("kill -KILL 0");
  } catch {
    // The process group may already be gone.
  }
}
if (plan.stdout) process.stdout.write(plan.stdout);
if (plan.stderr) process.stderr.write(plan.stderr);
if (plan.wait) {
  while (!existsSync(plan.wait) && !(forceExitPath && existsSync(forceExitPath))) {
    await delay(20);
  }
}
process.exitCode = plan.exitCode ?? 0;
`

// writeAdapterFixture installs adapterFixtureScript plus a "go" wrapper on
// a fresh PATH-prepended fixture directory, and returns the plan/record
// file paths the JS driver uses to script and observe each real adapter
// invocation. t.Cleanup arranges for any process left blocked on plan.wait
// to unstick once the test ends.
func writeAdapterFixture(t *testing.T, node string) (fixtureDir, planFile, recordFile string, env []string) {
	t.Helper()
	fixtureDir = t.TempDir()
	planFile = filepath.Join(fixtureDir, "plan.json")
	recordFile = filepath.Join(fixtureDir, "calls.ndjson")
	forceExitFile := filepath.Join(fixtureDir, "force-exit")
	fixtureScript := filepath.Join(fixtureDir, "adapter-fixture.mjs")
	if err := os.WriteFile(fixtureScript, []byte(adapterFixtureScript), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeGo := filepath.Join(fixtureDir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nexec \"$OPENCODE_NODE\" \"$OPENCODE_ADAPTER_FIXTURE\" \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(forceExitFile, []byte("exit\n"), 0o600)
		time.Sleep(150 * time.Millisecond)
	})
	env = []string{
		"PATH=" + fixtureDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"OPENCODE_NODE=" + node,
		"OPENCODE_ADAPTER_FIXTURE=" + fixtureScript,
		"OPENCODE_FIXTURE_PLAN=" + planFile,
		"OPENCODE_FIXTURE_RECORD=" + recordFile,
		"OPENCODE_FIXTURE_FORCE_EXIT=" + forceExitFile,
	}
	return fixtureDir, planFile, recordFile, env
}

// writeGuardFixtures installs a real, controllable pre-tool guard script at
// each of the four fixed native guard paths under worktreeRoot/.opencode/hooks,
// each wrapping adapterFixtureScript with its own hardcoded plan-file path
// (guard scripts receive no argv from the plugin, so per-guard behavior must
// be selected by which wrapper file ran, not by an argument). It returns the
// plan file path for each guard name, keyed by the fixed relative guard path
// used in preToolGuards.
func writeGuardFixtures(t *testing.T, fixtureDir string, worktreeRoot string) map[string]string {
	t.Helper()
	hooksDir := filepath.Join(worktreeRoot, ".opencode", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	guardNames := []string{
		"pretool-spawn-routing",
		"pretool-bead-close-guard",
		"pretool-bypass-guard",
		"pretool-pr-guard",
	}
	plans := make(map[string]string, len(guardNames))
	for _, name := range guardNames {
		planPath := filepath.Join(fixtureDir, name+".plan.json")
		plans[".opencode/hooks/"+name] = planPath
		wrapper := "#!/bin/sh\nexec env OPENCODE_FIXTURE_PLAN=\"" + planPath + "\" \"$OPENCODE_NODE\" \"$OPENCODE_ADAPTER_FIXTURE\"\n"
		if err := os.WriteFile(filepath.Join(hooksDir, name), []byte(wrapper), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return plans
}

// runNodeFixtureScript runs a real Node process executing script (a
// --input-type=module -e body) against plugin with worktreeArg as its
// second CLI argument. It prepends fixtureDir to PATH, strips any
// inherited environment entries named in stripEnvKeys, and appends
// extraEnv. Several of the tests below drive a real supervisor and
// adapter/guard subprocess tree and share this exact plumbing.
func runNodeFixtureScript(t *testing.T, node, plugin, worktreeArg, script, fixtureDir string, stripEnvKeys, extraEnv []string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, worktreeArg)
	skip := append([]string{"PATH"}, stripEnvKeys...)
	for _, entry := range os.Environ() {
		keep := true
		for _, key := range skip {
			if strings.HasPrefix(entry, key+"=") {
				keep = false
				break
			}
		}
		if keep {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "PATH="+fixtureDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	command.Env = append(command.Env, extraEnv...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	return output
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
	_, _, _, fixtureEnv := writeAdapterFixture(t, node)
	script := `
import {existsSync, readFileSync, writeFileSync} from "node:fs";
import {pathToFileURL} from "node:url";
import {setTimeout as delay} from "node:timers/promises";
const logs = [];
const prompts = [];
const groupSignals = [];
const directKills = [];
const timers = [];
let failNextLog = false;
let failNextPrompt = false;
let reenterPrompt = false;
let reenterLogSession = "";
globalThis.setTimeout = (callback, milliseconds) => { const timer = {callback, milliseconds, cleared: false}; timers.push(timer); return timer; };
globalThis.clearTimeout = (timer) => { timer.cleared = true; };
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  groupSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
const planFile = process.env.OPENCODE_FIXTURE_PLAN;
const recordFile = process.env.OPENCODE_FIXTURE_RECORD;
const supervisorSource = readFileSync(process.argv[1], "utf8");
const writePlan = (plan) => writeFileSync(planFile, JSON.stringify(plan));
const readCalls = () => {
  if (!existsSync(recordFile)) return [];
  return readFileSync(recordFile, "utf8").split("\n").filter(Boolean).map((line) => JSON.parse(line));
};
const waitForDeath = async (pid, timeoutMs) => {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try { originalProcessKill(pid, 0); } catch { return true; }
    await delay(10);
  }
  return false;
};
const blockPlan = {stdout: JSON.stringify({decision: "block", systemMessage: "repair the contract"}), exitCode: 0};
// This test now drives the plugin's real node:child_process.spawn path
// (not a mockable globalThis.Bun indirection): the "go" adapter is a real
// subprocess scripted through a plan file, and calls are observed through
// an NDJSON record the fixture appends to, not an in-process mock array.
// Three low-level supervisor-object pathologies from the old mock
// (ignoreCleanup, cleanupExitCode, controlFailure) are intentionally
// dropped: they simulated a supervisor shell that refuses to honor a
// SIGKILL group-kill, which the real (unmodified) supervisorProgram makes
// unreachable, since SIGKILL cannot be trapped or ignored by any process
// in the group. That defensive runAdapter() code is exercised only by the
// four dedicated real-process tests in this file when applicable.
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {
    app: {async log(entry) { if (failNextLog) { failNextLog = false; throw new Error("log unavailable"); } logs.push(entry); if (reenterLogSession) { const sessionID = reenterLogSession; reenterLogSession = ""; await hooks.event({event: {type: "session.idle", properties: {sessionID}}}); } }},
    session: {async promptAsync(entry) { prompts.push(entry); if (reenterPrompt) { reenterPrompt = false; await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: entry.path.id, id: entry.body.messageID}}}}); } if (failNextPrompt) { failNextPrompt = false; throw new Error("prompt unavailable"); } }},
};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
writePlan(blockPlan);
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: "real-user-message-1"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
let calls = readCalls();
if (calls.length !== 1) throw new Error("session.idle did not run exactly one adapter: " + calls.length);
const firstCall = calls[0];
if (["go"].concat(firstCall.argv).join("|") !== ["go", "run", "./cmd/spec-contract-hook", "--root", process.argv[2], "--provider", "opencode", "--event", "Stop"].join("|")) throw new Error("adapter argv: " + JSON.stringify(firstCall.argv));
if (firstCall.cwd !== process.argv[2]) throw new Error("adapter cwd: " + firstCall.cwd);
if (firstCall.stdin !== "") throw new Error("idle event unexpectedly carried a non-empty payload: " + JSON.stringify(firstCall.stdin));
if (!supervisorSource.includes("trap - HUP INT QUIT TERM PIPE") || !supervisorSource.includes("exec 3>&-") || !supervisorSource.includes("exec 0<&4") || !supervisorSource.includes("exec \"$@\"") || !supervisorSource.includes("kill -KILL 0")) throw new Error("supervisor did not preserve its structural argv, signal, private-fd, and identity-relative cleanup contract");
if (timers.length !== 1 || timers[0].milliseconds !== 60000 || !timers[0].cleared) throw new Error("adapter timeout was not explicit and cleared: " + JSON.stringify(timers));
if (groupSignals.length !== 0 || directKills.length !== 0) throw new Error("successful adapter used parent-side numeric process signalling: " + JSON.stringify({groupSignals, directKills}));
if (!(await waitForDeath(firstCall.ppid, 3000))) throw new Error("successful adapter did not clean up its supervisor");
if (logs.length !== 1 || logs[0].body.level !== "warn" || !logs[0].body.message.includes("bounded follow-up")) throw new Error("block log: " + JSON.stringify(logs));
if (prompts.length !== 1 || prompts[0].path.id !== "session-1" || !prompts[0].body.messageID || !prompts[0].body.parts[0].text.includes("repair the contract")) throw new Error("bounded prompt: " + JSON.stringify(prompts));
if (prompts[0].throwOnError !== true) throw new Error("prompt transport did not require SDK errors to throw");

await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (readCalls().length !== 1 || prompts.length !== 1) throw new Error("same idle turn repeated its continuation");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: prompts[0].body.messageID}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (readCalls().length !== 1) throw new Error("synthetic prompt reset its own budget");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: "real-user-message-1"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (readCalls().length !== 1) throw new Error("delayed update of the current real user message reset the budget");
writePlan(blockPlan);
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: "real-user-message-2"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
calls = readCalls();
if (calls.length !== 2 || prompts.length !== 2) throw new Error("real user turn did not reset budget");
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "session-1", id: prompts[0].body.messageID}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
if (readCalls().length !== 2) throw new Error("delayed update of an older injected message reset the budget");

writePlan({stdout: "{}", exitCode: 0});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "clean-session"}}});
calls = readCalls();
if (calls.length !== 3 || prompts.length !== 2 || logs.length !== 2) throw new Error("clean result was not a no-op");

writePlan({stdout: "not json", exitCode: 0});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "malformed-session"}}});
if (logs.length !== 3 || prompts.length !== 3 || !logs[2].body.message.includes("invalid JSON")) throw new Error("invalid adapter output was not surfaced conservatively: " + JSON.stringify(logs));

// The timeout scenario fires its (mocked) adapter-timeout callback almost
// immediately, which can race a freshly spawned real Node subprocess's own
// startup: the supervisor's SIGKILL cleanup may reap it before it gets far
// enough to record itself. That race is specific to this instant-timeout
// scenario, so later call counts are expressed relative to a checkpoint
// taken after it settles rather than as further hardcoded literals.
writePlan({wait: planFile + ".never"});
const timeoutTimerCount = timers.length;
const signalsBeforeTimeout = groupSignals.length;
const timeoutRun = hooks.event({event: {type: "session.idle", properties: {sessionID: "timeout-session"}}});
while (timers.length === timeoutTimerCount) await Promise.resolve();
timers.at(-1).callback();
await timeoutRun;
if (logs.length !== 4 || prompts.length !== 4 || !logs[3].body.message.includes("timeout")) throw new Error("timeout was not surfaced conservatively: " + JSON.stringify(logs));
if (groupSignals.length !== signalsBeforeTimeout || directKills.length !== 0) throw new Error("timeout used parent-side numeric process signalling: " + JSON.stringify({groupSignals, directKills}));

writePlan({exitCode: 7, stdout: "ignored nonzero output", stderr: "exact adapter failure"});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "signal-session"}}});
if (logs.length !== 5 || prompts.length !== 5 || !logs[4].body.message.includes("exact adapter failure")) throw new Error("nonzero adapter status and stderr were not surfaced conservatively: " + JSON.stringify(logs));

writePlan({exitCode: 0, stdout: "x".repeat(70000)});
const signalsBeforeOverflow = groupSignals.length;
await hooks.event({event: {type: "session.idle", properties: {sessionID: "oversize-session"}}});
if (logs.length !== 6 || prompts.length !== 6 || !logs[5].body.message.includes("exceeded")) throw new Error("oversized adapter output was not surfaced conservatively: " + JSON.stringify(logs));
if (groupSignals.length !== signalsBeforeOverflow || directKills.length !== 0) throw new Error("streamed overflow used parent-side numeric process signalling: " + JSON.stringify({groupSignals, directKills}));

const callsAfterOversize = readCalls().length;
await hooks.event({event: {type: "session.deleted", properties: {info: {id: "session-1"}}}});
writePlan(blockPlan);
await hooks.event({event: {type: "session.idle", properties: {sessionID: "session-1"}}});
calls = readCalls();
if (calls.length !== callsAfterOversize + 1 || prompts.length !== 7) throw new Error("session deletion did not clear bounded state");

failNextLog = true;
writePlan(blockPlan);
await hooks.event({event: {type: "session.idle", properties: {sessionID: "log-failure-session"}}});
calls = readCalls();
if (calls.length !== callsAfterOversize + 2 || prompts.length !== 8) throw new Error("diagnostic failure suppressed the model-visible follow-up");

failNextPrompt = true;
writePlan(blockPlan);
await hooks.event({event: {type: "session.idle", properties: {sessionID: "prompt-failure-session"}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "prompt-failure-session"}}});
calls = readCalls();
if (calls.length !== callsAfterOversize + 3 || prompts.length !== 9) throw new Error("failed prompt attempt was not bounded for the real turn");
writePlan(blockPlan);
await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "prompt-failure-session", id: "new-real-user"}}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "prompt-failure-session"}}});
calls = readCalls();
if (calls.length !== callsAfterOversize + 4 || prompts.length !== 10) throw new Error("real user turn did not reset a failed prompt attempt");

reenterPrompt = true;
writePlan(blockPlan);
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-session"}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-session"}}});
calls = readCalls();
if (calls.length !== callsAfterOversize + 5 || prompts.length !== 11) throw new Error("reentrant synthetic message reset the bounded attempt");

reenterLogSession = "reentrant-log-session";
writePlan(blockPlan);
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-log-session"}}});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "reentrant-log-session"}}});
calls = readCalls();
if (calls.length !== callsAfterOversize + 6 || prompts.length !== 12) throw new Error("reentrant diagnostic idle duplicated the bounded attempt");

const callsBeforeExhaustion = readCalls().length;
const promptsBeforeExhaustion = prompts.length;
const logsBeforeExhaustion = logs.length;
for (let index = 0; index <= 4096; index++) {
  await hooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "exhausted-session", id: "real-exhaustion-" + index}}}});
}
await hooks.event({event: {type: "session.idle", properties: {sessionID: "exhausted-session"}}});
calls = readCalls();
if (calls.length !== callsBeforeExhaustion || prompts.length !== promptsBeforeExhaustion) throw new Error("capacity exhaustion executed another follow-up");
if (logs.length !== logsBeforeExhaustion + 1 || !logs.at(-1).body.message.includes("bounded message-identity limit")) throw new Error("capacity exhaustion was not disclosed once: " + JSON.stringify(logs.slice(logsBeforeExhaustion)));
await hooks.event({event: {type: "session.deleted", properties: {info: {id: "exhausted-session"}}}});
writePlan(blockPlan);
await hooks.event({event: {type: "session.idle", properties: {sessionID: "exhausted-session"}}});
calls = readCalls();
if (calls.length !== callsBeforeExhaustion + 1 || prompts.length !== promptsBeforeExhaustion + 1) throw new Error("deleted exhausted session identifier did not begin fresh");

const cappedHooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
for (let index = 0; index < 256; index++) {
  await cappedHooks.event({event: {type: "message.updated", properties: {info: {role: "user", sessionID: "capacity-session-" + index, id: "capacity-message-" + index}}}});
}
const callsBeforeSessionCapacity = readCalls().length;
const promptsBeforeSessionCapacity = prompts.length;
const logsBeforeSessionCapacity = logs.length;
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
calls = readCalls();
if (calls.length !== callsBeforeSessionCapacity || prompts.length !== promptsBeforeSessionCapacity) throw new Error("untracked over-capacity session executed a follow-up");
if (logs.length !== logsBeforeSessionCapacity + 1 || !logs.at(-1).body.message.includes("bounded session limit")) throw new Error("session capacity yield was not disclosed once: " + JSON.stringify(logs.slice(logsBeforeSessionCapacity)));
await cappedHooks.event({event: {type: "session.deleted", properties: {sessionID: "capacity-overflow"}}});
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
calls = readCalls();
if (calls.length !== callsBeforeSessionCapacity || prompts.length !== promptsBeforeSessionCapacity) throw new Error("deleting an untracked session evicted tracked state");
await cappedHooks.event({event: {type: "session.deleted", properties: {sessionID: "capacity-session-0"}}});
writePlan(blockPlan);
await cappedHooks.event({event: {type: "session.idle", properties: {sessionID: "capacity-overflow"}}});
calls = readCalls();
if (calls.length !== callsBeforeSessionCapacity + 1 || prompts.length !== promptsBeforeSessionCapacity + 1) throw new Error("tracked deletion did not deterministically admit the yielded session");

const callsBeforePendingIdle = readCalls().length;
const promptsBeforePendingIdle = prompts.length;
const pendingTrigger = planFile + ".pending-trigger";
writePlan({stdout: JSON.stringify({decision: "block", systemMessage: "async repair"}), exitCode: 0, wait: pendingTrigger});
const pendingIdle = hooks.event({event: {type: "session.idle", properties: {sessionID: "pending-session"}}});
while (readCalls().length === callsBeforePendingIdle) await delay(10);
await hooks.event({event: {type: "session.idle", properties: {sessionID: "pending-session"}}});
calls = readCalls();
if (calls.length !== callsBeforePendingIdle + 1) throw new Error("pending streamed adapter admitted a duplicate idle run");
writeFileSync(pendingTrigger, "go\n");
await pendingIdle;
if (prompts.length !== promptsBeforePendingIdle + 1 || !prompts.at(-1).body.parts[0].text.includes("async repair")) throw new Error("pending streamed adapter did not complete one bounded follow-up");

const signalsBeforeStderrLimit = groupSignals.length;
const logsBeforeStderrLimit = logs.length;
writePlan({exitCode: 0, stderr: "e".repeat(70000)});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "stderr-limit-session"}}});
if (groupSignals.length !== signalsBeforeStderrLimit || directKills.length !== 0) throw new Error("stderr overflow used parent-side numeric process signalling");
if (logs.length !== logsBeforeStderrLimit + 1 || !logs.at(-1).body.message.includes("stderr exceeded")) throw new Error("stderr overflow was not surfaced conservatively: " + JSON.stringify(logs.at(-1)));

// A real whole-group SIGKILL races the status pipe's EOF against the
// child's "exit" event; unlike the old fully synchronous mock (which
// always resolved supervisorExit before the race even started), either
// order is possible with a real process, and both land on an equally
// conservative, non-crashing, non-numeric-signalling failure report. The
// specific message therefore is not pinned to one of the two outcomes.
const signalsBeforeLostLeader = groupSignals.length;
const logsBeforeLostLeader = logs.length;
const promptsBeforeLostLeader = prompts.length;
writePlan({selfDestructGroup: true});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "lost-supervisor-session"}}});
if (groupSignals.length !== signalsBeforeLostLeader || directKills.length !== 0) throw new Error("lost supervisor identity was signalled by stale numeric PID: " + JSON.stringify({groupSignals, directKills}));
if (logs.length !== logsBeforeLostLeader + 1 || prompts.length !== promptsBeforeLostLeader + 1 || !logs.at(-1).body.message.includes("reminder unavailable")) throw new Error("lost supervisor identity was not surfaced conservatively: " + JSON.stringify(logs.at(-1)));
`
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, root)
	command.Env = append(command.Env, fixtureEnv...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PATH=") {
			command.Env = append(command.Env, entry)
		}
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode plugin transport: %v\n%s", err, output)
	}
}

func TestOpenCodeSPECContractPluginReadsNodeCompatStatusPipeUnderBun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OpenCode source transport deliberately requires POSIX process groups")
	}
	bun := os.Getenv("OPENCODE_BUN")
	if bun == "" {
		resolved, lookupErr := exec.LookPath("bun")
		if lookupErr != nil {
			t.Skip("bun is required to exercise OpenCode's production JavaScript runtime")
		}
		bun = resolved
	}
	root := repoRoot(t)
	fixtureDir, planFile, recordFile, fixtureEnv := writeAdapterFixture(t, bun)
	if err := os.WriteFile(planFile, []byte(`{"stdout":"{}","exitCode":0}`), 0o600); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {pathToFileURL} from "node:url";
const module = await import(pathToFileURL(process.argv[1]).href);
const logs = [];
const prompts = [];
const hooks = await module.SpecContractGuard({
  client: {
    app: {log: async ({body}) => logs.push(body)},
    session: {promptAsync: async (request) => prompts.push(request)},
  },
  worktree: process.argv[2],
});
await hooks.event({event: {type: "session.idle", properties: {sessionID: "bun-status-pipe"}}});
console.log(JSON.stringify({logs, prompts}));
`
	output := runNodeFixtureScript(t, bun, plugin, root, script, fixtureDir, nil, fixtureEnv)
	var result struct {
		Logs    []json.RawMessage `json:"logs"`
		Prompts []json.RawMessage `json:"prompts"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &result); err != nil {
		t.Fatalf("decode Bun fixture output: %v\n%s", err, output)
	}
	if len(result.Logs) != 0 || len(result.Prompts) != 0 {
		t.Fatalf("noop adapter under Bun produced logs=%d prompts=%d", len(result.Logs), len(result.Prompts))
	}
	records, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatal(err)
	}
	trimmedRecords := strings.TrimSpace(string(records))
	if trimmedRecords == "" {
		t.Fatal("Bun adapter invocation record is empty")
	}
	if lines := strings.Count(trimmedRecords, "\n") + 1; lines != 1 {
		t.Fatalf("Bun adapter invocation count = %d, want 1\n%s", lines, records)
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
	pluginRoot := repoRoot(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the OpenCode native pre-tool contract")
	}
	plugin := filepath.Join(pluginRoot, ".opencode", "plugins", "spec-contract-guard.js")
	// This test now drives four real guard subprocesses (not a mockable
	// globalThis.Bun indirection): the worktree is a scratch directory with
	// its own controllable pretool-* fixtures at the fixed native guard
	// paths, since the real repo's guard scripts cannot be scripted per
	// scenario. The plugin itself is still loaded from the real repo.
	worktree := t.TempDir()
	fixtureDir, _, _, fixtureEnv := writeAdapterFixture(t, node)
	guardPlans := writeGuardFixtures(t, fixtureDir, worktree)
	expectedPaths := []string{
		".opencode/hooks/pretool-spawn-routing",
		".opencode/hooks/pretool-bead-close-guard",
		".opencode/hooks/pretool-bypass-guard",
		".opencode/hooks/pretool-pr-guard",
	}
	guardEnv := make([]string, 0, len(expectedPaths))
	for index, path := range expectedPaths {
		guardEnv = append(guardEnv, "OPENCODE_GUARD_PLAN_"+strconv.Itoa(index)+"="+guardPlans[path])
	}
	script := `
import {existsSync, readFileSync, realpathSync, writeFileSync} from "node:fs";
import {pathToFileURL} from "node:url";
const allow = () => ({exitCode: 0});
const toasts = [];
const logs = [];
const timers = [];
let holdToast = false;
globalThis.setTimeout = (callback, milliseconds) => { const timer = {callback, milliseconds, cleared: false}; timers.push(timer); return timer; };
globalThis.clearTimeout = (timer) => { timer.cleared = true; };
const recordFile = process.env.OPENCODE_FIXTURE_RECORD;
const guardPlanFiles = [process.env.OPENCODE_GUARD_PLAN_0, process.env.OPENCODE_GUARD_PLAN_1, process.env.OPENCODE_GUARD_PLAN_2, process.env.OPENCODE_GUARD_PLAN_3];
const expectedPaths = [
  ".opencode/hooks/pretool-spawn-routing",
  ".opencode/hooks/pretool-bead-close-guard",
  ".opencode/hooks/pretool-bypass-guard",
  ".opencode/hooks/pretool-pr-guard",
];
const expectedTimeouts = [5000, 30000, 5000, 5000];
const writeGuardPlan = (index, plan) => writeFileSync(guardPlanFiles[index], JSON.stringify(Object.assign({label: expectedPaths[index]}, plan)));
const readCalls = () => {
  if (!existsSync(recordFile)) return [];
  return readFileSync(recordFile, "utf8").split("\n").filter(Boolean).map((line) => JSON.parse(line));
};
const supervisorSource = readFileSync(process.argv[1], "utf8");
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {
  app: {async log(entry) { logs.push(entry); }},
  tui: {async showToast(entry) { toasts.push(entry); if (holdToast) await new Promise(() => {}); }},
  session: {async promptAsync() { throw new Error("pre-tool guard must not prompt the model"); }},
};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const before = hooks["tool.execute.before"];
if (typeof before !== "function") throw new Error("native tool.execute.before hook is absent");

writeGuardPlan(0, allow());
writeGuardPlan(1, allow());
writeGuardPlan(2, allow());
writeGuardPlan(3, allow());
await before({tool: "bash", sessionID: "safe-session", callID: "safe-call"}, {args: {command: "go test ./..."}});
let calls = readCalls();
if (calls.length !== 4) throw new Error("safe Bash call did not traverse four guards: " + calls.length);
if (!supervisorSource.includes("exec 0<&4") || !supervisorSource.includes("kill -KILL 0")) throw new Error("guard did not use the trusted payload and cleanup supervisor");
for (let index = 0; index < 4; index++) {
  const call = calls[index];
  if (call.label !== expectedPaths[index]) throw new Error("guard sequence: " + JSON.stringify(calls));
  if (realpathSync(call.cwd) !== realpathSync(process.argv[2])) throw new Error("guard cwd: " + JSON.stringify(call));
  const payload = JSON.parse(call.stdin);
  if (payload.tool_name !== "Bash" || payload.tool_input.command !== "go test ./...") throw new Error("legacy guard envelope: " + JSON.stringify(payload));
  if (timers[index].milliseconds !== expectedTimeouts[index] || !timers[index].cleared) throw new Error("guard deadline was not explicit and cleared: " + JSON.stringify(timers[index]));
}
if (toasts.length !== 0 || logs.length !== 0) throw new Error("safe guards produced feedback");

writeGuardPlan(0, {exitCode: 0, stdout: JSON.stringify({hookSpecificOutput: {additionalContext: "route this launch through AGM"}})});
holdToast = true;
await before({tool: "scheduled-tasks_create_scheduled_task", sessionID: "route-session", callID: "route-call"}, {args: {name: "nightly"}});
calls = readCalls();
if (calls.length !== 5 || calls.at(-1).label !== expectedPaths[0]) throw new Error("scheduled-task routing used the wrong guard set");
if (toasts.length !== 1 || toasts[0].body.variant !== "warning" || !toasts[0].body.message.includes("through AGM")) throw new Error("routing reminder did not preserve non-blocking native feedback: " + JSON.stringify(toasts));
const scheduledPayload = JSON.parse(calls.at(-1).stdin);
if (scheduledPayload.tool_name !== "mcp__scheduled-tasks__create_scheduled_task" || scheduledPayload.tool_input.name !== "nightly") throw new Error("scheduled-task guard envelope: " + JSON.stringify(scheduledPayload));
holdToast = false;

writeGuardPlan(0, allow());
writeGuardPlan(1, allow());
writeGuardPlan(2, {exitCode: 2, stdout: JSON.stringify({hookSpecificOutput: {permissionDecision: "deny", additionalContext: "use the approved wrapper"}})});
let denied = "";
try {
  await before({tool: "bash", sessionID: "deny-session", callID: "deny-call"}, {args: {command: "git commit --no-verify"}});
} catch (error) { denied = String(error?.message || error); }
if (!denied.includes("approved wrapper")) throw new Error("native denial was not preserved: " + denied);
calls = readCalls();
if (calls.length !== 8) throw new Error("denied guard allowed later scripts to run: " + calls.length);

writeGuardPlan(0, allow());
writeGuardPlan(1, allow());
writeGuardPlan(2, allow());
writeGuardPlan(3, {exitCode: 2, stderr: "route raw gh lifecycle through safe-pr"});
let stderrDenied = "";
try {
  await before({tool: "bash", sessionID: "pr-deny-session", callID: "pr-deny-call"}, {args: {command: "gh pr create"}});
} catch (error) { stderrDenied = String(error?.message || error); }
if (!stderrDenied.includes("safe-pr")) throw new Error("stderr-only native denial was not preserved: " + stderrDenied);
calls = readCalls();
if (calls.length !== 12) throw new Error("stderr-denied guard used the wrong script sequence: " + calls.length);

writeGuardPlan(0, allow());
writeGuardPlan(1, {exitCode: 0, stdout: JSON.stringify({hookSpecificOutput: {additionalContext: "bead-close-guard error (failing open): dependency unavailable"}})});
writeGuardPlan(2, allow());
writeGuardPlan(3, allow());
await before({tool: "bash", sessionID: "advisory-session", callID: "advisory-call"}, {args: {command: "bd close ce-test"}});
calls = readCalls();
if (calls.length !== 16 || toasts.length !== 2 || !toasts.at(-1).body.message.includes("failing open")) throw new Error("legacy advisory was upgraded to a block or lost: " + JSON.stringify(toasts));

writeGuardPlan(0, {exitCode: 0, stdout: "null"});
let malformed = "";
try {
  await before({tool: "bash", sessionID: "malformed-session", callID: "malformed-call"}, {args: {command: "echo safe"}});
} catch (error) { malformed = String(error?.message || error); }
if (!malformed.includes("non-object JSON response")) throw new Error("malformed guard output did not fail closed: " + malformed);
calls = readCalls();
if (calls.length !== 17) throw new Error("malformed guard output allowed later scripts to run: " + calls.length);

writeGuardPlan(0, {exitCode: 0, stdout: JSON.stringify({hookSpecificOutput: {permissionDecision: "allow"}, permissionDecision: "deny"})});
let conflicting = "";
try {
  await before({tool: "bash", sessionID: "conflicting-session", callID: "conflicting-call"}, {args: {command: "echo safe"}});
} catch (error) { conflicting = String(error?.message || error); }
if (!conflicting.includes("conflicting permission decisions")) throw new Error("conflicting guard decisions did not fail closed: " + conflicting);
calls = readCalls();
if (calls.length !== 18) throw new Error("conflicting guard decisions allowed later scripts to run: " + calls.length);

writeGuardPlan(0, {wait: guardPlanFiles[0] + ".never"});
const timerCount = timers.length;
let timedOut = "";
const timeoutRun = before({tool: "bash", sessionID: "timeout-session", callID: "timeout-call"}, {args: {command: "echo safe"}}).catch((error) => { timedOut = String(error?.message || error); });
while (timers.length === timerCount) await Promise.resolve();
timers.at(-1).callback();
await timeoutRun;
if (!timedOut.includes("failed (timeout)")) throw new Error("guard timeout did not fail closed: " + timedOut);

const beforeOversize = readCalls().length;
let oversize = "";
try {
  await before({tool: "bash", sessionID: "oversize-session", callID: "oversize-call"}, {args: {command: "x".repeat(1024 * 1024)}});
} catch (error) { oversize = String(error?.message || error); }
if (!oversize.includes("input exceeds 1048576 bytes") || readCalls().length !== beforeOversize) throw new Error("oversized guard input did not fail before launch: " + oversize);

const beforeUnknown = readCalls().length;
await before({tool: "read", sessionID: "read-session", callID: "read-call"}, {args: {filePath: "README.md"}});
if (readCalls().length !== beforeUnknown) throw new Error("unrelated tool executed Bash-only guards");
`
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, plugin, worktree)
	command.Env = append(command.Env, fixtureEnv...)
	command.Env = append(command.Env, guardEnv...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "PATH=") {
			command.Env = append(command.Env, entry)
		}
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("OpenCode native pre-tool guard projection: %v\n%s", err, output)
	}
}
