# Wayfinder Session Status Specification

<!-- Last audited at: 2026-07-08 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `wayfinder/cmd/wayfinder-session/internal/status`.

## Overview

`internal/status` owns Wayfinder session state files, including the canonical
V2 nine-waypoint schema and compatibility helpers for older phase terminology.
It validates `WAYFINDER-STATUS.md`, navigates waypoints, persists roadmap and
task metadata, and protects the Wayfinder workflow from invalid or legacy
state shapes.

## EARS Requirements

**WAYFINDER-STATUS-01** When a V2 status is validated, the system shall require schema version, project name, project type, risk level, current waypoint, status, created timestamp, and updated timestamp.

**WAYFINDER-STATUS-02** When a V2 status declares schema_version, the system shall require the value to be `2.0`.

**WAYFINDER-STATUS-03** When a V2 status declares project type, risk level, current waypoint, or status, the system shall reject values outside the V2 enumerations.

**WAYFINDER-STATUS-04** When V2 waypoints are enumerated, the system shall return the nine-waypoint sequence CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO.

**WAYFINDER-STATUS-05** When waypoint history contains a completed waypoint, the system shall require a completed timestamp.

**WAYFINDER-STATUS-06** When waypoint history contains legacy merged waypoints S4, S5, S9, or S10, the system shall reject the status as invalid V2 state.

**WAYFINDER-STATUS-07** When SPEC waypoint history is validated, the system shall require stakeholder approval metadata.

**WAYFINDER-STATUS-08** When PLAN waypoint history is validated, the system shall require tests-feature-created metadata.

**WAYFINDER-STATUS-09** When BUILD waypoint history is validated, the system shall require validation and deployment status metadata and reject values outside their enumerations.

**WAYFINDER-STATUS-10** When a V2 status has an empty current waypoint, the system shall return CHARTER as the next waypoint.

**WAYFINDER-STATUS-11** When the current waypoint is incomplete, the system shall return the current waypoint rather than advancing.

**WAYFINDER-STATUS-12** When the current waypoint is complete, the system shall advance to the next non-skipped waypoint and report an error after RETRO.

**WAYFINDER-STATUS-13** When roadmap task dependencies are validated, the system shall reject duplicate task IDs, missing dependency targets, and dependency cycles.

**WAYFINDER-STATUS-14** When a V2 status is completed or blocked, the system shall require completion_date or blocked_reason respectively.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_status_guardrails.feature`

## Test Traceability

- Unit package: `wayfinder/cmd/wayfinder-session/internal/status`
