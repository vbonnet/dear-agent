# AGM Artifacts Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/artifacts` defines the persistence contract for files and
documents produced by AGM sessions. The package intentionally exposes an
interface so backing stores can vary while preserving artifact identity,
session ownership, metadata, and lifecycle operations.

## Requirements

**AGM-ARTIFACTS-01** When an artifact is stored, the system shall preserve its ID, session ID, type, path, size, metadata, and creation timestamp.

**AGM-ARTIFACTS-02** When an artifact is requested by ID, the system shall return the matching artifact metadata or the backing store error.

**AGM-ARTIFACTS-03** When artifacts are listed for a session, the system shall return only artifacts associated with that session ID.

**AGM-ARTIFACTS-04** When an artifact is deleted by ID, the system shall remove that artifact from future retrieval and list results.

**AGM-ARTIFACTS-05** When artifact metadata is serialized, the system shall use the documented JSON field names for cross-process compatibility.

## BDD Traceability

- `agm/test/bdd/features/agm_runtime_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
