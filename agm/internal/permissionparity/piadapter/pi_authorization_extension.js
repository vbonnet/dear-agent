import { closeSync, fstatSync, openSync, readFileSync, readSync } from "node:fs";
import { spawn } from "node:child_process";
import { join } from "node:path";

// Keep this implementation in parity with adapter.go; the Go tests execute it.
const PLAN_TOOLS = new Set(["read", "grep", "find", "ls"]);
const MAX_HOOK_TIMEOUT_SECONDS = 120;
const MAX_HOOK_MANIFEST_BYTES = 1024 * 1024;
const MAX_HOOK_HANDLERS_PER_EVENT = 64;
const MAX_TERMINAL_HOOK_HANDLERS = 16;
// The canonical terminal chain declares 60s and 120s handlers. Preserve both
// validated budgets plus one second of deterministic orchestration allowance,
// while retaining a finite aggregate deadline for any longer chain.
const MAX_TERMINAL_RUN_MS = 181_000;
const MAX_HOOK_OUTPUT_BYTES = 1024 * 1024;
const MAX_TERMINAL_HOOK_OUTPUT_BYTES = 16 * 1024;
const MAX_TERMINAL_HOOK_FEEDBACK_CHARS = 16 * 1024;
const MAX_TERMINAL_REASON_CHARS = 12 * 1024;
const TERMINAL_CONTEXT_PREFIX = "Additional context: ";
const MAX_TERMINAL_CONTEXT_CHARS = MAX_TERMINAL_HOOK_FEEDBACK_CHARS - MAX_TERMINAL_REASON_CHARS - TERMINAL_CONTEXT_PREFIX.length - 1;
const MAX_TERMINAL_FOLLOW_UPS_PER_TURN = 8;
const SPEC_FEEDBACK_ID_PATTERN = /^[0-9a-f]{64}$/;
// A hook gets its declared execution budget.  If it has not exited at that
// point, terminate its whole process group, then force-kill shortly after.
// Terminal hooks cap that grace period at their aggregate deadline.
const HOOK_KILL_GRACE_MS = 100;

function readHookManifest(path) {
	const descriptor = openSync(path, "r");
	try {
		const info = fstatSync(descriptor);
		if (!info.isFile() || info.size > MAX_HOOK_MANIFEST_BYTES) throw new Error("Pi hook manifest exceeds its size limit");
		const buffer = Buffer.alloc(MAX_HOOK_MANIFEST_BYTES + 1);
		let used = 0;
		for (;;) {
			const count = readSync(descriptor, buffer, used, buffer.length - used, null);
			if (count === 0) break;
			used += count;
			if (used > MAX_HOOK_MANIFEST_BYTES) throw new Error("Pi hook manifest exceeds its size limit");
		}
		return buffer.subarray(0, used).toString("utf8");
	} finally {
		closeSync(descriptor);
	}
}

function validateProjectHooks(hooks) {
	const events = Object.entries(hooks);
	if (events.length > 32) return "Pi hook manifest exceeds the event limit";
	for (const [eventName, groups] of events) {
		if (!Array.isArray(groups) || groups.length > 64) return `Pi hook event ${eventName} must be a bounded array`;
		let handlerCount = 0;
		for (const group of groups) {
			if (!group || typeof group !== "object" || Array.isArray(group) || (group.matcher !== undefined && typeof group.matcher !== "string") || !Array.isArray(group.hooks) || group.hooks.length > 64) {
				return `Pi hook event ${eventName} contains an invalid group`;
			}
			for (const hook of group.hooks) {
			if (!hook || typeof hook !== "object" || Array.isArray(hook) || hook.type !== "command" || typeof hook.command !== "string" || !hook.command.trim() || hook.command.includes("\0")) {
					return `Pi hook event ${eventName} contains an invalid command handler`;
				}
				if (hook.timeout !== undefined && (typeof hook.timeout !== "number" || !Number.isFinite(hook.timeout) || hook.timeout <= 0 || hook.timeout > MAX_HOOK_TIMEOUT_SECONDS)) {
					return `Pi hook event ${eventName} contains an invalid timeout`;
				}
			}
			handlerCount += group.hooks.length;
			if (handlerCount > MAX_HOOK_HANDLERS_PER_EVENT) return `Pi hook event ${eventName} exceeds the handler limit`;
			if ((eventName === "Stop" || eventName === "SubagentStop") && handlerCount > MAX_TERMINAL_HOOK_HANDLERS) {
				return `Pi terminal hook event ${eventName} exceeds the handler limit`;
			}
		}
	}
	return "";
}

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
		const parsed = JSON.parse(readHookManifest(join(cwd, ".pi", "hooks.json")));
		if (!parsed || typeof parsed.hooks !== "object" || Array.isArray(parsed.hooks)) {
			return {hooks: {}, error: "Pi hook manifest must contain an object-valued hooks field"};
		}
		const validationError = validateProjectHooks(parsed.hooks);
		if (validationError) return {hooks: {}, error: validationError};
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
			const specFeedbackID = SPEC_FEEDBACK_ID_PATTERN.test(String(value?.dearAgentSpecFeedbackId || ""))
				? String(value.dearAgentSpecFeedbackId)
				: "";
			if (value?.decision === "block" || hookOutput?.permissionDecision === "deny") {
				return {block: true, reason: String(reason), specFeedbackID};
			}
			if (hookOutput?.additionalContext) return {context: String(hookOutput.additionalContext), specFeedbackID};
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
function appendBounded(current, value, limit) {
	const text = String(value || "").trim();
	if (!text || current.length >= limit) return current;
	const separator = current ? "\n" : "";
	const available = limit - current.length - separator.length;
	return available > 0 ? current + separator + text.slice(0, available) : current;
}

