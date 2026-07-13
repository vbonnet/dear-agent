# Gopls Watchdog Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`cmd/gopls-watchdog` detects orphaned language-server pressure and delegates
safe remediation to AGM's canonical orphan reaper.

## EARS Requirements

**GWD-01** When the watchdog samples pressure, the command shall evaluate orphan count, orphan RSS, and system file-descriptor fraction against configured thresholds.

**GWD-02** When only live gopls processes are numerous, the command shall not treat them as orphan-count or orphan-RSS leaks.

**GWD-03** When a threshold is breached, the command shall return the alarm exit code even if remediation or trail persistence fails.

**GWD-04** When remediation is enabled, the command shall delegate to `agm session reap-orphans --targets gopls --json` rather than signaling processes itself.

**GWD-05** When dry-run mode is selected, the command shall detect and log alarms without invoking the orphan reaper.

**GWD-06** When an alarm is recorded, the command shall include sample values, thresholds, breached metrics, messages, dry-run state, and optional remediation results.

**GWD-07** When JSON output is selected, the command shall emit sample, threshold, alarm, remediation, and overall status fields.

**GWD-08** When sampling or argument parsing fails, the command shall return the runtime-or-usage exit code distinct from an alarm.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_safety_command_guardrails.feature`
- Package tests: `cmd/gopls-watchdog/*_test.go`
