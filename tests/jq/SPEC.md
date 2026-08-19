# jq Policy Fixture Gate Specification

<!-- Last audited at: 2026-08-19 -->

## Overview

`tests/jq` replays every checked-in jq program against recorded inputs. The jq
programs under `.github/` encode policy decisions consumed by workflows and by
`infra/import.sh`, where a silent behavior change surfaces as a mis-audited
ruleset rather than as a test failure. This gate is what makes such a change
visible.

## EARS Requirements

**JQF-01** When a fixture case declares expected JSON output, the gate shall compare jq's output as JSON, so a difference in key order is not reported as a change.

**JQF-02** When a fixture case declares an expected error substring, the gate shall require jq to exit non-zero and its diagnostic to contain that substring.

**JQF-03** When a checked-in `.jq` file has no fixture case, the gate shall fail, so a new program cannot arrive untested.

**JQF-04** When a fixture case names a program that does not exist, the gate shall fail rather than exempt a real program from the coverage requirement.

**JQF-05** When a fixture case omits its description, the gate shall fail, so no fixture exists that nobody can explain.

**JQF-06** When no fixture case is discovered at all, the gate shall fail rather than pass vacuously.

**JQF-07** Where a jq source is a library of definitions with no final expression, the gate shall exercise it through an inline filter that includes it, and shall still count it as covered.

**JQF-08** When a jq program does not terminate, the gate shall abort that case on a deadline rather than block the suite.

**JQF-09** When jq is not installed, the gate shall skip rather than fail, and continuous integration shall assert jq is present so the skip cannot hide a regression.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
- Workflow: `.github/workflows/jq-lint.yml`
- Runner: `tests/jq/runner_test.go`
