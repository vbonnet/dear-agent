package hookparity_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newSupervisorFixtureBase skips this test on Windows or when node is
// unavailable, then returns the repo root, a fresh scratch directory, and
// the fixed "cleanup\n" trigger file path several real-supervisor tests
// below use to unstick their fakeGo script once the test is done. It
// registers that trigger via t.Cleanup so a test never has to repeat it.
func newSupervisorFixtureBase(t *testing.T) (node, root, fixtureDir, cleanupFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("OpenCode source transport deliberately requires POSIX process groups")
	}
	var err error
	node, err = exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to exercise the OpenCode supervisor contract")
	}
	root = repoRoot(t)
	fixtureDir = t.TempDir()
	cleanupFile = filepath.Join(fixtureDir, "cleanup")
	t.Cleanup(func() {
		_ = os.WriteFile(cleanupFile, []byte("cleanup\n"), 0o600)
		time.Sleep(100 * time.Millisecond)
	})
	return node, root, fixtureDir, cleanupFile
}

func TestOpenCodeSPECContractPluginTerminatesProcessGroup(t *testing.T) {
	node, root, fixtureDir, cleanupFile := newSupervisorFixtureBase(t)
	pidFile := filepath.Join(fixtureDir, "processes")
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

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {readFileSync} from "node:fs";
import {pathToFileURL} from "node:url";
const logs = [];
const prompts = [];
const cleanupSignals = [];
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  if (signal !== 0) cleanupSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
// The plugin now spawns adapters through the real node:child_process.spawn
// (not a mockable globalThis.Bun indirection), so this fixture verifies the
// actual OS-level process tree it produces rather than intercepting the call.
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log(entry) { logs.push(entry); }}, session: {async promptAsync(entry) { prompts.push(entry); }}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const started = Date.now();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "process-group"}}});
if (Date.now() - started > 5000) throw new Error("overflow termination exceeded five seconds");
if (logs.length !== 1 || !logs[0].body.message.includes("exceeded") || prompts.length !== 1) throw new Error("overflow was not surfaced: " + JSON.stringify({logs, prompts}));
const [recordedSupervisor, adapter, descendant] = readFileSync(process.env.OPENCODE_DESCENDANT_PID_FILE, "utf8").trim().split(/\s+/).map(Number);
if (cleanupSignals.length !== 0) throw new Error("cleanup used parent-side numeric process signalling: " + JSON.stringify(cleanupSignals));
for (const pid of [recordedSupervisor, adapter, descendant]) {
  let alive = true;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { originalProcessKill(pid, 0); } catch { alive = false; break; }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  if (alive) throw new Error("contained adapter process survived overflow termination: " + pid);
}
`
	runNodeFixtureScript(t, node, plugin, root, script, fixtureDir,
		[]string{"OPENCODE_DESCENDANT_PID_FILE", "OPENCODE_TEST_CLEANUP_FILE"},
		[]string{
			"OPENCODE_DESCENDANT_PID_FILE=" + pidFile,
			"OPENCODE_TEST_CLEANUP_FILE=" + cleanupFile,
		},
	)
}

func TestOpenCodeSPECContractPluginBoundsEscapedOutputWithoutStaleGroupSignal(t *testing.T) {
	node, root, fixtureDir, cleanupFile := newSupervisorFixtureBase(t)
	pidFile := filepath.Join(fixtureDir, "processes")
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

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {readFileSync, writeFileSync} from "node:fs";
import {pathToFileURL} from "node:url";
const logs = [];
const prompts = [];
const cleanupSignals = [];
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  if (signal !== 0) cleanupSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
// The plugin now spawns adapters through the real node:child_process.spawn
// (not a mockable globalThis.Bun indirection), so this fixture verifies the
// actual OS-level process tree it produces rather than intercepting the call.
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log(entry) { logs.push(entry); }}, session: {async promptAsync(entry) { prompts.push(entry); }}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const started = Date.now();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "escaped-pipe"}}});
const elapsed = Date.now() - started;
if (elapsed > 5000) throw new Error("escaped output pipe exceeded bounded settlement: " + elapsed);
if (logs.length !== 1 || prompts.length !== 1 || !logs[0].body.message.includes("exact escaped-pipe failure")) throw new Error("real adapter status/stderr were not preserved: " + JSON.stringify({logs, prompts}));
const [recordedSupervisor, adapter, escaped] = readFileSync(process.env.OPENCODE_ESCAPE_PID_FILE, "utf8").trim().split(/\s+/).map(Number);
if (cleanupSignals.length !== 0) throw new Error("cleanup used a stale adapter or other parent-side numeric PID: " + JSON.stringify({cleanupSignals, recordedSupervisor, adapter}));
for (const pid of [recordedSupervisor, adapter]) {
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
	runNodeFixtureScript(t, node, plugin, root, script, fixtureDir,
		[]string{"OPENCODE_NODE", "OPENCODE_ESCAPE_FIXTURE", "OPENCODE_ESCAPE_PID_FILE", "OPENCODE_ESCAPE_CLEANUP_FILE"},
		[]string{
			"OPENCODE_NODE=" + node,
			"OPENCODE_ESCAPE_FIXTURE=" + fixture,
			"OPENCODE_ESCAPE_PID_FILE=" + pidFile,
			"OPENCODE_ESCAPE_CLEANUP_FILE=" + cleanupFile,
		},
	)
}

func TestOpenCodeSPECContractPluginUsesOnlySupervisorOwnedCleanup(t *testing.T) {
	node, root, fixtureDir, cleanupFile := newSupervisorFixtureBase(t)
	pidFile := filepath.Join(fixtureDir, "supervisor-pid")
	fakeGo := filepath.Join(fixtureDir, "go")
	const fakeGoScript = `#!/bin/sh
