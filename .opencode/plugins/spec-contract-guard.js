import process from "node:process";
import {closeSync, constants as fsConstants, mkdtempSync, openSync, rmdirSync, unlinkSync, writeSync} from "node:fs";
import {tmpdir} from "node:os";
import {isAbsolute, join} from "node:path";
import {setTimeout as delay} from "node:timers/promises";
import {spawn} from "node:child_process";

// Repository-local OpenCode transport. Native before-tool hooks preserve the
// repository guard decisions; session termination itself cannot be blocked,
// so terminal feedback remains one bounded follow-up per real session turn.
const maxAdapterOutputBytes = 64 * 1024;
const maxGuardOutputBytes = 64 * 1024;
const maxGuardInputBytes = 1024 * 1024;
const maxSupervisorStatusBytes = 4;
const adapterTimeoutMs = 60_000;
const adapterDrainMs = 250;
const supervisorReapMs = 1_000;
const maxTrackedMessageIDs = 4096;
const maxTrackedSessions = 256;
const preToolGuards = Object.freeze([
  {path: ".opencode/hooks/pretool-spawn-routing", timeout: 5_000},
  {path: ".opencode/hooks/pretool-bead-close-guard", timeout: 30_000},
  {path: ".opencode/hooks/pretool-bypass-guard", timeout: 5_000},
  {path: ".opencode/hooks/pretool-pr-guard", timeout: 5_000},
]);

function preToolGuardPlan(tool) {
  if (tool === "bash") return preToolGuards;
  if (tool === "scheduled-tasks_create_scheduled_task" || tool === "mcp__scheduled-tasks__create_scheduled_task") return preToolGuards.slice(0, 1);
  return [];
}

function legacyToolName(tool) {
  if (tool === "bash") return "Bash";
  if (tool === "scheduled-tasks_create_scheduled_task") return "mcp__scheduled-tasks__create_scheduled_task";
  return tool;
}

function decodeGuardOutput(value) {
  if (typeof value === "string") return {bytes: new TextEncoder().encode(value).byteLength, text: value.trim()};
  const bytes = value instanceof Uint8Array ? value : new Uint8Array(value || []);
  return {bytes: bytes.byteLength, text: new TextDecoder().decode(bytes).trim()};
}

function parseGuardResponse(stdout) {
  if (!stdout) return {};
  let response;
  try { response = JSON.parse(stdout); } catch (error) {
    return {error: `returned invalid JSON: ${error?.message || error}`};
  }
  if (!response || typeof response !== "object" || Array.isArray(response)) {
    return {error: "returned a non-object JSON response"};
  }
  const hookOutput = response?.hookSpecificOutput;
  if (hookOutput !== undefined && (!hookOutput || typeof hookOutput !== "object" || Array.isArray(hookOutput))) {
    return {error: "returned an invalid hookSpecificOutput"};
  }
  for (const [name, value] of [
    ["hookSpecificOutput.additionalContext", hookOutput?.additionalContext],
    ["denialReason", response?.denialReason],
    ["reason", response?.reason],
  ]) {
    if (value !== undefined && typeof value !== "string") return {error: `returned a non-string ${name}`};
  }
  const message = [
    hookOutput?.additionalContext,
    response?.denialReason,
    response?.reason,
  ].find((value) => typeof value === "string" && value.trim())?.trim();
  const decisions = [hookOutput?.permissionDecision, response?.permissionDecision].filter((value) => value !== undefined);
  if (decisions.some((decision) => decision !== "allow" && decision !== "deny")) {
    return {error: "returned an unsupported permission decision"};
  }
  if (decisions.length === 2 && decisions[0] !== decisions[1]) return {error: "returned conflicting permission decisions"};
  const decision = decisions[0];
  return {blocked: decision === "deny", message};
}

function anonymousPayloadFD(payload) {
  const directory = mkdtempSync(join(tmpdir(), "dear-agent-opencode-hook-"));
  const path = join(directory, "payload");
  let descriptor;
  try {
    descriptor = openSync(path, fsConstants.O_CREAT | fsConstants.O_EXCL | fsConstants.O_RDWR, 0o600);
    unlinkSync(path);
    rmdirSync(directory);
    const bytes = new TextEncoder().encode(payload);
    let written = 0;
    while (written < bytes.byteLength) written += writeSync(descriptor, bytes, written, bytes.byteLength - written, written);
    return descriptor;
  } catch (error) {
    if (descriptor !== undefined) try { closeSync(descriptor); } catch { /* Preserve the creation error. */ }
    try { unlinkSync(path); } catch { /* The file may already be anonymous. */ }
    try { rmdirSync(directory); } catch { /* Preserve the creation error. */ }
    throw error;
  }
}

