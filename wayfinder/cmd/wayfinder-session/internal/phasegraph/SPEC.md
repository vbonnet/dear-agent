# Wayfinder Phasegraph Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `wayfinder/cmd/wayfinder-session/internal/phasegraph`.

## Overview

`internal/phasegraph` parses the Wayfinder phase dependency configuration used
to load upstream artifacts for a phase and resolve V1 phase identifiers to V2
names. It keeps dependency lookup pure and defensive by copying returned maps
and rejecting unsupported load strategy values.

## EARS Requirements

**WAYFINDER-PHASEGRAPH-01** When a phase dependency configuration file is loaded, the system shall read the file and parse it as YAML.

**WAYFINDER-PHASEGRAPH-02** When YAML parsing fails, the system shall return an error that identifies phase dependency configuration parsing.

**WAYFINDER-PHASEGRAPH-03** When dependencies or V1-to-V2 mappings are omitted, the system shall initialize them as empty maps.

**WAYFINDER-PHASEGRAPH-04** When a dependency load strategy is not `full` or `summary`, the system shall reject the configuration.

**WAYFINDER-PHASEGRAPH-05** When dependencies are requested for a V1 phase name, the system shall resolve it through the V1-to-V2 mapping before lookup.

**WAYFINDER-PHASEGRAPH-06** When dependencies are returned, the system shall return a copy so callers cannot mutate stored configuration.

**WAYFINDER-PHASEGRAPH-07** When phase names are requested, the system shall return the V2 phase names defined in the dependency graph.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_status_guardrails.feature`

## Test Traceability

- Unit package: `wayfinder/cmd/wayfinder-session/internal/phasegraph`
