# Engram Agent Profile Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/profile` stores immutable agent identity and mutable,
session-scoped discoveries used for durable handoff.

## EARS Requirements

**EPR-01** When a static profile is saved for the first time, the system shall validate it and preserve an explicitly supplied spawn timestamp.

**EPR-02** When a static profile already exists, the system shall reject replacement so agent identity remains immutable.

**EPR-03** When a dynamic profile is saved, the system shall validate it and atomically replace the session-scoped profile.

**EPR-04** When a dynamic profile is updated, the system shall load existing state or create empty session state before applying and persisting the mutation.

**EPR-05** When a discovery or task focus is recorded, the system shall update only the targeted dynamic session profile.

**EPR-06** When a complete profile handoff is loaded, the system shall require the static profile and tolerate a missing dynamic profile as empty session state.

**EPR-07** When agent or session identifiers contain unsafe path components, the system shall reject them before filesystem access.

**EPR-08** When sessions are listed, the system shall return only valid dynamic profile session identifiers in stable order.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_core_context_guardrails.feature`
- Package tests: `engram/internal/profile/*_test.go`
