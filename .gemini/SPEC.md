# Gemini Compatibility Configuration Surface Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

The `.gemini` directory is the repo-local Gemini CLI compatibility surface.
Gemini Code Assist was retired on 2026-07-17. Active pull-request review is
configured solely by `.github/workflows/gemini-review.yml`; its review policy
must not be duplicated in this directory.

## Requirements

**GEMINI-DIR-01** When Gemini compatibility configuration is present, the system shall keep it separate from the active harness configuration-directory parity set.

**GEMINI-DIR-02** When Gemini pull-request review is configured, the system shall keep its review policy in `.github/workflows/gemini-review.yml` and shall not retain a Code Assist configuration file.

**GEMINI-DIR-03** When Gemini review filtering is configured, the system shall restrict ignored paths to generated, vendored, testdata, lockfile, or minified artifacts.

**GEMINI-DIR-04** When Gemini style guidance is configured, the system shall keep it under `.gemini/styleguide.md`.

**GEMINI-DIR-05** When deprecated configuration-directory parity is validated, the system shall map `gemini-cli` compatibility to `.gemini`.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/config_directory_parity.feature` validates deprecated Gemini directory mapping.
