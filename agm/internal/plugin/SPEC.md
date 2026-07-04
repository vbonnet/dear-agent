# AGM Plugin Registry Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/plugin` defines the task-manager plugin contract and the
in-process registry used to discover task sources such as Claude tasks, Beads,
or future issue trackers through one normalized task shape.

## EARS Requirements

**PLUG-REG-01** When a task-manager plugin is registered, the system shall use the plugin metadata name as the registry key.

**PLUG-REG-02** When a second plugin registers with an existing name, the system shall reject the duplicate registration.

**PLUG-REG-03** When a plugin is requested by name, the system shall return the registered plugin or nil when no plugin matches.

**PLUG-REG-04** When registry names are listed, the system shall return a snapshot of registered plugin names without exposing internal map state.

**PLUG-REG-05** When auto-detection runs for a session directory, the system shall return the first registered plugin that reports support for that session.

**PLUG-REG-06** When a plugin reports tasks, the system shall normalize task identity, status, phase, labels, metadata, and timestamps through the shared `Task` type.

## BDD Traceability

- Feature: `agm/test/bdd/features/marketplace_parity.feature`
- Package tests: `agm/internal/plugin/registry_test.go`
