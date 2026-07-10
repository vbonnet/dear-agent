# Engram Instruction Detector Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/detectors` registers instruction-violation detectors and
normalizes their findings into harness-neutral telemetry events.

## EARS Requirements

**EDT-01** When a detector is registered under a new name, the system shall make it available by name and supported instruction type.

**EDT-02** When a detector name is registered more than once, the system shall reject the duplicate without replacing the existing detector.

**EDT-03** When all detectors run, the system shall aggregate every successful detector's violations.

**EDT-04** When one or more detectors fail, the system shall continue running other detectors and return their violations together with an aggregate detector error.

**EDT-05** When the global registry is first requested, the system shall initialize it once and register the default Bash command-pattern detector.

**EDT-06** When Bash content matches a configured anti-pattern, the system shall emit a unique, timestamped, high-confidence violation unless the corresponding instruction rule overrides confidence.

**EDT-07** When detector metadata contains agent, project, or phase context, the system shall preserve those fields in the violation and use an unknown-agent fallback when agent metadata is absent.

**EDT-08** When violation context exceeds the maximum retained length, the system shall truncate it before telemetry emission.

**EDT-09** When Bash content matches no configured anti-pattern, the system shall return no violation.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_analysis_configuration_guardrails.feature`
- Package tests: `engram/internal/detectors/*_test.go`
