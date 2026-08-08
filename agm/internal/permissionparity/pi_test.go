package permissionparity

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/permissionparity/piadapter"
)

func TestDecidePiToolCallModes(t *testing.T) {
	t.Parallel()
	mutating := PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git status"}}
	if got := DecidePiToolCall("plan", []string{"Bash(git status)"}, mutating, true); got.Action != PiBlock {
		t.Fatalf("plan decision = %#v", got)
	}
	if got := DecidePiToolCall("plan", []string{"PluginDeploy"}, PiToolCall{ToolName: "plugin_deploy"}, true); got.Action != PiBlock {
		t.Fatalf("plan extension-tool decision = %#v", got)
	}
	if got := DecidePiToolCall("plan", []string{"Read(/work/**)"}, PiToolCall{ToolName: "read", Input: map[string]any{"path": "/work/a"}}, false); got.Action != PiAllow {
		t.Fatalf("plan read decision = %#v", got)
	}
	if got := DecidePiToolCall("auto", nil, mutating, false); got.Action != PiAllow {
		t.Fatalf("auto decision = %#v", got)
	}
	if got := DecidePiToolCall("default", []string{"Bash(git status)"}, mutating, false); got.Action != PiAllow {
		t.Fatalf("matching default decision = %#v", got)
	}
	if got := DecidePiToolCall("default", nil, mutating, true); got.Action != PiAsk {
		t.Fatalf("interactive unmatched decision = %#v", got)
	}
	if got := DecidePiToolCall("default", nil, mutating, false); got.Action != PiBlock {
		t.Fatalf("non-interactive unmatched decision = %#v", got)
	}
	if got := DecidePiToolCall("default", []string{"PluginDeploy"}, PiToolCall{ToolName: "plugin_deploy", Input: map[string]any{"target": "local"}}, false); got.Action != PiAllow {
		t.Fatalf("custom extension tool decision = %#v", got)
	}
}