function terminationError(code, message) {
	const error = new Error(`${code}: ${message}`);
	error.code = code;
	return error;
}

function signalProcessGroup(child, signal) {
	if (!child.pid) return;
	try {
		// detached creates a new POSIX process group, so a hook cannot leave a
		// TERM-ignoring child behind when its shell is stopped.
		if (process.platform !== "win32") process.kill(-child.pid, signal);
		else child.kill(signal);
	} catch (error) {
		// A completed child races normal cleanup. Other errors are reported by
		// the child close/error path, which remains the hook result authority.
		if (error?.code !== "ESRCH") child.kill(signal);
	}
}

function appendOutput(chunks, size, value, limit) {
	if (size >= limit) return {size, overflow: true};
	const chunk = Buffer.from(value);
	const allowed = Math.min(chunk.length, limit - size);
	if (allowed > 0) chunks.push(chunk.subarray(0, allowed));
	return {size: size + allowed, overflow: chunk.length > allowed};
}

function runBoundedHook(command, args, options) {
	return new Promise((resolve) => {
		let child;
		try {
			child = spawn(command, args, {
				cwd: options.cwd,
				env: options.env,
				detached: process.platform !== "win32",
				stdio: ["pipe", "pipe", "pipe"],
			});
		} catch (error) {
			resolve({status: null, stdout: "", stderr: "", error});
			return;
		}

		const stdout = [];
		const stderr = [];
		let stdoutSize = 0;
		let stderrSize = 0;
		let timedOut = false;
		let overflowed = false;
		let settled = false;
		let forceTimer;
		const finish = (result) => {
			if (settled) return;
			settled = true;
			clearTimeout(timeoutTimer);
			clearTimeout(forceTimer);
			resolve({
				...result,
				stdout: Buffer.concat(stdout).toString("utf8"),
				stderr: Buffer.concat(stderr).toString("utf8"),
			});
		};
		const forceCleanup = () => signalProcessGroup(child, "SIGKILL");
		const beginTermination = (reason) => {
			if (timedOut || overflowed) return;
			if (reason === "timeout") timedOut = true;
			else overflowed = true;
			signalProcessGroup(child, "SIGTERM");
			const aggregateRemaining = options.deadline ? Math.max(0, options.deadline - Date.now()) : Infinity;
			forceTimer = setTimeout(forceCleanup, Math.max(0, Math.min(HOOK_KILL_GRACE_MS, aggregateRemaining)));
		};
		const timeoutTimer = setTimeout(() => beginTermination("timeout"), Math.max(1, options.timeout));
		child.stdout.on("data", (chunk) => {
			const captured = appendOutput(stdout, stdoutSize, chunk, options.maxBuffer);
			stdoutSize = captured.size;
			if (captured.overflow) beginTermination("overflow");
		});
		child.stderr.on("data", (chunk) => {
			const captured = appendOutput(stderr, stderrSize, chunk, options.maxBuffer);
			stderrSize = captured.size;
			if (captured.overflow) beginTermination("overflow");
		});
		child.stdin.on("error", () => {});
		child.stdin.end(options.input);
		child.on("error", (error) => finish({status: null, error}));
		child.on("close", (status, signal) => {
			const error = timedOut
				? terminationError("ETIMEDOUT", `hook exceeded ${options.timeout}ms`)
				: overflowed
					? terminationError("ERR_CHILD_PROCESS_STDIO_MAXBUFFER", "hook output exceeded its capture limit")
					: undefined;
			finish({status, signal, error});
		});
	});
}

