# DEAR Retro: Scheduled Tasks Architecturally Cannot Do Their Jobs (Cowork Sandbox Mismatch)

**Date:** 2026-06-11
**Severity:** Critical (systemic — 5 of 13 scheduled tasks broken or disabled;
~460+ wasted runs over one month; one task actively destroyed backlog data for
193+ hours; the entire "autonomous background operations" layer is effectively
fictional).
**Status:** Diagnosed. Root cause is an architectural mismatch, not a bug.
Remediation is designed via the Wayfinder process in
`docs/wayfinder/2026-06-11-host-scheduler/` (same PR) and tracked in Beads.

## Define

**What broke.** Claude Desktop (Cowork) scheduled tasks were used as the
substrate for host-side automation: maintaining bead-burndown workers,
orchestrating a backlog, running security/dependency audits, auditing `~/src`
repo health, and linting LinkedIn posts with Vale. Every one of these tasks
requires capabilities the execution environment does not have. Cowork
scheduled tasks run in an isolated Linux VM (gvisor user-mode networking,
virtiofs subpath mounts only for `userSelectedFolders`) which **cannot**:

- Access the macOS host filesystem (beyond explicitly mounted folders)
- Run host CLI tools (`git`, `bd`, `vale`, `govulncheck`, `brew`, `agm`, …)
- Start Claude Code tasks (`start_code_task` is Dispatch-only)
- Reach host processes or sockets (no bridged network; gvisor mediates all
  traffic)

The only tasks that work are MCP-backed ones (Gmail, Chrome, Calendar),
because host-side MCP servers declared in `claude_desktop_config.json` are
spawned **on the host** by the Desktop app and proxied into the sandbox via
the session bridge, with per-task `approvedPermissions` pre-authorizing tools
for unattended runs (`close-chrome-tabs` is the working proof).

**Expected vs actual.**

| Task | Expected | Actual |
|---|---|---|
| `bead-burndown-loop` (every 2h) | Maintain 3 concurrent burndown code workers | Sessions stuck searching for `start_code_task`; coordination deadlock; 30 `per_task_limit` skips recorded; rewritten to print a static reminder — a no-op that still burns a VM boot + model call per run |
| `orchestrator-loop` (hourly) | Read BACKLOG.md, dispatch work, log status | Wrote status logs **into its own input file**, destroying backlog data for 193+ hours across 88+ runs; ran with `userSelectedFolders: ["/Users/vbonnet/src"]` + `permissionMode: bypassPermissions` (a root cause of golden-tree drift); now disabled |
| `weekly-security-audit` | Run `govulncheck`/`pip-audit`/`npm audit` | Tools don't exist in the VM; disabled with a note; "replaced by" weekly-dep-health-check |
| `weekly-dep-health-check` | Dispatch the audit to a code task | Calls `start_code_task`, which is not available in the sandbox — the replacement inherits the exact failure that killed its predecessor |
| `src-repo-health-audit` (every 4h) | Verify branch/clean/ahead-behind invariants on 7 repos | Cannot run `git`; outputs a template marked "NEEDS HOST VERIFICATION" — institutionalized failure, 6 runs/day |
| `linkedin-cross-post` | Vale lint + humanizer audit + post | MCP posting works; Vale (`/opt/homebrew/bin/vale`) and host repo paths unreachable — quality gates silently skipped |

**Quantified waste (one month).**

- `bead-burndown-loop`: ~12 runs/day × 31 days ≈ 372 runs, all no-ops, plus
  30 recorded `per_task_limit` skips from instance pile-up.
- `orchestrator-loop`: 88+ runs before disablement; 193+ hours of backlog
  corruption (data loss, not just waste).
- `src-repo-health-audit`: 6 runs/day producing unverifiable templates.
- Plus the human time spent noticing, half-fixing, and re-breaking these
  (each "fix" was a rewrite into a different shape of no-op).
- Total: **~460+ wasted scheduled runs**, each a VM boot + Opus/Haiku
  invocation, delivering zero automation value.

