# Sentinel Configuration Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/sentinel/config` defines and validates sentinel monitoring,
recovery, event-bus, temporal, sweeper, loop, and escalation configuration.

## Requirements

**SCF-01** When no configuration file is supplied, the system shall return complete maintained defaults for every sentinel subsystem.

**SCF-02** When a configuration file is loaded, the system shall merge supplied values with defaults so omitted safety settings remain defined.

**SCF-03** When configuration syntax or values are invalid, the system shall return an explicit validation error.

**SCF-04** When exempt sessions or monitored patterns are configured, the system shall preserve their deterministic matching order.

**SCF-05** When the default configuration path is requested, the system shall resolve the canonical sentinel location under the user configuration directory.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_supervision_recovery_guardrails.feature`
- Package tests: `agm/internal/sentinel/config/*_test.go`
