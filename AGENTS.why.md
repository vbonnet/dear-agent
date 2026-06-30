# AGENTS.why.md — Decision Log

Co-located rationale for `AGENTS.md` and other root-level config in this repo.
Append new decisions at the bottom.

## Why this file?

The shared AGENTS.md philosophy establishes that config files have co-located
`.why.md` decision logs. This file explains dear-agent-specific choices so
future agents (and future-me) understand the reasoning, not just the rules.

## Why three tiers for output routing?

The `.dear-agent.yml` config and the AGENTS.md "Output Routing" section
together implement two of the three tiers used elsewhere in the project:

| Tier              | Mechanism                          | Role                                   |
|-------------------|------------------------------------|----------------------------------------|
| **Instruction**   | `AGENTS.md` rule                   | Tells agents what the rule is + why    |
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
| 2026-05-02 | Created `.dear-agent.yml` + Claude-scoped "Output Routing" instructions | Second incident of research artifacts committed to the canonical code repo (ai-tools, predecessor) instead of engram-research; added deterministic config lookup so agents don't have to infer routing. Migrated to `AGENTS.md` on 2026-06-24 so every harness inherits it. |
| 2026-05-02 | Deferred enforcement-tier hook | Two-tier approach addresses the observed failure (agents not reading instruction-tier rules); a hook adds maintenance cost and is forward-looking only since dear-agent has no `research/` tree yet. Revisit if a leak occurs. |
| 2026-05-13 | Added "Agent Delegation Enforcement" section to Claude-scoped instructions (instruction tier only) | DEAR retro on stuck tasks (`~/ai-conversation-logs/dear-retros/2026-05-13-enforcement-rules.md`) found a pattern of uncommitted work, ignored supervisor messages, and infinite retry loops. Codified six rules: incremental commits, supervisor messages as commands, 2-retry max, `gtimeout` on `git push`, post-merge worktree cleanup, and "committed to branch" in every DoD. Turn budgets from the source retro were rejected — they punish careful work and reward rushing. Enforcement (tier 3) is deferred; the rules are observable in retros, so a future stop-hook could check git state for uncommitted changes when an agent claims done. Migrated to `AGENTS.md` on 2026-06-24. |
| 2026-05-15 | Corrected the dogfooding mandate's stale `agm acceptance show` reference | Dogfooding the handoff-confidence work surfaced that the instruction mandate told agents to run `agm acceptance show` at task start, but no such subcommand exists in `agm` 2.0.0-dev (acceptance criteria live in `.dear-agent.yml`, formalized by `pkg/acceptance`). `agm session new` and `agm admin doctor` were verified to exist and kept. See `vbonnet/engram-research` `retrospectives/2026-05-15-handoff-confidence-and-agm-cli-drift.md`. |
| 2026-05-28 | Added "Core Engineering Principles" section to Claude-scoped instructions (instruction tier) | Distilled eight cross-cutting principles from the 2026-05-28 working session and version-controlled them so every agent on every machine inherits them: (1) no scope creep — one agent, one scoped plan; (2) enforcement is positive guidance, not punishment; (3) broken thing → DEAR retro → new agent → scoped plan, never fix in-line; (4) Go is the default, no Python for code we control; (5) use Wayfinder for plans and execution; (6) always route through dear-agent/AGM/VROOM; (7) JIT access — escalate permission blocks into model-wide fixes; (8) track everything in Beads. Placed first so it is the foundational lens; existing sections (Output Routing, Dogfooding, Agent Delegation Enforcement) are framed as instances of these principles, not exceptions. Instruction tier only — no new hook added (consistent with the deferred-enforcement stance above). Migrated to `AGENTS.md` on 2026-06-24. |
| 2026-05-29 | Added principle #9 "Atomic action wrappers" to the Core Engineering Principles section | Codifies a recurring pattern: when an action is only safe as an all-or-nothing chain (e.g. `chezmoi apply` → commit → push) or a raw command can't be permission-granted without over-granting (allowing `git push` also allows `git push --force`), wrap it in a small deterministic binary/script that enforces safety by construction, deny the raw command via a `PreToolUse` exit-2 hook, and `ALWAYS_ALLOW` the vetted wrapper. Turns a fuzzy permission question ("can this agent push?") into a crisp, auditable one ("can this agent run the binary we vetted?"). Instruction tier only — wrappers + hooks are built per-action as the need arises (`chezmoi-deploy`, `safe-push`). |
| 2026-06-03 | Added the **enforcement tier** for the dogfooding-routing rule (principle 6): a repo-scoped Claude `PreToolUse` hook `.claude/hooks/pretool-spawn-routing`, wired in `.claude/settings.json` (Beads `ce-qgf`, P0) | Principle 6 ("always route through dear-agent/AGM/VROOM") had only the instruction tier, and it was being routed around — raw `claude`/`cowork` sessions and Cowork tasks kept getting spawned, each a missing flywheel data point. This is the same instruction→config→**enforcement** escalation the output-routing rows above anticipated, now triggered by an observed leak. **Design choices and their why:** (a) *positive nudge, never a block* — the hook emits only `additionalContext`, never a `permissionDecision`, so it cannot block or auto-approve; it injects "here's the AGM path: …, because [flywheel + decision trail]" per principle 2. (b) *project `.claude/settings.json`, not the global chezmoi-managed one* — confirmed via docs that project hooks **merge additively** with user hooks, so the machine-level safety guards still run and the nudge only applies in this repo, version-controlled so every Claude session inherits it. (c) *shell, not Go* — a per-Bash-call hook needs zero build step and is exactly the "small deterministic wrapper (<50 lines)" principle 9 sanctions; it ships with its own test suite (`tests/bats/pretool-spawn-routing.bats`, 13 cases — the repo's sanctioned shell-test path, so it runs in CI and stays clear of the bash-20-line-limit policy that a hand-rolled `.test.sh` harness would trip) per the "hooks are vetted with tests" norm. (d) *deliberately narrow scope* (principle 1): matches only `Bash` raw-spawners (`claude`/`claude-code`/`cowork`, skipping their read-only subcommands) and `mcp__scheduled-tasks__create_scheduled_task`; `agm` and the in-session `Agent`/`Task` subagent tool are intentionally **not** matched (a subagent stays inside the parent AGM session). Honest limitation, documented not hidden: human/Desktop-layer spawns are outside any in-session hook's reach, so this is a best-effort net for the programmatic path, not a complete gate. |
| 2026-06-09 | Followed up the merged anti-stall spec (PR #278, `docs/design/anti-stall.md` + instruction-tier §Anti-Stall, Beads `ce-mzm`) with the two completeness items #278 omitted: this changelog row, and a reference-integrity test (`pkg/gracefulexit/antistall_reference_test.go`) | #278 added the instruction-tier rule but skipped the tier convention's two trailing obligations. **(a) This row.** Every instruction-tier change is logged here so the rationale survives. The spec consolidates guidance repeatedly re-issued per-prompt ("never ask 'should I keep going?'", "execute the backlog continuously", "'nothing found' is valid"), the same per-prompt-rot ADR-018 and the output-routing rows record; publishing once and referencing from `AGENTS.md` is the structural fix. **(b) The test.** A spec only works while `AGENTS.md` keeps pointing at it and the spec keeps stating its directives; a dangling reference or gutted spec silently retires the rule. The repo's recurring failure is exactly that — a doc lands, the wiring that makes agents see it rots untested. The test asserts the spec exists with all five directives + the boundary section, and that `AGENTS.md` still references it. Homed in `pkg/gracefulexit` (not a new package — avoids a `cmd/n` dead-package regression) because the spec's directive 2 *is* this package's no-overfit guardrail; the two are already coupled. Instruction tier only, consistent with the deferred-enforcement stance — the test is a link-rot guard, not a behavioural gate. |
| 2026-06-21 | Re-landed the **enforcement tier** for the wayfinder-only PR lifecycle: the repo-scoped Claude exit-2 `PreToolUse` hook `.claude/hooks/pretool-pr-guard` + its 22-case bats suite, wired in `.claude/settings.json`, with instruction-tier §PR Lifecycle, the `.dear-agent.yml` pr-policy mirror, and this row (Beads `ce-7vqf`) | The `cmd/safe-pr` + `internal/safepr` wrapper landed on main, but the original enforcement PR (#449) that added the deny hook was **closed without merging**, so raw `gh pr create` stayed globally allowed and the wrapper was sanctioned-but-not-enforced — the exact gap that lets PR spray recur. This re-lands only the missing enforcement artifacts (the Go wrapper is already present and untouched). **Design choices and their why:** (a) *hard block, not a nudge* — unlike pretool-spawn-routing, the failure (burndown workers spraying untraced `gh pr create` ~10× faster than the serial merge pipeline could land, with zero attribution; 2026-06-11/12 logjams) is proven and costly, so the hook exits 2 with principle-2 positive guidance (the safe-pr command line, the why, the escalation footer); (b) *enforcement overrides the global allow by construction* — a PreToolUse exit-2 hook denies regardless of permission allow rules, so the repo-scoped hook neutralizes the global `Bash(gh pr create *)` allow inside this repo without editing the chezmoi-managed global settings; (c) *scope* (principle 1): only `create|close|reopen` are denied — read-only pr verbs and the already-governed `gh pr merge` pass; `reopen` is denied with a points-at-a-human message (reopening is a human decision), not routed to safe-pr. **Honest limitations** (per the 2026-06-03 precedent): the hook matches literal command forms incl. launcher prefixes (`gtimeout`/`env`/`sudo`) and line continuations, but `bash -c`, command substitution, on-disk scripts, gh aliases, and `gh api -X POST /repos/…/pulls` bypass it — a best-effort net for the cooperative-but-forgetful agent, not a security boundary; the permission classifier stays the backstop. **Deferred to a human (chezmoi review):** removing the global `Bash(gh pr create *)`/`Bash(gh pr close *)` allows from `~/.claude/settings.json` and adding the `Bash(safe-pr:*)` allow to this repo's `.claude/settings.json` are permission-grant changes the auto-mode classifier blocks; without the allow-list `safe-pr` still works but is per-call prompted. Noted on ce-7vqf. |
| 2026-06-24 | Promoted critical project instructions from `.claude/CLAUDE.md` into `AGENTS.md`; reduced Claude/Gemini/nested agent entrypoints to imports | The repo had cross-harness ambitions but the authoritative policy surface was Claude-only. `AGENTS.md` is now the canonical model-agnostic instruction file for Claude, Codex, Gemini, Antigravity/agy, OpenCode, and future harnesses. Harness-specific files remain only as import/config/hook surfaces. |
