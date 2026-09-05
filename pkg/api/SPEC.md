# API Server Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/api` implements dear-agent's JSON HTTP control surface for workflow,
HITL, audit, and run-triggering operations. It keeps transport identity,
request validation, database reads, audit filtering, and workflow spawning
explicit so the API can be served over loopback or Tailscale without changing
workflow internals.

## Requirements

**API-SERVER-01** When a server is constructed without a logger, the system shall use `slog.Default`.

**API-SERVER-02** When a server is constructed without a version, the system shall infer build metadata or fall back to `dev`.

**API-SERVER-03** When `GET /status` is handled, the system shall include database ping status, version, caller identity, and UTC timestamp in the JSON response.

**API-SERVER-04** When workflow list or log limits are missing, invalid, or non-positive, the system shall use the documented default limit.

**API-SERVER-05** When workflow list or log limits exceed the ceiling, the system shall clamp them to the ceiling.

**API-SERVER-06** When workflow status is requested for an unknown run, the system shall return HTTP 404 with code `run_not_found`.

**API-SERVER-07** When HITL approval or rejection is requested without caller identity, the system shall return HTTP 401.

**API-SERVER-08** When HITL decision recording reports not found, already resolved, or role mismatch, the system shall map those cases to HTTP 404, 409, and 403 respectively.

**API-SERVER-09** When audit findings are requested without an audit store, the system shall return HTTP 503 with code `audit_disabled`.

**API-SERVER-10** When audit severity or state filters are invalid, the system shall return HTTP 400.

**API-SERVER-11** When `POST /run` is handled without a configured runner, the system shall return HTTP 503 with code `runner_disabled`.

**API-SERVER-12** When `POST /run` has no workflow file, the system shall return HTTP 400 with code `missing_file`.

**API-SERVER-13** When `ExecRunner` receives no allowed workflow roots, the system shall reject every workflow file request.

**API-SERVER-14** When `ExecRunner` resolves a workflow file outside every allowed root, the system shall reject the workflow file.

**API-SERVER-15** When `ExecRunner` spawns `workflow-run`, the system shall set `DEAR_AGENT_API_TRIGGERED_BY` to the caller login name.

**API-SERVER-16** When `GET /workflows` receives a run-state filter that WFLOW-08 rejects, the system shall return HTTP 400 with code `invalid_state`.

## BDD Traceability

- Feature: `agm/test/bdd/features/api_gateway_package_guardrails.feature`
- Test consequence: Deterministic HTTP integration tests in `pkg/api/run_state_test.go` prove API-SERVER-16 by returning HTTP 400 with `invalid_state` for an unknown filter despite closed storage while preserving HTTP 200 for the empty filter; this direct transport-boundary proof needs no additional Gherkin scenario.
