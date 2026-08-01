# SPEC: spec-governance/SPEC.md
# RELATED-SPEC: spec-governance/cmd/sync-skill-projections/SPEC.md
# RELATED-SPEC: spec-governance/earslint/SPEC.md
# RELATED-SPEC: spec-governance/internal/gittest/SPEC.md
# RELATED-SPEC: spec-governance/scripts/SPEC.md
# RELATED-SPEC: spec-governance/skillset/SPEC.md
# RELATED-SPEC: spec-governance/skills/audit-specs/agents/SPEC.md
# RELATED-SPEC: spec-governance/skills/audit-specs/scripts/specaudit/SPEC.md
# RELATED-SPEC: spec-governance/skills/write-spec/agents/SPEC.md
Feature: SPEC governance tooling evidence boundary
  SPEC governance tooling must prove its pinned, fail-closed behavior through
  executable tests rather than relying on co-located file presence alone.

  Scenario: Inventory stays pinned and deterministic
    When AGM exercises pinned SPEC inventory and strict extraction
    Then the SPEC governance behavioral contract should pass

  Scenario: Semantic findings require authenticated evidence
    When AGM exercises forged SPEC finding rejection
    Then the SPEC governance behavioral contract should pass

  Scenario: Offline audit rendering preserves decision data
    When AGM exercises complete offline SPEC audit rendering
    Then the SPEC governance behavioral contract should pass

  Scenario: Projection inventory cannot expand or silently drift
    When AGM exercises the fixed SPEC skill export boundary
    Then the SPEC governance behavioral contract should pass

  Scenario: Projection writes fail closed around authored state
    When AGM exercises fail-closed SPEC skill projection mutation
    Then the SPEC governance behavioral contract should pass

  Scenario: Native plugin and OpenAI skill metadata fail closed
    When AGM exercises strict SPEC governance package metadata
    Then the SPEC governance behavioral contract should pass

  Scenario: Installed audit tooling ignores the active working directory
    When AGM exercises the installed SPEC audit execution seam
    Then the SPEC governance behavioral contract should pass

  Scenario: Pinned Git evidence collection is bounded
    When AGM exercises bounded SPEC audit Git collection
    Then the SPEC governance behavioral contract should pass

  Scenario: Repository skill lint checks generated projections
    When AGM runs the repository SPEC skill drift gate
    Then the SPEC governance behavioral contract should pass
