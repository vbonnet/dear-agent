# Workflow Engine Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`
- Feature: `agm/test/bdd/features/workflow_package_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`pkg/workflow` executes declarative workflows with AI, shell, gate, state,
budget, permission, output, hook, HITL, and audit surfaces. The runner keeps the
engine backend-neutral while exposing enough structured state for durable
execution, resume, review, and cost controls.

## EARS Requirements

**WFLOW-01** When a runner is constructed, the system shall require an AI executor and initialize default logging, shell, and signal-channel behavior.

**WFLOW-02** When a workflow run starts, the system shall validate the plan, create a unique run identity, and begin durable state or audit recording when configured.

**WFLOW-03** When a workflow is resumed, the system shall load the saved snapshot and skip already-completed nodes while preserving their outputs for downstream templates.

**WFLOW-04** When a node declares permissions, budgets, outputs, hooks, or HITL policy, the system shall apply the configured enforcer or default implementation before treating the node as complete.

**WFLOW-05** When workflow execution emits audit events, the system shall write them to the audit sink and notify audit hooks without allowing hook failure to abort the run.

**WFLOW-06** When a workflow enables constitutional enforcement without declaring any invariants, the system shall reject the workflow before recording a run, invoking lifecycle hooks, or executing a node.

**WFLOW-07** When a configured definition hook rejects a validated workflow, the system shall finish the run as failed and return the contextual rejection before enforcing or executing any node.

## Permission contract

`DefaultPermissionEnforcer` is permissive when a policy section is absent. When
declared, filesystem read/write entries use path globs, network entries accept
an exact host or its subdomains (or `*`), and tool entries require an exact
canonical tool name. A rejected check returns `ErrPermissionDenied`.
