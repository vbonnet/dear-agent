# AGM Reservation Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/reservation` stores advisory file and path reservations for
parallel AGM agents. Reservations are JSON-backed, time-limited, and scoped by
session so agents can avoid editing each other's files without requiring a
central service.

## Requirements

**AGM-RESERVATION-01** When the default store path is requested, the system shall use `~/.agm/reservations.json` when the home directory is available.

**AGM-RESERVATION-02** When reservations are created, the system shall record session ID, pattern, creation time, and expiration time for each pattern.

**AGM-RESERVATION-03** When any store operation loads reservations, the system shall treat a missing reservations file as an empty store.

**AGM-RESERVATION-04** When reservations are reserved, checked, listed, released, or cleaned up, the system shall filter expired reservations before returning active state.

**AGM-RESERVATION-05** When a path is checked, the system shall ignore reservations held by the current session.

**AGM-RESERVATION-06** When a path matches another session's exact pattern or valid glob pattern, the system shall report the reservation owner, matched pattern, and expiration time.

**AGM-RESERVATION-07** When a reservation pattern is an invalid glob, the system shall not treat it as a match unless the exact path matched earlier.

**AGM-RESERVATION-08** When reservations are released for a session, the system shall remove all reservations held by that session and return the count removed.

**AGM-RESERVATION-09** When reservations are saved, the system shall create the parent directory with owner-only permissions and atomically replace the JSON store using a temporary file.

**AGM-RESERVATION-10** When cleanup removes no expired reservations, the system shall avoid rewriting the store file.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
