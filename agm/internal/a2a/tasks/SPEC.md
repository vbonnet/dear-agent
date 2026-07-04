# A2A Task Claimer Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/tasks` manages markdown-channel task ownership. It atomically
claims and unclaims task channels, records owner metadata, and lists unclaimed
channels that are awaiting response.

## EARS Requirements

**A2A-TASK-01** When a task is claimed, the system shall require an existing active channel and acquire a per-channel lock before reading or writing ownership metadata.

**A2A-TASK-02** When a task is already claimed, the system shall reject a second claim and report the current owner.

**A2A-TASK-03** When a claim succeeds, the system shall inject owner, timestamp, and reason metadata and update the latest status to `in-progress`.

**A2A-TASK-04** When a task is unclaimed, the system shall require the caller to match the current owner before removing ownership metadata.

**A2A-TASK-05** When unclaiming succeeds, the system shall update the latest status to `awaiting-response`.

**A2A-TASK-06** When listing claimable tasks, the system shall skip missing active directories, hidden files, instruction files, claimed channels, and channels not awaiting response.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/tasks/claimer_test.go`
