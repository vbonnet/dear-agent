# Repo Health Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/repo-health` audits repository health across code quality, architecture,
agent hygiene, and configuration drift. It emits Markdown or JSON reports and
uses a status-derived exit code for CI or scheduled monitoring.

## Requirements

**REPO-HEALTH-01** When no root is provided, the system shall resolve the repository root from git.

**REPO-HEALTH-02** When JSON output is requested, the system shall print the full report as indented JSON.

**REPO-HEALTH-03** When Markdown output is requested, the system shall render a human-readable health summary.

**REPO-HEALTH-04** When output file paths are provided, the system shall write JSON and Markdown artifacts to those paths.

**REPO-HEALTH-05** When coverage is requested, the system shall run coverage collection as part of code-quality metrics.

**REPO-HEALTH-06** When metrics cannot be measured because tools or commands are unavailable, the system shall record informational unavailable notes rather than failing the verdict.

**REPO-HEALTH-07** When the report status is healthy, degraded, or critical, the system shall map status to exit codes 0, 1, and 2 respectively.

**REPO-HEALTH-08** When `--exit-zero` is provided, the system shall exit 0 regardless of the report verdict.

## BDD Traceability

- `agm/test/bdd/features/quality_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
