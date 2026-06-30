# Instruction Entrypoint Parity Specification

<!-- Last audited at: NEEDS-AUDIT -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Root and scoped agent instruction entrypoint files.

## Overview

`AGENTS.md` is the canonical model-agnostic instruction surface for this repo.
Harness-specific instruction files are compatibility shims for tools that probe
for their own filename. They must import `AGENTS.md` first and avoid duplicating
shared rules.

## EARS Requirements

**IEP-01** When an agent reads root instructions, the system shall provide a top-level `AGENTS.md` as the canonical instruction file.

**IEP-02** When a Claude-compatible entrypoint exists, the system shall make `CLAUDE.md` import `AGENTS.md` before any Claude-specific instructions.

**IEP-03** When a Gemini-compatible entrypoint exists, the system shall make `GEMINI.md` import `AGENTS.md` before any Gemini-specific instructions.

**IEP-04** When a Codex-compatible entrypoint exists, the system shall make `CODEX.md` import `AGENTS.md` before any Codex-specific instructions.

**IEP-05** When an AGY or Antigravity-compatible entrypoint exists, the system shall make `AGY.md` import `AGENTS.md` before any AGY-specific instructions.

**IEP-06** When an OpenCode-compatible entrypoint exists, the system shall make `OPENCODE.md` import `AGENTS.md` before any OpenCode-specific instructions.

**IEP-07** When a scoped instruction entrypoint exists below a tool directory, the system shall import the nearest relative path back to the root `AGENTS.md` before scoped instructions.

**IEP-08** When a harness-specific entrypoint is import-only, the system shall not duplicate shared `AGENTS.md` policy sections in that file.

**IEP-09** When instruction entrypoints are changed, the system shall require tests that enforce the import-first contract.
