# Hook Harness Parity Specification

<!-- Last audited at: 2026-07-21 -->

**Version:** 1.1
**Status:** Baseline
**Scope:** Repository-scoped hook manifests and shared guardrail hook scripts.

## Overview

Hook parity means each active interactive harness receives the same repository
guardrails through that harness's native hook configuration surface. Claude Code
uses `.claude/settings.json`; Codex CLI uses `.codex/hooks.json`; AGY uses
`.agents/hooks.json`; OpenCode uses `.opencode/hooks.json`; Pi uses native
extension events projected through `.pi/hooks.json`.

## EARS Requirements

**HHP-01** When the repository defines hook-capable active harnesses, the system shall provide hook configuration surfaces for `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli`.

**HHP-02** When a hook-capable active harness configures `PreToolUse`, the system shall include spawn-routing, bead-close, bypass, and PR-lifecycle guardrails.

**HHP-03** When a hook-capable active harness configures stop events, the system shall include guardrail feedback on `Stop` and `SubagentStop`.

**HHP-04** When a hook manifest references a local hook script, the system shall keep the referenced script present and executable in the matching harness directory.

**HHP-05** When a non-Claude hook-capable harness supports Beads lifecycle events, the system shall configure `SessionStart`, `UserPromptSubmit`, `PreCompact`, and `PostCompact` events with a harness-specific `bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <harness>-hook` command.

**HHP-06** When an active hook-capable harness is added, the system shall require BDD and Go tests that verify required hook events, guardrail commands, script existence, and executable mode across the hook parity matrix.

**HHP-07** When hook installation help is shown, the system shall describe hook installation as AGM hook support rather than Claude-only support.

**HHP-08** When managed Pi receives native session, input, compaction, settled-agent, or tool-call events, the system shall project them to the shared lifecycle and guardrail hook contract.

**HHP-09** While Beads lacks a native `pi-hook` command, the system shall use the behaviorally equivalent `codex-hook` lifecycle adapter and shall keep that compatibility boundary explicit in Pi's co-located specification.

**HHP-10** When a supported provider transport invokes the provider-neutral staged SPEC contract adapter at its terminal capability, the system shall evaluate the staged contract once and emit one complete provider-native response within fixed input and output limits without claiming that a mutable source transport is installed or runtime-loaded; a successful staged-change reminder shall direct every provider projection to `docs/spec-authoring.md` and the single-source `spec-governance/skills/write-spec/SKILL.md` workflow without copying that skill or claiming native discovery.

**HHP-16** When an operator audits the installed SPEC helper, the status surface shall remain read-only while separately reporting the stable cooperative leaf and the digest-derived content-addressed leaf, and it shall report aggregate current success only when both required identities are current; for each identity it shall report missing or stale bytes and reject any leaf or trusted ancestor that is not owned by UID 0, non-writable by group and world, a non-symlink of the required kind, searchable by unprivileged launchers at every ancestor, and readable and executable without set-user-ID, set-group-ID, or other special mode bits at the leaf; before reporting a missing leaf it shall still validate the complete existing trusted ancestry, inaccessible or unsafe trusted ancestry shall be reported as untrusted rather than an inspection failure, and after hashing the surface shall revalidate both the open descriptor and deployed pathname against the admitted identity before reporting the digest.

**HHP-19** When the installed SPEC helper status surface rebuilds its expected artifact from unchanged source and provenance, the build shall use stable source-derived stamp input, path-independent compilation, disabled ambient Go workspace mode, and disabled implicit VCS stamping so separate invocations produce comparable bytes instead of wall-clock drift.

**HHP-20** Where a cooperative terminal adapter cannot establish a stable retry identity because its invocation or bounded input is invalid, the adapter shall yield termination with advisory feedback when the native protocol offers a non-retrying advisory shape, and where Antigravity can attach a reason only by continuing it shall instead emit an identity-less native allow without a reason rather than create an unbounded Stop loop, without weakening the separately enforced changed-SPEC CI decision.

**HHP-22** When automation needs the installed-helper status exit contract, the system shall provide a directly runnable built status artifact that emits one aggregate JSON result with separate stable and content-addressed details and preserves exit 0 only when both identities are current, 1 when either is missing, stale, or untrusted, and 2 for inspection or usage failure; a Make convenience target may expose Make's documented recipe-failure translation.

**HHP-24** When a provider-native hook response exceeds the serialized output limit, the adapter shall emit a complete compact response without changing the terminal outcome: a block remains a block, an AGY continuation remains a continuation, and a top-level or hook-specific yield remains non-blocking while preserving its native event and valid deterministic feedback identity.

**HHP-26** When a helper verifies its own revision-bound digest, the system shall require the running executable to be the digest-derived content-addressed path, shall authenticate that path's bytes and trusted identity, and on a platform that exposes a handle on the running image shall additionally require that image to be the same file whose bytes were hashed, so an atomic replacement between exec and verification cannot leave an older image running against newer expected bytes. Where no such handle exists the content-addressed pathname is the only available binding, and that residual shall be recorded rather than reported as proof.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`

## Package Test Traceability

- `cmd/spec-contract-hook/main_test.go`
