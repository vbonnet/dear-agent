# Merge Velocity Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/merge-velocity` collects and emits merge-pipeline throughput metrics for a
selected GitHub repository.

## EARS Requirements

**MVC-01** When no repository is supplied, the command shall reject collection before initializing telemetry.

**MVC-02** When telemetry is initialized, the command shall honor the explicit endpoint or the standard OTLP environment endpoint.

**MVC-03** When merge velocity is collected, the command shall bound the GitHub operation with a 30-second context.

**MVC-04** When collection succeeds, the command shall record merges per day, open pull-request count, median merge hours, and created-versus-merged delta.

**MVC-05** When collection or telemetry initialization fails, the command shall return a contextual error without printing fabricated metrics.

**MVC-06** When the command exits, the system shall attempt a bounded telemetry flush and report flush failures.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_maintenance_command_guardrails.feature`
- Package tests: build, vet, and BDD package guardrails