func TestPiPolicyMatchingIsAnchoredAndMapsNativeTools(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		allow string
		call  PiToolCall
		want  bool
	}{
		{name: "bash exact", allow: "Bash(git status)", call: PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git status"}}, want: true},
		{name: "bash exact rejects suffix", allow: "Bash(git status)", call: PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git status --short"}}},
		{name: "bash wildcard", allow: "Bash(git status *)", call: PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git status --short"}}, want: true},
		{name: "claude colon wildcard", allow: "Bash(git:*)", call: PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git log -1"}}, want: true},
		{name: "colon wildcard rejects chained command", allow: "Bash(git:*)", call: PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git status; rm -rf /tmp/nope"}}},
		{name: "wildcard rejects command substitution", allow: "Bash(git:*)", call: PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git show $(danger)"}}},
		{name: "quoted shell character remains literal", allow: "Bash(git:*)", call: PiToolCall{ToolName: "bash", Input: map[string]any{"command": "git log --format='subject; body'"}}, want: true},
		{name: "read path", allow: "Read(/work/**)", call: PiToolCall{ToolName: "read", Input: map[string]any{"path": "/work/pkg/file.go"}}, want: true},
		{name: "find maps glob", allow: "Glob(/work/**)", call: PiToolCall{ToolName: "find", Input: map[string]any{"path": "/work/pkg"}}, want: true},
		{name: "grep maps grep", allow: "Grep(/work/**)", call: PiToolCall{ToolName: "grep", Input: map[string]any{"path": "/work/pkg"}}, want: true},
		{name: "write category", allow: "Write", call: PiToolCall{ToolName: "write", Input: map[string]any{"path": "/any"}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := PiPolicyAllows([]string{test.allow}, test.call)
			if got != test.want {
				t.Fatalf("PiPolicyAllows(%q, %#v) = %v", test.allow, test.call, got)
			}
		})
	}
}

func TestEnsurePiAuthorizationExtensionIsPrivateAndIdempotent(t *testing.T) {
	root := t.TempDir()
	path, err := EnsurePiAuthorizationExtension(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("extension mode = %o", info.Mode().Perm())
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(first), `pi.on("tool_call"`) || !strings.Contains(string(first), "AGM_PI_PERMISSION_POLICY") {
		t.Fatalf("unexpected extension payload: %s", first)
	}
	secondPath, err := EnsurePiAuthorizationExtension(root)
	if err != nil {
		t.Fatal(err)
	}
	if secondPath != path {
		t.Fatalf("second path = %q, want %q", secondPath, path)
	}
}

func TestEnsurePiPolicyFileIsPrivateNormalizedAndIdempotent(t *testing.T) {
	root := t.TempDir()
	path, err := piadapter.EnsurePolicyFile(root, "native-session", `{"allow":["Bash(git status)"]}`)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("policy stat = %v, info=%v", err, info)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"allow":["Bash(git status)"]}` {
		t.Fatalf("policy = %q, err=%v", data, err)
	}
	again, err := piadapter.EnsurePolicyFile(root, "native-session", string(data))
	if err != nil || again != path {
		t.Fatalf("idempotent policy = %q, err=%v", again, err)
	}
	if _, err := piadapter.EnsurePolicyFile(root, "", string(data)); err == nil {
		t.Fatal("empty Pi policy session key was accepted")
	}
	if _, err := piadapter.EnsurePolicyFile(root, "native-session", `{"allow":"all"}`); err == nil {
		t.Fatal("malformed Pi policy was accepted")
	}
}

func TestEnsurePiAuthorizationExtensionRejectsSymlinkBoundaries(t *testing.T) {
	target := t.TempDir()
	rootLink := filepath.Join(t.TempDir(), "pi-extension")
	if err := os.Symlink(target, rootLink); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsurePiAuthorizationExtension(rootLink); err == nil {
		t.Fatal("symlink extension root was accepted")
	}

	root := t.TempDir()
	targetFile := filepath.Join(t.TempDir(), "target.js")
	if err := os.WriteFile(targetFile, []byte("untrusted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetFile, filepath.Join(root, "agm-authorization.js")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsurePiAuthorizationExtension(root); err == nil {
		t.Fatal("symlink extension target was accepted")
	}
}

func TestPiProductionTerminalTimeoutBudgetsAreHonored(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the Pi-native extension fixture")
	}
	extensionRoot := t.TempDir()
	extensionPath, err := EnsurePiAuthorizationExtension(extensionRoot)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repository, ".pi", "hooks.json")); err != nil {
		t.Fatalf("stat production Pi hook manifest: %v", err)
	}
	slowProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(slowProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	const slowHooks = `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"sleep 3; printf '{\"hookSpecificOutput\":{\"additionalContext\":\"long terminal handler completed\"}}\\n'","timeout":60}]}]}}`
	if err := os.WriteFile(filepath.Join(slowProject, ".pi", "hooks.json"), []byte(slowHooks), 0o600); err != nil {
		t.Fatal(err)
	}

	const script = `
import {readFileSync} from "node:fs";
const source = readFileSync(process.argv[1], "utf8");
const mod = await import("data:text/javascript;base64," + Buffer.from(source).toString("base64"));
for (const eventName of ["Stop", "SubagentStop"]) {
  let clock = 0;
  const observed = [];
  const result = await mod.runProjectHooks(eventName, {}, process.argv[2], {
    now() { return clock; },
    async runCommand(_command, args, options) {
      observed.push({command: args[1], timeout: options.timeout, maxBuffer: options.maxBuffer});
      clock += options.timeout + (observed.length === 1 ? 500 : 0);
      return {status: 0, stdout: "", stderr: ""};
    },
  });
  if (result !== undefined) throw new Error(eventName + " production hook chain unexpectedly failed: " + JSON.stringify(result));
  if (observed.length !== 2 || observed[0].timeout !== 60000 || observed[1].timeout !== 120000) {
    throw new Error(eventName + " production timeouts were shortened: " + JSON.stringify(observed));
  }
  if (!observed[0].command.includes("--event " + eventName) || !observed[1].command.includes("stop-guardrail-feedback")) {
    throw new Error(eventName + " did not execute the production terminal chain: " + JSON.stringify(observed));
  }
  if (observed.some((entry) => entry.maxBuffer !== 16384)) throw new Error(eventName + " output capture was not bounded: " + JSON.stringify(observed));
}
const slowResult = await mod.runProjectHooks("Stop", {}, process.argv[3]);
if (slowResult?.context !== "long terminal handler completed") throw new Error("declared 60-second handler was shortened below its three-second execution: " + JSON.stringify(slowResult));
`
	command := exec.Command(node, "--input-type=module", "-e", script, filepath.Clean(extensionPath), filepath.Clean(repository), filepath.Clean(slowProject))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("production Pi terminal budgets: %v\n%s", err, output)
	}
}

func TestPiTerminalHookTimeoutKillsTermIgnoringProcessGroup(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the Pi-native extension fixture")
	}
	root := t.TempDir()
	extensionPath, err := EnsurePiAuthorizationExtension(root)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	childPIDPath := filepath.Join(t.TempDir(), "term-ignoring-child.pid")
	const hooks = `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"trap '' TERM; (trap '' TERM; while :; do sleep 1; done) & child=$!; printf '%s' \"$child\" > \"$PI_CHILD_PID_FILE\"; while :; do sleep 1; done","timeout":0.2}]}]}}`
	if err := os.WriteFile(filepath.Join(project, ".pi", "hooks.json"), []byte(hooks), 0o600); err != nil {
		t.Fatal(err)
	}
	const script = `