**Impact on autonomous operations.** The burndown loop was supposed to keep 3
workers burning down the `ce-6as` backlog (110 items) — actual automated
closes: ~0, with 18 beads stuck `in_progress`. The security audit layer has
been dark since disablement. Repo-health invariants on `~/src` (the golden
checkouts) went unverified during the exact window when scheduled tasks
themselves were causing golden-tree drift. The autonomy story this
infrastructure exists to deliver was not degraded — it was absent.

## Execute (Investigation)

**Why does the Cowork sandbox exist?** It is a deliberate security boundary,
and a sound one. Unattended scheduled agents process injection-prone content
(web pages, inboxes, repo files). The VM contains a prompt-injected or
misbehaving agent: no host filesystem, no keychain, no `~/.ssh`, no golden
checkouts. `orchestrator-loop` is the proof of why it should exist — given
`bypassPermissions` and an rw mount of `~/src`, it corrupted data. The
sandbox is not the defect. **Scheduling host-work inside it is.**

**Why do MCP-backed tasks succeed?** Because MCP servers are not in the
sandbox. The Desktop app spawns them on the host (`LocalMcpServerManager`)
and proxies tool calls across the session bridge; `approvedPermissions` on
the task pre-authorizes specific tools for unattended use. The capability
boundary is therefore *tool-shaped, not environment-shaped*: a sandboxed task
can do anything a host-side MCP server is willing to expose as a tool. This
is the one sanctioned channel through the wall, and we never used it for our
own tooling.

**What options exist for host-side scheduling?** (full analysis in the
Wayfinder research phase; summary here)

1. **launchd** — native, reboot-surviving, zero new dependencies. This
   machine already runs ~10 custom LaunchAgents, including
   `com.vbonnet.dev-tools-update` which already invokes headless Claude Code
   (`claude --dangerously-skip-permissions --allowedTools "Bash" -p …`) as an
   escalation step, and `com.dear-agent.bumblebee` has an idempotent
   installer in-repo (`cmd/dear-agent-bumblebee/launchagent.go`). Known
   gotchas are catalogued from the existing fleet: explicit `PATH`/`HOME`,
   `WorkingDirectory` (launchd runs in `/`), log rotation designed-in (two
   prior unbounded-log incidents: 741 MB, 976 MB), overlap locks via atomic
   `mkdir` (no flock on macOS), never both `StartCalendarInterval` and
   `StartInterval` (double-fire), `ThrottleInterval` on KeepAlive jobs.
2. **`agm loop`** — *already shipped in this repo* (`agm/cmd/agm/loop.go`,
   `agm/internal/ops/loop.go`): named recurring commands with cadence,
   SQLite-backed run history (`~/.agm/loops.db`), paused/resumed lifecycle,
   and a `agm loop tick` entrypoint designed to be driven by cron/launchd.
   The scheduling substrate this retro asks for was already built and sat
   unused while we wrote no-op Cowork prompts.
3. **Claude Code native** — nothing fits: cloud routines (`/schedule`) run on
   Anthropic infra with no host access; Desktop scheduled tasks are the very
   sandbox at issue; in-session `CronCreate`//loop` dies with the session;
   hooks are event-driven, not time-driven. Nothing announced for
   host-access scheduled tasks as of 2026-06.
4. **Temporal** (brain-v2 pattern) — proven shape (host worker polling
   `brain-v2-host-tasks`, schedules registered idempotently in code,
   durable retries, Web UI), but a heavy dependency chain (Colima VM →
   Docker → Temporal → SSH tunnel → worker) that is **currently dead**
   (worker crash-looping on Python 3.9 vs PEP 604 typing; 976 MB error log
   written into read-only `~/src/brain-v2`; tunnel failing with Colima
   down). Four failure points before any job runs, vs zero for launchd.
   Also Python, which we don't ship (principle 4).

