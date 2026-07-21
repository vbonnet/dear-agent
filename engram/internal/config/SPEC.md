# Engram Configuration Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/internal/config` loads Engram's core, company, team, and user
configuration tiers with security-sensitive precedence and cache invalidation.

## EARS Requirements

**ECF-01** When configuration is loaded, the system shall apply built-in runtime defaults before merging core, company, team, and user tiers in that order.

**ECF-02** When the core configuration is missing or any present tier contains invalid YAML, the system shall return an actionable load error.

**ECF-03** When optional company, team, or user configuration files are absent, the system shall continue with the remaining tiers.

**ECF-04** When a later tier supplies a non-empty scalar or configured section, the system shall override or merge the corresponding earlier-tier value according to that field's merge contract.

**ECF-05** When plugin disable lists are merged, the system shall append later-tier entries instead of discarding earlier restrictions.

**ECF-06** When core or company configuration enforces telemetry, the system shall keep telemetry enabled despite team or user attempts to disable or enforce it.

**ECF-07** When paths begin with a home-directory marker, the system shall expand them for Engram storage, telemetry, VCS, and plugin search paths.

**ECF-08** When loaded configuration files remain unchanged, the system shall return the cached configuration safely under concurrent access.

**ECF-09** When a loaded tier's modification time changes or an optional tier appears or disappears, the system shall invalidate the cache and reload configuration.

**ECF-10** When harness-effort settings are supplied by multiple tiers, the system shall merge aliases, task types, tiers, and subagent preference without discarding unrelated entries.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_analysis_configuration_guardrails.feature`
- Package tests: `engram/internal/config/*_test.go`
