# Engram Harness Parity Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Engram retrieval, metadata persistence, and MCP/A2A visibility across active AGM harnesses.

## Overview

Engram parity means every active AGM harness can carry the same retrieved
memory context and persist the same retrieval metadata. The storage contract is
harness-neutral: session manifests and Dolt rows store `EngramMetadata` rather
than Claude-specific fields. Harness-specific prompts or startup paths may
inject the retrieved content differently, but the durable record must remain
the same for Claude Code, Codex CLI, AGY, OpenCode, and Pi.

## EARS Requirements

**ENG-01** When AGM attaches Engram context to a session, the system shall persist retrieval metadata in `manifest.EngramMetadata`.

**ENG-02** When AGM writes session storage, the system shall store Engram enabled state, query, IDs, loaded timestamp, and count in harness-neutral fields.

**ENG-03** When AGM reads session storage, the system shall reconstruct Engram metadata without depending on a Claude session UUID.

**ENG-04** When an active harness receives startup context, the system shall have a declared Engram injection surface for that harness.

**ENG-05** When an active harness lacks a native memory API, the system shall use prompt/context injection and manifest persistence rather than a Claude-specific fallback.

**ENG-06** When AGM exposes MCP operations, the system shall include Engram-related Wayfinder tools without assuming Claude Code is the caller.

**ENG-07** When AGM bridges error memory, the system shall expose the bridge through ops-layer contracts rather than harness-local state.

**ENG-08** When an active harness or supported model family is added, the system shall require Engram parity tests before the addition is considered supported.

**ENG-09** When Hippocampus discovers consolidation evidence, the system shall provide transcript adapters for Claude Code, Codex CLI, Antigravity, OpenCode, and Pi.

**ENG-10** When Hippocampus uses LLM-assisted extraction, the system shall route through a model-family-neutral callback contract.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_parity.feature`
