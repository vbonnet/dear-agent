# Agent Harness and Model Parity Specification

<!-- Last audited at: 2026-07-01 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** AGM harness adapters, active harness parity, and model-family routing

## Overview

`agm/internal/agent` owns the harness adapter contract used by AGM to create,
resume, send to, inspect, export, import, and terminate AI agent sessions. It
also owns the model alias registry used by CLI creation flows, OpenCode model
selection, and cross-harness tier aliases.

Claude Code is the reference implementation. Codex CLI, AGY, and OpenCode are
active parity harnesses. Gemini CLI is accepted only for deprecated
compatibility.

## EARS Requirements

### Harness Parity

**AGP-01** When AGM enumerates active harnesses, the system shall return `claude-code`, `codex-cli`, `agy`, and `opencode-cli` in canonical parity order.

**AGP-02** When AGM validates a deprecated compatibility harness, the system shall accept `gemini-cli` without adding it to the active parity set.

**AGP-03** When a user supplies a legacy Antigravity harness spelling, the system shall normalize `antigravity` and `agy-cli` to `agy` before validation, factory lookup, or model lookup.

**AGP-04** When AGM resolves an active harness adapter, the system shall return a concrete adapter whose `Name()` matches the normalized harness identifier.

**AGP-05** When AGM builds OpenCode model choices, the system shall include model aliases from every active harness and the OpenRouter-compatible model family source while excluding deprecated-only Gemini CLI aliases.

**AGP-13** When AGM validates active harness adapter conformance, the system shall run the same non-I/O adapter contract across every active harness and require canonical identity, non-empty version, sane capabilities, default model coverage, test model coverage, model aliases, and model family coverage.

### Model Families

**AGP-06** When AGM enumerates supported model families, the system shall return Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen in priority order.

**AGP-07** When AGM maps a supported model family to a default model, the system shall return at least one syntactically safe model identifier for each family.

**AGP-08** When AGM exposes OpenRouter-compatible model aliases, the system shall include GLM 5.2, DeepSeek V4, Nemotron, and Qwen family aliases in that priority order.

**AGP-09** When AGM validates an unknown future model identifier, the system shall allow syntactically safe identifiers and reject shell metacharacters before any value can be interpolated into a tmux command.

### Cross-Harness Tier Aliases

**AGP-10** When a user selects a Claude tier alias for another active harness, the system shall resolve the tier to that harness's closest native model alias.

**AGP-11** When a user selects an active harness's test mode, the system shall choose a low-cost test model for `claude-code`, `codex-cli`, `agy`, and `opencode-cli`.

### BDD Enforcement

**AGP-12** When a new active harness or model family is added, the system shall require BDD scenarios and registry tests that cross-cut the active parity matrix before the change is complete.