import {readFileSync} from "node:fs";
const source = readFileSync(process.argv[1], "utf8");
const mod = await import("data:text/javascript;base64," + Buffer.from(source).toString("base64"));
const started = Date.now();
const result = await mod.runProjectHooks("Stop", {}, process.argv[2]);
const elapsed = Date.now() - started;
if (!result?.block || !result.reason.includes("ETIMEDOUT")) throw new Error("TERM-ignoring terminal hook did not fail closed: " + JSON.stringify(result));
if (elapsed > 1500) throw new Error("TERM-ignoring terminal hook exceeded its bounded cleanup window: " + elapsed);
const childPID = Number(readFileSync(process.argv[3], "utf8"));
await new Promise((resolve) => setTimeout(resolve, 100));
try {
  process.kill(childPID, 0);
  throw new Error("TERM-ignoring hook descendant survived process-group cleanup: " + childPID);
} catch (error) {
  if (error?.code !== "ESRCH") throw error;
}
`
	command := exec.Command(node, "--input-type=module", "-e", script, filepath.Clean(extensionPath), filepath.Clean(project), filepath.Clean(childPIDPath))
	command.Env = append(os.Environ(), "PI_CHILD_PID_FILE="+childPIDPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("TERM-ignoring Pi hook cleanup: %v\n%s", err, output)
	}
}

func TestPiTerminalHookTimeoutSettlesWhenEscapedProcessRetainsPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX detached-process semantics are required for this regression")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the Pi-native extension fixture")
	}
	root := t.TempDir()
	extensionPath, err := EnsurePiAuthorizationExtension(root)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	childPIDPath := filepath.Join(t.TempDir(), "escaped-child.pid")
	t.Cleanup(func() {
		raw, readErr := os.ReadFile(childPIDPath)
		if readErr != nil {
			return
		}
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
		if parseErr != nil || pid <= 1 {
			return
		}
		process, findErr := os.FindProcess(pid)
		if findErr == nil {
			_ = process.Kill()
		}
	})
	escapeHelperPath := filepath.Join(t.TempDir(), "escape-hook-pipes.mjs")
	const escapeHelper = `
import {spawn} from "node:child_process";
import {writeFileSync} from "node:fs";
const escaped = spawn(process.execPath, ["--input-type=module", "-e", "process.on('SIGTERM', () => {}); setInterval(() => {}, 1000);"], {
  detached: true,
  stdio: ["ignore", "inherit", "inherit"],
});
writeFileSync(process.env.PI_CHILD_PID_FILE, String(escaped.pid));
escaped.unref();
await new Promise(() => {});
`
	if err := os.WriteFile(escapeHelperPath, []byte(escapeHelper), 0o600); err != nil {
		t.Fatal(err)
	}
	const hooks = `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"\"$PI_NODE\" \"$PI_ESCAPE_HELPER\"","timeout":0.2}]}]}}`
	if err := os.WriteFile(filepath.Join(project, ".pi", "hooks.json"), []byte(hooks), 0o600); err != nil {
		t.Fatal(err)
	}
	const script = `
