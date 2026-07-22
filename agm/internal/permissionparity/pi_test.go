package permissionparity

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	const hooks = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"hookSpecificOutput\":{\"additionalContext\":\"first guard ran\"}}\\n'","timeout":1},{"type":"command","command":"cat >/dev/null; printf 'project guard rejected' >&2; exit 42","timeout":1}]}],"SessionStart":[{"hooks":[{"type":"command","command":"tee \"$PI_HOOK_CAPTURE\" >/dev/null","timeout":1}]}],"UserPromptSubmit":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"decision\":\"block\",\"reason\":\"prompt rejected\"}\\n'","timeout":1}]}],"PreCompact":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"hookSpecificOutput\":{\"permissionDecision\":\"deny\",\"additionalContext\":\"compaction rejected\"}}\\n'","timeout":1}]}],"Stop":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"decision\":\"block\",\"reason\":\"finish the regression\"}\\n'","timeout":1}]}],"SubagentStop":[{"hooks":[{"type":"command","command":"cat >/dev/null; printf '{\"decision\":\"block\",\"reason\":\"review the delegated result\"}\\n'","timeout":1}]}]}}`
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
	capturePath := filepath.Join(t.TempDir(), "hook-input.json")
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
  hookResult = mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git push"}}, process.argv[2]);
  if (hookResult?.reason?.includes("project guard rejected") || !hookResult?.reason?.includes("ETIMEDOUT")) break;
}
if (!hookResult?.block || !hookResult.reason.includes("project guard rejected")) throw new Error("project hook did not fail closed with its declared reason: " + JSON.stringify(hookResult));
const timeoutHook = mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git push"}}, process.argv[5]);
if (!timeoutHook?.block || !timeoutHook.reason.includes("ETIMEDOUT") || !timeoutHook.reason.includes("timeout stderr") || timeoutHook.reason.includes("timeout context")) throw new Error("project hook timeout masked its execution failure: " + JSON.stringify(timeoutHook));
const nonzeroHook = mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git push"}}, process.argv[6]);
if (!nonzeroHook?.block || !nonzeroHook.reason.startsWith("PreToolUse hook exited with status 42") || !nonzeroHook.reason.includes("advisory only")) throw new Error("project hook nonzero status was masked by advisory context: " + JSON.stringify(nonzeroHook));
const unmatchedHook = mod.runProjectHooks("PreToolUse", {toolName:"read", input:{path:"README.md"}}, process.argv[2]);
if (unmatchedHook !== undefined) throw new Error("matcher ran for an unrelated tool");
const malformedHook = mod.runProjectHooks("PreToolUse", {toolName:"bash", input:{command:"git status"}}, process.argv[3]);
if (!malformedHook?.block || !malformedHook.reason.includes("cannot load Pi hook manifest")) throw new Error("malformed hook manifest did not fail closed");
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
await handlers.get("agent_settled")({}, ctx);
if (statuses.at(-1) !== "AGM auto/ready") throw new Error("missing settled status");
if (userMessages.length !== 1 || userMessages[0][0] !== "finish the regression" || userMessages[0][1].deliverAs !== "followUp") throw new Error("structured Stop feedback was not delivered");
await handlers.get("tool_result")({type:"tool_result", toolName:"subagent", input:{agent:"reviewer"}, content:[], isError:false}, ctx);
if (userMessages.length !== 2 || userMessages[1][0] !== "review the delegated result" || userMessages[1][1].deliverAs !== "followUp") throw new Error("structured SubagentStop feedback was not delivered");
await commands.get("agm-model")("openai/gpt-5.6-terra", ctx);
if (selectedModel !== model || notifications.at(-1) !== "AGM model: openai/gpt-5.6-terra") throw new Error("model transition failed");
`
	command := exec.Command(node, "--input-type=module", "-e", script, filepath.Clean(path), filepath.Clean(project), filepath.Clean(malformedProject), filepath.Clean(capturePath), filepath.Clean(timeoutProject), filepath.Clean(nonzeroProject))
	command.Env = append(os.Environ(),
		"AGM_PI_PROJECT_DIR="+filepath.Clean(project),
		"AGM_PI_PERMISSION_POLICY_FILE="+filepath.Clean(policyPath),
		"PI_HOOK_CAPTURE="+filepath.Clean(capturePath),
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
