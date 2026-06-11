# Spec-Compliance Plan — AGM, VROOM, Wayfinder

**Author:** audit/spec-compliance-plan agent run, 2026-05-24
**Base commit:** `ce5957b737` (main, post-PR #140)
**Branch:** `audit/spec-compliance-plan`
**Scope:** Phase-1 deliverable — assess + plan. No fixes in this PR.

> **Read-the-room note before you start.** This plan is brutally honest by
> design. dear-agent ships a *lot* of working code — every unit test passes
> (356 packages green), the build is clean, AGM CLI is largely real, Wayfinder
> has 31K LOC of test code, the MCP server auto-registers correctly, and the
> golden-main rollback / worktree-reaper infrastructure shipped in the last 10
> days actually works. The point of this document is not "everything is
> broken." The point is: **VROOM is a façade**, **Wayfinder's phase numbering
> is mathematically incoherent**, **the AGM hooks layer has zero coverage**,
> and **roughly a third of the ADRs filed under `docs/adr/` describe code that
> does not exist**. Those four sentences are the headline.

---

## 0. Methodology and how to read this document

### Where the findings come from

Six parallel sub-agents read the tree at HEAD (`ce5957b737`) and reported
back. The bulk of the evidence is cited inline as `file:line`. Where a claim
is hedged ("appears", "likely"), the parent agent did not verify the cite.

| # | Audit | Tools | Output |
|---|-------|-------|--------|
| 1 | AGM CLI | Explore (read-only) | inventory + spec compliance per command |
| 2 | AGM MCP | Explore (read-only) | tool surface + registration audit |
| 3 | VROOM supervisor | Explore (read-only) | model-conformance check |
| 4 | Wayfinder | Explore (read-only) | DAG engine + subcommand + drift |
| 5 | Test coverage | test-analyzer | per-component test ratios + gaps |
| 6 | Spec/ADR compliance | architecture-validator | declared-vs-actual mapping |

The full test suite ran `go test -short ./...` in parallel: **356 PASS / 0
FAIL / 46 packages have no test files**. Build (`go build ./...`) is green.
The integration tests (52 files, `//go:build integration`) **never run in
CI**; their freshness cannot be inferred from a green main.

### Severity scale

- **P0 — Misleading.** Doc/code says feature exists; reality says no. Will
  burn an integration partner or a future agent immediately.
- **P1 — Load-bearing untested.** Code exists, is on the hot path, has no
  test sibling. Silent failure mode.
- **P2 — Drift / collision.** Two names for one thing, or one name for two.
  Slows down everyone but kills no one.
- **P3 — Hygiene.** Naming, structure, doc placement.

### Vocabulary used here

Per `/Users/vbonnet/src/dear-agent/CONTEXT.md` (the SoT):

- **Wayfinder** = planning phase that precedes execution.
- **VROOM** = supervisory execution framework (3 supervisors: Meta-Orch CTO /
  Orch COO / Overseer CRO; per-task Primary/Secondary/Tertiary; Workers /
  Auditors / SRE agents).
- **AGM** = the tool VROOM drives. Spawns/messages/monitors tmux-backed
  agent sessions.
- **DEAR** = Define → Execute → Audit → Retro retrospective loop (process
  sense; collides with `pkg/workflow.Hooks` which uses different E and R).

The **old 5-role VROOM** (V/R/O/O/M = Verifier/Requester/Orchestrator/
Overseer/Meta-Orchestrator with a lexicographic value evaluator) is
**superseded**. Where this plan mentions "Verifier" or "Requester" as a
*role*, it is referring to the old/stale model in the existing code.

---

## 1. Current-state assessment (component-by-component)

### 1.1 AGM CLI — substantially working

**Inventory:** ~25 subcommands across six groups (session / send /
supervisor / worktree / acceptance / admin / benchmark). Source under
`/Users/vbonnet/src/dear-agent/agm/cmd/agm/`. ~491 `Test*` functions in
`cmd/agm/` alone, ~542 `*_test.go` files across all of `agm/`.

| Command (subset) | Status | Evidence |
|---|---|---|
| `agm session new` | WORKING | `cmd/agm/new.go:83` + 4 integration test files |
| `agm session list` | WORKING | `cmd/agm/list_dolt.go`; Dolt-backed |
| `agm session import` | WORKING | `cmd/agm/import.go:19`; interactive TTY path has TODO test |
| `agm session gc` | WORKING / UNTESTED-IN-ISOLATION | `cmd/agm/session_gc.go:20`; no `gc_test.go` |
| `agm send msg` / `compact` / `approve` / `reject` | WORKING | parallel delivery, 2h/3-cap cooldown, audit trail to `~/.agm/compaction-prompts/` |
| `agm supervisor run` | LAUNCHER ONLY | `cmd/agm/supervisor.go:92`; execs `claude --dangerously-load-development-channels server:agm-bus` — **does not implement a loop** (see §1.3) |
| `agm supervisor heartbeat`/`status` | WORKING | `cmd/agm/supervisor.go:121,133` |
| `agm worktree sweep` | WORKING | PR #125; `cmd/agm/worktree_sweep.go:39` |
| `agm acceptance show` | WORKING | `cmd/agm/acceptance.go:32` |
| `agm admin doctor` | WORKING | `cmd/agm/doctor.go:42` (2-mode: quick + `--validate`) |
| `agm admin clean` | WORKING (interactive default) | `cmd/agm/clean.go` |
| `agm benchmark swe-lite` | WORKING (predictions only) | `cmd/agm/benchmark_swe.go:60` — no Docker grading on this host (memory) |
| `agm friction` | **MISSING (ADR-023 design-only)** | no source file |
| `agm session new --from <id> --scope` | **MISSING (ADR-023 design-only)** | no `--from` flag in `cmd/agm/new.go` |

### 1.2 AGM MCP server — works; one suspicious tool-name drift; no protocol tests

**Location:** `/Users/vbonnet/src/dear-agent/agm/cmd/agm-mcp-server/`
(main.go, config.go, tools.go, a2a_handler.go + 4 tests).

**Tools registered in code** (`tools.go:56–309`):

```
agm_list_sessions, agm_search_sessions, agm_get_session_metadata,
agm_archive_session, agm_kill_session, agm_list_ops,
engram_list_wayfinder_sessions, engram_get_wayfinder_session
```

**Tools currently exposed to this session** (from the deferred-tools list):

```
agm_list_sessions, agm_search_sessions, agm_get_session,
agm_archive_session, agm_kill_session, agm_list_ops,
engram_list_wayfinder_sessions, engram_get_wayfinder_session
```

**Drift:** `agm_get_session_metadata` (code) vs **`agm_get_session`** (live
MCP). PR #140 (2026-05-23) was the rename. Two possibilities:

1. The live MCP is running an older binary (most likely — re-register or
   restart and recheck before treating this as a code bug).
2. The Claude harness caches tool names from a previous registration.

This is a **session-level** problem, not necessarily a code-level one. **The
code at `tools.go:111` says `agm_get_session_metadata`.**

**Auto-registration** (`config.go:131–191`): handles both flat
(`{"agm": {...}}`) and nested (`{"mcpServers": {...}}`) shapes, preserves
user args/env, idempotent, atomic. Tested in `config_register_test.go`
(5 tests). Verified WORKING.

**Critical gap — JSON-RPC protocol tests are MISSING.** 12 unit tests, none
exercise the JSON-RPC 2.0 wire format, none validate tool input schemas,
none check error response shape. A subtle schema mismatch will break clients
silently. (`tools.go:159–180` — `mcpSuccess` / `mcpError` are untested.)

**Engram MCP forwarder** (`tools.go:312–390`): 5s timeout, no retry, no
circuit breaker, no tests. If Engram is down, wayfinder MCP tools fail
opaquely.

### 1.3 VROOM — façade

**This is the worst finding in this audit.** The doctrine in
`CONTEXT.md`/`ADR-002` describes a three-supervisor mesh of long-lived
loops with mutual-unblock-first semantics, an append-only decision trail,
and per-task Primary/Secondary/Tertiary ownership. **None of that exists in
running code.**

**What's actually in `pkg/vroom/vroom/`** (4 files, ~293 LOC total):

| File | LOC | What it does |
|---|---|---|
| `topics.go` | 19 | string constants for event topics (one comment still says "Verifier") |
| `payloads.go` | 33 | struct definitions for event payloads |
| `emitter.go` | 78 | thin fire-and-forget event publisher to an `EventPublisher` interface |
| `emitter_test.go` | 163 | exercises the emitter; one test uses `role="verifier"` |

**What `agm supervisor run` actually does**
(`agm/cmd/agm/supervisor.go:195–241`):

1. Checks `CLAUDE_CODE_OAUTH_TOKEN` is set and `ANTHROPIC_API_KEY` is *not*.
2. Execs `claude --dangerously-load-development-channels server:agm-bus`.
3. Hands stdin/stdout/stderr to the child. **Returns.**

There is no autonomous loop. There is no built-in mutual-unblock-first
behavior. There is no supervisor-check skill (the name does not appear
anywhere in the repo). Heartbeats are emitted only if the *user* invokes
`/loop 5m agm supervisor heartbeat --id s1` from inside the launched Claude
session.

**Drift / stale-model debris:**

- `pkg/vroom/vroom/topics.go:14` — comment says "when the Verifier
  evaluates an output" (superseded role). CONTEXT.md §3 flags this
  explicitly; ADR-002 §92–97 explicitly *defers* the fix.
- `pkg/vroom/vroom/emitter_test.go:112` — test passes `role="verifier"`.
- `agm/internal/rbac/role.go:22–23` — `RoleVerifier` and `RoleRequester`
  constants still exported.
- `agm/internal/rbac/profiles.go:299–338` — 40+ LOC of permission profiles
  for the superseded roles.
- `agm/docs/adr/ADR-020`…`ADR-025` — verified stubs/redirects to top-level
  ADR-002 ✅ (this part of the cleanup landed).

**What about the decision trail?** The emitter publishes to an in-memory
`EventPublisher`. Nothing persists the events. There is no append-only log
on disk. CONTEXT.md §"Decision trail" promises one; the code does not
provide one.

**What about Primary/Secondary/Tertiary?** Documented in CONTEXT.md
lines 85–97. Zero code references. Heartbeat records carry `PrimaryFor` /
`TertiaryFor` (`supervisor.go:44–50`) but those are *peer-supervisor*
relationships, not per-task ownership.

**What about Workers / Auditors / SRE agents?** Distinct *permission
profiles* exist in `agm/internal/rbac/`. Behavior differentiation does not.
They are all spawned via the same `agm session new` codepath.

**Net:** VROOM-as-doctrine and VROOM-as-code are different things. The
doctrine is sound. The code is a launcher + event emitter + heartbeat
file-watcher. Calling that "the VROOM execution framework" is a stretch.

### 1.4 Wayfinder — works, but the phase model is mathematically broken

**Build:** green. **Unit tests:** 93 `*_test.go`, ~31K LOC. **E2E test
exists:** `wayfinder/cmd/wayfinder-session/internal/integration/wayfinder_v2_test.go:23–115`
runs W0 → D1 → D2 → D3 → D4 → S6 → S8 → S11.

**The phase-count problem.** SPEC/README/SKILL all say "9-phase
consolidation" (CHARTER → PROBLEM → RESEARCH → DESIGN → SPEC → PLAN → SETUP
→ BUILD → RETRO). The code defines **12 V1 IDs** (D1–D4, S4–S11) in
`wayfinder/internal/phaseisolation/definitions.go:4–32` and maps them to
9 V2 names with **lossy duplicates**:

```go
PhaseS6  -> V2Design   // S7 also -> V2Plan
PhaseS9  -> V2Build    // S10 also -> V2Build
PhaseS10 -> V2Build
```

CHARTER (V2) has no V1 entry in the map. The CLI advertises "12 sequential
phases" at `wayfinder/cmd/wayfinder/cmd/root.go:28`. **The 9-phase claim is
either marketing or a future state — the implementation is 12-and-a-half
phases.**

**The TypeScript-that-isn't problem.** `ARCHITECTURE.md:5,27,166`,
`SPEC.md:58,79,112–146`, `SKILL.md:217`, plus code comments at
`internal/w0/detector.go:2` and `internal/phaseisolation/types.go:3` claim
"ported from the TypeScript implementation in cortex/lib/" or describe the
phase orchestrator as "TypeScript." There is **zero TypeScript** in
`wayfinder/` outside of `wayfinder/review/` (a self-contained
multi-persona-review tool which legitimately is TS). The phase
orchestration is 100% Go.

**Other drift (not exhaustive):**

- `internal/w0/detector.go:70` checks for `W0-project-charter.md`; the
  shell gate `lib/d1-gate-check.sh:8` checks for `W0-charter.md`. A
  W0 created under either name will silently pass one and silently fail
  the other.
- SPEC YAML schema documents `current_phase` / `phases` / `quality`;
  code structs (`internal/status/types_v2.go`) use `CurrentWaypoint` /
  `WaypointHistory` / `QualityMetrics`. Any reader using the SPEC will
  hit field-not-found.