import {readFileSync} from "node:fs";
const source = readFileSync(process.argv[1], "utf8");
const mod = await import("data:text/javascript;base64," + Buffer.from(source).toString("base64"));
let childPID = 0;
try {
  const started = Date.now();
  const result = await mod.runProjectHooks("Stop", {}, process.argv[2]);
  const elapsed = Date.now() - started;
  if (!result?.block || !result.reason.includes("ETIMEDOUT")) throw new Error("escaped-pipe terminal hook did not fail closed: " + JSON.stringify(result));
  if (elapsed > 1500) throw new Error("escaped-pipe terminal hook exceeded its bounded cleanup window: " + elapsed);
  childPID = Number(readFileSync(process.argv[3], "utf8"));
  if (!Number.isSafeInteger(childPID) || childPID <= 1) throw new Error("invalid escaped hook PID: " + childPID);
  process.kill(childPID, 0);
} finally {
  if (childPID > 1) {
    try {
      process.kill(childPID, "SIGKILL");
    } catch (error) {
      if (error?.code !== "ESRCH") throw error;
    }
  }
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, filepath.Clean(extensionPath), filepath.Clean(project), filepath.Clean(childPIDPath))
	command.Env = append(os.Environ(), "PI_CHILD_PID_FILE="+childPIDPath, "PI_ESCAPE_HELPER="+escapeHelperPath, "PI_NODE="+node)
	if output, err := command.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			t.Fatalf("escaped-pipe Pi hook exceeded the test deadline: %v\n%s", ctx.Err(), output)
		}
		t.Fatalf("escaped-pipe Pi hook cleanup: %v\n%s", err, output)
	}
}

func TestEmbeddedPiExtensionDecisionParity(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required to execute the Pi-native extension fixture")
	}
	root := t.TempDir()
	path, err := EnsurePiAuthorizationExtension(root)
	if err != nil {
		t.Fatal(err)
	}
	policyPath, err := piadapter.EnsurePolicyFile(root, "pi-native-session", `{"allow":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	const hooks = `{"hooks":{"PreToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"hookSpecificOutput\":{\"additionalContext\":\"first guard ran\"}}\\n'","timeout":1}]},{"matcher":"Bash","hooks":[{"type":"command","command":"cat >/dev/null; printf 'project guard rejected' >&2; exit 42","timeout":1}]}],"SessionStart":[{"hooks":[{"type":"command","command":"tee \"$PI_HOOK_CAPTURE\" >/dev/null","timeout":1}]}],"UserPromptSubmit":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"decision\":\"block\",\"reason\":\"prompt rejected\"}\\n'","timeout":1}]}],"PreCompact":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"hookSpecificOutput\":{\"permissionDecision\":\"deny\",\"additionalContext\":\"compaction rejected\"}}\\n'","timeout":1}]}],"Stop":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf 'first\\n' >> \"$PI_TERMINAL_HOOK_CAPTURE\"; printf '{\"decision\":\"block\",\"reason\":\"finish the regression\",\"dearAgentSpecFeedbackId\":\"%s\"}\\n' \"${PI_SPEC_FEEDBACK_ID:-}\"","timeout":1},{"type":"command","command":"cat >/dev/null; printf 'second\\n' >> \"$PI_TERMINAL_HOOK_CAPTURE\"; printf '{\"hookSpecificOutput\":{\"additionalContext\":\"second terminal handler ran\"}}\\n'","timeout":1}]}],"SubagentStop":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"decision\":\"block\",\"reason\":\"review the delegated result\"}\\n'","timeout":1}]}]}}`
	if err := os.WriteFile(filepath.Join(project, ".pi", "hooks.json"), []byte(hooks), 0o600); err != nil {
		t.Fatal(err)
	}
	malformedProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(malformedProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedProject, ".pi", "hooks.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	timeoutProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(timeoutProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	const timeoutHooks = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"hookSpecificOutput\":{\"additionalContext\":\"timeout context\"}}\\n'; printf 'timeout stderr' >&2; while :; do :; done","timeout":1}]}]}}`
	if err := os.WriteFile(filepath.Join(timeoutProject, ".pi", "hooks.json"), []byte(timeoutHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	nonzeroProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(nonzeroProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	const nonzeroHooks = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"hookSpecificOutput\":{\"additionalContext\":\"advisory only\"}}\\n'; exit 42","timeout":1}]}]}}`
	if err := os.WriteFile(filepath.Join(nonzeroProject, ".pi", "hooks.json"), []byte(nonzeroHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidShapeProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(invalidShapeProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	const invalidShapeHooks = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"true","timeout":999}]}]}}`
	if err := os.WriteFile(filepath.Join(invalidShapeProject, ".pi", "hooks.json"), []byte(invalidShapeHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidCommandProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(invalidCommandProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	const invalidCommandHooks = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"bad\u0000command","timeout":1}]}]}}`
	if err := os.WriteFile(filepath.Join(invalidCommandProject, ".pi", "hooks.json"), []byte(invalidCommandHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	oversizedProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(oversizedProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oversizedProject, ".pi", "hooks.json"), []byte(strings.Repeat("x", 1024*1024+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	terminalLimitProject := t.TempDir()
	if err := os.MkdirAll(filepath.Join(terminalLimitProject, ".pi"), 0o700); err != nil {
		t.Fatal(err)
	}
	terminalHandlers := strings.TrimSuffix(strings.Repeat(`{"type":"command","command":"true","timeout":1},`, 17), ",")
	terminalLimitHooks := `{"hooks":{"Stop":[{"hooks":[` + terminalHandlers + `]}]}}`
	if err := os.WriteFile(filepath.Join(terminalLimitProject, ".pi", "hooks.json"), []byte(terminalLimitHooks), 0o600); err != nil {
		t.Fatal(err)
	}
	capturePath := filepath.Join(t.TempDir(), "hook-input.json")
	terminalCapturePath := filepath.Join(t.TempDir(), "terminal-handlers.txt")
	script := `
