# AGM API Status Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/api` owns lightweight status access for AGM state detection. It
serves in-memory session status over HTTP and persists status snapshots as
atomic JSON files for local consumers.

## Requirements

**AGM-API-01** When the API server starts, the system shall register `/status`, `/status/{session}`, and `/health` GET endpoints.

**AGM-API-02** When a non-GET request reaches a status or health endpoint, the system shall return method-not-allowed.

**AGM-API-03** When all session status is requested, the system shall return a JSON object with sessions, count, and timestamp fields.

**AGM-API-04** When an unknown session status is requested, the system shall return a not-found JSON response with unknown state, low confidence, and an explanatory error.

**AGM-API-05** When session status is updated in memory, the system shall preserve state, timestamp, evidence, confidence, and last-updated metadata.

**AGM-API-06** When a status file writer is created, the system shall create the status directory with owner-only directory permissions.

**AGM-API-07** When status is written to disk, the system shall write owner-only JSON through a temporary file and atomic rename.

**AGM-API-08** When status files are listed, the system shall return only non-directory `.json` status file names without the extension.

## BDD Traceability

- `agm/test/bdd/features/agm_control_surface_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
