# DEAR Retro: gopls MCP Server FD Proliferation

**Date:** 2026-06-15
**Severity:** High (systemic FD exhaustion → every `go build` fails, see ce-710r)
**Status:** Open — analysis complete, action items filed

> **Routing note:** Per `.dear-agent.yml` and CLAUDE.md, DEAR retros are
> temporal artifacts that normally route to `~/src/engram-research`, and
> `docs/retros/**` is a `forbidden-path`. This file is in-repo at the user's
> explicit request (PR deliverable), consistent with the existing
> `docs/retros/` history. Move to engram-research if the routing policy is to
> be enforced strictly.

---

## Define

**The invariant that was violated:**

The aggregate file-descriptor footprint of all per-session MCP language
servers MUST stay within a safe fraction of `kern.maxfiles`, so that ordinary
work (notably `go build`, which opens thousands of FDs) never fails with
`ENFILE` / "package errors is not in std". This is a host-capacity invariant,
not a best practice.

**What broke (ce-710r):** `go build` began failing across the host with
`package errors is not in std` and `ENFILE`. Root cause was FD-table
exhaustion. `pkill -x gopls` cleared it, but **10 new gopls processes
respawned almost immediately** — one per live Claude Code session — so the
relief lasted seconds.

**Measured system state at retro time (real data, not estimates):**

```
kern.maxfiles            = 184320
kern.maxfilesperproc     =  92160
current total open FDs   = 101789   (55% of maxfiles, system-wide)

live `gopls mcp` servers = 7        (one per live Claude Code session)
FDs held per gopls mcp   = 4835     (4139 of them REG = open source files)
total FDs held by gopls  = 33894    (18% of maxfiles, 33% of all open FDs)
total gopls RSS          = 3.8 GB
```

**The exhaustion math:**

```
184320 / 4835 ≈ 38 gopls instances exhausts maxfiles on gopls alone.
```

With 7 sessions we are already at 55% of `maxfiles`. The host has perhaps
~20–25 *concurrent* Claude sessions' worth of headroom before gopls alone
tips it over — and it shares that budget with every other process (the
Claude Desktop app tree alone runs 20+ helper processes).

---

## Execute — Research

### 1. What exactly are these MCP servers?

`gopls mcp` — the Go language server (`golang.org/x/tools/gopls v0.22.0`)
running in **headless MCP mode** (`gopls mcp`, stdio transport). It exposes
Go code-intelligence tools to Claude Code (`go_diagnostics`, `go_search`,
`go_symbol_references`, `go_rename_symbol`, `go_package_api`, `go_vulncheck`,
etc.). Each `gopls mcp` parent forks one `~/go/bin/gopls` LSP worker child,
so each *session* shows as **2** gopls processes (parent + worker).

### 2. How does Claude Code start them?

Two independent sources both point at `gopls`:

- **Project `.mcp.json`** (`/Users/vbonnet/src/dear-agent/.mcp.json`):
  ```json
  "gopls": { "type": "stdio", "command": "gopls", "args": ["mcp"], "env": {} }
  ```
- **The `gopls-lsp` plugin** (`claude-plugins-official/gopls-lsp/1.0.0`),
  installed and `.in_use`, which provides Go LSP intelligence to any session.

Claude Code spawns **stdio MCP servers as direct child processes of the
session process**, on session start. There is no pooling: every session gets
its own private `gopls mcp` child.

### 3. Why one per session?

Confirmed by PPID inspection — all 7 `gopls mcp` processes have **live**
Claude parents:

```
gopls 44326 <- ppid 44316 (.../MacOS/claude)   [live]
gopls 46046 <- ppid 46036 (.../MacOS/claude)   [live]
... (×7, every parent alive)
```

This is the stdio-MCP contract: a stdio server is bound 1:1 to the client
that spawned it over its stdin/stdout pipes. N concurrent sessions ⇒ N gopls
backends by construction. **This is the dominant cost driver**, and it is
*not* a leak in the bug sense — it is the design working as specified, at a
scale the host cannot absorb.

### 4. What state do they hold? Could they share?

