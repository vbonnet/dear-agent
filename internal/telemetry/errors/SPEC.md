# Telemetry Errors Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`internal/telemetry/errors` renders telemetry-related problems as tiered
messages for simple and technical users. Templates preserve actionable
recommendations while avoiding unnecessary technical detail for alpha users.

## Requirements

**TEL-ERR-01** When a telemetry error is rendered for a technical user, the system shall include category, technical message, context entries, recommendations, debug command, and learn-more URL when present.

**TEL-ERR-02** When a telemetry error is rendered for a simple user, the system shall include the simple message, simplified recommendations, and a help contact line.

**TEL-ERR-03** When user type is detected and `~/.engram/config.yml` exists, the system shall classify the user as technical.

**TEL-ERR-04** When user type is detected and CLI flags are present in context, the system shall classify the user as technical.

**TEL-ERR-05** When no technical-user signal is present, the system shall classify the user as simple.

**TEL-ERR-06** When plugin loading fails, the system shall include missing plugin names, health-check debugging guidance, and plugin troubleshooting documentation.

**TEL-ERR-07** When deprecated plugin versions are detected, the system shall include the plugin name, current version, latest version, update command, and changelog URL.

**TEL-ERR-08** When ecphory token utilization is high, the system shall include utilization percentage, token count, analysis command guidance, and tuning documentation.

**TEL-ERR-09** When telemetry storage nears its limit, the system shall include current size, maximum size, utilization percentage, storage cleanup guidance, and storage documentation.

**TEL-ERR-10** When telemetry is disabled, the system shall report that usage insights and health checks are unavailable and explain that local telemetry must be enabled.

**TEL-ERR-11** When JSONL parsing fails, the system shall include file path, line number, reason, a tail-based debug command, and telemetry troubleshooting documentation.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
