# Session Sweeper Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `agm/internal/sweeper`.

## Overview

`sweeper` reclaims dead-pane tmux sessions and stuck AGM sessions according to a
rate-limited configuration. It is used as a safety net around session lifecycle
automation: protected sessions are skipped, dry runs report without mutating,
and process liveness checks determine whether a pane is truly dead before kill
and archive actions are attempted.

## EARS Requirements

**SWEEPER-01** When sweeping is disabled, the system shall return no sweep result.

**SWEEPER-02** When the sweep interval has not elapsed since the previous sweep, the system shall skip sweeping.

**SWEEPER-03** When configured duration strings are invalid, the system shall return a parse error.

**SWEEPER-04** When a tmux session is protected, the system shall skip that session.

**SWEEPER-05** When a tmux session has a live pane process, the system shall skip dead-pane cleanup for that session.

**SWEEPER-06** When a tmux session has a dead pane process and dry run is enabled, the system shall report the session without killing it.

**SWEEPER-07** When a tmux session has a dead pane process and dry run is disabled, the system shall kill the session.

**SWEEPER-08** When a stuck AGM session exceeds the configured maximum age, the system shall request archival through the configured archiver.

**SWEEPER-09** When listing AGM sessions fails, the system shall record the failure in the sweep result.

**SWEEPER-10** When default configuration is requested, the system shall provide enabled cleanup thresholds and protected session names.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`

## Test Traceability

- Unit package: `agm/internal/sweeper`
