# A2A Model Card Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/modelcard` describes agent identity, capabilities, tools,
roles, harness metadata, model metadata, and runtime status for A2A discovery.

## EARS Requirements

**A2A-CARD-01** When a model card is created, the system shall initialize required identity fields, active status, timestamps, text input and output modes, and empty capability, tool, and tag collections.

**A2A-CARD-02** When a model card status changes, the system shall update both status and `UpdatedAt`.

**A2A-CARD-03** When a model card is registered, the system shall require agent ID, name, and role before adding it to the registry.

**A2A-CARD-04** When registry queries run, the system shall support lookup by agent ID, role, capability, all agents, and active-or-busy agents.

**A2A-CARD-05** When an unknown agent status is updated, the system shall return an error instead of creating an implicit card.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
- Package tests: `agm/internal/a2a/modelcard/modelcard_test.go`
