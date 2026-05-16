# AGENTS.why.md — Decision Log

Co-located rationale for `AGENTS.md` and other root-level config in this repo.
Append new decisions at the bottom.

## Why this file?

The shared AGENTS.md philosophy establishes that config files have co-located
`.why.md` decision logs. This file explains dear-agent-specific choices so
future agents (and future-me) understand the reasoning, not just the rules.

## Why three tiers for output routing?

The `.dear-agent.yml` config and the CLAUDE.md "Output Routing" section
together implement two of the three tiers used elsewhere in the project:

| Tier              | Mechanism                          | Role                                   |
|-------------------|------------------------------------|----------------------------------------|
| **Instruction**   | `.claude/CLAUDE.md` rule           | Tells agents what the rule is + why    |
| **Configuration** | `.dear-agent.yml`                  | Authoritative answer to "where does X go?" |
| **Enforcement**   | (not implemented; see below)       | Blocks the action at runtime           |

The two-tier (instruction + config) implementation was a deliberate stopping
point. Research artifacts leaked into the canonical code repo twice in the
predecessor (ai-tools); the first time the only signal was a CLAUDE.md
sentence buried in a long file, which agents clearly weren't reading. Adding
a *deterministic config lookup* gives an agent a single short file to read
and the answer it needs, with no judgment required. That handles the failure
mode actually observed.

A third "enforcement" tier (a pretool hook that blocks `Write` / `Edit` to
forbidden paths) was considered and deferred. Reasons:
- The forbidden-path globs (`research/*.md`, `research/*.txt`) are
  forward-looking — dear-agent does not currently have a `research/` tree —
  so a hook would mostly be checking a directory that doesn't exist.
- The existing pretool hooks (pretool-bash-blocker, pretool-npm-safety) are
  Go binaries with their own test suites. Adding a fourth hook is a
  meaningful surface area increase and not justified until the two-tier
  approach has been observed to fail.

If a leak occurs despite the config, escalate to the enforcement tier:
add an `agm/cmd/agm-hooks/pretool-output-routing` hook that reads
`.dear-agent.yml` and rejects writes to forbidden paths.

## Why dear-agent is "code", not "research"

`dear-agent` ships agent infrastructure (AGM, Engram, Wayfinder, codegen).
Research artifacts — analysis docs, transcripts, literature reviews,
findings — belong in the dedicated corpus repo, not interleaved with code.

The corpus repo is `engram-research`. Routing analysis docs and transcripts
there keeps:
- dear-agent history focused on code changes (clean blame, faster `git log`).
- Research artifacts colocated with the rest of the corpus where engram's
  ingestion / indexing tools can find them.

---

## Design Decisions Log