async function runPreToolGuard(root, guard, payload) {
  if (typeof root !== "string" || !isAbsolute(root)) {
    return {error: "OpenCode did not provide an absolute project worktree"};
  }
  if (process.platform === "win32") {
    return {error: "OpenCode repository pre-tool guards require a POSIX shell"};
  }
  const result = await runAdapter(root, {
    args: [join(root, guard.path)],
    input: payload,
    timeout: guard.timeout,
    raw: true,
  });
  if (result.error) return {error: `${guard.path}: ${result.error}`};
  const stdout = decodeGuardOutput(result?.stdout);
  const stderr = decodeGuardOutput(result?.stderr);
  if (stdout.bytes + stderr.bytes > maxGuardOutputBytes) {
    return {error: `${guard.path} exceeded ${maxGuardOutputBytes} combined output bytes`};
  }
  const parsed = parseGuardResponse(stdout.text);
  if (parsed.error) return {error: `${guard.path} ${parsed.error}`};
  if (result?.exitCode === 2 || parsed.blocked) {
    return {blocked: true, message: parsed.message || stderr.text || `${guard.path} denied this tool call`};
  }
  if (result?.exitCode !== 0) {
    return {error: `${guard.path} failed with exit ${result?.exitCode}${stderr.text ? `: ${stderr.text}` : ""}`};
  }
  return parsed.message ? {message: parsed.message} : {};
}

// The detached shell remains the group leader until its private stdin control
// pipe receives cleanup or EOF. Its monitor then self-signals the current
// process group, avoiding parent-side numeric PID/PGID races. The real hook
// receives neither control stdin nor the supervisor-only status descriptor.
const supervisorProgram = `
trap '' HUP INT QUIT TERM PIPE
exec 5<&0
(
  trap '' HUP INT QUIT TERM PIPE
  exec 3>&-
  exec 4>&-
  exec 5>&-
  IFS= read -r _ || :
  while :; do
    kill -KILL 0
    /bin/sleep 0.05 || :
  done
) 0<&5 >/dev/null 2>&1 &
cleanup_monitor=$!
exec 5<&-
(
  trap - HUP INT QUIT TERM PIPE
  exec 3>&-
  exec 5>&-
  exec 0<&4
  exec 4>&-
  exec "$@"
) &
hook_pid=$!
exec 4>&-
wait "$hook_pid"
hook_status=$?
printf '%s\\n' "$hook_status" >&3 || :
exec 3>&-
exec 1>/dev/null 2>/dev/null
wait "$cleanup_monitor" || :
IFS= read -r _ || :
while :; do
  kill -KILL 0
  /bin/sleep 0.05 || :
done
`;

function boundedReader(stream, streamName, budget, terminate, maxBytes = maxAdapterOutputBytes) {
  const webReader = stream?.getReader?.();
  // OpenCode's Bun runtime exposes node:child_process extra descriptors as
  // Node streams, but Bun 1.3.14's Readable.toWeb wrapper never delivers or
  // closes fd 3. The native async iterator preserves bounded backpressure in
  // Bun and Node without a second runtime-specific spawn path.
  const iterator = webReader ? undefined : stream?.[Symbol.asyncIterator]?.();
  if (!webReader && !iterator) throw new Error(`SPEC contract adapter ${streamName} stream is unavailable`);
  let cancelPromise;
  let cancelled = false;
  const cancel = () => {
    if (cancelPromise) return cancelPromise;
    cancelPromise = (async () => {
      cancelled = true;
      try {
        if (webReader) {
          await webReader.cancel();
        } else {
          stream.destroy?.();
          await iterator.return?.();
        }
      } catch { /* Process termination is the authoritative boundary. */ }
    })();
    return cancelPromise;
  };
  const read = async () => {
    const chunks = [];
    let streamBytes = 0;
    try {
      for (;;) {
        const {done, value} = webReader ? await webReader.read() : await iterator.next();
        if (done) break;
        const chunk = value instanceof Uint8Array ? value : new Uint8Array(value || []);
        if (budget.used + chunk.byteLength > maxBytes) {
          terminate({kind: "overflow", streamName});
          return "";
        }
        budget.used += chunk.byteLength;
        streamBytes += chunk.byteLength;
        chunks.push(chunk);
      }
    } catch (error) {
      if (!cancelled) {
        terminate({kind: "read", streamName, error});
        return "";
      }
    } finally {
      if (webReader) {
        try { webReader.releaseLock(); } catch { /* A cancelled reader may already be released. */ }
      }
    }
    const joined = new Uint8Array(streamBytes);
    let offset = 0;
    for (const chunk of chunks) { joined.set(chunk, offset); offset += chunk.byteLength; }
    return new TextDecoder().decode(joined);
  };
  return {cancel, read};
}

