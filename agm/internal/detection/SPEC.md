# Conversation UUID Detection Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/detection` associates Claude conversation UUIDs with AGM
manifests while keeping confidence and persistence decisions explicit.

## Requirements

**DET-01** When a manifest already has a manually associated Claude UUID, the system shall preserve it as a high-confidence manual result.

**DET-02** When history contains a project match, the system shall classify confidence from the match age and configured detection window.

**DET-03** When no history match exists, the system shall return an explicit low-confidence `none` result instead of fabricating a UUID.

**DET-04** When auto-association is enabled with a high-confidence history result, the system shall persist the UUID through the configured Dolt storage adapter.

**DET-05** When confidence is below high or auto-association is disabled, the system shall not mutate the manifest.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
- Package tests: `agm/internal/detection/*_test.go`
