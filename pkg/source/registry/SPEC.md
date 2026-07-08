# Source Registry Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source/registry` maps stable backend names to source adapter factories.
It is the process-local extension point for built-in adapters and link-time
plugin adapters while keeping runtime selection independent of concrete
packages.

## Requirements

**SOURCE-REGISTRY-01** When the registry package initializes, the system shall register `sqlite`, `obsidian`, `llm-wiki`, and `openviking` backends.

**SOURCE-REGISTRY-02** When registered backend names are listed, the system shall return names sorted alphabetically.

**SOURCE-REGISTRY-03** When a backend name is registered with an empty name or nil factory, the system shall ignore the registration.

**SOURCE-REGISTRY-04** When a backend name is registered more than once, the system shall replace the previous factory with the latest factory.

**SOURCE-REGISTRY-05** When a caller opens an unknown backend, the system shall return an error that includes the unknown backend and the registered names.

**SOURCE-REGISTRY-06** When opening SQLite, Obsidian, or llm-wiki with empty config, the system shall reject the open request.

**SOURCE-REGISTRY-07** When opening OpenViking with JSON config, the system shall parse connection fields from JSON.

**SOURCE-REGISTRY-08** When the registry is reset in tests, the system shall remove all registered factories.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
