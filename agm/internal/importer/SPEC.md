# Orphaned Conversation Import Specification

<!-- Last audited at: 2026-07-20 -->

## Overview

`agm/internal/importer` registers and imports saved conversations into AGM's
harness-neutral manifest store while preserving harness-specific resume metadata.

## Requirements

**IMP-01** When importing a saved conversation, the system shall validate required identifiers, session names, storage availability, and duplicate ownership before creating a manifest.

**IMP-02** When the import harness is Claude Code, Codex CLI, or AGY, the system shall preserve that harness's canonical conversation and resume metadata in the shared manifest.

**IMP-03** When an unsupported harness is requested, the system shall reject the import instead of silently treating it as Claude Code.

**IMP-04** When creating an imported session, the system shall generate an AGM session ID, sanitize the tmux name, and persist the manifest through Dolt.

**IMP-05** When registering an already tracked Claude conversation, the system shall return the existing manifest identity as an idempotent no-op.

**IMP-06** When registration must resolve a workspace, the system shall apply explicit workspace, project-inferred workspace, and configured fallback in that order.

**IMP-07** When history metadata is unavailable but the saved conversation is discoverable, the system shall preserve importability using filesystem metadata rather than discarding the conversation.

**IMP-08** When importing an AGY conversation, the system shall persist its conversation ID, native conversation database, transcript path, discovered working directory, and current AGY default model so a subsequent cold resume can reproduce the native conversation.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/importer/*_test.go`
