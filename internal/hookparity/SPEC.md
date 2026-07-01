# Hook Harness Parity Specification

<!-- Last audited at: 2026-07-01 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Repository-scoped hook manifests and shared guardrail hook scripts.

## Overview

Hook parity means each active interactive harness receives the same repository
guardrails through that harness's native hook configuration surface. Claude Code
uses `.claude/settings.json`; Codex CLI uses `.codex/hooks.json`; AGY uses
`.agents/hooks.json`; OpenCode uses `.opencode/hooks.json`.

## EARS Requirements

**HHP-01** When the repository defines hook-capable active harnesses, the system shall provide hook configuration surfaces for `claude-code`, `codex-cli`, `agy`, and `opencode-cli`.

**HHP-02** When a hook-capable active harness configures `PreToolUse`, the system shall include spawn-routing, bead-close, bypass, and PR-lifecycle guardrails.

**HHP-03** When a hook-capable active harness configures stop events, the system shall include guardrail feedback on `Stop` and `SubagentStop`.

**HHP-04** When a hook manifest references a local hook script, the system shall keep the referenced script present and executable in the matching harness directory.

**HHP-05** When a non-Claude hook-capable harness supports Beads lifecycle events, the system shall configure `SessionStart`, `UserPromptSubmit`, `PreCompact`, and `PostCompact` events with a harness-specific `bd --db ~/beads/context-engine/.beads <harness>-hook` command.

**HHP-06** When an active hook-capable harness is added, the system shall require BDD and Go tests that verify required hook events, guardrail commands, script existence, and executable mode across the hook parity matrix.

**HHP-07** When hook installation help is shown, the system shall describe hook installation as AGM hook support rather than Claude-only support.
