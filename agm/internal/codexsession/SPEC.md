# Codex Saved Session Specification

<!-- Last audited at: 2026-07-03 -->

## Purpose

`agm/internal/codexsession` resolves Codex CLI saved-session metadata from the
local `.codex` session stores so AGM can import, resume, and archive Codex
conversations without depending on terminal state alone.

## EARS Requirements

**CXS-01** When resolving a Codex session by ID, the system shall require a non-empty session ID and default an empty home directory to the current user's home.

**CXS-02** When scanning Codex session stores, the system shall search both active sessions and archived sessions.

**CXS-03** When a JSONL file contains a matching `session_meta` entry with a working directory, the system shall return cleaned metadata for that session.

**CXS-04** When multiple saved-session files match the same ID, the system shall return the newest match by file modification time.

**CXS-05** When unreadable or malformed cache entries are encountered during discovery, the system shall skip those entries and continue scanning.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
