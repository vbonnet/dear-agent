# Deployment Manifest Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-DEPLOY-01** When dear-agent deployment is requested, the system shall resolve artifacts and targets from the versioned deployment manifest.

**DECL-DEPLOY-02** If a required artifact or target is invalid, the system shall fail before reporting deployment success.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