Each `gopls mcp` holds **4,139 open regular files** — it keeps the entire Go
source tree it has indexed open (snapshot of every `.go` file, plus the
module cache, `go.mod`/`go.sum`, build artifacts). Observed cwd for these
instances is the **`~/src/dear-agent` monorepo** (modules: root, `agm`,
`engram`, `wayfinder`), which is large — indexing it whole inflates the FD
count per instance. Two factors compound:

- **Breadth:** indexing a multi-module monorepo opens far more files than a
  single small package would.
- **No sharing:** every session re-indexes and re-opens the *same* files
  independently. Seven sessions on the same repo hold seven identical 4,800-FD
  snapshots.

gopls **does** support a shared backend: `gopls -remote=auto` forwards all
LSP commands to a single shared daemon (auto-started, shared across clients),
which is exactly how editors avoid N gopls processes. See Audit §3.

### 5. What happens when a session ends? Is there cleanup?

- **Clean exit:** Claude Code closes the stdio pipes; gopls sees EOF and
  exits. Works in the common case.
- **Abrupt death (OOM, `kill -9`, Desktop crash):** the gopls child is
  reparented to PID 1 and **leaks**. This repo already ships a reaper for
  exactly that case:
  `agm session reap-orphans` (`agm/internal/orphan/`), which kills
  `gopls` / `agm-mcp-server` processes **whose PPID == 1** (provable-orphan
  signal — safe by construction, never touches a live-parented server).

**Critical finding:** the orphan reaper is *necessary but insufficient for
this incident.* All 7 offending gopls had **live** parents, so PPID==1 never
matched. The reaper addresses the *crash/leak* axis; this incident is the
*concurrency* axis (too many simultaneously-live sessions), which no
PPID-based reaper can or should touch.

---

## Audit

### 1. Can Claude Code share a single gopls instance across sessions?

Not for **stdio** MCP servers — the transport is per-session pipes. The
sharing has to happen *below* the MCP layer, inside gopls itself, via its
daemon mode (`-remote=auto`). The MCP frontends stay 1-per-session but would
forward to one shared backend that holds one FD set. **Needs validation** —
see §3.

### 2. Can we set per-process FD limits (ulimit) on the MCP server?

Yes, and it is the cheapest guardrail. A wrapper script
(`gopls-mcp` → `ulimit -n 2048; exec gopls mcp "$@"`) referenced from
`.mcp.json` would cap each instance. Caveat: gopls genuinely *needs* a few
thousand FDs on a large monorepo; set too low it degrades (failed file reads,
incomplete diagnostics) rather than failing loudly. A cap of ~2048–3072 trims
the current 4,835 without starving it, and — more importantly — **bounds the
blast radius** so a runaway can't consume the whole table. The macOS shell
soft limit is already `1048576`, so the OS is not limiting gopls today.

### 3. Should we disable gopls MCP entirely / use a different integration?

Disabling is the blunt instrument (remove `gopls` from `.mcp.json` and/or
disable the `gopls-lsp` plugin). It eliminates the FD cost completely but
loses semantic Go tooling (`go_diagnostics`, rename, references) that this
Go-heavy repo benefits from. **Recommendation: don't disable globally;**
prefer the daemon-share + ulimit path, and offer disable as an opt-out for
sessions that don't need Go intelligence.

The promising middle path is **`gopls -remote=auto mcp`**: run the MCP server
as a thin frontend onto a shared daemon so N sessions ⇒ 1 backend FD set.
`-remote` is a valid top-level gopls flag in v0.22.0. **This must be
empirically verified** — confirm that `gopls -remote=auto mcp` actually
attaches to the shared daemon (and that the daemon, not each frontend, holds
the file FDs) before committing to it. Do not assert it works on the strength
of the help text alone.

### 4. Resource cost of N concurrent sessions × 1 gopls each

| Sessions | gopls procs | FDs (×4835) | % of maxfiles | RSS (×~0.55 GB) |
|---------:|------------:|------------:|--------------:|----------------:|
| 1        | 2           | 4,835       | 3%            | 0.55 GB         |
| 7 (now)  | 14          | 33,894      | 18%           | 3.8 GB          |
| 20       | 40          | 96,700      | 52%           | ~11 GB          |
| 38       | 76          | 183,730     | ~100%         | ~21 GB          |