import {readFileSync} from "node:fs";
const source = readFileSync(process.argv[1], "utf8");
const mod = await import("data:text/javascript;base64," + Buffer.from(source).toString("base64"));
const cases = [
  ["default", ["Bash(git status)"], {toolName:"bash", input:{command:"git status"}}, false, "allow"],
  ["default", ["Bash(git:*)"], {toolName:"bash", input:{command:"git status; rm -rf /tmp/nope"}}, true, "ask"],
  ["default", ["Bash(git:*)"], {toolName:"bash", input:{command:"git log --format='subject; body'"}}, false, "allow"],
  ["default", [], {toolName:"bash", input:{command:"rm -rf nope"}}, false, "block"],
  ["plan", ["Bash(git status)"], {toolName:"bash", input:{command:"git status"}}, true, "block"],
  ["plan", ["PluginDeploy"], {toolName:"plugin_deploy", input:{}}, true, "block"],
	["default", ["PluginDeploy"], {toolName:"plugin_deploy", input:{}}, false, "allow"],
	["plan", ["Read(/work/**)"], {toolName:"read", input:{path:"/work/a"}}, false, "allow"],
  ["auto", [], {toolName:"write", input:{path:"/tmp/x"}}, false, "allow"],
];
for (const [mode, allow, call, interactive, want] of cases) {
  const got = mod.decide(mode, allow, call, interactive).action;
  if (got !== want) throw new Error(mode + ": got " + got + ", want " + want);
}
let hookResult;
for (let attempt = 0; attempt < 3; attempt++) {
  hookResult = await mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git push"}}, process.argv[2]);
  if (hookResult?.reason?.includes("project guard rejected") || !hookResult?.reason?.includes("ETIMEDOUT")) break;
}
if (!hookResult?.block || !hookResult.reason.includes("project guard rejected")) throw new Error("project hook did not fail closed with its declared reason: " + JSON.stringify(hookResult));
const timeoutHook = await mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git push"}}, process.argv[5]);
if (!timeoutHook?.block || !timeoutHook.reason.includes("ETIMEDOUT") || !timeoutHook.reason.includes("timeout stderr") || timeoutHook.reason.includes("timeout context")) throw new Error("project hook timeout masked its execution failure: " + JSON.stringify(timeoutHook));
const nonzeroHook = await mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git push"}}, process.argv[6]);
if (!nonzeroHook?.block || !nonzeroHook.reason.startsWith("PreToolUse hook exited with status 42") || !nonzeroHook.reason.includes("advisory only")) throw new Error("project hook nonzero status was masked by advisory context: " + JSON.stringify(nonzeroHook));
const unmatchedHook = await mod.runProjectHooks("PreToolUse", {toolName:"edit", input:{path:"README.md"}}, process.argv[2]);
if (unmatchedHook !== undefined) throw new Error("matcher ran for an unrelated tool");
const malformedHook = await mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git status"}}, process.argv[3]);
if (!malformedHook?.block || !malformedHook.reason.includes("cannot load Pi hook manifest")) throw new Error("malformed hook manifest did not fail closed");
const invalidShapeHook = await mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git status"}}, process.argv[7]);
if (!invalidShapeHook?.block || !invalidShapeHook.reason.includes("invalid timeout")) throw new Error("invalid hook schema did not fail closed: " + JSON.stringify(invalidShapeHook));
const invalidCommandHook = await mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git status"}}, process.argv[9]);
if (!invalidCommandHook?.block || !invalidCommandHook.reason.includes("invalid command handler")) throw new Error("invalid hook command did not fail closed: " + JSON.stringify(invalidCommandHook));
const oversizedHook = await mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git status"}}, process.argv[8]);
if (!oversizedHook?.block || !oversizedHook.reason.includes("size limit")) throw new Error("oversized hook manifest did not fail closed: " + JSON.stringify(oversizedHook));
const terminalLimitHook = await mod.runProjectHooks("Stop", {}, process.argv[10]);
if (!terminalLimitHook?.block || !terminalLimitHook.reason.includes("terminal hook event Stop exceeds the handler limit")) throw new Error("terminal handler limit did not fail closed: " + JSON.stringify(terminalLimitHook));
let deadlineClock = 0;
let deadlineRuns = 0;
let deadlineTimeout = 0;
const deadlineHook = await mod.runProjectHooks("Stop", {}, process.argv[2], {
  now() { return deadlineClock; },
  async runCommand(_command, _args, options) {
    deadlineRuns++;
    deadlineTimeout = options.timeout;
    deadlineClock += 181001;
    return {status: 0, stdout: '{"decision":"block","reason":"first bounded blocker"}\n', stderr: ""};
  },
});
if (!deadlineHook?.block || !deadlineHook.reason.includes("first bounded blocker") || !deadlineHook.reason.includes("deadline exceeded")) throw new Error("terminal deadline did not fail closed with collected blockers: " + JSON.stringify(deadlineHook));
if (deadlineRuns !== 1 || deadlineTimeout !== 1000) throw new Error("terminal handler escaped its declared per-handler or total budget: " + JSON.stringify({deadlineRuns, deadlineTimeout}));
let aggregateRuns = 0;
const aggregateHook = await mod.runProjectHooks("Stop", {}, process.argv[2], {
  now() { return 0; },
  async runCommand(_command, _args, options) {
    aggregateRuns++;
    if (options.maxBuffer !== 16384 || options.timeout !== 1000) throw new Error("terminal spawn bounds were not applied: " + JSON.stringify(options));
    return {status: 0, stdout: JSON.stringify({decision:"block", reason:"x".repeat(20000)}) + "\n", stderr: ""};
  },
});
if (!aggregateHook?.block || aggregateRuns !== 2 || aggregateHook.reason.length > 16384) throw new Error("terminal aggregate collection was not bounded while preserving handler execution: " + JSON.stringify({aggregateRuns, reasonLength: aggregateHook?.reason?.length}));
const handlers = new Map();
const commands = new Map();
let activeTools;
let selectedModel;
const userMessages = [];
const statuses = [];
const notifications = [];
const fakePi = {
  on(name, handler) { handlers.set(name, handler); },
  registerCommand(name, options) { commands.set(name, options.handler); },
  setActiveTools(tools) { activeTools = tools; },
  async setModel(model) { selectedModel = model; return true; },
  sendUserMessage(message, options) { userMessages.push([message, options]); },
};
mod.default(fakePi);
const model = {provider: "openai", id: "gpt-5.6-terra"};
const ctx = {
  hasUI: false,
  modelRegistry: {find(provider, id) { return provider === model.provider && id === model.id ? model : undefined; }},
  ui: {setStatus(_key, value) { statuses.push(value); }, notify(value) { notifications.push(value); }},
};
await handlers.get("session_start")({type:"session_start"}, ctx);
if (statuses.at(-1) !== "AGM default/ready") throw new Error("missing default ready status");
const lifecycleInput = JSON.parse(readFileSync(process.argv[4], "utf8"));
if (lifecycleInput.hook_event_name !== "SessionStart") throw new Error("missing lifecycle event name");
if (lifecycleInput.session_id !== "pi-native-session") throw new Error("missing native session identity");
if (lifecycleInput.cwd !== process.argv[2]) throw new Error("missing approved cwd");
const inputResult = await handlers.get("input")({type:"input", text:"unsafe prompt", source:"interactive"}, ctx);
if (inputResult?.action !== "handled") throw new Error("blocking UserPromptSubmit hook did not consume input");
const compactResult = await handlers.get("session_before_compact")({type:"session_before_compact", reason:"manual"}, ctx);
if (!compactResult?.cancel) throw new Error("blocking PreCompact hook did not cancel compaction");
await handlers.get("agent_start")({}, ctx);
if (statuses.at(-1) !== "AGM default/working") throw new Error("missing working status");
await commands.get("agm-mode")("plan", ctx);
if (activeTools.join(",") !== "read,grep,find,ls") throw new Error("plan tools: " + activeTools);
if (statuses.at(-1) !== "AGM plan/working") throw new Error("missing plan status");
await commands.get("agm-mode")("auto", ctx);
if (!activeTools.includes("write") || statuses.at(-1) !== "AGM auto/working") throw new Error("auto mode transition failed");
const advisoryDecision = await handlers.get("tool_call")({toolName:"read", input:{path:"README.md"}}, ctx);
if (advisoryDecision !== undefined || !notifications.some((value) => String(value).includes("Pi PreToolUse hook: first guard ran"))) throw new Error("advisory PreToolUse context was not surfaced");
process.env.PI_SPEC_FEEDBACK_ID = "a".repeat(64);
await handlers.get("agent_settled")({}, ctx);
if (statuses.at(-1) !== "AGM auto/ready") throw new Error("missing settled status");
if (userMessages.length !== 1 || !userMessages[0][0].includes("finish the regression") || !userMessages[0][0].includes("second terminal handler ran") || userMessages[0][1].deliverAs !== "followUp") throw new Error("aggregated structured Stop feedback was not delivered: " + JSON.stringify(userMessages));
if (readFileSync(process.env.PI_TERMINAL_HOOK_CAPTURE, "utf8") !== "first\nsecond\n") throw new Error("terminal hook handlers did not both execute");
const extensionInput = await handlers.get("input")({type:"input", source:"extension", text:"finish the regression"}, ctx);
if (extensionInput !== undefined) throw new Error("extension follow-up was projected through UserPromptSubmit");
await handlers.get("agent_settled")({}, ctx);
if (userMessages.length !== 1) throw new Error("extension follow-up reset the Stop continuation budget");
await handlers.get("tool_result")({type:"tool_result", toolName:"subagent", input:{agent:"reviewer"}, content:[], isError:false}, ctx);
if (userMessages.length !== 2 || userMessages[1][0] !== "review the delegated result" || userMessages[1][1].deliverAs !== "followUp") throw new Error("structured SubagentStop feedback was not delivered");
await handlers.get("agent_settled")({}, ctx);
await handlers.get("tool_result")({type:"tool_result", toolName:"subagent", input:{agent:"reviewer"}, content:[], isError:false}, ctx);
if (userMessages.length !== 2) throw new Error("stop_hook_active did not bound repeated Stop/SubagentStop follow-ups: " + JSON.stringify(userMessages));
if (!notifications.some((value) => String(value).includes("yielding after one continuation"))) throw new Error("bounded terminal-hook yield was not surfaced: " + notifications.join(" | "));
process.env.PI_SPEC_FEEDBACK_ID = "b".repeat(64);
await handlers.get("agent_settled")({}, ctx);
if (userMessages.length !== 3 || !userMessages[2][0].includes("finish the regression")) throw new Error("fresh SPEC feedback identity was suppressed by sibling continuation state: " + JSON.stringify(userMessages));
await handlers.get("agent_settled")({}, ctx);
if (userMessages.length !== 3) throw new Error("repeated SPEC feedback identity escaped the bounded outer loop: " + JSON.stringify(userMessages));
for (const freshID of ["c", "d", "e", "f", "0", "1"]) {
  process.env.PI_SPEC_FEEDBACK_ID = freshID.repeat(64);
  await handlers.get("agent_settled")({}, ctx);
}
if (userMessages.length !== 9) throw new Error("fresh SPEC identities did not consume the finite per-turn continuation budget: " + JSON.stringify(userMessages));
process.env.PI_SPEC_FEEDBACK_ID = "2".repeat(64);
await handlers.get("agent_settled")({}, ctx);
if (userMessages.length !== 9) throw new Error("ninth fresh SPEC identity escaped the finite per-turn continuation budget: " + JSON.stringify(userMessages));
if (!notifications.some((value) => String(value).includes("bounded per-turn continuation budget"))) throw new Error("per-turn continuation budget yield was not surfaced: " + notifications.join(" | "));
await handlers.get("input")({type:"input", source:"rpc", text:"try the completed work again"}, ctx);
await handlers.get("agent_settled")({}, ctx);
if (userMessages.length !== 10) throw new Error("RPC input did not reset the Stop continuation budget");
await commands.get("agm-model")("openai/gpt-5.6-terra", ctx);
if (selectedModel !== model || notifications.at(-1) !== "AGM model: openai/gpt-5.6-terra") throw new Error("model transition failed");
`
	command := exec.Command(node, "--input-type=module", "-e", script, filepath.Clean(path), filepath.Clean(project), filepath.Clean(malformedProject), filepath.Clean(capturePath), filepath.Clean(timeoutProject), filepath.Clean(nonzeroProject), filepath.Clean(invalidShapeProject), filepath.Clean(oversizedProject), filepath.Clean(invalidCommandProject), filepath.Clean(terminalLimitProject))
	command.Env = append(os.Environ(),
		"AGM_PI_PROJECT_DIR="+filepath.Clean(project),
		"AGM_PI_PERMISSION_POLICY_FILE="+filepath.Clean(policyPath),
		"PI_HOOK_CAPTURE="+filepath.Clean(capturePath),
		"PI_TERMINAL_HOOK_CAPTURE="+filepath.Clean(terminalCapturePath),
		"PI_SESSION_ID=pi-native-session",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("extension parity: %v\n%s", err, output)
	}

	failClosedScript := `
