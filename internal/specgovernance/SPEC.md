# SPEC governance fixed skill-set integration specification

**Scope:** Root-module validation and projection tooling. The canonical skill
workflows remain under `spec-governance/skills`.

## EARS Requirements

**SPEC-GOV-SKILLSET-01** When SPEC governance discovery is validated, the system shall use one fixed canonical skill set containing only `audit-specs` and `write-spec`.

**SPEC-GOV-SKILLSET-02** If an additional canonical skill directory is present or declared by a native manifest, then the system shall reject the SPEC governance distribution even when its discovery surfaces agree.

**SPEC-GOV-SKILLSET-03** When a caller receives the canonical skill set, the system shall prevent that caller from mutating the owned set.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
- Feature: `agm/test/bdd/features/marketplace_parity.feature`
