# Session Reference Detection Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/sessionid`.

## Overview

`internal/sessionid` detects Claude Code session identifiers and session
permalinks in text that is about to become permanent public GitHub state. It is
the shared detector behind the two sanctioned publish paths: `safe-pr` scans a
pull request's title, body, and mutation comment, and `safe-push` scans the
messages of the commits a push would publish.

The package matches only shapes unique to a Claude session reference. Bare
UUIDs are excluded by design: this repository carries them in worktree paths
and fixtures, and a rule that fires on ordinary work is a rule that gets
bypassed.

## EARS Requirements

**SESSIONID-01** When scanned text contains a Claude session permalink, the system shall report one session-url finding for that permalink.

**SESSIONID-02** When scanned text contains a bare Claude session identifier, the system shall report one session-id finding for that identifier.

**SESSIONID-03** When a bare session identifier is contained within a reported permalink, the system shall suppress the nested identifier finding.

**SESSIONID-04** When scanned text contains no session reference, the system shall report no findings.

**SESSIONID-05** When a finding is reported, the system shall report the matched text and its one-based line number.

**SESSIONID-06** When redaction is requested for text containing session references, the system shall replace every reference with the redaction marker and shall preserve all surrounding text.

**SESSIONID-07** When findings are described for an operator message, the system shall collapse identical matches into one entry listing every line on which they occur.

## BDD Traceability

- No BDD change, with reason: this package is a pure text-detection library with
  no user-visible workflow of its own. Its observable behaviour reaches users
  only through `safe-pr` and `safe-push`, whose refusals are covered by the
  deterministic unit consequences below.

## Test Traceability

- Unit package: `internal/sessionid`
- Enforcement call sites: `internal/safepr` (pull request text),
  `internal/safegit` (commit messages)
