# Isolated SPEC governance Git test helper specification

## EARS Requirements

**SPEC-GOV-GITTEST-01** When an isolated SPEC governance test creates or mutates a Git fixture, the system shall disable inherited Git configuration, prompts, and hooks.

**SPEC-GOV-GITTEST-02** When a Git fixture needs an identity, the system shall use test-only author and committer metadata.

## BDD Traceability

- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