- `wayfinder/cmd/wayfinder-session/commands/migrate_all.go` contains
  literal placeholder strings ("the git history") from an aborted
  find-and-replace. Looks like a corrupted refactor.
- Subcommands `coord` and `config` exist as Go files but are not wired
  into `session.go:49–73`, so they are invisible from the CLI.
- No `validate-phase` *CLI* subcommand exists. The `wayfinder:validate-phase`
  skill (visible in this session) is backed by validator logic that is
  baked into `complete-phase` (`cmd/wayfinder-session/internal/validator/validator.go:15–28`).
- ADR references in `wayfinder/SPEC.md:187,192,197,202` (`docs/wayfinder/ADR-001…004`)
  point at a directory that does not exist.

**Wayfinder review (`wayfinder/review/`):** self-contained TS, 22 `.test.ts`
files, builds and tests, npm install needs `--legacy-peer-deps` (known —
memory `wayfinder-review-npm.md`).

### 1.5 Test coverage — uneven, with a clearly visible hole

Headline: overall coverage **~53.5 %**. 10,884 `.go` files, 4,760 `*_test.go`
files (43.7 % test-file ratio). 9,206 `Test*` functions. **All unit tests
pass.** But:

| Area | Coverage feel | Why it matters |
|---|---|---|
| `agm/cmd/agm-hooks/*` (13 binaries) | **0 %** | Hooks are the safety/governance layer that block dangerous Bash, enforce branch protection, report state on Stop. A broken hook silently allows what it was meant to block. |
| `agm/internal/ops/` (session_send.go, session_kill.go, session_status.go, sandbox_cleanup.go, compact_trigger.go, workspace.go) | none | Core lifecycle ops. Bugs here strand sessions. |
| `pkg/vroom/vroom/` | only `emitter.go` is tested | The whole "decision trail" idea has no test |
| `pkg/vcs/` (5 files incl. `hooks.go` with ValidateMemoryPair / ValidateEngramFrontmatter / InstallPreCommitHook) | **0 %** | Guards engram corpus integrity at commit time |
| `pkg/benchmarks/` (17 files, 2 tests) | stub-only | Per memory `dear-agent-benchmarks-state.md`: every suite returns "stub executor: not executed"; real path is `agm benchmark swe-lite` which bypasses this package |
| `pkg/workflow/audit_jsonl.go`, `audit_stdout.go`, `audit_otel.go`, `audit_engram.go` | none | DEAR retros depend on audit emission |
| `wayfinder/internal/phaseisolation/orchestrator.go` | none | The thing that actually drives Wayfinder phase sequencing |
| Integration tests (52 files, `//go:build integration`) | **never run in CI** | `agm/test/integration/ci_skip_test.go:10` and `agm/workflowbus/ci_skip_test.go:10` exit early under `testing.Short()` and `CI=true` |

Known flakes (memory `dear-agent-ci-flakes.md`): enrichment data race;
workflowbus signal-count; GOMAXPROCS=6 expectation; `TmuxLock_CrossProcess`
(can red both Build & Test even on docs-only PRs).

Known FIXMEs in test code:
- `wayfinder/cmd/wayfinder-session/internal/review/persona_integration_test.go` — `// FIXME: this is broken`
- `agm/internal/ops/hygiene_check_test.go` — `// FIXME: broken`

