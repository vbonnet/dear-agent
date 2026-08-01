# Wayfinder History Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Append-only canonical session event history.

## EARS Requirements

**WAYFINDER-HISTORY-01** When history is created, the system shall bind it to the configured project history file.

**WAYFINDER-HISTORY-02** When an event is appended, the system shall assign a timestamp and encode one complete JSON line.

**WAYFINDER-HISTORY-03** While concurrent callers append events, the system shall preserve complete non-interleaved records.

**WAYFINDER-HISTORY-04** When history is empty, the system shall return an empty event sequence.

**WAYFINDER-HISTORY-05** When a history line is corrupted, the system shall report the parse failure rather than silently treating it as a valid event.

**WAYFINDER-HISTORY-06** When events are filtered by canonical phase or type, the system shall return only matching records in history order.

**WAYFINDER-HISTORY-07** When a project contains only the legacy `WAYFINDER-HISTORY.md` JSON Lines log, the system shall atomically rename it to `WAYFINDER-HISTORY.jsonl` before reading, appending, or archiving so the audit trail remains contiguous.

**WAYFINDER-HISTORY-08** When an event is appended, the system shall recursively replace project-directory and home-directory path roots in newly persisted data values with `$PROJECT_DIR` and `$HOME` without mutating caller-owned data or rewriting existing history bytes.

Path-root recognition is privacy-first for diagnostic text: whitespace and common diagnostic punctuation immediately after an exact root are delimiters, even where POSIX could interpret that punctuation as part of a rare sibling filename. Platform path separators remain descendant delimiters; ordinary filename continuations are not rewritten.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/history/history_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
