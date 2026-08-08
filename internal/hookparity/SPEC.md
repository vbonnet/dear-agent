# Hook Harness Parity Specification

<!-- Last audited at: 2026-08-08 -->

**Version:** 1.4
**Status:** Baseline
**Scope:** Repository-scoped hook manifests and shared guardrail hook scripts.

## Overview

Hook parity means each active interactive harness receives equivalent governed
outcomes through capabilities that its native surface actually exposes. Claude Code
uses `.claude/settings.json`; Codex CLI uses `.codex/hooks.json`; AGY uses a
named `.agents/hooks.json` map; OpenCode uses a `.opencode/plugins/` plugin;
Pi uses native
extension events projected through `.pi/hooks.json`.

## EARS Requirements

**HHP-01** When the repository defines hook-capable active harnesses, the system shall provide hook configuration surfaces for `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli`.

**HHP-02** When a hook-capable active harness configures `PreToolUse`, the system shall include spawn-routing, bead-close, bypass, and PR-lifecycle guardrails.

**HHP-03** When a harness supports terminal events, the system shall configure only its native terminal capability: Claude, Codex, and Pi expose Stop and SubagentStop; AGY exposes Stop only; OpenCode uses bounded `session.idle` follow-up rather than a stop hook.

**HHP-04** When a hook manifest references a local hook script, the system shall keep the referenced script present and executable in the matching harness directory.

**HHP-05** When a non-Claude hook-capable harness supports Beads lifecycle events, the system shall configure `SessionStart`, `UserPromptSubmit`, `PreCompact`, and `PostCompact` events with a harness-specific `bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <harness>-hook` command.

**HHP-06** When an active hook-capable harness is added, the system shall require BDD and Go tests that verify required hook events, guardrail commands, script existence, and executable mode across the hook parity matrix.

**HHP-07** When hook installation help is shown, the system shall describe hook installation as AGM hook support rather than Claude-only support.

**HHP-08** When managed Pi receives native session, input, compaction, settled-agent, or tool-call events, the system shall project them to the shared lifecycle and guardrail hook contract.

**HHP-09** While Beads lacks a native `pi-hook` command, the system shall use the behaviorally equivalent `codex-hook` lifecycle adapter and shall keep that compatibility boundary explicit in Pi's co-located specification.

**HHP-10** When a harness reaches its supported terminal capability, the system shall invoke one provider-neutral staged SPEC contract adapter through a provider-specific transport with bounded continuation or follow-up feedback and no claim that a mutable source transport is installed or runtime-loaded; a successful staged-change reminder shall direct every provider projection to `docs/spec-authoring.md` and the single-source `spec-governance/skills/write-spec/SKILL.md` workflow without copying that skill or claiming native discovery.

**HHP-11** When a harness lacks a lifecycle event or a neutral pre-tool decision that preserves its ordinary permission flow, the system shall omit the active projection and shall record the capability boundary rather than translate success into automatic approval or unconditional prompting; a governance-required legacy path may remain only as exact inert retirement metadata with no hook payload or runtime claim.

**HHP-12** Where a harness can surface remediation only as an idle-session follow-up, when the neutral guard returns feedback after a distinct user turn, the harness shall issue no more than one follow-up for that turn, shall not treat its own or repeated message updates as new turns, shall cease follow-ups safely after its bounded turn-history capacity is reached, shall cap globally retained session state by yielding untracked sessions rather than evicting continuation state, and shall admit a yielded session after explicit deletion frees capacity.

**HHP-13** When AGY configures its bounded SPEC review, its source manifest shall name the absolute operator-owned adapter and that adapter shall select exactly one valid absolute Git workspace supplied by the native hook input rather than rely on the hook process current directory.

**HHP-14** When AGY supplies an invalid, missing, or ambiguous workspace root, the adapter shall bound remediation to one continuation per stable conversation and failure identity without assuming whether the native execution sequence starts at zero or one; a repeated or missing identity, or unavailable private claim state, shall allow termination.

**HHP-15** When Pi projects Stop or SubagentStop through multiple matching handlers, the Pi adapter shall execute every matching handler admitted by its total count and execution-deadline budgets before returning bounded aggregate blocking reasons and advisory contexts; it shall honor each validated manifest timeout up to 120 seconds without silently shortening the canonical 60-second and 120-second terminal chain, cap captured output, bound feedback while collecting it, and fail closed when a count or aggregate deadline budget is exhausted.

**HHP-16** When an operator audits the installed SPEC helper, the status surface shall remain read-only while reporting missing or stale bytes and rejecting any leaf or trusted ancestor that is not owned by UID 0, non-writable by group and world, a non-symlink of the required kind, and executable at the leaf.

**HHP-17** When AGM launches unattended Codex with a recognizable terminal SPEC adapter, the system shall replace that source adapter with the validated root-owned helper, bind the canonical repository root, disable the mutable project copy, and digest-pin the effective session command.

**HHP-18** When Claude or Codex terminal feedback runs, the adapter shall bind its attempt to the native session and deterministic feedback identity, shall include Codex's bounded native turn identity in the Codex attempt, shall clear Claude's prior session claim on the native `UserPromptSubmit` boundary, shall allow at most one blocker among concurrent invocations of the same attempt, shall block once for a fresh identity even when a sibling hook caused the provider-global active continuation, shall yield a repeated identity without claiming compliance, and shall use the provider-global flag only as a liveness fallback when private claim state is unavailable.

**HHP-19** When the installed SPEC helper status surface rebuilds its expected artifact from unchanged source and provenance, the build shall use stable source-derived stamp input, path-independent compilation, and disabled implicit VCS stamping so separate invocations produce comparable bytes instead of wall-clock drift.

**HHP-20** Where a cooperative terminal adapter cannot establish a stable retry identity because its invocation or bounded input is invalid, the adapter shall yield termination with advisory feedback instead of creating an unbounded fail-closed loop, without weakening the separately enforced changed-SPEC CI decision.

**HHP-21** When a plain Codex source hook runs without an AGM or Claude repository-root environment variable, the adapter command shall resolve the canonical Git worktree root from the native session working directory before invoking repository source.

**HHP-22** When automation needs the installed-helper status exit contract, the system shall provide a directly runnable built status artifact that emits one JSON result and preserves exit 0 for current, 1 for missing, stale, or untrusted, and 2 for inspection or usage failure; a Make convenience target may expose Make's documented recipe-failure translation.

**HHP-23** When Pi aggregates terminal handlers, the SPEC adapter shall return a bounded deterministic feedback identity and the persistent Pi extension shall allow one follow-up for each fresh identity despite sibling continuation state, suppress repeats, retain a finite per-turn continuation budget, and reset that budget on a real interactive or RPC turn.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`

## Package Test Traceability

- `cmd/spec-contract-hook/main_test.go`
- `tests/buildstamp/buildstamp_test.go`
