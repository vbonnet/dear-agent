# VROOM Human Gate Specification

<!-- Last audited at: 2026-08-18 -->

## Overview

`internal/vroomgate` is the single canonical list of human-gated beads — work a
human must drive and an autonomous VROOM worker must never pick up. Every stage
of the dispatch pipeline (`vroom-dispatch-direct`, `vroom-prompt-gen`) consults
this package so a gate cannot be honoured by one entry point and ignored by
another.

## EARS Requirements

**VHG-01** When a bead id appears in the gate list, the system shall report it as human-gated.

**VHG-02** When a bead id does not appear in the gate list, the system shall report it as not human-gated, leaving the remaining eligibility filters to decide.

**VHG-03** When the gate list is enumerated, the system shall return every gated id in sorted order.

**VHG-04** While a VROOM dispatch-path binary filters candidate beads, the system shall consult this package rather than a binary-local copy of the list.

## BDD Traceability

- Test consequence: Deterministic unit tests own this contract end to end. `internal/vroomgate/gate_test.go` proves VHG-01 through VHG-03 against the list itself, and the consumer tests `TestHumanGatedBeadsAreNeverCandidates` (`cmd/vroom-dispatch-direct/main_test.go`) and `TestSelectCandidatesHonoursSharedHumanGate` (`cmd/vroom-prompt-gen/main_test.go`) prove VHG-04 by enumerating `vroomgate.IDs()` and asserting no gated bead survives either binary's candidate selection. No new BDD feature is required: this is a private in-process seam with no observable session behaviour for the BDD harness to drive, and both consuming commands already carry their own BDD linkage.
