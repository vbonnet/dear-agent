<!-- Last audited at: 2026-06-15 -->
# ADR-035: Wayfinder Enforcement at the AGM Session Boundary

**Status:** Accepted  
**Date:** 2026-06-15  
**Deciders:** vbonnet  
**Supersedes:** —  
**Linked beads:** ce-fvkz (closed), ce-5l64 (dogfooding recurrence)

---

## Context

Wayfinder is the declared 9-phase SDLC engine (CHARTER → PROBLEM → RESEARCH →
DESIGN → SPEC → PLAN → SETUP → BUILD → RETRO). The AGENTS.md guarded-delivery
policy requires consequential work to carry an approved Wayfinder trace. In practice, enforcement was
aspirational: a sticky note, not a gate.

Two compounding problems made enforcement impossible until now:

1. **ce-fvkz (keystone bug):** `start-phase` writes `WAYFINDER-STATUS.md` and
   `WAYFINDER-HISTORY.md` to the project directory but does _not_ commit them.
   Every subsequent `start-phase` call begins with a git dirty-check and refuses
   because those marker files are uncommitted. The workaround—`--allow-dirty`—
   was registered on the cobra command but consistently rejected as "unknown
   flag" by agents using `wayfinder session start-phase` (the session
   sub-dispatcher doesn't expose the flag). Phase advancement required a manual
   commit between every single transition, making the workflow unusable under
   automation.

2. **Invisible entry point:** `git worktree add` is the raw action agents use
   to start work. Nothing at that boundary requires or creates a Wayfinder
   session. Work proceeded, completed, and was merged with zero Wayfinder
   telemetry.

The result: 100% of work ran under emergency bypass. The AGM/VROOM dogfooding
telemetry flywheel received no
data, and the SDLC engine got no real-world runs to improve.

---

## Decision

### 1. Fix ce-fvkz: auto-commit marker files on `start-phase`

`start-phase` now calls `CommitPhaseStart` immediately after writing STATUS and
HISTORY, mirroring the `CommitPhaseCompletion` call already in `complete-phase`.
This keeps the worktree clean after every phase transition so the next
`start-phase` does not see uncommitted files. The fix is surgical: one new
function in `internal/git/git.go`, one call site in `commands/start_phase.go`.

### 2. Fix project-dir defaulting to the read-only golden tree

`wayfinder start` previously routed new projects to `gitRoot/wf/` which, when
called from inside `~/src/dear-agent`, resolved to `~/src/dear-agent/wf/`—
inside the read-only golden tree. The fallback was `~/src/ws/{workspace}/wf/`,
also read-only. Both caused silent `MkdirAll` failures.

New logic: when the detected git root is under `~/src/`, the project root is
redirected to `~/worktrees/{repo}/wf/`. The workspace fallback is changed from
`~/src/ws/{workspace}/wf/` to `~/worktrees/{workspace}/wf/`.

### 3. Gate `git worktree add` at the Claude Code layer

A new `PreToolUse` hook (`pretool-worktree-guard`) blocks any `Bash` call that
contains `git worktree add` and exits 2 with the positive guidance required by
AGENTS.md. The message explains that `agm session new` is the sanctioned
entry point, and shows the equivalent command.

Rationale for this specific gate: `git worktree add` is the sole programmatic
path an in-session agent can take to start new parallel work. It is narrow,
auditable, and does not affect read operations or the worktree list command.

This is a nudge that can be escalated to a hard block under the AGENTS.md
permission-block rule if the bypass rate proves persistently
high.

### 4. Tiered enforcement config (tracked, not yet gated)

Full gating at `agm session new` is deferred. Enforcement tiers are defined
here as the target state; the agm-layer gate is a follow-on bead.

| Tier | Repos | Required phases |
|------|-------|-----------------|
| Strict | `dear-agent`, `brain-v2` | All 9 phases |
| Light | `engram-research`, `vbonnet.ai` | CHARTER → BUILD → RETRO |
| Exempt | ad-hoc investigation sessions | Logged, not gated |

Tier lookup is by the `workspace` field in the AGM session manifest. The
`dear-agent` workspace maps to Strict; `research` maps to Light; unnamed /
ad-hoc sessions are Exempt.

---

## Consequences

### Positive

- Phase transitions no longer require a manual commit between each step.
  The worktree is clean after every `start-phase` and `complete-phase`, so
  automation can sequence phases without human intervention.
- New work sessions touched by the `git worktree add` hook get a visible
  reminder to use `agm session new`, increasing AGM coverage without
  breaking existing workflows that run the session creation separately.
- `wayfinder start` no longer silently fails inside `~/src/` repos.

### Negative / Trade-offs

- `start-phase` now creates an additional commit per phase (the "start" marker
  commit). This adds noise to `git log`. The trade-off is considered worth it:
  the alternative (uncommitted state that breaks the next phase transition) is
  worse. Phase start commits use the `wayfinder: start <PHASE>` prefix and can
  be filtered with `git log --grep='^wayfinder:'`.
- The `git worktree add` block will fire on legitimate uses outside an AGM
  session context (e.g. quick investigation branches). The hook exits 2 with a
  clear message, so the cost is one extra step (run `agm session new` or add an
  explicit `--no-wayfinder` flag when the hook is updated to support it).

---

## Alternatives Considered

**A. Gitignore WAYFINDER-STATUS.md / WAYFINDER-HISTORY.md**  
Would eliminate the dirty check problem but remove phase history from the repo.
Rejected: the commit history of phase transitions is valuable for audit and
retrospective (it feeds the DEAR flywheel).

**B. Filter marker files from the dirty check**  
Exclude WAYFINDER-STATUS.md and WAYFINDER-HISTORY.md from `GetUncommittedFilesInProjectDir`.
This avoids the extra commit but leaves the marker files untracked between
transitions, which is surprising to `git status` readers and breaks the audit
trail assumption.

**C. Hard block `git worktree add` from day one**  
Safer long-term but breaks brownfield sessions where agents ran `git worktree add`
before the hook was deployed. Starting as a positive-guidance exit-2 lets
existing workflows surface the guidance without destroying in-flight work.

**D. Gate at `agm session new` immediately**  
The right end-state, but requires the tiered enforcement lookup service and
session-ID propagation to the worktree. Doing that without fixing ce-fvkz first
would gate work behind a broken SDLC engine. Sequence: fix the engine (this ADR),
then gate the entry point (follow-on).

---

## Implementation Plan

Done in this ADR's PR:

1. `wayfinder/cmd/wayfinder-session/internal/git/git.go` — `CommitPhaseStart`
2. `wayfinder/cmd/wayfinder-session/commands/start_phase.go` — call site
3. `wayfinder/cmd/wayfinder/cmd/start.go` — golden-tree project-dir fix
4. `.claude/hooks/pretool-worktree-guard` — hook script
5. `.claude/settings.json` — register the hook

Follow-on beads (not in this PR):

- Implement `.wayfinder` session-ID file written by `agm session new` into the
  new worktree so each worktree is traceable to a Wayfinder session.
- Implement tiered enforcement at `agm session new` (session manifest carries
  `wayfinder_tier`; strict-tier sessions that lack a wayfinder project dir are
  rejected at creation time).
- Update `pretool-worktree-guard` to accept `--no-wayfinder` override (with
  mandatory `--reason`, routed through `internal/override`) for genuine
  exemptions.