import {readFileSync} from "node:fs";
const source = readFileSync(process.argv[1], "utf8");
const mod = await import("data:text/javascript;base64," + Buffer.from(source).toString("base64"));
const handlers = new Map();
const statuses = [];
const notifications = [];
mod.default({
  on(name, handler) { handlers.set(name, handler); },
  registerCommand() {},
});
const ctx = {
  hasUI: false,
  ui: {
    setStatus(_key, value) { statuses.push(value); },
    notify(value) { notifications.push(value); },
  },
};
await handlers.get("session_start")({type:"session_start"}, ctx);
if (statuses.at(-1) !== "AGM default/permission") throw new Error("policy load did not enter permission state");
if (!notifications.some((value) => String(value).includes("cannot load AGM Pi permission policy"))) throw new Error("policy failure was not surfaced: " + notifications.join(" | "));
const decision = await handlers.get("tool_call")({toolName:"read", input:{path:"README.md"}}, ctx);
if (!decision?.block || !decision.reason.includes("cannot load AGM Pi permission policy")) throw new Error("missing policy did not fail closed");
`
	failClosed := exec.Command(node, "--input-type=module", "-e", failClosedScript, filepath.Clean(path))
	failClosed.Env = append(os.Environ(),
		"AGM_PI_PROJECT_DIR="+filepath.Clean(project),
		"AGM_PI_PERMISSION_POLICY_FILE="+filepath.Join(t.TempDir(), "missing-policy.json"),
	)
	if output, err := failClosed.CombinedOutput(); err != nil {
		t.Fatalf("extension fail-closed policy: %v\n%s", err, output)
	}
}