async function runAdapter(worktree, invocation = {}) {
  if (typeof worktree !== "string" || !worktree) {
    return {error: "OpenCode did not provide a project worktree"};
  }
  if (process.platform === "win32") {
    return {error: "OpenCode SPEC contract adapter requires POSIX process-group termination"};
  }
  let proc;
  let payloadFD;
  try {
    payloadFD = anonymousPayloadFD(invocation.input || "");
    proc = spawn("/bin/sh", [
      "-c", supervisorProgram, "dear-agent-opencode-spec-supervisor",
      ...(invocation.args || ["go", "run", "./cmd/spec-contract-hook", "--root", worktree, "--provider", "opencode", "--event", "Stop"]),
    ], {
      cwd: worktree, detached: true, stdio: ["pipe", "pipe", "pipe", "pipe", payloadFD],
    });
  } catch (error) {
    return {error: `could not start shared Go SPEC contract adapter supervisor: ${error?.message || error}`};
  } finally {
    if (payloadFD !== undefined) try { closeSync(payloadFD); } catch { /* The child owns its duplicated descriptor. */ }
  }
  // The control pipe is a real Node stream once the child is gone (e.g. a
  // timeout or overflow kill races the cleanup write), a write against it
  // reports failure asynchronously via an "error" event rather than a
  // synchronous throw or a rejected promise. Node crashes the whole process
  // on an unhandled stream "error" event, so this pipe needs its own
  // listener independent of requestSupervisorCleanup's try/catch, which
  // only ever observes synchronous failures.
  proc.stdin?.on("error", () => { /* Surfaced through requestSupervisorCleanup and the reap check below instead. */ });

  const readers = [];
  const budget = {used: 0};
  let termination;
  let resolveTermination;
  let cleanupRequestPromise;
  const terminationPromise = new Promise((resolve) => { resolveTermination = resolve; });
  const supervisorExit = new Promise((resolve) => {
    proc.once("exit", (exitCode, signal) => {
      resolve({exitCode, signal: signal || ""});
    });
    proc.once("error", (error) => {
      resolve({error});
    });
  });
  const requestSupervisorCleanup = () => {
    if (cleanupRequestPromise) return cleanupRequestPromise;
    // stdin is a supervisor-only control pipe. The real hook receives only
    // the anonymous payload descriptor as stdin, while control-pipe EOF gives
    // the monitor an independent parent-death cleanup signal. No parent-side
    // numeric process identity is ever used.
    cleanupRequestPromise = (async () => {
      try {
        if (!proc.stdin?.write || !proc.stdin?.end) throw new Error("supervisor control pipe is unavailable");
        await Promise.resolve(proc.stdin.write("cleanup\n"));
        if (proc.stdin.flush) await Promise.resolve(proc.stdin.flush());
        await Promise.resolve(proc.stdin.end());
        return undefined;
      } catch (error) {
        try { await Promise.resolve(proc.stdin?.end?.()); } catch { /* EOF remains the fallback cleanup request. */ }
        return error;
      }
    })();
    return cleanupRequestPromise;
  };
  const terminate = (cause) => {
    if (termination) return;
    termination = cause;
    resolveTermination(termination);
    void requestSupervisorCleanup();
    for (const reader of readers) void reader.cancel();
  };
  const settleReaders = async (readerPromises, milliseconds) => {
    const settled = Promise.allSettled(readerPromises);
    return Promise.race([
      settled.then((results) => ({results, timedOut: false})),
      delay(milliseconds).then(() => ({results: undefined, timedOut: true})),
    ]);
  };

  const statusStream = proc.stdio?.[3];
  let timeout;
  try {
    let readerSetupError;
    let outputReaders;
    let statusReader;
    try {
      if (!statusStream) {
        throw new Error("supervisor status pipe is unavailable");
      }
      outputReaders = [];
      const stdoutReader = boundedReader(proc.stdout, "stdout", budget, terminate);
      outputReaders.push(stdoutReader);
      readers.push(stdoutReader);
      const stderrReader = boundedReader(proc.stderr, "stderr", budget, terminate);
      outputReaders.push(stderrReader);
      readers.push(stderrReader);
      statusReader = boundedReader(
        statusStream,
        "status",
        {used: 0},
        (cause) => terminate({kind: "status", error: cause.error || new Error("supervisor status exceeded its bounded frame")}),
        maxSupervisorStatusBytes,
      );
      readers.push(statusReader);
    } catch (error) {
      readerSetupError = error;
      terminate({kind: "read", streamName: "output", error});
    }
    if (readerSetupError) {
      const cleanup = await Promise.race([
        requestSupervisorCleanup().then((error) => ({error, timedOut: false})),
        delay(adapterDrainMs).then(() => ({error: new Error("supervisor cleanup request did not settle"), timedOut: true})),
      ]);
      const reaped = await Promise.race([
        supervisorExit.then((outcome) => ({outcome, timedOut: false})),
        delay(supervisorReapMs).then(() => ({outcome: undefined, timedOut: true})),
      ]);
      if (reaped.timedOut) return {error: `trusted adapter supervisor did not exit after reader setup failure${cleanup.error ? `: ${cleanup.error?.message || cleanup.error}` : ""}`};
      if (reaped.outcome?.signal !== "SIGKILL") {
        const detail = reaped.outcome?.error?.message || reaped.outcome?.error || reaped.outcome?.signal || reaped.outcome?.exitCode;
        return {error: `trusted adapter supervisor did not exit with SIGKILL after reader setup failure${detail !== undefined && detail !== "" ? `: ${detail}` : ""}`};
      }
      if (cleanup.error) return {error: `could not request shared Go SPEC contract adapter supervisor cleanup: ${cleanup.error?.message || cleanup.error}`};
      return {error: String(readerSetupError?.message || readerSetupError)};
    }
    const readerPromises = outputReaders.map((reader) => reader.read());
    const statusPromise = statusReader.read().then((frame) => {
      if (!/^(0|[1-9][0-9]{0,2})\n?$/.test(frame)) {
        terminate({kind: "status", error: new Error("supervisor returned an invalid adapter status frame")});
        return {stopped: true};
      }
      const exitCode = Number.parseInt(frame, 10);
      if (exitCode > 255) {
        terminate({kind: "status", error: new Error("supervisor returned an out-of-range adapter status")});
        return {stopped: true};
      }
      return {exitCode};
    });
    timeout = setTimeout(() => terminate({kind: "timeout"}), invocation.timeout || adapterTimeoutMs);
    const phase = await Promise.race([
      statusPromise.then((status) => ({kind: "status", status})),
      supervisorExit.then((outcome) => ({kind: "supervisor-exit", outcome})),
      terminationPromise.then((cause) => ({kind: "termination", cause})),
    ]);

    let adapterExitCode;
    let readerSettlement;
    if (phase.kind === "status" && !phase.status.stopped) {
      adapterExitCode = phase.status.exitCode;
      readerSettlement = await settleReaders(readerPromises, adapterDrainMs);
      if (readerSettlement.timedOut) {
        for (const reader of readers) void reader.cancel();
        readerSettlement = await settleReaders(readerPromises, adapterDrainMs);
      }
    } else if (!termination && (phase.kind === "supervisor-exit" || phase.status?.stopped)) {
      const outcome = phase.kind === "supervisor-exit" ? phase.outcome : await supervisorExit;
      termination = {kind: "supervisor-exit", outcome};
      resolveTermination(termination);
      for (const reader of readers) void reader.cancel();
    }

    if (termination && !readerSettlement) {
      readerSettlement = await settleReaders(readerPromises, adapterDrainMs);
      if (readerSettlement.timedOut) {
        for (const reader of readers) void reader.cancel();
        readerSettlement = await settleReaders(readerPromises, adapterDrainMs);
      }
    }
    const cleanup = await Promise.race([
      requestSupervisorCleanup().then((error) => ({error, timedOut: false})),
      delay(adapterDrainMs).then(() => ({error: new Error("supervisor cleanup request did not settle"), timedOut: true})),
    ]);
    if (cleanup.error && termination?.kind !== "supervisor-exit") {
      termination = {kind: "terminate", cause: termination || {kind: "complete"}, error: cleanup.error};
    }
    const reaped = await Promise.race([
      supervisorExit.then((outcome) => ({outcome, timedOut: false})),
      delay(supervisorReapMs).then(() => ({outcome: undefined, timedOut: true})),
    ]);
    if (reaped.timedOut) {
      const cleanupDetail = termination?.kind === "terminate" ? `; cleanup channel failed: ${termination.error?.message || termination.error}` : "";
      termination = {kind: "reap", cause: termination, error: new Error(`trusted adapter supervisor did not exit after supervisor-owned cleanup${cleanupDetail}`)};
    } else if (termination?.kind !== "supervisor-exit" && reaped.outcome?.signal !== "SIGKILL") {
      const detail = reaped.outcome?.error?.message || reaped.outcome?.error || reaped.outcome?.signal || reaped.outcome?.exitCode;
      termination = {
        kind: "cleanup-exit",
        cause: termination,
        error: new Error(`trusted adapter supervisor did not exit with SIGKILL after supervisor-owned cleanup${detail !== undefined && detail !== "" ? `: ${detail}` : ""}`),
      };
    }

    if (termination?.kind === "overflow") return {error: `SPEC contract adapter ${termination.streamName} exceeded ${maxAdapterOutputBytes} combined output bytes`};
    if (termination?.kind === "timeout") return {error: "shared Go SPEC contract adapter failed (timeout)"};
    if (termination?.kind === "read") return {error: `could not read shared Go SPEC contract adapter ${termination.streamName}: ${termination.error?.message || termination.error}`};
    if (termination?.kind === "status") return {error: `could not read shared Go SPEC contract adapter status: ${termination.error?.message || termination.error}`};
    if (termination?.kind === "supervisor-exit") {
      const detail = termination.outcome?.error?.message || termination.outcome?.error || termination.outcome?.signal || termination.outcome?.exitCode;
      return {error: `shared Go SPEC contract adapter supervisor exited before reporting status${detail !== undefined && detail !== "" ? `: ${detail}` : ""}`};
    }
    if (termination?.kind === "terminate") return {error: `could not request shared Go SPEC contract adapter supervisor cleanup: ${termination.error?.message || termination.error}`};
    if (termination?.kind === "reap") return {error: termination.error.message};
    if (termination?.kind === "cleanup-exit") return {error: termination.error.message};
    if (!readerSettlement?.results) return {error: "shared Go SPEC contract adapter output did not settle after bounded cancellation"};
    const rejected = readerSettlement.results.find((result) => result.status === "rejected");
    if (rejected) return {error: `shared Go SPEC contract adapter transport failed: ${rejected.reason?.message || rejected.reason}`};
    const stdout = readerSettlement.results[0].value.trim();
    const stderr = readerSettlement.results[1].value.trim();
    if (invocation.raw) return {exitCode: adapterExitCode, stdout, stderr};
    if (adapterExitCode !== 0) {
      return {error: `shared Go SPEC contract adapter failed${stderr ? `: ${stderr}` : ""}`};
    }
    if (!stdout || stdout === "{}") return {noop: true};
    try {
      const response = JSON.parse(stdout);
      if (typeof response.systemMessage !== "string" || !response.systemMessage.trim()) return {error: "shared Go SPEC contract adapter returned unsupported OpenCode response"};
      return {message: response.systemMessage, blocked: response.decision === "block"};
    } catch (error) { return {error: `shared Go SPEC contract adapter returned invalid JSON: ${error?.message || error}`}; }
  } finally {
    if (timeout !== undefined) clearTimeout(timeout);
    if (!cleanupRequestPromise) void requestSupervisorCleanup();
    await Promise.race([
      Promise.allSettled(readers.map((reader) => reader.cancel())),
      delay(adapterDrainMs),
    ]);
    if (statusStream && typeof statusStream.destroy === "function") {
      try { statusStream.destroy(); } catch { /* The bounded status reader may already have closed it. */ }
    }
  }
}

