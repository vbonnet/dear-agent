# AGM Plugin Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/agm-plugin/commands` contains the Claude plugin command surfaces and tests
that constrain their declared tool permissions and exit behavior.

## Requirements

**APC-01** When a plugin command declares allowed tools, the system shall use valid tool-spec syntax and include every tool required by that command.

**APC-02** When the AGM exit command runs, the system shall place its target argument before optional flags and avoid direct blocked tmux or shell-control commands.

**APC-03** When the AGM exit command records its completion marker, the system shall use the maintained write-capable tool surface.

**APC-04** When plugin command permissions change, the system shall fail executable command tests if required or forbidden tools drift.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/agm-plugin/commands/*_test.go`
