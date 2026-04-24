# Resource Management Research

Research into CPU/memory resource controls for Claude Code sessions.

## 1. `nice` Equivalents for npm/TypeScript

### 1.1 `nice -n 19` for npm/npx/tsc

**Works reliably.** `nice -n 19 npm test` sets the process to lowest CPU scheduling
priority. The nice value is inherited by all child processes via `fork()` — the entire
process tree stays at nice 19 (children cannot raise priority without root).

**Platform differences:**
- Linux and macOS both support `nice -n 19` identically for CPU scheduling.
- macOS additionally offers `taskpolicy -b` which throttles CPU, disk I/O, and network
  I/O simultaneously — strictly more aggressive than `nice` alone.
- For cross-platform hooks, `nice -n 19` is the portable choice.

### 1.2 `NODE_OPTIONS=--max-old-space-size=512`

**Works reliably** in all modern Node.js versions. The value is in MiB.

- Default on 64-bit: ~1400–2048 MiB.
- 512 MiB is safe for `tsc` on medium projects; 256 may be tight for large monorepos.
- Caps only V8 old-generation heap — native allocations, Buffers, etc. are uncapped.
- Setting too low causes excessive GC and eventual `FATAL ERROR: Allocation failed`.

### 1.3 `UV_THREADPOOL_SIZE`

Controls libuv's I/O thread pool (default: 4, max: 1024). Must be set before Node.js
starts.

**What uses it:** async `fs.*`, `dns.lookup()`, `crypto.pbkdf2/scrypt/randomBytes`,
async `zlib`. Network I/O (TCP/HTTP) does **not** use it — those use epoll/kqueue directly.

**Verdict: not useful for resource limiting.** Reducing it only causes I/O queueing,
doesn't cap CPU or memory. Leave at default.

### 1.4 Other Useful Knobs

| Knob | Effect |
|------|--------|
| `--max-semi-space-size=N` (MiB) | Caps V8 young-gen heap. Reducing to 2–8 increases minor GC but lowers peak RSS. |
| `ulimit -v N` | Caps virtual memory (Linux). Hard kill on exceed. |
| `ulimit -t N` | CPU time limit in seconds. |
| `timeout N cmd` | Wall-clock deadman switch. |
| cgroups v2 `memory.max` / `cpu.max` | Gold standard for containers/CI — kernel-enforced, no process cooperation. |

### 1.5 PreToolUse Hook Implementation

Claude Code PreToolUse hooks can modify tool inputs via `updatedInput`. A hook matching
`tool_name == "Bash"` can parse `tool_input.command`, detect npm/npx/node/tsc as the
first token, and prepend `nice -n 19` + inject `NODE_OPTIONS`.

```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "updatedInput": {
      "command": "nice -n 19 NODE_OPTIONS='--max-old-space-size=512' npm test"
    }
  }
}
```

**Edge cases to handle:**
- Command already has `nice` prefix
- Commands in pipes or `&&` chains (only prefix the first segment)
- Multiple hooks modifying the same tool input (last writer wins)

---

## 2. Idle Claude Code CPU Usage (10–16%)

### 2.1 Observed Behavior

Measured idle CPU per Claude Code process:

| Scenario | Idle CPU |
|----------|----------|
| Single resumed session | ~10–16% |
| Multiple concurrent sessions | ~20% each (contention) |
| Memory per process | 400–720 MB RSS |

### 2.2 Binary Architecture

- **Runtime:** Bun (JavaScriptCore-based), not Node.js. The binary is a ~233 MB
  statically-linked executable embedding JSC.
- **Threads per process:** 6–16 (6 idle, 16 active).

### 2.3 Root Causes

Three compounding factors drive idle CPU:

**a) Inotify file watchers (37 watches per process)**

Each process holds 1 inotify instance watching directories under `~/.claude/` (tasks,
hooks, projects, session-env, shell-snapshots, config). High-churn shared directories
cause frequent cross-session events:
- `session-env/`: 2105 files
- `shell-snapshots/`: 995 files
- `paste-cache/`: 1008 files

**b) Timer file descriptors (5 timerfd per process)**

- **StatusLine refresh:** re-runs the configured command periodically.
- **Cron scheduler:** ticks every 300s even with no registered cron jobs.
- **JSC GC timer:** periodic garbage collection on a 400–700 MB heap.

**c) Event loop overhead (3 epoll instances per process)**

Three separate event loops with `eventfd` for cross-thread signaling.

### 2.4 Mitigations

| Mitigation | Impact | How |
|-----------|--------|-----|
| Remove `statusLine` from settings.json | Moderate | Eliminates periodic command exec + spawn overhead |
| Set `refreshInterval` to high value (e.g., 60s) | Low–Moderate | Reduces statusLine re-execution frequency |
| Clean up ephemeral dirs | Moderate | Fewer inotify events from shared dir churn |
| Limit concurrent sessions | High | Each session costs ~10–20% baseline CPU |
| Reduce inotify watch surface | High | Fewer dirs under `~/.claude/` watched |

### 2.5 Key Takeaway

The idle CPU is an inherent property of Claude Code's architecture: JSC GC + inotify on
shared high-churn dirs + periodic timers. The most impactful user-controllable lever is
**limiting concurrent sessions** and **removing or slowing the statusLine**. There is no
single config flag to eliminate the baseline cost.

### 2.6 No MCP/Cron Contribution

- MCP servers are spawned on-demand per session, not persistent daemons.
- No scheduled tasks (`scheduled_tasks.json`) were found active.
- CronCreate does not contribute to idle CPU unless jobs are registered.
