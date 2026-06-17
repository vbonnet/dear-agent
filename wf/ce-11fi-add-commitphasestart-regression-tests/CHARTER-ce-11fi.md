---
phase: CHARTER
title: ce-11fi CommitPhaseStart regression tests
date: 2026-06-17
status: complete
---

# Charter: ce-11fi — add CommitPhaseStart unit tests

Add unit tests for CommitPhaseStart (added in PR #488) and CommitSessionInit
(new bootstrap fix) to guard against ce-fvkz recurrence.

## Success criteria
- CommitPhaseStart has unit test coverage
- CommitSessionInit committed after session start
- Worktree is clean after both calls (no uncommitted files)
