# A2A Artifact Manager Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/artifacts` stores channel-scoped artifacts outside the message
body and emits compact memory pointers that agents can include in A2A channels.

## EARS Requirements

**A2A-ART-01** When an artifact manager is created without an explicit base directory, the system shall use the default A2A artifact directory and expand workspace-relative paths.

**A2A-ART-02** When storing an artifact, the system shall reject missing source files before creating channel index entries.

**A2A-ART-03** When storing an artifact succeeds, the system shall copy the artifact into the channel directory and append a timestamped entry to the global artifact index.

**A2A-ART-04** When listing artifacts for a channel, the system shall omit `INDEX.md` and return artifact names in deterministic sorted order.

**A2A-ART-05** When generating an artifact pointer, the system shall include the artifact path and optional summary and key points without embedding artifact contents.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/artifacts/manager_test.go`
