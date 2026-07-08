# AGM Agent Selection Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`agm/internal/agents` loads AGENTS.md harness-routing configuration and selects
the harness for a session name. It is a compatibility layer for older
keyword-based routing rules and must remain predictable while newer parity
registries validate active harness and model support.

## EARS Requirements

**AAS-01** When the default config is requested, the system shall use schema version `1.0`, default harness `claude-code`, and an empty preference list.

**AAS-02** When configuration is loaded, the system shall check local `./AGENTS.md` before global `~/.config/agm/AGENTS.md`.

**AAS-03** When no configuration file can be loaded, the system shall return the default config without error.

**AAS-04** When a configuration file has malformed YAML, the system shall warn and return the default config without checking lower-precedence paths.

**AAS-05** When a loaded config omits `default_harness`, the system shall warn and use `claude-code`.

**AAS-06** When a loaded preference has no keywords, the system shall warn and skip that preference.

**AAS-07** When a loaded preference has an empty harness, the system shall warn and skip that preference.

**AAS-08** When a loaded config omits schema version, the system shall default the schema version to `1.0`.

**AAS-09** When a session name is empty, the system shall select the configured default harness.

**AAS-10** When selecting a harness, the system shall compare session names and keywords case-insensitively.

**AAS-11** When selecting a harness, the system shall use substring matching rather than regular expressions.

**AAS-12** When multiple preferences match, the system shall return the harness from the first matching preference.

**AAS-13** When no preference matches, the system shall return the configured default harness.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_selection_guardrails.feature`
- Package tests: `agm/internal/agents/config_test.go`
- Package tests: `agm/internal/agents/selector_test.go`
- Package tests: `agm/internal/agents/selector_property_test.go`