printf '%s\n' "$PPID" > "$OPENCODE_SELF_CLEANUP_PID_FILE"
(
  while test ! -f "$OPENCODE_TEST_CLEANUP_FILE"; do /bin/sleep 0.05; done
  kill -KILL 0
) </dev/null >/dev/null 2>&1 &
printf '{}'
`
	if err := os.WriteFile(fakeGo, []byte(fakeGoScript), 0o700); err != nil {
		t.Fatal(err)
	}

	plugin := filepath.Join(root, ".opencode", "plugins", "spec-contract-guard.js")
	script := `
import {readFileSync} from "node:fs";
import {pathToFileURL} from "node:url";
const logs = [];
const prompts = [];
const attemptedSignals = [];
const originalProcessKill = process.kill.bind(process);
process.kill = (pid, signal) => {
  if (signal !== 0) attemptedSignals.push({pid, signal});
  throw new Error("parent numeric signalling is forbidden");
};
// The plugin now spawns adapters through the real node:child_process.spawn
// (not a mockable globalThis.Bun indirection), so this fixture verifies the
// actual OS-level process tree it produces rather than intercepting the call.
const mod = await import(pathToFileURL(process.argv[1]).href);
const client = {app: {async log(entry) { logs.push(entry); }}, session: {async promptAsync(entry) { prompts.push(entry); }}};
const hooks = await mod.SpecContractGuard({worktree: process.argv[2], client});
const started = Date.now();
await hooks.event({event: {type: "session.idle", properties: {sessionID: "self-cleanup"}}});
if (Date.now() - started > 5000) throw new Error("supervisor self-cleanup exceeded bounded settlement");
if (logs.length !== 0 || prompts.length !== 0) throw new Error("clean adapter result was not preserved: " + JSON.stringify({logs, prompts}));
if (attemptedSignals.length !== 0) throw new Error("parent used numeric process signalling: " + JSON.stringify(attemptedSignals));
const supervisorPID = Number(readFileSync(process.env.OPENCODE_SELF_CLEANUP_PID_FILE, "utf8").trim());
let alive = true;
for (let attempt = 0; attempt < 100; attempt++) {
  try { originalProcessKill(supervisorPID, 0); } catch { alive = false; break; }
  await new Promise((resolve) => setTimeout(resolve, 10));
}
if (alive) throw new Error("supervisor ignored its private self-cleanup request");
`
	runNodeFixtureScript(t, node, plugin, root, script, fixtureDir,
		[]string{"OPENCODE_TEST_CLEANUP_FILE", "OPENCODE_SELF_CLEANUP_PID_FILE"},
		[]string{
			"OPENCODE_TEST_CLEANUP_FILE=" + cleanupFile,
			"OPENCODE_SELF_CLEANUP_PID_FILE=" + pidFile,
		},
	)
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
import {setTimeout as delay} from "node:timers/promises";
import {pathToFileURL} from "node:url";
process.kill = (pid, signal) => {
  writeFileSync(process.env.OPENCODE_NUMERIC_SIGNAL_VIOLATION, JSON.stringify({pid, signal}));
  throw new Error("parent numeric signalling is forbidden");
};
// The plugin now spawns adapters through the real node:child_process.spawn
// (not a mockable globalThis.Bun indirection), so this fixture verifies the
// actual OS-level process tree it produces rather than intercepting the call.
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
