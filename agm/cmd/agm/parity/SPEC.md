# AGM Command Harness Parity Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: NEEDS-AUDIT -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm` command behavior that starts, resumes, sends to, or controls active AI harness sessions through tmux.

## Overview

AGM command parity means that active harnesses share one command contract even
when their terminal UI and model names differ. Claude Code is the reference
implementation. Codex CLI, AGY, and OpenCode must remain first-class active
tmux harnesses. Gemini CLI remains deprecated compatibility.

## EARS Requirements

**ACP-01** When AGM exposes the `session new` command, the system shall describe the command as creating an AGM-managed harness session rather than a Claude-only session.

**ACP-02** When AGM creates a session for an active harness, the system shall dispatch to a tmux launch implementation for `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli`.

**ACP-03** When AGM resumes an active harness session, the system shall dispatch a harness-specific tmux resume command for `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli`.

**ACP-04** When AGM sends a message to an active harness session, the system shall use tmux delivery for `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli`.

**ACP-05** When AGM changes a running session model, the system shall resolve the session harness before resolving model aliases.

**ACP-06** When AGM changes a running session model, the system shall validate the requested model against the shared harness model registry before writing anything to tmux.

**ACP-07** When AGM previews a model change with `--dry-run`, the system shall report the resolved harness, resolved model, and tmux command without requiring a live tmux session.

**ACP-08** When AGM cannot resolve a running session's harness for model changes, the system shall preserve backward compatibility by using the Claude Code command path unless an explicit harness is provided.

**ACP-09** When an active harness is added, the system shall require command parity tests that verify `session new`, `resume`, `send msg`, and `send set-model` have explicit behavior for the new harness.

**ACP-10** When AGM documents or tests command parity, the system shall keep deprecated Gemini CLI coverage separate from the active parity set.

**ACP-11** When a production Cobra command source imports AGM's canonical tmux package, the system shall require that source to declare an executable command parity contract.

**ACP-12** When command parity contracts are validated, the system shall require an explicit strategy for Claude Code, Codex CLI, AGY, OpenCode, and Pi for every tmux-facing command.

**ACP-13** When a tmux-facing command does not depend on model behavior, the system shall declare it model-independent so the same command contract applies to Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen families.

**ACP-14** When a harness lacks native runtime mode switching or verified draft preservation, the system shall declare a restart or best-effort fallback rather than silently claiming native parity.

**ACP-15** When `agm send stash` or `agm session unstick` preserves human input, the system shall resolve the session harness and use its declared preservation key while warning when the mapping is not verified.

**ACP-16** When `agm send mode` targets Codex CLI or AGY, the system shall return an actionable harness-specific restart configuration because neither harness exposes a verified in-session mode switch.

**ACP-17** When AGM starts AGY, the system shall request its interactive prompt mode and shall map automatic permission mode to the native startup flag.

**ACP-18** When AGM starts Codex, the system shall map plan and automatic permission modes to native startup flags instead of attempting an unsupported in-session mode switch.

**ACP-19** When an AGM session is created with `--persistent`, the system shall omit the shell exit suffix for Claude Code, Codex, AGY, and OpenCode launch commands.

**ACP-20** When AGM creates a `codex-cli` session inside an existing current tmux pane, the system shall validate Codex authentication, dispatch through the canonical Codex launcher, and observe the Codex composer before registering success; if launch or readiness fails, the system shall preserve the pre-existing tmux session and remove only registration artifacts created by the attempt.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/cmd/agm/parity/contracts_test.go`
