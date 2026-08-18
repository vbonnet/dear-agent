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

**ROUTE-04** The system shall not provide a baseline or grandfathered-exception interface.

**ROUTE-05** When one or more non-exempt forbidden paths are found, the system shall print the violating paths and exit with a policy-failure status.

**ROUTE-06** When a configured pattern contains `**`, the system shall match zero or more complete path segments while applying standard glob syntax within each segment.

**ROUTE-07** When an unsupported argument is supplied, the system shall return a usage error.

**ROUTE-08** When the Dear Agent tracked tree is tested, the system shall require zero temporal-artifact violations.

**ROUTE-09** If `.dear-agent.yml` is missing or declares no forbidden path patterns, the system shall fail closed with a policy error.

**ROUTE-10** When a temporal artifact name uses underscore or camelCase token boundaries, the system shall apply the same routing policy as the canonical hyphenated form.

**ROUTE-11** When a filename token pattern matches a supported source-code extension outside an explicitly temporal directory, the system shall preserve the file as living source code.
