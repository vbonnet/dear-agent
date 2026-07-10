# Workflow Bus Bridge Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/workflowbus` bridges workflow gate events from `agm-bus` to a running
workflow signaler.

## Requirements

**WFB-01** When a bus message names a supported workflow gate event, the system shall extract the gate name and signal the configured workflow session.

**WFB-02** When a message is unrelated to a workflow gate, the system shall ignore it without signaling the workflow.

**WFB-03** When the broker connection drops, the system shall reconnect and resume consumption without registering the session twice.

**WFB-04** When bridge configuration lacks a session ID or signaler, the system shall reject startup.

**WFB-05** When multiple supported gate signals arrive, the system shall preserve their delivery order.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/workflowbus/*_test.go`
