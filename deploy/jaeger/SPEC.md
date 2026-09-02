# Local Jaeger Collector Configuration Specification

<!-- Last audited at: 2026-09-01 -->

## Overview

`deploy/jaeger/config.yaml` configures the always-on local Jaeger v2 collector
that receives OTLP spans from dear-agent processes on this host. It is a
deployed artifact: `deploy/manifest.yaml` registers it, `dear-deploy sync`
stages it, and `dear-deploy status` reports drift against source.

`deploy/launchd/SPEC.md` governs how the launch agent is installed and kept
runnable. It says nothing about what the collector listens on or stores, so it
cannot own this file's observable behavior. These requirements do.

## EARS Requirements

**JAEGER-CFG-01** When the local collector accepts OTLP traffic, the collector configuration shall bind every receiver protocol to a loopback address so that no non-local peer can submit spans to an unauthenticated always-on listener.

**JAEGER-CFG-02** When the local collector exposes its query API and UI, the collector configuration shall bind those endpoints to a loopback address so that locally collected trace contents are not readable from the network the host is attached to.

**JAEGER-CFG-03** When the collector stores traces, the collector configuration shall declare a bounded in-memory backend with a maximum trace count so that an unbounded producer cannot exhaust host memory.

**JAEGER-CFG-04** When the collector configuration changes in source, the deployment gate shall report drift against the deployed copy rather than allowing the running collector to diverge silently.

**JAEGER-CFG-05** When `otel-local` starts a collector in the foreground, the command shall pass the managed configuration explicitly, and if that configuration is absent the command shall warn that the upstream defaults listen on every interface.

## BDD Traceability

- Package tests: `cmd/otel-local/config_contract_test.go`
- Manifest registration: `deploy/manifest.yaml` (`jaeger-collector-config`)

This directory owns its own `SPEC.md`, so it carries no `SPEC.owner` edge: an
owner file pointing back at a co-located specification is a self-edge and adds
no ownership information.