**How do other frameworks handle this?** They cluster at three points:
vendor-cloud/no-host-access (ChatGPT scheduled tasks), ephemeral
sandbox + policy-mediated outputs (GitHub Agentic Workflows: container,
egress firewall, "safe outputs" channel), and host-daemon/full-access
(OpenClaw gateway cron, CrewAI-on-cron, self-hosted LangGraph). Cowork sits
near the gh-aw end. The instructive pattern is gh-aw's: keep the sandbox,
mediate capabilities through a narrow audited surface — which is exactly
what verb-scoped host MCP tools give us.

## Audit

**Who/what introduced the broken pattern, and was it documented?** The
pattern (schedule host-work in Cowork) accreted across multiple sessions
creating tasks (`orchestrator-loop` created 2026-04-20-ish, burndown loop
2026-06-09, src-repo-health-audit 2026-06-10). It was never written down as
a design — there is no ADR or design doc proposing "Cowork scheduled tasks
as the automation substrate." Meanwhile the repo's own design doc
`docs/design/2026-05-23-security-pipeline-bumblebee-deepsec.md` **already
stated the rule** ("Cowork scheduled tasks have zero host access; schedule
Bumblebee via launchd"), and memory (`cowork-scheduled-tasks-location.md`)
recorded "no `userSelectedFolders` ⇒ zero host access" — the knowledge
existed in three places and constrained nothing.

**Were there warnings?** Yes, explicit and repeated:

1. `weekly-security-audit` was disabled with the note "Cowork sandbox can't
   access host tools" — a precise diagnosis, written into the SKILL.md
   frontmatter, by 2026-06-08 at the latest.
2. `orchestrator-loop`'s SKILL.md contains a "Why this exists" scar from a
   2026-05-12 double-posting incident — evidence the file was being patched
   reactively while the environmental mismatch went unexamined.
3. `bead-burndown-loop`'s own rewritten prompt says verbatim: "scheduled
   tasks run in a Cowork sandbox and CANNOT start code tasks or access the
   host filesystem."

**Did the broken pattern propagate after the first failure?** Yes, three
times, *each time citing the constraint it then violated*:

- `weekly-dep-health-check` (created 2026-06-08, *after* the
  security-audit disablement note) says "You MUST use start_code_task" —
  the tool whose absence killed its predecessor.
- `bead-burndown-loop` (2026-06-09) was rewritten into a reminder for a
  Dispatch session that is not reliably running and has no signaling
  mechanism — a known-broken handoff, scheduled 12×/day.
- `src-repo-health-audit` (2026-06-10, **the same day as the audit that
  triggered this retro**) was created acknowledging in its own prompt that
  it "CANNOT run commands on the host," then scheduled every 4 hours anyway.

This is the most damning audit finding: the failure mode was *known,
documented in the artifacts themselves, and propagated anyway*. Each task
individually "handled" the constraint by writing it down and proceeding,
because no enforcement existed at task-creation time and no principle said
"a scheduled task that cannot verify its own effect must not be created."

**The dispatcher pattern specifically.** The workaround (scheduled task →
instructions → Dispatch reads → Dispatch starts code task) failed on four
independent legs: Dispatch isn't always running; no signaling mechanism
exists from scheduled task to Dispatch; `notifyOnCompletion` requires an
active Dispatch session; and the orchestrator that was supposed to *be* the
dispatcher was itself broken. A chain of best-effort handoffs with no
acknowledgment, no retry, and no owner is not a pattern — it is hope with
extra steps. (Principle 9 names the fix: when a multi-step chain must be
all-or-nothing, build the atomic wrapper; don't trust agents to sequence
it.)

**Total cost.** ~460+ wasted runs (each VM boot + model call); 193+ hours of
backlog data corruption; the burndown autonomy target (3 workers, 110-item
epic) delivered ~0 automated closes with 18 beads stranded `in_progress`;
the security-audit layer dark for weeks; multiple human sessions spent on
rewrites that changed the shape of the failure instead of its cause.

## Retro

**Root cause.** Architectural mismatch between task requirements and
execution environment: every broken task requires host capabilities; the
chosen scheduler runs in an environment explicitly designed to withhold
them. Everything else — the deadlocks, the corrupted backlog, the template
reports, the dispatcher daisy-chain — is downstream of scheduling host-work
in a host-less environment.

**Contributing causes.**

1. **No placement rule at task-creation time.** Nothing forced the question
   "does this task need host capabilities, and if so, why is it in Cowork?"
2. **Workarounds normalized the mismatch.** Each rewrite encoded the
   constraint into the prompt instead of escalating it (violating principle
   7: escalate blocks into fixes, and principle 3: broken thing → retro →
   scoped plan, never patch in-line).
3. **Existing tools were invisible.** `agm loop`, the Bumblebee launchd
   installer, and the dev-tools-update `claude -p` pattern all existed and
   were never considered — partly a discoverability failure, partly
   dogfooding failure (principle 6).
4. **No-op runs are silent.** A scheduled task that produces a useless
   report looks identical to a healthy one in `scheduled-tasks.json`; there
   is no effect-verification loop ("did the thing the task exists for
   actually happen?").

**Systemic fixes** (designed in the accompanying Wayfinder plan):

1. **Placement rule (policy):** scheduled work is classified by capability
   need. Host capabilities → host scheduler (launchd → `agm loop tick`).
   Cloud/MCC-only capabilities → Cowork scheduled task is fine. The rule
   lives in CLAUDE.md/design docs and in the task-creation skill.
2. **Host scheduler (build):** launchd-driven `agm loop tick` as the single
   host-side scheduling substrate; jobs defined as `agm loop` entries
   (SQLite history = audit trail); an idempotent installer following the
   Bumblebee pattern; headless `claude -p` invocation for agentic jobs
   following the dev-tools-update pattern, with overlap locks and rotated
   logs per the launchd gotcha catalogue.
3. **Migrate the five broken tasks** to the host scheduler (burndown
   maintenance, dep/security audit, repo health) or to a split design
   (linkedin-cross-post: MCP posting stays in Cowork; Vale lint runs
   host-side). Disable the no-op Cowork shells.
4. **Effect verification:** every migrated job ends by asserting its
   intended effect (workers actually running, report actually written,
   beads actually updated) and records the assertion in the loop run
   history; a watchdog loop flags jobs whose effects are stale.
5. **Optional narrow bridge (later phase):** verb-scoped host MCP tools
   (never generic exec) on agm-mcp-server for the few Cowork tasks that
   legitimately need one host action, per the gh-aw-style
   narrow-audited-surface model, with `approvedPermissions` scoping and
   gateway audit logging.

**Beads to file** (epic + children, `--tags scheduled-tasks`):

- P0: Host scheduler epic — launchd → `agm loop tick` substrate + installer.
- P0: Migrate bead-burndown maintenance to host scheduler; disable Cowork shell.
- P1: Migrate dep/security audit (govulncheck/npm audit/brew) to host loop.
- P1: Migrate src-repo-health-audit to host loop with real git checks.
- P1: Split linkedin-cross-post (host-side Vale gate; Cowork keeps MCP post).
- P1: Effect-verification + stale-effect watchdog for host loops.
- P2: Placement rule documented in CLAUDE.md + task-creation skill.
- P2: Verb-scoped host MCP tools on agm-mcp-server (design gate before build).
- P2: brain-v2 host-worker is crash-looping and writing a 976 MB error log
  into read-only `~/src/brain-v2` (separate repo; filed for visibility).

**Lessons.**

- A constraint written into a prompt is a confession, not a mitigation. If
  a task's own instructions say it cannot do its job, the task must not be
  scheduled — that sentence is the signal to stop and re-architect.
- "Replaced by X" requires X to work. The security-audit → dep-health-check
  handoff laundered a known failure into a new task name.
- The capability boundary of sandboxed schedulers is tool-shaped: the right
  question is never "how do we get the sandbox out of the way" but "which
  narrow, audited tool surface do we expose into it" — or "why is this job
  in the sandbox at all."
- Check the toolbox before building (or hoping): the repo already contained
  the scheduler (`agm loop`), the launchd installer pattern (Bumblebee), and
  the headless-claude pattern (dev-tools-update).
