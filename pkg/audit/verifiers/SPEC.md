# Audit Verifiers Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/audit/verifiers` ships first-party verifier implementations used to prove
the audit verifier dispatch path. The current no-op verifier is intentionally
minimal, but it exercises the same interface that external verifier plugins use.

## Requirements

**AUDIT-VERIFIERS-01** When a no-op verifier has no explicit name, the system shall report its name as `noop`.

**AUDIT-VERIFIERS-02** When a no-op verifier has an explicit name, the system shall report that configured verifier name.

**AUDIT-VERIFIERS-03** When a no-op verifier describes itself, the system shall return a human-readable dispatch reference description.

**AUDIT-VERIFIERS-04** When a no-op verifier reports review depth, the system shall report casual review depth.

**AUDIT-VERIFIERS-05** When a no-op verifier verifies any target, the system shall return no findings and no error.

**AUDIT-VERIFIERS-06** When the audit runner dispatches a no-op verifier, the system shall still record a verifier outcome for the dispatch.

## BDD Traceability

- `agm/test/bdd/features/audit_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
