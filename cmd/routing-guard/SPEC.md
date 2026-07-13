# Routing Guard Specification

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

<!-- Last audited at: 2026-07-03 -->

## Purpose

`cmd/routing-guard` enforces the repository routing policy for temporal
Wayfinder and DEAR artifacts. It reads forbidden path globs from
`.dear-agent.yml`, scans tracked, staged, diffed, or supplied file lists, and
fails when matching paths are introduced outside their knowledge-base home.

## EARS Requirements

**ROUTE-01** When routing-guard starts, the system shall load forbidden path globs from `.dear-agent.yml` instead of embedding a separate copy of the policy.

**ROUTE-02** When routing-guard runs in `--all` mode, the system shall evaluate every tracked repository file against the configured forbidden path globs.

**ROUTE-03** When routing-guard runs in `--staged`, `--diff`, or `--files` mode, the system shall evaluate only the requested file set and preserve repo-relative path matching.

**ROUTE-04** When a forbidden path is listed in the baseline file, the system shall exempt that path from the reported violation set.

**ROUTE-05** When one or more non-exempt forbidden paths are found, the system shall print the violating paths and exit with a policy-failure status.
