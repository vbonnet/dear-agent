# VROOM Worker Route Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`internal/vroomprompt` renders the harness-neutral worker route embedded in
generated VROOM prompts.

## EARS Requirements

**VWR-01** When the compatibility default is requested, the system shall return Claude Code, the historical Opus identifier, auto permission mode, and the OSS workspace.

**VWR-02** When a route is rendered, the system shall preserve the selected harness identifier.

**VWR-03** When a route is rendered, the system shall preserve the selected model or model-family alias.

**VWR-04** When a route is rendered, the system shall preserve the AGM permission mode and workspace.

**VWR-05** When active harness or supported model-family routes are selected, the system shall render the same worker-rule shape without a Claude-specific credential requirement.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_operations_command_guardrails.feature`
- Package tests: `internal/vroomprompt/*_test.go`
