# Session Skill Extractor Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Provider-neutral extraction of reusable skills from session transcripts.

## EARS Requirements

**SKILL-EXTRACT-01** When extraction is requested, the system shall invoke the supplied `ModelCaller` contract rather than selecting a model provider in extraction logic.

**SKILL-EXTRACT-02** When a transcript contains no extractable interaction, the system shall return an empty result without creating a skill file.

**SKILL-EXTRACT-03** When existing skill directories are scanned, the system shall return deduplicated skill names.

**SKILL-EXTRACT-04** When a model response reports no new skill, the system shall return a no-op extraction result.

**SKILL-EXTRACT-05** When a valid skill response is accepted outside dry-run mode, the system shall write the generated skill to a sanitized filename.

**SKILL-EXTRACT-06** When dry-run mode is enabled, the system shall report the proposed extraction without writing a file.

## Test Traceability

- Package tests: `cmd/session-skill-extractor/extractor_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
