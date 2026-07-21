# Orphaned Conversation Import Specification

<!-- Last audited at: 2026-07-20 -->

## Overview

`agm/internal/importer` registers and imports saved conversations into AGM's
harness-neutral manifest store while preserving harness-specific resume metadata.

## Requirements

**IMP-01** When importing a saved conversation, the system shall validate required identifiers, session names, storage availability, and duplicate ownership before creating a manifest.

**IMP-02** When the import harness is Claude Code, Codex CLI, AGY, or Pi, the system shall preserve that harness's canonical conversation and resume metadata in the shared manifest.

**IMP-03** When an unsupported harness is requested, the system shall reject the import instead of silently treating it as Claude Code.

**IMP-04** When creating an imported session, the system shall generate an AGM session ID, sanitize the tmux name, and persist the manifest through Dolt.

**IMP-05** When registering an already tracked Claude conversation, the system shall return the existing manifest identity as an idempotent no-op.

**IMP-06** When registration must resolve a workspace, the system shall apply explicit workspace, project-inferred workspace, and configured fallback in that order.

**IMP-07** When history metadata is unavailable but the saved conversation is discoverable, the system shall preserve importability using filesystem metadata rather than discarding the conversation.

**IMP-08** When importing an AGY conversation, the system shall persist its conversation ID, native conversation database, transcript path, and discovered working directory while leaving the model unset because AGY's public saved-conversation metadata does not expose the native selection.

**IMP-09** When importing a Pi conversation, the system shall locate an exact native JSONL header ID without following symlinks, enforce bounded file sizes, copy the transcript into AGM-owned private storage, and persist the native ID, private session directory, exact transcript path, and absolute cwd.

**IMP-10** When Pi import persistence fails after creating a private copy, the system shall remove that copy before returning the database error.

**IMP-11** When a Pi transcript establishes a native provider and model, the system shall preserve that provider-qualified model for cold resume; when no native model provenance exists, the system shall leave the override empty.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/importer/*_test.go`
