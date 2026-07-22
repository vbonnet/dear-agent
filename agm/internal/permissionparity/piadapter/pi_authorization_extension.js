import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join } from "node:path";

// Keep this implementation in parity with adapter.go; the Go tests execute it.
const PLAN_TOOLS = new Set(["read", "grep", "find", "ls"]);

function piToolName(toolName) {
	const raw = String(toolName || "");
	const builtIn = ({bash: "Bash", read: "Read", edit: "Edit", write: "Write", grep: "Grep", find: "Glob", ls: "Read"})[
		raw.toLowerCase()
	];
	if (builtIn) return builtIn;
	if (!/^[A-Za-z0-9_-]+$/.test(raw)) return "";
	return raw.split(/[_-]/).filter(Boolean).map((part) => part[0].toUpperCase() + part.slice(1)).join("");
}

function loadProjectHooks(cwd) {
	try {
		const parsed = JSON.parse(readFileSync(join(cwd, ".pi", "hooks.json"), "utf8"));
		if (!parsed || typeof parsed.hooks !== "object" || Array.isArray(parsed.hooks)) {
			return {hooks: {}, error: "Pi hook manifest must contain an object-valued hooks field"};
		}
		return {hooks: parsed.hooks};
	} catch (error) {
		if (error?.code === "ENOENT") return {hooks: {}};
		return {hooks: {}, error: `cannot load Pi hook manifest: ${error?.message || error}`};
	}
}

function hookMatches(matcher, toolName) {
	if (!matcher) return true;
	return String(matcher).split("|").map((part) => part.trim()).includes(piToolName(toolName));
}

function hookInput(eventName, call, cwd) {
	const event = call?.event || {};
	return JSON.stringify({
		hook_event_name: eventName,
		session_id: process.env.PI_SESSION_ID || process.env.AGM_SESSION_NAME || "",
		cwd,
		stop_hook_active: Boolean(call?.stopHookActive),
		tool_name: piToolName(call?.toolName),
		tool_input: call?.input || {},
		prompt: typeof event.text === "string" ? event.text : "",
		event_data: event,
	});
}

function parseHookOutput(output) {
	const lines = String(output || "").split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
	for (let index = lines.length - 1; index >= 0; index--) {
		try {
			const value = JSON.parse(lines[index]);
			const hookOutput = value?.hookSpecificOutput || {};
			const reason = value?.reason || hookOutput?.additionalContext || "hook rejected the event";
			if (value?.decision === "block" || hookOutput?.permissionDecision === "deny") {
				return {block: true, reason: String(reason)};
			}
			if (hookOutput?.additionalContext) return {context: String(hookOutput.additionalContext)};
		} catch {
			// Hooks may print diagnostics before an optional final JSON decision.
		}
	}
	return undefined;
}

// runProjectHooks projects Pi events through the repository's declarative
// hook manifest. Project trust and tool authorization remain separate: this
// loads only after AGM has explicitly approved the selected working directory,
// while the mandatory authorization decision still runs afterward.
export function runProjectHooks(eventName, call, cwd = process.cwd()) {
	const loaded = loadProjectHooks(cwd);
	if (loaded.error) return {block: true, reason: loaded.error};
	const hooks = loaded.hooks;
	const contexts = [];
	for (const group of hooks[eventName] || []) {
		if (!hookMatches(group.matcher, call?.toolName)) continue;
		for (const hook of group.hooks || []) {
			if (hook.type !== "command" || typeof hook.command !== "string") continue;
			const result = spawnSync("/bin/sh", ["-c", hook.command], {
				cwd,
				env: {...process.env, PI_PROJECT_DIR: cwd, CLAUDE_PROJECT_DIR: cwd},
				input: hookInput(eventName, call, cwd),
				encoding: "utf8",
				timeout: Math.max(1, Number(hook.timeout) || 30) * 1000,
				maxBuffer: 1024 * 1024,
			});
			const structured = parseHookOutput(result.stdout);
			if (structured?.block) return structured;
			if (result.error || result.status !== 0) {
				const stderr = String(result.stderr || "").trim();
				let failure;
				let partial = stderr;
				if (result.error) {
					failure = String(result.error.message || result.error).trim();
				} else {
					failure = result.signal
						? `${eventName} hook terminated by ${result.signal}`
						: `${eventName} hook exited with status ${result.status}`;
					partial ||= String(structured?.context || result.stdout || "").trim();
				}
				const detail = [failure, partial].filter(Boolean).join(": ");
				return {block: true, reason: detail || `${eventName} hook failed`};
			}
			if (structured?.context) contexts.push(structured.context);
		}
	}
	if (contexts.length > 0) return {context: contexts.join("\n")};
	return undefined;
}

