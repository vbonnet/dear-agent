# Engram Retrieval Tracking Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/tracking` batches retrieval metadata updates and atomically
persists access counts to Engram frontmatter.

## EARS Requirements

**ERT-01** When an Engram is accessed, the system shall increment its pending session count and retain the most recent access timestamp safely under concurrent calls.

**ERT-02** When pending access records are flushed, the system shall attempt every Engram update even if one update fails.

**ERT-03** When an access update succeeds, the system shall remove it from the pending log; when it fails, the system shall retain it for retry.

**ERT-04** When metadata is updated, the system shall add the pending count to existing retrieval count and set the latest access time.

**ERT-05** When legacy metadata lacks creation time or encoding strength, the system shall derive a creation fallback and assign neutral encoding strength.

**ERT-06** When updated frontmatter is persisted, the system shall preserve Engram content and file permissions and use temporary-file replacement.

**ERT-07** When serialization or replacement fails, the system shall return an error and clean up temporary output where possible.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_observability_guardrails.feature`
- Package tests: `engram/internal/tracking/*_test.go`
