# VROOM Gate Specification

<!-- Last audited at: 2026-08-18 -->

**Version:** 1.0
**Status:** Active
**Scope:** `internal/vroomgate`.

## Overview

`internal/vroomgate` holds the single canonical list of human-gated beads: work
that must never be handed to an autonomous VROOM worker. The list was previously
duplicated per binary, and the copies drifted, so a bead gated in the dispatcher
was still materialised as a prompt file by the generator. A gate that only part
of the pipeline honours is not a gate. Every command on the VROOM dispatch path
consults this package instead of a command-local map.

Its governing requirement is VDD-30 in `cmd/vroom-dispatch-direct/SPEC.md`; this
document records the contract the shared package itself owes its callers.

## EARS Requirements

**VROOMGATE-01** When a bead identifier is checked, the system shall report it as gated if and only if it appears in the canonical list.

**VROOMGATE-02** When the gated identifiers are requested, the system shall return every entry in the canonical list in a deterministic order.

**VROOMGATE-03** When a caller enumerates gated identifiers, the system shall return a copy so a caller cannot mutate the canonical list.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/vroomgate`

## Non-Goals

- Deciding why a bead is gated; the reason is recorded beside each entry.
- Gating at execution time; consumers filter candidates before dispatch.
