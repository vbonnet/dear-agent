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

**ACP-02** When AGM creates a session for an active harness, the system shall dispatch to a tmux launch implementation for `claude-code`, `codex-cli`, `agy`, and `opencode-cli`.

**ACP-03** When AGM resumes an active harness session, the system shall dispatch a harness-specific tmux resume command for `claude-code`, `codex-cli`, `agy`, and `opencode-cli`.

**ACP-04** When AGM sends a message to an active harness session, the system shall use tmux delivery for `claude-code`, `codex-cli`, `agy`, and `opencode-cli`.

**ACP-05** When AGM changes a running session model, the system shall resolve the session harness before resolving model aliases.

**ACP-06** When AGM changes a running session model, the system shall validate the requested model against the shared harness model registry before writing anything to tmux.

**ACP-07** When AGM previews a model change with `--dry-run`, the system shall report the resolved harness, resolved model, and tmux command without requiring a live tmux session.

**ACP-08** When AGM cannot resolve a running session's harness for model changes, the system shall preserve backward compatibility by using the Claude Code command path unless an explicit harness is provided.

**ACP-09** When an active harness is added, the system shall require command parity tests that verify `session new`, `resume`, `send msg`, and `send set-model` have explicit behavior for the new harness.

**ACP-10** When AGM documents or tests command parity, the system shall keep deprecated Gemini CLI coverage separate from the active parity set.
