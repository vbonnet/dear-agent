# AGY Saved Session Specification

<!-- Last audited at: 2026-07-20 -->

## Purpose

`agm/internal/agysession` resolves saved Antigravity CLI conversation metadata
from the local AGY app-data directory so AGM can import and resume AGY
conversations with their workspace, transcript, and permission-mode context.

## EARS Requirements

**AGYS-01** When resolving an AGY conversation by ID, the system shall require a non-empty conversation ID and locate the matching conversation database under the AGY app-data directory.

**AGYS-02** When transcript files exist for the conversation, the system shall expose their paths and use their modification time when it is newer than the database file.

**AGYS-03** When the last-conversations cache maps the conversation ID to a workspace, the system shall use that workspace without scanning logs.

**AGYS-04** When cache lookup cannot determine the workspace, the system shall scan at most the 64 newest regular AGY logs by modification time and at most 2 MiB per log for conversation and workspace markers; if older candidates or unscanned bytes remain and no match is found, the system shall return a distinguishable budget-exhaustion error rather than report a complete miss.

**AGYS-05** When resolving the latest AGY conversation for a workspace, the system shall prefer the last-conversations cache and fall back to newest-first log discovery; if any candidate is truncated before a complete match is established, the system shall return budget exhaustion rather than accept a prefix match or an older-file match as latest.

## BDD Traceability

- Feature: `agm/test/bdd/features/agy_saved_session_discovery.feature`
- Related feature: `agm/test/bdd/features/harness_parity.feature`
