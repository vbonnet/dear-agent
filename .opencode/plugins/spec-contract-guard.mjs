import process from "node:process";

// Source-only OpenCode transport. It cannot block session termination; a
// failing result is one bounded follow-up prompt per real session turn.
const maxAdapterOutputBytes = 64 * 1024;
const adapterTimeoutMs = 60_000;
const maxTrackedMessageIDs = 4096;
const maxTrackedSessions = 256;

function boundedReader(stream, streamName, budget, terminate) {
  const reader = stream?.getReader?.();
  if (!reader) throw new Error(`SPEC contract adapter ${streamName} stream is unavailable`);
  let cancelled = false;
  const cancel = async () => {
    if (cancelled) return;
    cancelled = true;
    try { await reader.cancel(); } catch { /* Process termination is the authoritative boundary. */ }
  };
  const read = async () => {
    const chunks = [];
    let streamBytes = 0;
    try {
      for (;;) {
        const {done, value} = await reader.read();
        if (done) break;
        const chunk = value instanceof Uint8Array ? value : new Uint8Array(value || []);
        if (budget.used + chunk.byteLength > maxAdapterOutputBytes) {
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
  if (process.platform === "win32" || typeof process.kill !== "function") {
    return {error: "OpenCode SPEC contract adapter requires POSIX process-group termination"};
  }
  let proc;
  try {
    proc = Bun.spawn(["go", "run", "./cmd/spec-contract-hook", "--root", worktree, "--provider", "opencode", "--event", "Stop"], {
      cwd: worktree, detached: true, stdin: "ignore", stdout: "pipe", stderr: "pipe",
    });
  } catch (error) { return {error: `could not start shared Go SPEC contract adapter: ${error?.message || error}`}; }
  const readers = [];
  let termination;
  const terminate = (cause) => {
    if (termination) return;
    termination = cause;
    try {
      // detached:true makes the direct `go run` child its POSIX process-group
      // leader. A negative PID therefore terminates both the Go driver and the
      // compiled adapter (plus any descendants retaining the output pipes).
      process.kill(-proc.pid, "SIGKILL");
    } catch (error) {
      termination = {kind: "terminate", cause, error};
      try { proc.kill("SIGKILL"); } catch { /* Stream cancellation still bounds the caller. */ }
    }
    for (const reader of readers) void reader.cancel();
  };
  const budget = {used: 0};
  try {
    readers.push(boundedReader(proc.stdout, "stdout", budget, terminate));
    readers.push(boundedReader(proc.stderr, "stderr", budget, terminate));
  } catch (error) {
    terminate({kind: "read", streamName: "output", error});
    try { await proc.exited; } catch { /* The start/read error is the useful diagnostic. */ }
    return {error: String(error?.message || error)};
  }
  const timeout = setTimeout(() => terminate({kind: "timeout"}), adapterTimeoutMs);
  const settled = await Promise.allSettled([readers[0].read(), readers[1].read(), proc.exited]);
  clearTimeout(timeout);
  if (termination?.kind === "overflow") return {error: `SPEC contract adapter ${termination.streamName} exceeded ${maxAdapterOutputBytes} combined output bytes`};
  if (termination?.kind === "timeout") return {error: "shared Go SPEC contract adapter failed (timeout)"};
  if (termination?.kind === "read") return {error: `could not read shared Go SPEC contract adapter ${termination.streamName}: ${termination.error?.message || termination.error}`};
  if (termination?.kind === "terminate") return {error: `could not terminate shared Go SPEC contract adapter process group: ${termination.error?.message || termination.error}`};
  const rejected = settled.find((result) => result.status === "rejected");
  if (rejected) return {error: `shared Go SPEC contract adapter transport failed: ${rejected.reason?.message || rejected.reason}`};
  const stdout = settled[0].value.trim();
  const stderr = settled[1].value.trim();
  const exitCode = settled[2].value;
  const signal = proc.signalCode || proc.signal;
  if (signal || exitCode !== 0) {
    const cause = signal ? ` (${signal})` : "";
    return {error: `shared Go SPEC contract adapter failed${cause}${stderr ? `: ${stderr}` : ""}`};
  }
  if (!stdout || stdout === "{}") return {noop: true};
  try {
    const response = JSON.parse(stdout);
    if (typeof response.systemMessage !== "string" || !response.systemMessage.trim()) return {error: "shared Go SPEC contract adapter returned unsupported OpenCode response"};
    return {message: response.systemMessage, blocked: response.decision === "block"};
  } catch (error) { return {error: `shared Go SPEC contract adapter returned invalid JSON: ${error?.message || error}`}; }
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
