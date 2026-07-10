# Engram Identity Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/identity` detects a user identity from ordered trust sources
and caches the result in memory and on disk.

## EARS Requirements

**EID-01** When identity detection runs without a valid cache entry, the system shall try configured detectors from highest to lowest priority.

**EID-02** When GCP application-default credentials and an account email are available, the system shall return a verified GCP identity.

**EID-03** When identity is obtained from Git configuration or `ENGRAM_USER_EMAIL`, the system shall mark the identity as unverified.

**EID-04** When a detector cannot identify a user or reports a source-specific failure, the system shall continue to the next detector.

**EID-05** When no detector identifies a user, the system shall return an explicit no-identity error.

**EID-06** When a cached identity is unexpired and its source files are unchanged, the system shall reuse it without rerunning detectors.

**EID-07** When a disk cache is expired, corrupted, or invalidated by a newer identity source, the system shall treat it as a cache miss.

**EID-08** When writing the disk cache, the system shall use a private cache directory, private file permissions, and atomic replacement.

**EID-09** When cache clearing is requested, the system shall clear both memory and disk identity caches.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/identity/*_test.go`
