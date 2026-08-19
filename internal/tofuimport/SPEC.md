# OpenTofu Import Planning Specification

<!-- Last audited at: 2026-08-19 -->

## Overview

`internal/tofuimport` decides how existing GitHub objects are bound to
OpenTofu state addresses. Importing is a sequence of irreversible,
partially-ordered state mutations, and a wrong binding does not fail loudly:
it points a state address at the wrong remote object, so the next plan
proposes changes to a ruleset nobody meant to touch. Every decision here
therefore fails closed on ambiguous evidence.

## EARS Requirements

**TIP-01** When the evaluated inventory carries a repository name that is not a valid GitHub identity segment, the package shall reject the whole inventory rather than import the remainder.

**TIP-02** When the inventory declares two repositories that differ only by letter case, or declares one repository as both active and archived, the package shall reject the inventory.

**TIP-03** When the inventory omits the repository whose checked-in ruleset is canonical, the package shall reject it.

**TIP-04** When a ruleset listing contains an entry without a positive numeric id or a non-empty name, the package shall reject the listing rather than treat it as proof that no ruleset exists.

**TIP-05** When more than one ruleset matches the name sought for a repository, the package shall refuse the import rather than select one.

**TIP-06** When the canonical repository's ruleset resolves to any provider id other than the recorded canonical id, the package shall refuse the import.

**TIP-07** When a repository provably has no ruleset, the package shall report that the plan will create one and shall propose no import.

**TIP-08** When a state address already exists but is bound to a different repository or provider id than the plan expects, the package shall report a stale binding rather than skip the address as already imported.

**TIP-09** When any repository's identity cannot be resolved, the package shall emit no plan steps at all, so no partial import can begin.

**TIP-10** When no ruleset listing was collected for an active repository, the package shall refuse to infer that the repository has no ruleset.

**TIP-11** When a failed import's provider output does not match a recognized absent-object message, the package shall classify it as a real failure.

**TIP-12** When a plan is rendered for execution, the package shall emit one tab-separated record per step so an address containing quotes and brackets survives intact.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Package tests: `internal/tofuimport/*_test.go`
- Script behavior: `tests/bats/infra-import.bats`
