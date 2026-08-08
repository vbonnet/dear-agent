// Source-only OpenCode transport. It cannot block session termination; a
// failing result is one bounded follow-up prompt per real session turn.
const maxAdapterOutputBytes = 64 * 1024;
const adapterTimeoutMs = 60_000;
const maxTrackedMessageIDs = 4096;
const maxTrackedSessions = 256;

function boundedText(value, streamName) {
  const bytes = value instanceof Uint8Array ? value : new Uint8Array(value || []);
  if (bytes.byteLength > maxAdapterOutputBytes) throw new Error(`SPEC contract adapter ${streamName} exceeded ${maxAdapterOutputBytes} bytes`);
  return new TextDecoder().decode(bytes);
}

function runAdapter(worktree) {
  if (typeof worktree !== "string" || !worktree) {
    return {error: "OpenCode did not provide a project worktree"};
  }
  let result;
  try {
    result = Bun.spawnSync(["go", "run", "./cmd/spec-contract-hook", "--root", worktree, "--provider", "opencode", "--event", "Stop"], {
      cwd: worktree, stdin: "ignore", stdout: "pipe", stderr: "pipe", timeout: adapterTimeoutMs, maxBuffer: maxAdapterOutputBytes,
    });
  } catch (error) { return {error: `could not start shared Go SPEC contract adapter: ${error?.message || error}`}; }
  let stdout, stderr;
  try { stdout = boundedText(result.stdout, "stdout").trim(); stderr = boundedText(result.stderr, "stderr").trim(); }
  catch (error) { return {error: String(error?.message || error)}; }
  const signal = result.signalCode || result.signal;
  if (result.error || result.exitedDueToTimeout || signal || result.exitCode !== 0) {
    const cause = result.exitedDueToTimeout ? " (timeout)" : signal ? ` (${signal})` : "";
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
    const result = runAdapter(worktree || directory);
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