### 1.6 Spec / ADR compliance

**Top-level ADRs** at `/Users/vbonnet/src/dear-agent/docs/adr/` (14 files):

| ADR | Title | Mapped to code |
|---|---|---|
| 001 | Monorepo consolidation | **DONE** |
| 002 | VROOM execution architecture | **SPEC ONLY** — see §1.3 |
| 009 | Work item as first-class substrate | **Intent only by design** (says so in line 9) |
| 010 | Workflow engine architecture | **PARTIAL** — DAG runner exists; SQLite state + role registry from ADR-010 D1/D2 not verified |
| 011 | DEAR audit subsystem | **NOT FOUND** as a `pkg/audit` package |
| 012 | Provider transport layer | **PARTIAL** (harness adapter pattern) |
| 013 | Tailscale API | NOT FOUND |
| 014 | Plugin system | NOT FOUND |
| 015 | Signal aggregator | PARTIAL (referenced, not verified) |
| 016 | Recommendation MCP server | NOT FOUND |
| 017 | Gateway platform adapters | NOT FOUND |
| 018 | Graceful exit framework default | UNKNOWN (not verified) |
| 022 | Backlog suggestion system | PARTIAL (`pkg/backlog/orchestrator.go`) |
| 023 | Friction reporting + session handoff | **DESIGN ONLY by design** (states so) |

**AGM-scoped ADRs** at `agm/docs/adr/`:

- `ADR-001`…`ADR-019` are real and largely implemented (verified
  spot-checks).
- `ADR-020`…`ADR-025` are correctly stubbed redirects to top-level ADR-002.

**`agm/CAPABILITY-MATRIX.md`** is a BDD test-mock matrix, not a production
capability matrix. The filename oversells it; nothing in the file is false.

**The five known terminology collisions from CONTEXT.md §Known Terminology
Collisions** as of 2026-05-24:

| # | Collision | Status |
|---|---|---|
| 1 | VROOM 5-role vs 3-supervisor | **UNRESOLVED in code** (`pkg/vroom/vroom/topics.go:14`, RBAC, tests) |
| 2 | DEAR three meanings (process / `pkg/workflow.Hooks` / backlog prefix) | **UNRESOLVED in code** (`OnEnforce/OnResolve` still shadow the canonical DEAR loop) |
| 3 | `pkg/vroom` code encodes superseded model | **UNRESOLVED** (duplicate of #1) |
| 4 | Two ADR directories (`docs/adr/` vs `docs/adrs/`) | **RESOLVED** ✅ |
| 5 | ADR sprawl (~100+ ADRs) | **AUDIT EXISTS, FOLLOW-UPS NOT LANDED** |

---

## 2. Gap analysis — prioritized

### 2.1 P0 — misleading docs / code

| # | Gap | Where | Why P0 |
|---|---|---|---|
| P0-1 | **VROOM is documented as supervisory mesh, shipped as launcher+heartbeat** | `pkg/vroom/vroom/` (4 files), `agm/cmd/agm/supervisor.go:195–241` | Architectural centerpiece in `CONTEXT.md` is a paper tiger. Any orchestration story sold on this is wrong. |
| P0-2 | **Wayfinder claims 9 phases, implements 12 with lossy mapping** | `wayfinder/internal/phaseisolation/definitions.go:19–33` | Phase semantics are load-bearing for resume/rewind/validate; the duplicates break round-tripping. |
| P0-3 | **TypeScript implementation claim is false** (Wayfinder docs + code comments) | `ARCHITECTURE.md:5,27,166`; `SPEC.md:58,79,112–146`; `SKILL.md:217`; `internal/w0/detector.go:2`; `internal/phaseisolation/types.go:3` | A reader will look for code that doesn't exist. |
| P0-4 | **All 13 AGM hook binaries have 0 % coverage** | `agm/cmd/agm-hooks/*/main.go` | Hooks are the only thing standing between Claude Desktop and unsafe operations. They have been changed in the last 30 days (worktree reaper, pre-merge-commit). |
| P0-5 | **`agm_get_session_metadata` (code) vs `agm_get_session` (live MCP)** | `agm/cmd/agm-mcp-server/tools.go:111` vs the function list in this very session | Either PR #140 didn't actually re-register, or the harness cached the old name. Either way, a client calling the documented name will fail. |
| P0-6 | **ADR-009 and ADR-023 are design-only but filed indistinguishably from implementation ADRs** | `docs/adr/ADR-009*.md:9`, `docs/adr/ADR-023*.md:3` | Readers can't tell which ADRs are aspirational without opening each one. |

### 2.2 P1 — load-bearing untested

| # | Gap | Where |
|---|---|---|
| P1-1 | **`pkg/vcs/hooks.go` has zero tests** (ValidateMemoryPair / ValidateEngramFrontmatter / InstallPreCommitHook) | `pkg/vcs/hooks.go:23,47,130` |
| P1-2 | **Integration tests skip under `CI=true` and `-short`** — they never run | `agm/test/integration/ci_skip_test.go:10`, `agm/workflowbus/ci_skip_test.go:10` |
| P1-3 | **`agm/internal/ops/session_send.go`, `_kill.go`, `_status.go`, `sandbox_cleanup.go` lack tests** | `agm/internal/ops/session_send.go:29` etc. |
| P1-4 | **`pkg/workflow/audit_*.go` adapters lack tests** — DEAR retros depend on them | `pkg/workflow/audit_jsonl.go`, `audit_stdout.go`, `audit_otel.go`, `audit_engram.go` |
| P1-5 | **No JSON-RPC protocol tests for AGM MCP** — `mcpSuccess` / `mcpError` untested | `agm/cmd/agm-mcp-server/tools.go:159–180` |
| P1-6 | **Decision trail is in-memory only** — no append-only persistence | `pkg/vroom/vroom/emitter.go:54–65` |
| P1-7 | **W0 artifact-naming inconsistency silently disables the gate** | `wayfinder/internal/w0/detector.go:70` vs `wayfinder/lib/d1-gate-check.sh:8` |
| P1-8 | **Wayfinder SPEC YAML schema does not match code struct** | `wayfinder/cmd/wayfinder-session/SPEC.md:64–138` vs `wayfinder/internal/status/types_v2.go` |
| P1-9 | **`agm friction` (ADR-023) missing** | no source file |
| P1-10 | **`agm session new --from`/session-handoff (ADR-023) missing** | `cmd/agm/new.go` |

### 2.3 P2 — drift / collision

| # | Gap |
|---|---|
| P2-1 | `pkg/vroom/vroom/topics.go:14` "Verifier" comment + `emitter_test.go:112` `role="verifier"` |
| P2-2 | `agm/internal/rbac/role.go:22–23` exports `RoleVerifier`, `RoleRequester` (superseded) |
| P2-3 | `pkg/workflow.Hooks` uses `OnEnforce/OnResolve` shadowing canonical DEAR loop |
| P2-4 | Wayfinder `coord` and `config` subcommands exist but aren't wired to CLI (`wayfinder/cmd/wayfinder-session/commands/coord.go`, `config.go` — not added in `session.go:49–73`) |
| P2-5 | `wayfinder/cmd/wayfinder-session/commands/migrate_all.go` has corrupted placeholder strings ("the git history") |
| P2-6 | Wayfinder `commands/validate-phase.md` documents a "validate-phase" skill but there is no `validate-phase` CLI subcommand (logic is baked into `complete-phase`) |
| P2-7 | `wayfinder/SPEC.md:187,192,197,202` references `docs/wayfinder/ADR-*` — directory does not exist |
| P2-8 | `pkg/benchmarks/` is stub-only; real path is `agm benchmark swe-lite` which bypasses it |
| P2-9 | GOAL.md mixes implemented and aspirational doctrines without flagging which is which |
| P2-10 | `agm/CAPABILITY-MATRIX.md` filename oversells its content (it's a BDD test-mock matrix) |
| P2-11 | ADR-010 evolution stalled — DAG runner exists, substrate-quality layer (SQLite state, roles registry) not verified |

### 2.4 P3 — hygiene

| # | Gap |
|---|---|
| P3-1 | Multiple FIXME-broken tests committed to main (`persona_integration_test.go`, `hygiene_check_test.go`) |
| P3-2 | BATS smoke tests under `tests/bats/` are not in CI |
| P3-3 | `agm/` has 50+ legacy `*-REPORT.md` / `*-COMPLETION.md` markdowns at the top level — noise that obscures real docs |
| P3-4 | `agm session gc` has no isolated `gc_test.go` |
| P3-5 | `cmd/agm/autoimport_test.go` has TODO for interactive-harness testing |
| P3-6 | `pkg/workflow/state_sqlite.go` exists but it isn't clear whether it is wired in (ADR-010 D2 was the intent) |

---

## 3. Testing strategy

### 3.1 Three tiers

**Tier 0 — Unit (already running).** `go test -short ./...`. 356 packages
green. Investment target: bring **hooks (`agm/cmd/agm-hooks/*`)**,
**ops layer (`agm/internal/ops/*`)**, **`pkg/vcs/hooks.go`**, and
**`pkg/workflow/audit_*.go`** into this tier by extracting logic from
`main()` into testable functions.

**Tier 1 — Component integration (currently skipped).** 52 files tagged
`//go:build integration`. Today they never run. Recommendation: wire a
**nightly CI job** that runs `go test -tags=integration ./...` on a
runner with `tmux`, `gh`, `dolt`, and a stub `claude` binary on `$PATH`.
Accept flakes initially; track and quarantine via a dashboard. Failure of
this nightly does *not* block merges — its purpose is to keep integration
tests honest.

**Tier 2 — End-to-end (does not exist).** No test currently spawns a
real Meta-Orch → Orch → Worker flow. No test exercises `agm session new`
through Claude → message → completion → cleanup. **This is the gap that
will hide the biggest bugs.**

### 3.2 Does E2E need a VM / container?

The honest answer is: **partial container, full VM not required.**

External dependencies the tests actually need:

| Dep | What we'd need | Today | Plan |
|---|---|---|---|
| `tmux` | binary on PATH | installed locally | ship in CI image |
| `gh` | binary on PATH | installed locally | ship in CI image |
| `dolt` | embedded; tests use SQLite adapter | adapter exists | use in-memory mode |
| `git` | binary | installed | already in CI |
| `claude` CLI | spawnable agent | requires OAuth token | **stub binary** that fakes the harness JSON-stream — sufficient for orchestration tests |
| Docker | for SWE-bench grading | **not on this host** | required only for benchmark resolve-rates; out of scope for the orchestration E2E |
| Sandbox (OverlayFS / Bubblewrap) | for sandbox-isolation tests | macOS doesn't have it | tag tests `linux-only`, run only on Linux runner |

**Recommendation:** Build a **`tests/e2e/`** directory using `testscript`
(github.com/rogpeppe/go-internal/testscript) + a stub `claude` binary that
reads scripted responses from a fixtures file. Run on a single Linux GitHub
Actions runner image (Ubuntu 22.04) with `tmux`/`gh`/`dolt` pre-installed.
No VM. No persistent infrastructure. Cost is minutes of CI time per run.

Defer the SWE-bench grading container (Docker harness) until / unless we
decide to invest in resolve-rate measurement on this host.

### 3.3 What to write first

In dependency order (each layer's tests use the previous layer's behavior):

1. **Hook unit tests** (no external deps). `pretool-bash-blocker`,
   `pre-merge-commit`, `posttool-worktree-tracker`, `stop-state-reporter`.
   Extract `main()` into a `Run(args, env, stdin) (exitCode, stdout, stderr, error)`
   function. Table-drive.
2. **`pkg/vcs/hooks.go`** — `ValidateMemoryPair`, `ValidateEngramFrontmatter`.
   Pure functions. Should be 1 PR.
3. **`agm/internal/ops/` session ops** — `session_send.go`, `session_kill.go`,
   `session_status.go`. Mock the Dolt adapter (already abstracted).
4. **MCP JSON-RPC integration** — feed canned JSON-RPC requests to
   `tools.go` handlers; assert response shape, error shape, schema.
5. **AGM CLI end-to-end smoke** — `testscript`-driven `agm session new` →
   `agm send msg` → `agm session list` → `agm session archive`, against a
   stub `claude` binary.
6. **VROOM supervisor smoke** — only meaningful *after* we decide what
   VROOM-in-code is supposed to be (see Decision 1 below).

---

## 4. Fix sequence

### Stage A — Stop the bleeding (1 PR each)

A1. **Add P0/P1 statuses to the doc tree.** Front-matter each `docs/adr/*.md`
    with `Status: Implemented | Partial | Design-only | Superseded`. Same
    for `docs/design/*.md`. Add a one-line table at the top of GOAL.md
    flagging which sections are aspirational. *Cost: hours. Risk: zero.*

A2. **Rebuild and reinstall the AGM MCP binary** to resolve the
    `agm_get_session` vs `agm_get_session_metadata` drift. Verify the
    deferred tool list updates. If after rebuild the live MCP still
    exposes `agm_get_session`, treat as a code bug in registration and
    open a follow-up. *Cost: minutes.*

A3. **Remove or fix the corrupted `migrate_all.go` placeholders.** It's
    user-visible breakage. *Cost: minutes.*

A4. **Fix the W0 artifact-name mismatch.** Decide which name is canonical
    (`W0-project-charter.md` per SPEC) and update
    `wayfinder/lib/d1-gate-check.sh:8`. *Cost: minutes.*

A5. **Remove the false "TypeScript" claims** from `wayfinder/ARCHITECTURE.md`,
    `wayfinder/SPEC.md`, `wayfinder/SKILL.md`, and the Go file headers
    that say "ported from TypeScript." *Cost: 1 PR.*

### Stage B — Test what already exists (parallel, no dependencies)

B1. **Hook unit tests** for the 13 binaries (P0-4). One PR per hook —
    they're independent. Use `/wayfinder` to scope each, `/loop` to drive
    a worker through them in succession.

B2. **`pkg/vcs/hooks.go` tests** (P1-1).

B3. **`agm/internal/ops/session_*.go` tests** (P1-3). Each function is a
    candidate for one PR — keep diffs small.

B4. **AGM MCP JSON-RPC tests** (P1-5). One PR adds a `tools_protocol_test.go`
    that feeds JSON-RPC requests directly to handlers and asserts the
    response envelope.

B5. **Pre-build the CI nightly integration job** but don't enable it as
    blocking. Land the workflow + an Ubuntu image with deps; let it run
    against `main` for two weeks before treating failures as actionable.

### Stage C — Resolve VROOM (depends on Decision 1)

C1. **Either** ship the supervisor loop (long path: pick scope, write the
    skill, persist the decision trail, ship the unblock-first runtime,
    write a real E2E)…

C2. …**or** retract the supervisor mesh from `CONTEXT.md`/`ADR-002` and
    re-describe VROOM as what it actually is today: an
    event-emitter + heartbeat + interactive supervisor sessions driven
    by `/loop` from the user side.

C3. **Either way:** remove `RoleVerifier`, `RoleRequester` from
    `agm/internal/rbac/role.go:22–23` (or formally deprecate via
    `// Deprecated:` comment + migration notice). Rename
    `TopicDecisionEvaluated` (or annotate the comment to match the new
    model). Fix `emitter_test.go:112`.

### Stage D — Resolve Wayfinder phase mismatch (depends on Decision 2)

D1. **Either** collapse 12 V1 IDs into 9 V2 names (breaking; needs
    converter / migration for in-flight sessions)…

D2. …**or** update SPEC/README/SKILL to honestly say "12 phases" and
    remove the V1→V2 mapping noise.

D3. **Either way:** wire `coord` and `config` subcommands into
    `session.go`, or delete the files. Fix the dangling ADR references
    in `wayfinder/SPEC.md:187,192,197,202`.

### Stage E — ADR housekeeping (depends on Decision 3)

E1. **Either** move design-only ADRs to `docs/design/` (and rename so
    they're obvious — e.g., `DESIGN-009`) …

E2. …**or** add a mandatory `Status:` front-matter line + a
    `docs/adr/README.md` index that groups by status.

### Stage F — Aspirational features (defer)

F1. `agm friction` (ADR-023) — implement after Stage C settles the
    decision-trail story.

F2. `agm session new --from`/handoff — same dependency.

F3. ADR-010 substrate evolution — separate workstream.

F4. `pkg/benchmarks/` real executor — separate workstream, blocked on
    Docker availability for grading.

---

## 5. Decision points (need user input before execution starts)

These are the questions whose answers change which version of the plan we
execute. Each has a recommendation; the user should override or confirm.

### Decision 1 — What is VROOM, going forward?

**Context.** `CONTEXT.md` and `ADR-002` describe a 3-supervisor mesh of
long-lived loops with mutual-unblock-first, an append-only decision trail,
and per-task Primary/Secondary/Tertiary ownership. The code in
`pkg/vroom/vroom/` (293 LOC) is an event emitter; `agm supervisor run` is
a launcher; the supervisor-check skill does not exist. To complicate
matters, `GOAL.md` §"Single Orchestration Layer" says **"Current 3 layers
(meta-orch → orchestrator → worker) collapse to 1 smart orchestrator at
current scale."** That is a direct internal contradiction with
`CONTEXT.md`.

**Options:**

| | Option A — Ship the mesh | Option B — Retract to single-orchestrator | Option C — Reaffirm intent, accept gap publicly |
|---|---|---|---|
| Reality | matches CONTEXT.md/ADR-002 | matches GOAL.md "Single Orchestration Layer" | matches today's code |
| Effort | weeks; needs supervisor-check skill, persistent decision trail, real loop runtime, E2E tests | days; revise CONTEXT.md/ADR-002 to describe a single Orchestrator with optional Secondaries | hours; add a "Status: Vision" header to the relevant CONTEXT.md sections |
| Risk | high (untested architecture); blocks other work | medium (admits drift to anyone who already trusted the mesh model) | low (honest), but freezes the gap in place |
| Test surface | full Tier-2 E2E for supervisor mesh | small — most of the AGM CLI already works | none new |

**Recommendation:** **Option B** (retract to single-orchestrator) for now.
GOAL.md §"Single Orchestration Layer" explicitly calls the 3-layer split
premature at current scale; the code agrees. Re-introduce the mesh as a
roadmap item when there are 10+ concurrent workers (GOAL.md's own
threshold). Save the supervisor-check skill / append-only trail / mesh
runtime as work items behind a feature flag.

If the user prefers **Option A**, expect 3–4 weeks before any of it is
testable end-to-end.

### Decision 2 — Wayfinder is 9 phases or 12?

**Context.** Code defines 12 V1 IDs and maps them lossily to 9 V2 names.
SPEC/README/SKILL all say 9. The CLI says 12. Round-tripping is broken
because S9/S10 both map to BUILD and S6/S7 partially duplicate
DESIGN/PLAN.

**Options:**

| | A — Collapse to 9 (truth-up) | B — Document 12 (truth-down) |
|---|---|---|
| Effort | big — V1→V2 migration of in-flight sessions, deprecation of S4/S5/S9/S10 in code | small — doc PRs only |
| Risk | breaks any session mid-flight | none |
| Future story | clean 9-phase narrative survives | 12-phase reality survives; "9-phase" name retired |

**Recommendation:** **Option A**, but staged: in *this* batch only fix
the SPEC↔code field-name drift (`current_phase` vs `CurrentWaypoint`) and
add a `WAYFINDER-PHASES.md` truth-table doc. Plan the actual collapse as
a separate ADR + migration PR.

### Decision 3 — Where do design-only ADRs live?

**Options:**

A. Move them to `docs/design/` (ADR-009, ADR-023 → `docs/design/`).
B. Keep them in `docs/adr/` but add a mandatory `Status: Design-only` header and a `docs/adr/README.md` index grouping by status.
C. Leave as-is (don't touch).

**Recommendation:** **Option B.** Moves no files (preserves inbound
references), adds one header line per ADR, one README. Hours of work.

### Decision 4 — Integration tests in CI

**Options:**

A. Add a nightly Linux job (Ubuntu image with tmux/gh/dolt) that runs
   `go test -tags=integration ./...`. Non-blocking. Track flakes.
B. Keep integration tests local-only. Document expectations.
C. Containerize integration tests (Docker Compose / Testcontainers).
   Move them in-tree to a per-PR gate.

**Recommendation:** **Option A** for the next 4 weeks. If flake rate is
< 5 %, promote to blocking. Option C is a larger investment and shouldn't
gate the other work.

### Decision 5 — Stale role exports (`RoleVerifier`, `RoleRequester`)

**Options:**

A. Delete now — small breaking change to internal `rbac` package.
B. Deprecate with `// Deprecated:` comments and a 90-day removal window.
C. Keep — accept the collision indefinitely.

**Recommendation:** **Option A**. The `rbac` package is internal to AGM;
no external consumers should depend on these. Cost: minutes, plus
sweeping `agm/internal/rbac/profiles.go` of the old profiles. Do this
as part of Stage C.

### Decision 6 — How to use `/wayfinder`, `/goal`, `/loop` during execution

The user mentioned these as available tools during the fix phase. Mapping:

- **`/wayfinder`** — use to scope each Stage-A/B PR. The wayfinder phase
  graph (D1 → D2 → D3 → D4 → S6 → S7 → S8 → S11) gives a sensible
  structure: scope the bug, sketch the fix, write the test, ship.
- **`/goal`** — re-anchor against `GOAL.md` at the start of each session.
  Particularly useful after the GOAL.md "Implemented / In-progress /
  Roadmap" restructure lands (Stage A1).
- **`/loop`** — drive the hook-test PRs (Stage B1) and the AGM ops-test
  PRs (Stage B3). Each batch is N independent PRs; `/loop` paces them.
  Use dynamic interval and let the agent self-pace.

**Decision needed:** confirm that this is the intended use, and confirm
the user wants Stage B work driven by a `/loop` invocation rather than
by hand. If yes, the AFK form is `/loop /wayfinder hook-coverage-batch`
or equivalent (subject to skill naming).

---

## 6. Success criteria

The plan is "done" when **all of the following are measurable on `main`**:

| # | Criterion | How we measure |
|---|---|---|
| 1 | Every top-level ADR has a `Status:` header | `grep -L "^Status:" docs/adr/*.md` is empty |
| 2 | GOAL.md sections labelled Implemented / In-Progress / Roadmap | header structure verifiable by `grep` |
| 3 | AGM MCP tool names in code match the live registration | `agm-mcp-server --list-tools` matches both `docs/adr/*` references and Claude's exposed `mcp__agm__*` set |
| 4 | All 13 AGM hook binaries have ≥ 1 unit test | `find agm/cmd/agm-hooks -name '*.go' -path '*_test.go'` returns ≥ 13 |
| 5 | `pkg/vcs/hooks.go` has tests for ValidateMemoryPair / ValidateEngramFrontmatter / InstallPreCommitHook | direct file presence |
| 6 | AGM MCP has a JSON-RPC protocol test file | `agm/cmd/agm-mcp-server/tools_protocol_test.go` exists |
| 7 | Wayfinder W0 gate uses one canonical artifact name | `grep -r W0-charter.md wayfinder/` returns only the canonical file |
| 8 | VROOM either has a real loop runtime *or* CONTEXT.md/ADR-002 honestly describe the launcher+emitter model | depends on Decision 1 |
| 9 | `RoleVerifier`, `RoleRequester` removed from `agm/internal/rbac/role.go` | grep returns empty |
| 10 | `pkg/vroom/vroom/topics.go:14` comment matches the canonical model | manual check |
| 11 | `wayfinder/cmd/wayfinder-session/commands/migrate_all.go` has no placeholder strings | grep returns empty |
| 12 | Nightly integration job exists and runs on `main` | `.github/workflows/integration-nightly.yml` present, runs green ≥ 1× |
| 13 | `docs/adr/ADR-009` and `ADR-023` are clearly labelled as design-only (front-matter + index) | manual check |
| 14 | All FIXME-broken tests are either fixed or quarantined with a tracking issue | `grep -R "FIXME: this is broken\|FIXME: broken" --include='*_test.go'` returns empty |
| 15 | Wayfinder phase count truth-table exists (`docs/wayfinder/WAYFINDER-PHASES.md` or equivalent) | file present |

**Non-goals for this plan iteration:**

- Implementing `agm friction` (ADR-023). Defer to post-Stage C.
- Implementing session handoff (`agm session new --from`). Same.
- Wiring a real SWE-bench executor in `pkg/benchmarks/`. Separate workstream;
  Docker dependency.
- Resolving collision #5 (ADR sprawl). The audit at
  `docs/audits/2026-05-17-adr-inventory-prune.md` already plans this; not
  re-litigated here.

---

## 7. What this plan deliberately does *not* do

- **Does not commit to numerical coverage targets.** "X% by Y date" is
  performative. Coverage *of the right code* matters; the hooks-have-0%
  finding (P0-4) is the lesson.
- **Does not touch the engram corpus.** Out of scope.
- **Does not propose a rewrite of any subsystem.** The audit found drift,
  not rot. The AGM CLI, MCP, Wayfinder phase engine, and shared ops layer
  are all real, working code that the next 30 days should be additive
  toward — not replaced.
- **Does not adjudicate the GOAL.md vs CONTEXT.md contradiction on
  supervisor count.** Decision 1 is exactly that question, returned to the
  user.

---

## 8. Risk register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| The MCP tool-name drift is a code bug, not a stale-binary bug | Medium | High (clients break) | Stage A2 verifies before assuming |
| Stage B hook-tests reveal that some hooks are wrong, not just untested | Medium | Medium | Land tests one hook at a time; each PR is reversible |
| Decision 1 stalls — VROOM remains a façade indefinitely | Medium | High (architectural credibility) | Force a decision before any Stage C work starts; do not let Stages A/B fix VROOM by accident |
| Wayfinder phase collapse breaks in-flight sessions | Low (if staged) | Medium | Stage D1 includes a migration step; D2 documents 12 phases in the interim |
| Stage A1 (status headers in ADRs) creates merge conflicts with parallel ADR work | Low | Low | Front-load A1; coordinate via VROOM Orchestrator / `/loop` |
| Integration nightly is flaky enough to be ignored | Medium | Medium | Quarantine known flakes (`dear-agent-ci-flakes.md` already documents 4); rerun policy as default |
| `~/src` read-only guardrail bypass via `cd ~/src/<repo> && git …` | Low for this plan (no `~/src` writes) | Catastrophic if triggered | Use worktrees as already mandated by `.claude/CLAUDE.md` |

---

## 9. Appendix — sub-agent report locations

The full sub-agent transcripts produced this plan. They were not written
to disk; the synthesis here is the deliverable. The agent IDs (for resume
via `SendMessage`) for the two that finished with usage metadata are
`aa870bd898fe975ef` (test coverage) and `abf0054753d07e008` (spec/ADR
compliance). The four Explore agents do not have resumable IDs.

If a reviewer wants to re-run any specific audit, the prompts used are
reproducible from the dispatch in `audit/spec-compliance-plan`'s first
agent message.
