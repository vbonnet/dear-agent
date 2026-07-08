# Gateway HTTP Adapter Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/gateway/adapters/http` wraps `pkg/api.Server` so API-triggered workflow
runs are dispatched through `pkg/gateway`. HTTP routing, caller identity, and
request validation remain in `pkg/api`; gateway policy and command handling
remain centralized in `pkg/gateway`.

## Requirements

**GATEWAY-HTTP-01** When the HTTP adapter wraps a nil API server, the system shall panic.

**GATEWAY-HTTP-02** When the HTTP adapter wraps a nil gateway, the system shall panic.

**GATEWAY-HTTP-03** When the HTTP adapter wraps a server, the system shall replace the server runner with a gateway-backed runner.

**GATEWAY-HTTP-04** When the HTTP adapter reports its name, the system shall return `http`.

**GATEWAY-HTTP-05** When the HTTP adapter is run under a supervisor, the system shall block until context cancellation.

**GATEWAY-HTTP-06** When the HTTP adapter receives normal context cancellation, the system shall return nil.

**GATEWAY-HTTP-07** When the gateway-backed runner dispatches a run request, the system shall map API caller identity into gateway caller identity.

**GATEWAY-HTTP-08** When the gateway-backed runner receives a gateway error response, the system shall return that error to the API layer.

**GATEWAY-HTTP-09** When the gateway-backed runner receives a success response, the system shall map run ID, workflow, and PID fields back into the API run response.

## BDD Traceability

- `agm/test/bdd/features/api_gateway_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
