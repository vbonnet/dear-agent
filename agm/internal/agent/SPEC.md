# Agent Harness and Model Parity Specification

<!-- Last audited at: 2026-07-20 -->

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

**AGP-18** When AGM resolves Nemotron or Qwen family defaults, the system shall use the canonical OpenRouter slugs `nvidia/nemotron-3-ultra-550b-a55b` and `qwen/qwen3.6-max-preview`.

**AGP-09** When AGM validates an unknown future model identifier, the system shall allow syntactically safe identifiers and reject shell metacharacters before any value can be interpolated into a tmux command.

### Cross-Harness Tier Aliases

**AGP-10** When a user selects a Claude tier alias for another active harness, the system shall resolve the tier to that harness's closest native model alias.

**AGP-11** When a user selects an active harness's test mode, the system shall choose a low-cost test model for `claude-code`, `codex-cli`, `agy`, and `opencode-cli`.

### AGY Model Lifecycle

**AGP-20** When AGM resolves an AGY model alias or accepts an AGY public model label, the system shall pass an exact label exposed by the installed AGY public model catalog through `--model`, including labels containing spaces or parentheses.

**AGP-24** When AGM resumes an AGY manifest containing an unambiguous retired `2.5-pro` or `2.0-flash-lite` alias or its former full identifier, the system shall translate it to the closest current AGY public model label before constructing the resume command; the ambiguous former default `2.5-flash` on a saved conversation is governed by AGP-28.

**AGP-25** When MCP creates an AGY session, the system shall wait through first-run trust and initialization until the AGY composer is ready before delivering the required startup prompt; cancellation or readiness failure shall enter the shared creation rollback path.

**AGP-26** When the AGM process receives SIGINT or SIGTERM, the root command context shall cancel and every command-scoped active-harness readiness wait, including create, cold resume, post-create prompt delivery, post-resume prompt delivery, and AGY send metadata backfill, shall return without continuing into prompt delivery, attach, or metadata mutation.

**AGP-27** When a user supplies a cross-harness tier alias with different letter case, the system shall canonicalize the alias key case-insensitively while preserving any exact case-sensitive public model label.

**AGP-28** When an imported or manually associated AGY conversation has no observable native model, the system shall leave its manifest model unset and cold-resume without `--model` so AGY retains the saved conversation selection; when a pre-provenance saved-conversation record contains the ambiguous former default `2.5-flash` or `gemini-2.5-flash`, the resume path shall clear that stored override before command construction.

**AGP-29** When `send set-model` changes a running AGY conversation, the system shall persist the selection only after observing a new confirmation that exactly names the requested public model; a stale, mismatched, or unavailable confirmation shall clear the stored model override so a later cold resume cannot force an unselected model.

### Codex Workdir Trust (ce-cmsq)

**AGP-14** When a Codex CLI session is created or resumed through the codex-cli adapter, the system shall record the working directory as a trusted Codex project in `$CODEX_HOME/config.toml` (default `~/.codex/config.toml`) before sending the launch command, so a fresh non-git sandbox directory cannot block Codex startup on its interactive trust prompt.

**AGP-15** When the trusted-projects config already contains an entry for the working directory — at any trust level — the system shall leave the config unmodified, preserving explicit user distrust decisions.

**AGP-16** When appending a trust entry, the system shall preserve the existing config bytes, escape the directory path as a TOML basic-string key, and replace the file atomically; if the existing config fails to parse, the system shall leave the file untouched and report an error rather than risk a duplicate-table append that would break every subsequent Codex launch.

**AGP-17** If pre-trusting the working directory fails, the codex-cli adapter shall warn and still attempt the launch.

### BDD Enforcement

**AGP-12** When a new active harness or model family is added, the system shall require BDD scenarios and registry tests that cross-cut the active parity matrix before the change is complete.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
