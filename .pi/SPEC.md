# Pi Configuration Surface Specification

<!-- Last audited at: 2026-07-21 -->

## Overview

The `.pi` directory is the repo-local configuration surface for Pi. Pi reads
the root `AGENTS.md` directly; this directory adds native discovery for the
AGM, Wayfinder, and research-pipeline skills without copying their living
source files.

Project-local Pi resources are executable trust boundaries. AGM launches Pi
with explicit project approval only after selecting the caller-provided working
directory. The managed AGM authorization extension remains outside this
directory so a project resource cannot replace the control-plane policy path.

## Requirements

**PI-DIR-01** When Pi loads repository resources, the system shall use the root `AGENTS.md` as the shared instruction source rather than publishing a divergent Pi-only copy.

**PI-DIR-02** When Pi loads skills, the system shall discover the living AGM, Wayfinder, and research-pipeline skill trees through `.pi/settings.json`.

**PI-DIR-03** When AGM launches Pi, the system shall keep its mandatory authorization extension in AGM-owned private storage and pass it with an explicit `--extension` argument.

**PI-DIR-04** When AGM approves Pi project resources, the system shall treat project approval separately from tool authorization, runtime isolation, and per-call permission decisions.

**PI-DIR-05** When configuration-directory parity is validated, the system shall map the active `pi-cli` harness to `.pi`.

**PI-DIR-06** When Pi emits native lifecycle or tool events, the managed AGM extension shall apply the declarative `.pi/hooks.json` projection.

## BDD Traceability

- `agm/test/bdd/features/config_directory_parity.feature` validates Pi's active configuration-directory mapping.
- `agm/test/bdd/features/wayfinder_parity.feature` and `agm/internal/wayfinderparity/coverage_test.go` validate Pi's configured Wayfinder and research-pipeline skill discovery surfaces.
- `agm/test/bdd/features/permission_parity.feature` validates the separate managed authorization surface.
