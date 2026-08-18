# Pi Hook Bridge Specification

<!-- Last audited at: 2026-07-21 -->

**PI-HOOK-01** When managed Pi starts, the AGM authorization extension shall load `.pi/hooks.json` only from the explicitly approved working directory.

**PI-HOOK-02** When Pi requests a tool, the extension shall pass a harness-neutral tool event on standard input to every matching `PreToolUse` command and block the call when a guard rejects it.

**PI-HOOK-03** When Pi emits session, input, compaction, or settled-agent events, the extension shall project them to the shared lifecycle hook events.

**PI-HOOK-04** When Pi invokes repository guardrails, the `.pi/guardrails` wrappers shall reuse the living OpenCode shell implementations rather than copy their policy logic.

**PI-HOOK-05** While Beads has no `pi-hook` command, Pi shall use the behaviorally equivalent `codex-hook` lifecycle adapter with Dolt auto-commit enabled for SessionStart, UserPromptSubmit, PreCompact, and PostCompact, and the documentation shall identify that compatibility boundary.

**PI-HOOK-06** When an approved project contains a malformed or unreadable `.pi/hooks.json`, the managed extension shall fail closed for tool calls instead of treating the manifest as an empty guardrail set.

**PI-HOOK-07** When a Pi lifecycle hook runs, the extension shall provide the shared event name, native session identity, approved working directory, stop-loop state, and event payload on standard input.

**PI-HOOK-08** When a successful hook emits a structured block decision, the extension shall consume rejected user input, cancel rejected compaction, or block the rejected tool; when a Stop hook blocks, Pi shall deliver its remediation as a follow-up user turn with a bounded loop state.

**PI-HOOK-09** When a successful hook emits additional context, Pi shall continue running the remaining matching hooks and shall still fail closed if a later hook rejects the event.

**PI-HOOK-10** When the conventional Pi `subagent` extension tool completes, the managed extension shall project the tool result to `SubagentStop` and shall deliver blocking remediation to the parent turn because Pi subagents run as isolated child processes.

**PI-HOOK-11** When Pi loads repository resources, the system shall keep executable guardrail wrappers outside the reserved `.pi/hooks/` legacy-extension directory so current Pi releases do not pause for migration input.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
