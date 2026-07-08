# Gemini Compatibility Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.gemini` directory is the repo-local compatibility configuration surface
for Gemini Code Assist review behavior. Gemini remains deprecated for active
harness parity, but its compatibility configuration is still tracked explicitly.

## Requirements

**GEMINI-DIR-01** When Gemini compatibility configuration is present, the system shall keep it separate from the active harness configuration-directory parity set.

**GEMINI-DIR-02** When Gemini Code Assist review is configured, the system shall keep review settings in `.gemini/config.yaml`.

**GEMINI-DIR-03** When Gemini review filtering is configured, the system shall restrict ignored paths to generated, vendored, testdata, lockfile, or minified artifacts.

**GEMINI-DIR-04** When Gemini style guidance is configured, the system shall keep it under `.gemini/styleguide.md`.

**GEMINI-DIR-05** When deprecated configuration-directory parity is validated, the system shall map `gemini-cli` compatibility to `.gemini`.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates deprecated Gemini directory mapping.
