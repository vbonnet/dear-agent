---
phase: RESEARCH
phase_name: Research — scheduling options for host-capability jobs
wayfinder_session_id: b56e8212-3f64-4bbe-97b2-44dea52da1e8
created_at: 2026-06-11
phase_engram_hash: "n/a"
phase_engram_path: "n/a"
---

# D2: Research

Five options investigated in depth (parallel research agents, 2026-06-11;
sources: code.claude.com docs, host logs/configs, this repo, ~/src/brain-v2,
the existing LaunchAgent fleet).

## Option 1 — Claude Code / Anthropic native scheduling

| Mechanism | Host access | Verdict |
|---|---|---|
| Cloud routines (`/schedule`, Apr 2026 research preview) | None — runs on Anthropic infra, fresh repo clone, cloud connectors only | Unusable for host jobs |
| Desktop (Cowork) scheduled tasks | Linux VM sandbox; virtiofs mounts of `userSelectedFolders`; host MCP via app proxy | The broken status quo; fine for MCP-only jobs |
| In-session `CronCreate` / `/loop` | Full host access **while a session is open**; dies with session; 7-day expiry | Not durable scheduling |
| Hooks (SessionStart/Stop/…) | Event-driven only; no timers | Not a scheduler |
| Agent SDK / `claude -p` headless | Full host access; invokable from any scheduler; `--allowedTools`, `--permission-mode`, `--output-format json` | **Viable execution leg** — needs an external trigger |

Anthropic has announced nothing about host-access scheduled tasks (checked
docs + changelog, 2026-06-11). Headless auth gotcha: `--bare` disables
OAuth → needs `ANTHROPIC_API_KEY` (metered) or settings `apiKeyHelper`;
non-bare from launchd risks keychain prompts. Cost note: from 2026-06-15,
`claude -p`/SDK usage draws from a separate Agent SDK credit pool (Q1 in
charter).

## Option 2 — launchd (host-native trigger)

~10 custom LaunchAgents already run on this machine. Empirical gotcha
catalogue extracted from the fleet:

- launchd default env is bare (`/usr/bin:/bin:/usr/sbin:/sbin`, cwd `/`):
  every working plist sets `PATH`/`HOME`/`WorkingDirectory` explicitly; the
  one that doesn't (`com.brain-v2.host-worker`) is the one crash-looping.
- Log rotation must be designed in: two unbounded-log incidents (741 MB
  ai-conversation-sync stderr; 976 MB host-worker error log inside
  read-only `~/src`). Patterns in use: paired `*-logrotate` plist
  (copytruncate when no quiet window) or self-rotation at script start.
- No `flock(1)` on macOS → overlap guard = atomic `mkdir` + PID file +
  comm-name verification (`ai-conversation-sync-wrapper.sh` reference).
- Never both `StartCalendarInterval` and `StartInterval` (documented
  double-fire incident). `ThrottleInterval` mandatory on KeepAlive jobs.
- Exit-code hygiene: deliberate skips exit 0 so launchctl's last-exit
  column stays meaningful.
- Best templates: `com.vbonnet.agm-bus.plist` (Go daemon),
  `com.vbonnet.dev-tools-update.plist` (calendar job env block, and the
  **existing working `claude -p` invocation**: `claude
  --dangerously-skip-permissions --allowedTools "Bash" -p "<prompt>"`).
- In-repo precedent: `cmd/dear-agent-bumblebee` ships an idempotent
  launchagent installer (`install-launchagent`) with a testable seam.

## Option 3 — `agm loop` (existing in-repo scheduler substrate)

`agm loop new <name> --cadence 5m|1h|24h --cmd "<bash>"`; `agm loop tick`
runs all due loops; every run persisted to `~/.agm/loops.db` (WAL SQLite:
started/finished, exit code, stdout/stderr); pause/resume lifecycle; README
suggests `* * * * * agm loop tick` as the trigger. **This is the missing
middle layer**: launchd provides one durable trigger; agm loop provides job
definitions, cadence, run history, and an audit trail — and it dogfoods AGM
(principle 6). Gap analysis: no built-in overlap lock per loop and no
effect-verification concept → handled in DESIGN (wrapper + verify command
convention) rather than engine changes.

## Option 4 — MCP bridge (host MCP server callable from the sandbox)

Confirmed **feasible and already proven**: host MCP servers from
`claude_desktop_config.json` are spawned on the host and proxied into
sandbox sessions; the `close-chrome-tabs` scheduled task already invokes
host-side Chrome MCP tools unattended via per-task `approvedPermissions`.
`agm-mcp-server` (Go, official MCP SDK, gateway middleware, auto-registers
into Claude config) is the natural extension point.

Security analysis: a generic `run_command` tool would negate the sandbox
(scheduled tasks consume injection-prone content; the sandbox exists to
contain exactly that). Sound shape = **verb-scoped tools** (fixed argv,
validated enums, no shell) + allowlist execution (brain-v2's
`execute_safe_command` is reference prior art: shlex + no-shell exec +
timeout + output cap + metachar deny) + gateway audit logging. GitHub
Agentic Workflows validates this architecture class (sandbox + narrow
policy-mediated surface). Verdict: **correct pattern, not needed for the
5 broken jobs** (none of them needs to stay in Cowork) → defer, design-gated.

## Option 5 — Temporal worker (brain-v2 pattern)

Proven shape on this machine: launchd-supervised Python worker polling
`brain-v2-host-tasks` against self-hosted Temporal (Docker in Colima,
reached via SSH tunnel); ~12 schedules registered idempotently in code;
durable retries; Web UI; even AGM-spawning activities. But: **four failure
points before any job runs** (Colima → Docker → Temporal → tunnel → worker)
and the whole chain is currently dead (worker crash-looping on Python
3.9/PEP 604; tunnel refusing; silent for an unknown period). Its failure
mode is itself a cautionary exhibit for this retro. Python conflicts with
principle 4; a Go worker would be possible but adopts the same dependency
chain. Verdict: rejected for this problem; revisit only if jobs need
multi-step durable workflows beyond what loop + wrapper can express.

## Survey — how other frameworks place scheduled agents

Three clusters: vendor-cloud/no-host-access (ChatGPT tasks);
ephemeral-sandbox + policy-mediated outputs (GitHub Agentic Workflows:
container, egress firewall, "safe outputs"); host-daemon/full-access
(OpenClaw gateway cron, CrewAI-on-cron, self-hosted LangGraph). Cowork sits
in the middle cluster. Lesson: the industry-converged answer to "sandboxed
agent needs host effects" is a narrow audited tool surface, never a hole in
the wall — confirming Option 4's shape and Option 1's limits.

## Build / buy / adapt decision

**Adapt** (>70% overlap with existing assets): launchd (exists, fleet
conventions known) → `agm loop tick` (exists, unused) → job commands that
are either plain Go binaries/scripts or headless `claude -p` via a wrapper
(pattern exists in dev-tools-update.sh). Net-new code is small and
principle-9-shaped: one tick plist + installer, one run wrapper (lock +
rotate + verify), loop definitions, and migration of 5 jobs.
