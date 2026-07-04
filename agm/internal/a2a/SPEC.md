# A2A Agent Card Registry Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a` bridges AGM session manifests to A2A Agent Cards. It creates
discoverable card JSON from harness, role, tag, purpose, and lifecycle metadata
and keeps the on-disk card registry aligned with active AGM manifests.

## EARS Requirements

**A2A-REG-01** When an AGM manifest is converted to an A2A Agent Card, the system shall preserve the session name as the card name and use the manifest purpose as the preferred description.

**A2A-REG-02** When a manifest has no purpose, the system shall produce a harness-based fallback description instead of leaving the card description empty.

**A2A-REG-03** When a manifest includes harness, role tags, context tags, or recognized session-name patterns, the system shall expose those values as A2A skills.

**A2A-REG-04** When a manifest has no inferred skills, the system shall include a general-purpose skill so the card remains discoverable.

**A2A-REG-05** When the registry writes an Agent Card, the system shall create a `0600` JSON file named after the session in the configured cards directory.

**A2A-REG-06** When registry synchronization sees archived or orphaned sessions, the system shall remove their card files while preserving active session cards.

**A2A-REG-07** When registry listing encounters unreadable or malformed card files, the system shall skip those files without failing the whole listing.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Feature: `agm/test/bdd/features/mcp_parity.feature`
- Package tests: `agm/internal/a2a/cards_test.go`
- Package tests: `agm/internal/a2a/registry_test.go`