FDs are the binding constraint, and they are shared with every other process
— so real-world exhaustion arrives well before 38 sessions.

### 5. Is there a Claude Code setting to control MCP server lifecycle?

MCP servers are configured in `.mcp.json` / `settings.json > mcpServers`, but
Claude Code does not expose a "share one stdio server across sessions" knob —
stdio lifecycle is fixed at 1:1 with the session. The levers we control are:
(a) *what* command `.mcp.json` points at (→ wrapper for ulimit / `-remote`),
(b) *whether* the server is enabled, (c) cleanup of leaked instances
(`reap-orphans`), and (d) host-level monitoring. There is no setting that
makes N sessions share one stdio child.

---

## Retro — Proposals

### Why the existing reaper didn't save us (root cause)

The 2026-05-01 resource-cleanup retro and `agm session reap-orphans` solved
the **leak** axis (orphaned, PID-1-reparented servers from crashed sessions).
This incident is the **concurrency** axis: too many *healthy* sessions, each
correctly holding its own gopls. We had a one-axis defense against a
two-axis problem. The reaper is still correct and should keep running; it is
just aimed at a different failure mode.

### Immediate (config — reduce proliferation now)

1. **Wrapper with `ulimit` cap.** Replace `.mcp.json`'s
   `command: "gopls", args: ["mcp"]` with a `gopls-mcp` wrapper that runs
   `ulimit -n 3072; exec gopls -remote=auto mcp "$@"`. Caps blast radius and
   (pending §3 validation) shares one backend. Ship as a small vetted script,
   per CLAUDE.md principle 9 (atomic wrappers > loosened raw commands).
2. **Run the reaper on a tick.** Ensure `agm session reap-orphans` actually
   fires periodically (host-scheduler / `agm loop tick`, ce-cd14) so the
   *leak* axis stays closed even though it didn't cause this incident.

### Short-term (FD guardrails)

3. **Validate `gopls -remote=auto mcp`** empirically — does it attach to a
   shared daemon, and does the daemon (not each frontend) hold the file FDs?
   This is the highest-leverage fix; gate the wrapper rollout on it.
4. **Per-process ulimit verified in CI/preflight** so the cap can't silently
   regress.

### Medium-term (shared architecture)

5. **One shared gopls daemon per repo root**, with all session MCP frontends
   forwarding to it (`-remote=auto`). Target: N sessions ⇒ 1 FD set instead
   of N. Collapses the table above from O(N) to O(1) in FDs.
6. **Cap concurrent gopls-bearing sessions** (or scope gopls to the active
   module rather than the whole monorepo) so a single repo's indexing cost
   doesn't scale linearly with session count.

### Monitoring (proactive — alert before exhaustion)

7. **FD-pressure probe on the loop tick:** compute
   `total_open_fds / kern.maxfiles`; warn at 60%, act at 75% (kill orphans;
   surface a "too many live sessions" notice). Cheap: `lsof | wc -l` vs
   `sysctl kern.maxfiles`. Wire it into the same scheduler that runs
   `reap-orphans`.
8. **Per-target FD accounting** in `agm admin doctor` — report gopls /
   agm-mcp-server instance count and aggregate FD draw so the operator sees
   pressure building before `go build` starts failing.

### Action items (filed in Beads — context-engine)

- **P0** — Validate `gopls -remote=auto mcp` shared-daemon behavior; if it
  works, ship the `gopls-mcp` wrapper (ulimit + remote) and point `.mcp.json`
  at it. (blocks ce-710r class)
- **P1** — FD-pressure probe + threshold alerts on the loop tick (warn 60% /
  act 75%).
- **P1** — Confirm `agm session reap-orphans` is actually scheduled on the
  host tick (depends on ce-cd14 host-scheduler).
- **P2** — `agm admin doctor`: per-target instance + FD accounting.
- **P2** — Decide policy on monorepo-wide vs per-module gopls scoping.

### Lesson

A per-session resource that is individually reasonable (4,835 FDs is fine for
one gopls) becomes a host-capacity bug when multiplied by concurrency the
host was never sized for. Reapers defend the *leak* axis; only sharing,
capping, and monitoring defend the *concurrency* axis. Both axes need a
defense, and the FD budget — not RSS — is the one that bites first.
