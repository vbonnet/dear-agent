# SPEC: tools/specaudit/SPEC.md
# RELATED-SPEC: tools/skill-projections/SPEC.md
Feature: SPEC governance tooling evidence boundary
  The focused checks exercise pinned read-only audit outcomes plus pre-Git
  retained-root identity, resource-bounded deterministic delegate projection,
  and checked elapsed-time fail-closed behavior. They do not execute skill
  discovery, invocation, or maintainer decisions.

  Scenario: Pinned inventory ignores dirty worktree content
    When AGM runs the focused pinned SPEC inventory unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Deterministic review leads are not semantic verdicts
    When AGM runs the focused non-verdict SPEC audit lead unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Reciprocal SPEC and BDD drift is diagnosed
    When AGM runs the focused reciprocal SPEC and BDD diagnostic unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Positive findings require shared evidence and a pending migration plan
    When AGM runs the focused pinned finding validation unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Immutable inventory and reviewer ledger stay separate
    When AGM runs the focused v2 inventory and ledger unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Input authentication has an explicit platform boundary
    When AGM runs the focused SPEC audit platform applicability unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Offline audit rendering is escaped and bounded
    When AGM runs the focused bounded offline rendering unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Successful audit commands preserve the target repository
    When AGM runs the focused read-only audit boundary unit check
    Then the focused SPEC governance unit check should pass

  Scenario: Skill delegates are checked exactly and created without replacement
    When AGM runs the focused skill projection safety unit check
    Then the focused SPEC governance unit check should pass