export const SpecContractGuard = async ({client, worktree, directory}) => {
  const sessions = new Map();
  let sessionCapacityWarned = false;
  const log = async (body) => {
    try { await client.app.log({body}); } catch { /* Diagnostics must not gate the model-visible follow-up. */ }
  };
  const discloseSessionCapacity = () => {
    if (sessionCapacityWarned) return;
    sessionCapacityWarned = true;
    void log({service: "dear-agent-spec-contract", level: "warn", message: "OpenCode SPEC contract reminder reached its bounded session limit; yielding untracked sessions until tracked state is deleted."});
  };
  const stateFor = (sessionID) => {
    let state = sessions.get(sessionID);
    if (!state) {
      // Forgetting a tracked session could allow a duplicate continuation on a
      // later idle event. At capacity, conservatively yield the new session
      // until an explicit deletion frees deterministic admission space.
      if (sessions.size >= maxTrackedSessions) return undefined;
      state = {attempted: false, exhausted: false, real: new Set(), injected: new Set()};
      sessions.set(sessionID, state);
    }
    return state;
  };
  const remember = (state, collection, messageID) => {
    if (collection.has(messageID)) return "known";
    if (state.exhausted || state.real.size + state.injected.size >= maxTrackedMessageIDs) {
      state.exhausted = true;
      state.attempted = true;
      return "exhausted";
    }
    collection.add(messageID);
    return "new";
  };
  const beforeTool = async (input, output) => {
    const plan = preToolGuardPlan(input?.tool);
    if (plan.length === 0) return;
    const root = worktree || directory;
    const toolName = legacyToolName(input.tool);
    const payload = JSON.stringify({tool_name: toolName, tool_input: output?.args || {}});
    if (new TextEncoder().encode(payload).byteLength > maxGuardInputBytes) {
      throw new Error(`OpenCode repository guard input exceeds ${maxGuardInputBytes} bytes`);
    }
    for (const guard of plan) {
      const result = await runPreToolGuard(root, guard, payload);
      if (result.error) throw new Error(`OpenCode repository guard unavailable: ${result.error}`);
      if (result.blocked) throw new Error(result.message);
      if (!result.message) continue;
      const discloseFailure = (error) => void log({
          service: "dear-agent-pretool-guard",
          level: "warn",
          message: `${result.message} (OpenCode toast unavailable: ${error?.message || error})`,
        });
      try {
        if (!client.tui?.showToast) throw new Error("TUI toast transport is unavailable");
        void Promise.resolve(client.tui.showToast({body: {message: result.message, variant: "warning"}})).catch(discloseFailure);
      } catch (error) {
        discloseFailure(error);
      }
    }
  };
  const event = async ({event}) => {
    if (event.type === "session.deleted") {
      const sessionID = event.properties?.info?.id || event.properties?.sessionID;
      if (sessionID && sessions.delete(sessionID)) sessionCapacityWarned = false;
      return;
    }
    if (event.type === "message.updated") {
      const info = event.properties?.info;
      const sessionID = info?.sessionID;
      if (sessionID && info?.role === "user" && info?.id) {
        const state = stateFor(sessionID);
        if (!state) { discloseSessionCapacity(); return; }
        if (state.injected.has(info.id) || state.real.has(info.id)) return;
        if (remember(state, state.real, info.id) === "exhausted") {
          void log({service: "dear-agent-spec-contract", level: "warn", message: "OpenCode SPEC contract reminder state reached its bounded message-identity limit; yielding for the remainder of this session."});
          return;
        }
        state.attempted = false;
      }
      return;
    }
    if (event.type !== "session.idle") return;
    const sessionID = event.properties?.sessionID;
    const state = sessionID ? stateFor(sessionID) : undefined;
    if (sessionID && !state) { discloseSessionCapacity(); return; }
    if (state?.attempted || state?.exhausted) return;
    if (state) state.attempted = true;
    const result = await runAdapter(worktree || directory);
    if (sessionID && sessions.get(sessionID) !== state) return;
    if (result.noop) { if (state) state.attempted = false; return; }
    const level = result.error || result.blocked ? "warn" : "info";
    const message = result.error ? `OpenCode SPEC contract reminder unavailable: ${result.error}` : `OpenCode SPEC contract bounded follow-up: ${result.message}`;
    if (!sessionID) { void log({service: "dear-agent-spec-contract", level, message}); return; }
    const messageID = crypto.randomUUID();
    // Publish the synthetic identity before awaiting the request. OpenCode may
    // emit message.updated reentrantly while prompt_async is still resolving.
    if (remember(state, state.injected, messageID) === "exhausted") {
      void log({service: "dear-agent-spec-contract", level: "warn", message: "OpenCode SPEC contract reminder state reached its bounded message-identity limit; yielding for the remainder of this session."});
      return;
    }
    void log({service: "dear-agent-spec-contract", level, message});
    try {
      await client.session.promptAsync({path: {id: sessionID}, body: {messageID, parts: [{type: "text", text: message}]}, throwOnError: true});
    } catch (error) {
      void log({service: "dear-agent-spec-contract", level: "warn", message: `OpenCode SPEC contract follow-up failed: ${error?.message || error}`});
    }
  };
  return {"tool.execute.before": beforeTool, event};
};
