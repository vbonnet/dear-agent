import process from "node:process";
import {closeSync} from "node:fs";
import {setTimeout as delay} from "node:timers/promises";

// Source-only OpenCode transport. It cannot block session termination; a
// failing result is one bounded follow-up prompt per real session turn.
const maxAdapterOutputBytes = 64 * 1024;
const maxSupervisorStatusBytes = 4;
const adapterTimeoutMs = 60_000;
const adapterDrainMs = 250;
const supervisorReapMs = 1_000;
const maxTrackedMessageIDs = 4096;
const maxTrackedSessions = 256;

// The detached shell remains the group leader until its private stdin control
// pipe receives cleanup or EOF. Its monitor then self-signals the current
// process group, avoiding parent-side numeric PID/PGID races. The real hook
// receives neither control stdin nor the supervisor-only status descriptor.
const supervisorProgram = `
trap '' HUP INT QUIT TERM PIPE
exec 4<&0
(
  trap '' HUP INT QUIT TERM PIPE
  exec 3>&-
  exec 4<&-
  IFS= read -r _ || :
  while :; do
    kill -KILL 0
    /bin/sleep 0.05 || :
  done
) 0<&4 >/dev/null 2>&1 &
cleanup_monitor=$!
exec 4<&-
(
  trap - HUP INT QUIT TERM PIPE
  exec 3>&-
  exec </dev/null
  exec "$@"
) &
hook_pid=$!
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
  const reader = stream?.getReader?.();
  if (!reader) throw new Error(`SPEC contract adapter ${streamName} stream is unavailable`);
  let cancelPromise;
  const cancel = () => {
    if (cancelPromise) return cancelPromise;
    cancelPromise = (async () => {
      try { await reader.cancel(); } catch { /* Process termination is the authoritative boundary. */ }
    })();
    return cancelPromise;
  };
  const read = async () => {
    const chunks = [];
    let streamBytes = 0;
    try {
      for (;;) {
        const {done, value} = await reader.read();
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
      terminate({kind: "read", streamName, error});
      return "";
    } finally {
      try { reader.releaseLock(); } catch { /* A cancelled reader may already be released. */ }
    }
    const joined = new Uint8Array(streamBytes);
    let offset = 0;
    for (const chunk of chunks) { joined.set(chunk, offset); offset += chunk.byteLength; }
    return new TextDecoder().decode(joined);
  };
  return {cancel, read};
}

async function runAdapter(worktree) {
  if (typeof worktree !== "string" || !worktree) {
    return {error: "OpenCode did not provide a project worktree"};
  }
  if (process.platform === "win32") {
    return {error: "OpenCode SPEC contract adapter requires POSIX process-group termination"};
  }
  let proc;
  try {
    proc = Bun.spawn([
      "/bin/sh", "-c", supervisorProgram, "dear-agent-opencode-spec-supervisor",
      "go", "run", "./cmd/spec-contract-hook", "--root", worktree, "--provider", "opencode", "--event", "Stop",
    ], {
      cwd: worktree, detached: true, stdio: ["pipe", "pipe", "pipe", "pipe"],
    });
  } catch (error) {
    return {error: `could not start shared Go SPEC contract adapter supervisor: ${error?.message || error}`};
  }

  const readers = [];
  const budget = {used: 0};
  let termination;
  let resolveTermination;
  let cleanupRequestPromise;
  const terminationPromise = new Promise((resolve) => { resolveTermination = resolve; });
  const supervisorExit = Promise.resolve(proc.exited).then(
    (exitCode) => {
      return {exitCode, signal: proc.signalCode || proc.signal || ""};
    },
    (error) => {
      return {error};
    },
  );
  const requestSupervisorCleanup = () => {
    if (cleanupRequestPromise) return cleanupRequestPromise;
    // stdin is a supervisor-only control pipe. The real hook receives
    // /dev/null, while EOF also gives the monitor an independent parent-death
    // cleanup signal. No parent-side numeric process identity is ever used.
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

  const statusFD = proc.stdio?.[3];
  let timeout;
  try {
    let readerSetupError;
    let outputReaders;
    let statusReader;
    try {
      if (!Number.isInteger(statusFD) || statusFD < 0 || typeof Bun.file !== "function") {
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
        Bun.file(statusFD).stream(),
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
    timeout = setTimeout(() => terminate({kind: "timeout"}), adapterTimeoutMs);
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
    if (Number.isInteger(statusFD) && statusFD >= 0) {
      try { closeSync(statusFD); } catch { /* The bounded status reader may already have closed it. */ }
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
  return {event: async ({event}) => {
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
  }};
};
