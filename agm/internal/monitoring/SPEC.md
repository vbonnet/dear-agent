# AGM Monitoring Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/monitoring` writes and reads loop heartbeat files so AGM
supervisors can diagnose stale or stuck loops across process boundaries. It
supports base heartbeat fields and optional diagnostic state for richer
recovery decisions.

## Requirements

**AGM-MONITORING-01** When a heartbeat writer is created without an explicit directory, the system shall use `~/.agm/heartbeats` and create it with restricted directory permissions.

**AGM-MONITORING-02** When a heartbeat is written, the system shall store timestamp, session name, interval seconds, cycle number, and success status in `loop-<session>.json`.

**AGM-MONITORING-03** When diagnostic heartbeat data is supplied, the system shall include last tick error, last tick duration, and state.

**AGM-MONITORING-04** When heartbeat data is read, the system shall parse the JSON heartbeat file for the requested session or return the read or parse error.

**AGM-MONITORING-05** When heartbeats are listed, the system shall read `loop-*.json` files and skip unreadable or malformed heartbeat files.

**AGM-MONITORING-06** When heartbeat staleness is checked with interval-based logic, the system shall report stale after interval plus 60 seconds, warn after 80 percent of that threshold, and ok otherwise.

**AGM-MONITORING-07** When heartbeat staleness is checked with an explicit max age, the system shall report stale after max age, warn after 80 percent of max age, and ok otherwise.

**AGM-MONITORING-08** When no heartbeat is available, the system shall report stale.

**AGM-MONITORING-09** When heartbeat removal is requested, the system shall remove `loop-<session>.json` from the configured or default heartbeat directory.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