| Date | Decision | Context |
|------|----------|---------|
| 2026-05-02 | Created `.dear-agent.yml` + CLAUDE.md "Output Routing" section | Second incident of research artifacts committed to the canonical code repo (ai-tools, predecessor) instead of engram-research; added deterministic config lookup so agents don't have to infer routing |
| 2026-05-02 | Deferred enforcement-tier hook | Two-tier approach addresses the observed failure (agents not reading CLAUDE.md rules); a hook adds maintenance cost and is forward-looking only since dear-agent has no `research/` tree yet. Revisit if a leak occurs. |
| 2026-05-13 | Added "Agent Delegation Enforcement" section to `.claude/CLAUDE.md` (instruction tier only) | DEAR retro on stuck tasks (`~/ai-conversation-logs/dear-retros/2026-05-13-enforcement-rules.md`) found a pattern of uncommitted work, ignored supervisor messages, and infinite retry loops. Codified six rules: incremental commits, supervisor messages as commands, 2-retry max, `gtimeout` on `git push`, post-merge worktree cleanup, and "committed to branch" in every DoD. Turn budgets from the source retro were rejected — they punish careful work and reward rushing. Enforcement (tier 3) is deferred; the rules are observable in retros, so a future stop-hook could check git state for uncommitted changes when an agent claims done. |
| 2026-05-15 | Corrected the dogfooding mandate's stale `agm acceptance show` reference | Dogfooding the handoff-confidence work surfaced that the CLAUDE.md mandate instructed agents to run `agm acceptance show` at task start, but no such subcommand exists in `agm` 2.0.0-dev (acceptance criteria live in `.dear-agent.yml`, formalized by `pkg/acceptance`). `agm session new` and `agm admin doctor` were verified to exist and kept. See `docs/retros/2026-05-15-handoff-confidence-and-agm-cli-drift.md`. |
| 2026-05-28 | Added "Core Engineering Principles" section to `.claude/CLAUDE.md` (instruction tier) | Distilled eight cross-cutting principles from the 2026-05-28 working session and version-controlled them so every agent on every machine inherits them: (1) no scope creep — one agent, one scoped plan; (2) enforcement is positive guidance, not punishment; (3) broken thing → DEAR retro → new agent → scoped plan, never fix in-line; (4) Go is the default, no Python for code we control; (5) use `/wayfinder` for plans and execution; (6) always route through dear-agent/AGM/VROOM; (7) JIT access — escalate permission blocks into model-wide fixes; (8) track everything in Beads. Placed first so it is the foundational lens; existing sections (Output Routing, Dogfooding, Agent Delegation Enforcement) are framed as instances of these principles, not exceptions. Instruction tier only — no new hook added (consistent with the deferred-enforcement stance above). |
| 2026-05-29 | Added principle #9 "Atomic action wrappers" to the Core Engineering Principles section | Codifies a recurring pattern: when an action is only safe as an all-or-nothing chain (e.g. `chezmoi apply` → commit → push) or a raw command can't be permission-granted without over-granting (allowing `git push` also allows `git push --force`), wrap it in a small deterministic binary/script that enforces safety by construction, deny the raw command via a `PreToolUse` exit-2 hook, and `ALWAYS_ALLOW` the vetted wrapper. Turns a fuzzy permission question ("can this agent push?") into a crisp, auditable one ("can this agent run the binary we vetted?"). Instruction tier only — wrappers + hooks are built per-action as the need arises (`chezmoi-deploy`, `safe-push`). |
| 2026-06-03 | Added the **enforcement tier** for the dogfooding-routing rule (principle 6): a repo-scoped `PreToolUse` hook `.claude/hooks/pretool-spawn-routing`, wired in a new project `.claude/settings.json` (Beads `ce-qgf`, P0) | Principle 6 ("always route through dear-agent/AGM/VROOM") had only the instruction tier (CLAUDE.md §Dogfooding), and it was being routed around — raw `claude`/`cowork` sessions and Cowork tasks kept getting spawned, each a missing flywheel data point. This is the same instruction→config→**enforcement** escalation the output-routing rows above anticipated, now triggered by an observed leak. **Design choices and their why:** (a) *positive nudge, never a block* — the hook emits only `additionalContext`, never a `permissionDecision`, so it cannot block or auto-approve; it injects "here's the AGM path: …, because [flywheel + decision trail]" per principle 2. (b) *project `.claude/settings.json`, not the global chezmoi-managed one* — confirmed via docs that project hooks **merge additively** with user hooks, so the machine-level safety guards still run and the nudge only applies in this repo, version-controlled so every agent/machine inherits it. (c) *shell, not Go* — a per-Bash-call hook needs zero build step and is exactly the "small deterministic wrapper (<50 lines)" principle 9 sanctions; it ships with its own test suite (`tests/bats/pretool-spawn-routing.bats`, 13 cases — the repo's sanctioned shell-test path, so it runs in CI and stays clear of the bash-20-line-limit policy that a hand-rolled `.test.sh` harness would trip) per the "hooks are vetted with tests" norm. (d) *deliberately narrow scope* (principle 1): matches only `Bash` raw-spawners (`claude`/`claude-code`/`cowork`, skipping their read-only subcommands) and `mcp__scheduled-tasks__create_scheduled_task`; `agm` and the in-session `Agent`/`Task` subagent tool are intentionally **not** matched (a subagent stays inside the parent AGM session). Honest limitation, documented not hidden: human/Desktop-layer spawns are outside any in-session hook's reach, so this is a best-effort net for the programmatic path, not a complete gate. |
