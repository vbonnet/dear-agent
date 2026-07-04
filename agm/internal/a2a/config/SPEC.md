# A2A Configuration Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/config` loads Engram, A2A, Wayfinder, workspace, and AGM
configuration from `~/.engram/config.yaml`, applies defaults, expands paths,
and exposes accessors used by A2A channel and coordination packages.

## EARS Requirements

**A2A-CFG-01** When no configuration file exists, the system shall return the default configuration without error.

**A2A-CFG-02** When a configuration file exists, the system shall merge non-empty user fields over defaults while preserving defaults for omitted fields.

**A2A-CFG-03** When configuration loading fails due to read or parse errors, the system shall return defaults together with a descriptive error.

**A2A-CFG-04** When configuration is loaded repeatedly without force reload, the system shall return the cached configuration.

**A2A-CFG-05** When callers request configured paths, the system shall expand home-directory and environment-variable syntax before returning the path.

**A2A-CFG-06** When configuration is validated, the system shall require version `1.0` and non-empty critical A2A and workspace paths.

## BDD Traceability

- Feature: `agm/test/bdd/features/config_directory_parity.feature`
- Feature: `agm/test/bdd/features/wayfinder_parity.feature`
- Package tests: `agm/internal/a2a/config/config_test.go`
