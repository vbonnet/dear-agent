# Source Health Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/src-health` is a host-side dispatch-loop canary. It checks configured
`~/src` repositories for existence, branch, cleanliness, and upstream
ahead-behind state.

## Requirements

**SRC-HEALTH-01** When no repository list is provided, the system shall check the default primary source repositories.

**SRC-HEALTH-02** When repository names are provided, the system shall resolve relative names under `~/src`.

**SRC-HEALTH-03** When a repository directory is missing, the system shall mark that repository unhealthy with a not-found error.

**SRC-HEALTH-04** When git branch detection fails, the system shall mark that repository unhealthy with the git error.

**SRC-HEALTH-05** When a repository has staged or unstaged changes, the system shall mark that repository unhealthy.

**SRC-HEALTH-06** When upstream status is checked, the system shall run Git subprocesses with bounded timeouts and noninteractive authentication, and report available ahead and behind commit counts.

**SRC-HEALTH-07** When JSON output is requested, the system shall emit aggregate health and per-repository status as indented JSON.

**SRC-HEALTH-08** When any repository is unhealthy in text mode, the system shall exit with code 1.

## BDD Traceability

- `agm/test/bdd/features/quality_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