function permissionTarget(call) {
	const input = call.input || {};
	const value = (...keys) => {
		for (const key of keys) {
			if (typeof input[key] === "string") return input[key];
		}
		return "";
	};
	switch ((call.toolName || "").toLowerCase()) {
		case "bash": return ["Bash", value("command")];
		case "read": return ["Read", value("path", "file_path")];
		case "edit": return ["Edit", value("path", "file_path")];
		case "write": return ["Write", value("path", "file_path")];
		case "grep": return ["Grep", value("path")];
		case "find": return ["Glob", value("path")];
		case "ls": return ["Read", value("path")];
		default: return [piToolName(call.toolName), ""];
	}
}

function parseEntry(entry) {
	entry = String(entry || "").trim();
	if (!entry) return null;
	const open = entry.indexOf("(");
	if (open < 0) return [entry, ""];
	if (open === 0 || !entry.endsWith(")")) return null;
	return [entry.slice(0, open), entry.slice(open + 1, -1)];
}

function patternMatches(pattern, value) {
	pattern = pattern.trim();
	value = value.trim();
	if (pattern.endsWith(":" + "*")) {
		const base = pattern.slice(0, -2).trim();
		return value === base || value.startsWith(base + " ");
	}
	if (pattern.startsWith("~/") && process.env.HOME) {
		pattern = process.env.HOME.replace(/\/$/, "") + "/" + pattern.slice(2);
	}
	const escaped = pattern.split("*").map((part) => part.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join(".*");
	return new RegExp("^" + escaped + "$").test(value);
}

function containsUnquotedShellControl(command) {
	let quote = "";
	let escaped = false;
	for (let index = 0; index < command.length; index++) {
		const current = command[index];
		if (escaped) {
			escaped = false;
			continue;
		}
		if (quote === "'") {
			if (current === "'") quote = "";
			continue;
		}
		if (quote === '"') {
			if (current === "\\") escaped = true;
			else if (current === '"') quote = "";
			else if (current === "`" || (current === "$" && command[index + 1] === "(")) return true;
			continue;
		}
		if (current === "\\") escaped = true;
		else if (current === "'" || current === '"') quote = current;
		else if (";&|<>`\n\r".includes(current) || (current === "$" && command[index + 1] === "(")) return true;
	}
	return false;
}

export function policyAllows(allow, call) {
	const [category, value] = permissionTarget(call);
	if (!category) return false;
	if (category === "Bash" && containsUnquotedShellControl(value)) return false;
	for (const raw of allow || []) {
		const entry = parseEntry(raw);
		if (!entry || entry[0] !== category) continue;
		if (!entry[1] || patternMatches(entry[1], value)) return true;
	}
	return false;
}

export function decide(mode, allow, call, interactive) {
	if (mode === "auto") return {action: "allow", reason: "AGM auto mode"};
	if (mode === "plan" && !PLAN_TOOLS.has(String(call.toolName || "").toLowerCase())) {
		return {action: "block", reason: "Tool is disabled in AGM plan mode"};
	}
	if (policyAllows(allow, call)) return {action: "allow", reason: "Matches AGM permission policy"};
	if (interactive) return {action: "ask", reason: "Not pre-approved by AGM permission policy"};
	return {action: "block", reason: "Unmatched AGM permission call blocked without an interactive UI"};
}

export function toolsForMode(mode) {
	return mode === "plan"
		? ["read", "grep", "find", "ls"]
		: ["read", "bash", "edit", "write", "grep", "find", "ls"];
}

export default function (pi) {
	let mode = process.env.AGM_PI_PERMISSION_MODE || "default";
	const launchID = process.env.AGM_PI_LAUNCH_ID || "";
	let state = "ready";
	let allow = [];
	let stopHookActive = false;
	const projectDir = process.env.AGM_PI_PROJECT_DIR || process.cwd();
	let policyLoadError = "";
	try {
		const policyFile = process.env.AGM_PI_PERMISSION_POLICY_FILE || "";
		const rawPolicy = policyFile ? readFileSync(policyFile, "utf8") : (process.env.AGM_PI_PERMISSION_POLICY || '{"allow":[]}');
		const policy = JSON.parse(rawPolicy);
		if (Array.isArray(policy.allow)) allow = policy.allow;
		else throw new Error("allow must be an array");
	} catch (error) {
		allow = [];
		state = "permission";
		policyLoadError = `cannot load AGM Pi permission policy: ${error?.message || error}`;
	}
	const updateStatus = (ctx) => ctx.ui.setStatus("agm-pi", `AGM ${mode}/${state}${launchID ? ` ${launchID}` : ""}`);

	pi.on("session_start", async (_event, ctx) => {
		updateStatus(ctx);
		if (policyLoadError) ctx.ui.notify(policyLoadError, "error");
		const result = runProjectHooks("SessionStart", {event: _event}, projectDir);
		if (result?.block) ctx.ui.notify(`Pi SessionStart hook: ${result.reason}`, "warning");
	});
	pi.on("input", async (_event, ctx) => {
		const result = runProjectHooks("UserPromptSubmit", {event: _event}, projectDir);
		if (result?.block) {
			ctx.ui.notify(`Pi UserPromptSubmit hook: ${result.reason}`, "warning");
			return {action: "handled"};
		}
		return undefined;
	});
	pi.on("session_before_compact", async (_event, ctx) => {
		const result = runProjectHooks("PreCompact", {event: _event}, projectDir);
		if (result?.block) {
			ctx.ui.notify(`Pi PreCompact hook: ${result.reason}`, "warning");
			return {cancel: true};
		}
		return undefined;
	});
	pi.on("session_compact", async (_event, ctx) => {
		const result = runProjectHooks("PostCompact", {event: _event}, projectDir);
		if (result?.block) ctx.ui.notify(`Pi PostCompact hook: ${result.reason}`, "warning");
	});
	pi.on("agent_start", async (_event, ctx) => {
		state = "working";
		updateStatus(ctx);
	});
	pi.on("agent_settled", async (_event, ctx) => {
		state = "ready";
		updateStatus(ctx);
		const result = runProjectHooks("Stop", {event: _event, stopHookActive}, projectDir);
		if (result?.block) {
			stopHookActive = true;
			ctx.ui.notify(`Pi Stop hook: ${result.reason}`, "warning");
			pi.sendUserMessage(result.reason, {deliverAs: "followUp"});
		} else {
			stopHookActive = false;
			if (result?.context) ctx.ui.notify(`Pi Stop hook: ${result.context}`, "warning");
		}
	});
	pi.on("tool_result", async (_event, ctx) => {
		if (String(_event?.toolName || "").toLowerCase() !== "subagent") return undefined;
		const result = runProjectHooks("SubagentStop", {
			toolName: _event.toolName,
			input: _event.input,
			event: _event,
		}, projectDir);
		if (result?.block) {
			ctx.ui.notify(`Pi SubagentStop hook: ${result.reason}`, "warning");
			pi.sendUserMessage(result.reason, {deliverAs: "followUp"});
		} else if (result?.context) {
			ctx.ui.notify(`Pi SubagentStop hook: ${result.context}`, "warning");
		}
		return undefined;
	});

	pi.registerCommand("agm-mode", {
		description: "Set AGM permission mode (plan, default, or auto)",
		handler: async (args, ctx) => {
			const requested = String(args || "").trim();
			if (!["plan", "default", "auto"].includes(requested)) {
				ctx.ui.notify("Usage: /agm-mode plan|default|auto", "warning");
				return;
			}
			mode = requested;
			pi.setActiveTools(toolsForMode(mode));
			updateStatus(ctx);
			ctx.ui.notify(`AGM permission mode: ${mode}`, "info");
		},
	});
	pi.registerCommand("agm-model", {
		description: "Select an exact provider/model through AGM",
		handler: async (args, ctx) => {
			const requested = String(args || "").trim();
			const separator = requested.indexOf("/");
			if (separator <= 0 || separator === requested.length - 1) {
				ctx.ui.notify("Usage: /agm-model provider/model", "warning");
				return;
			}
			const provider = requested.slice(0, separator);
			const modelId = requested.slice(separator + 1);
			const model = ctx.modelRegistry.find(provider, modelId);
			if (!model) {
				ctx.ui.notify(`AGM model unavailable: ${requested}`, "error");
				return;
			}
			if (!await pi.setModel(model)) {
				ctx.ui.notify(`AGM model has no configured authentication: ${requested}`, "error");
				return;
			}
			ctx.ui.notify(`AGM model: ${requested}`, "info");
		},
	});

	pi.on("tool_call", async (event, ctx) => {
		if (policyLoadError) return {block: true, reason: policyLoadError};
		const guardrail = runProjectHooks("PreToolUse", {toolName: event.toolName, input: event.input, event}, projectDir);
		if (guardrail?.block) return guardrail;
		const result = decide(mode, allow, {toolName: event.toolName, input: event.input}, ctx.hasUI);
		if (result.action === "allow") return undefined;
		if (result.action === "block") return {block: true, reason: result.reason};
		const confirmed = await ctx.ui.confirm(
			"AGM permission required",
			`${event.toolName} is not pre-approved by the AGM policy. Allow this call?`,
		);
		return confirmed ? undefined : {block: true, reason: "Denied by user"};
	});
}
