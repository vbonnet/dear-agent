# AGY Saved Session Specification

<!-- Last audited at: 2026-07-21 -->

## Purpose

`agm/internal/agysession` resolves saved Antigravity CLI conversation metadata
from the local AGY app-data directory so AGM can import and resume AGY
conversations with their workspace, transcript, and permission-mode context.

## EARS Requirements

**AGYS-01** When resolving an AGY conversation by ID, the system shall require a bounded safe path-component identifier before filesystem lookup and locate the matching conversation database under the AGY app-data directory.

**AGYS-02** When transcript files exist for the conversation, the system shall expose their paths and use their modification time when it is newer than the database file.

**AGYS-03** When the last-conversations cache maps the conversation ID to a workspace, the system shall use that workspace without scanning logs.

**AGYS-04** When cache lookup cannot determine the workspace, the system shall inspect at most 257 AGY log-directory entries, using the 257th only to prove exhaustion while retaining at most 256 entries for bounded processing; it shall scan at most the 64 newest regular logs selected from those retained entries by modification time and read at most 2 MiB per log for conversation and workspace markers; if directory entries, older selected candidates, or unscanned bytes remain, including bytes appended during the bounded read, the system shall return a distinguishable budget-exhaustion error unless a known-conversation match inside the bounded candidates is already conclusive.

**AGYS-05** When resolving the latest AGY conversation for a workspace, the system shall prefer the last-conversations cache and fall back to newest-first log discovery; if directory entries remain unprocessed or any candidate is truncated before a complete match is established, the system shall return budget exhaustion rather than accept a bounded-directory, prefix, or older-file match as latest.

**AGYS-06** When an enumerated log disappears during metadata collection or before its bounded scan opens the file, the system shall treat it as a stale rotation snapshot and continue with remaining candidates; when metadata lookup, open, or scan fails for any other reason, the system shall fail explicitly rather than silently omit a potentially newer log.

**AGYS-07** When any AGM surface creates an AGY conversation, the system shall acquire the same cancellation-aware cross-process lock for its canonical workspace before launch and retain it through provider-native identity discovery and persistence.

**AGYS-08** When AGM correlates a freshly created AGY conversation, the system shall snapshot the workspace's provider-native conversation before launch, fail closed on corrupt or incomplete snapshot metadata, and after readiness accept only a safe new conversation identifier while honoring caller cancellation during bounded discovery retries.

**AGYS-09** When AGM correlates an AGY conversation through direct adapter or shared operations creation, the system shall delegate snapshot, bounded retry, validation, and metadata selection to the canonical `CreateIdentityTracker` rather than implement a surface-specific discovery loop.

**AGYS-10** When the canonical AGY workspace lock attempts acquisition, the system shall retry with caller cancellation only for contention and shall stop after the first attempt and return the cause for any other flock error.

**AGYS-11** When the last-conversations cache maps a workspace to a conversation whose database no longer exists, the system shall treat that cache entry as stale and continue newest-first through bounded log discovery, skipping every logged conversation whose database is also gone until it finds a usable saved conversation; it shall return conversation-not-found only when the complete bounded search finds none while still failing explicitly on unsafe identifiers, corruption, unreadable metadata, or discovery-budget exhaustion.

**AGYS-12** When AGM creates, resumes, or discovers an AGY session, the system shall resolve existing workspace inputs, provider cache keys, and provider log markers to one canonical physical path before lock derivation, launch, identity matching, or new metadata persistence while retaining a cleaned absolute spelling for a removed historical workspace.

**AGYS-13** When a fresh AGY provider conversation is created lazily on first input, every prompt-bearing AGM creation surface shall launch the bare interactive harness, wait through trust handling for its composer, deliver the startup prompt exactly once while retaining the canonical workspace lock, and only then discover and persist the new native identity; a fresh shared-lifecycle request without a startup prompt shall fail before tmux mutation, and bootstrap, discovery, cancellation, or registration failure shall retain normal rollback ownership.

## BDD Traceability

- Feature: `agm/test/bdd/features/agy_saved_session_discovery.feature`
- Related feature: `agm/test/bdd/features/harness_parity.feature`
