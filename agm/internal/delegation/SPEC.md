# AGM Delegation Tracker Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/delegation` tracks outbound task delegations between AGM sessions
so sender sessions cannot archive cleanly while delegated work remains
unresolved.

## Requirements

**AGM-DELEGATION-01** When a tracker is created, the system shall create its delegation directory with owner-only directory permissions.

**AGM-DELEGATION-02** When the default delegation directory is requested, the system shall resolve it under `~/.agm/delegations`.

**AGM-DELEGATION-03** When a delegation is recorded, the system shall set status to `pending`, set a UTC creation timestamp, and append a JSONL entry to the sender session file.

**AGM-DELEGATION-04** When delegation records are written, the system shall use owner-only file permissions.

**AGM-DELEGATION-05** When a delegation is resolved, the system shall update the matching message ID with the requested terminal status and a UTC resolution timestamp.

**AGM-DELEGATION-06** When a delegation message ID cannot be found for a session, the system shall return an error naming the missing message and session.

**AGM-DELEGATION-07** When pending delegations are requested for a missing session file, the system shall return no pending delegations and no error.

**AGM-DELEGATION-08** When delegation records are read, the system shall skip malformed JSONL entries rather than failing the entire session scan.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
