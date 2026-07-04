# AGM Readiness Signal Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/readiness` waits for AGM session association signals written as
ready files under the AGM state directory. It bounds startup waits, validates
ready-file payloads, handles crash signals, and removes stale or consumed
signals so session creation cannot hang indefinitely.

## EARS Requirements

**READY-01** When readiness timeout is read from the environment, the system shall clamp valid values between the configured minimum and maximum timeout.

**READY-02** When waiting for a ready file, the system shall create the AGM state directory with user-only permissions before watching it.

**READY-03** When a ready file already exists before the watcher starts, the system shall parse it, handle ready or crashed status, and remove the consumed file.

**READY-04** When watching for readiness, the system shall use both filesystem notifications and periodic fallback checks until the timeout expires.

**READY-05** When a ready file reports crashed status, the system shall remove the file and return a startup crash error.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