export async function runProjectHooks(eventName, call, cwd = process.cwd(), runtime = {}) {
	const loaded = loadProjectHooks(cwd);
	if (loaded.error) return {block: true, reason: loaded.error};
	const hooks = loaded.hooks;
	const terminal = eventName === "Stop" || eventName === "SubagentStop";
	const now = typeof runtime.now === "function" ? runtime.now : Date.now;
	const runHook = typeof runtime.runCommand === "function" ? runtime.runCommand : runBoundedHook;
	const terminalDeadline = terminal ? now() + MAX_TERMINAL_RUN_MS : 0;
	const contexts = [];
	let contextText = "";
	let reasonText = "";
	let terminalFailed = false;
	const specFeedbackIDs = [];
	const terminalFailure = (reason) => {
		if (!terminal) return {block: true, reason};
		terminalFailed = true;
		reasonText = appendBounded(reasonText, reason || `${eventName} hook failed`, MAX_TERMINAL_REASON_CHARS);
		return undefined;
	};
	hookGroups:
	for (const group of hooks[eventName] || []) {
		if (!hookMatches(group.matcher, call?.toolName)) continue;
		for (const hook of group.hooks) {
			let timeout = (hook.timeout || 30) * 1000;
			if (terminal) {
				const remaining = terminalDeadline - now();
				if (remaining <= 0) {
					terminalFailure(`${eventName} hook execution deadline exceeded before all handlers ran`);
					break hookGroups;
				}
				timeout = Math.max(1, Math.min(timeout, remaining));
			}
			let result;
			try {
				result = await runHook("/bin/sh", ["-c", hook.command], {
					cwd,
					env: {...process.env, PI_PROJECT_DIR: cwd, CLAUDE_PROJECT_DIR: cwd},
					input: hookInput(eventName, call, cwd),
					encoding: "utf8",
					timeout,
					maxBuffer: terminal ? MAX_TERMINAL_HOOK_OUTPUT_BYTES : MAX_HOOK_OUTPUT_BYTES,
					deadline: terminal ? terminalDeadline : 0,
				});
			} catch (error) {
				const failure = terminalFailure(`${eventName} hook could not start: ${error?.message || error}`);
				if (failure) return failure;
				continue;
			}
			const structured = parseHookOutput(result.stdout);
			if (terminal && structured?.specFeedbackID && !specFeedbackIDs.includes(structured.specFeedbackID) && specFeedbackIDs.length < MAX_TERMINAL_HOOK_HANDLERS) {
				specFeedbackIDs.push(structured.specFeedbackID);
			}
			if (structured?.block) {
				const failure = terminalFailure(structured.reason);
				if (failure) return failure;
				continue;
			}
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
				const hookFailure = terminalFailure(detail || `${eventName} hook failed`);
				if (hookFailure) return hookFailure;
				continue;
			}
			if (structured?.context) {
				if (terminal) contextText = appendBounded(contextText, structured.context, MAX_TERMINAL_CONTEXT_CHARS);
				else contexts.push(structured.context);
			}
		}
	}
	if (terminalFailed) {
		const context = contextText ? `${TERMINAL_CONTEXT_PREFIX}${contextText}` : "";
		return {block: true, reason: [reasonText, context].filter(Boolean).join("\n") || `${eventName} hook failed`, specFeedbackIDs};
	}
	if (contextText) return {context: contextText, specFeedbackIDs};
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
	// Stop continuations are bounded independently for parent and subagent
	// completion. Pi does not have Claude's Stop-hook retry field, so the
	// extension passes its explicit loop state into the declarative projection
	// and refuses a second follow-up for the same terminal event.
	const activeStopEvents = new Set();
	const lastSpecFeedbackByEvent = new Map();
	const terminalFollowUpsThisTurn = new Map();
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
		const result = await runProjectHooks("SessionStart", {event: _event}, projectDir);
		if (result?.block) ctx.ui.notify(`Pi SessionStart hook: ${result.reason}`, "warning");
	});
	pi.on("input", async (_event, ctx) => {
		// A synthetic follow-up created below must not reset the one-shot stop
		// budget. Pi reports extension-created follow-ups with source=extension;
		// only a real interactive or RPC user turn starts a fresh bounded turn.
		if (_event?.source === "extension") return undefined;
		if (_event?.source === "interactive" || _event?.source === "rpc") {
			activeStopEvents.clear();
			lastSpecFeedbackByEvent.clear();
			terminalFollowUpsThisTurn.clear();
		}
		const result = await runProjectHooks("UserPromptSubmit", {event: _event}, projectDir);
		if (result?.block) {
			ctx.ui.notify(`Pi UserPromptSubmit hook: ${result.reason}`, "warning");
			return {action: "handled"};
		}
		return undefined;
	});
	pi.on("session_before_compact", async (_event, ctx) => {
		const result = await runProjectHooks("PreCompact", {event: _event}, projectDir);
		if (result?.block) {
			ctx.ui.notify(`Pi PreCompact hook: ${result.reason}`, "warning");
			return {cancel: true};
		}
		return undefined;
	});
	pi.on("session_compact", async (_event, ctx) => {
		const result = await runProjectHooks("PostCompact", {event: _event}, projectDir);
		if (result?.block) ctx.ui.notify(`Pi PostCompact hook: ${result.reason}`, "warning");
	});
	pi.on("agent_start", async (_event, ctx) => {
		state = "working";
		updateStatus(ctx);
	});
	const handleTerminalHook = async (eventName, call, ctx) => {
		const stopHookActive = activeStopEvents.has(eventName);
		const result = await runProjectHooks(eventName, {...call, stopHookActive}, projectDir);
		if (result?.block) {
			const specFeedbackIDs = Array.isArray(result.specFeedbackIDs) ? result.specFeedbackIDs : [];
			const nextSpecFeedbackID = specFeedbackIDs.at(-1) || "";
			const freshSpecFeedback = nextSpecFeedbackID && nextSpecFeedbackID !== lastSpecFeedbackByEvent.get(eventName);
			const followUps = terminalFollowUpsThisTurn.get(eventName) || 0;
			if (followUps >= MAX_TERMINAL_FOLLOW_UPS_PER_TURN) {
				ctx.ui.notify(`Pi ${eventName} hook reached its bounded per-turn continuation budget; yielding: ${result.reason}`, "warning");
				return;
			}
			if (stopHookActive && !freshSpecFeedback) {
				ctx.ui.notify(`Pi ${eventName} hook is still blocking; yielding after one continuation to avoid a retry loop: ${result.reason}`, "warning");
				return;
			}
			activeStopEvents.add(eventName);
			terminalFollowUpsThisTurn.set(eventName, followUps + 1);
			if (nextSpecFeedbackID) lastSpecFeedbackByEvent.set(eventName, nextSpecFeedbackID);
			ctx.ui.notify(`Pi ${eventName} hook: ${result.reason}`, "warning");
			pi.sendUserMessage(result.reason, {deliverAs: "followUp"});
			return;
		}
		activeStopEvents.delete(eventName);
		if (result?.context) ctx.ui.notify(`Pi ${eventName} hook: ${result.context}`, "warning");
	};
	pi.on("agent_settled", async (_event, ctx) => {
		state = "ready";
		updateStatus(ctx);
		await handleTerminalHook("Stop", {event: _event}, ctx);
	});
	pi.on("tool_result", async (_event, ctx) => {
		if (String(_event?.toolName || "").toLowerCase() !== "subagent") return undefined;
		await handleTerminalHook("SubagentStop", {
			toolName: _event.toolName,
			input: _event.input,
			event: _event,
		}, ctx);
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
		const guardrail = await runProjectHooks("PreToolUse", {toolName: event.toolName, input: event.input, event}, projectDir);
		if (guardrail?.block) return guardrail;
		if (guardrail?.context) ctx.ui.notify(`Pi PreToolUse hook: ${guardrail.context}`, "warning");
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
