# SPEC governance skill projection generator specification

## EARS Requirements

**SPEC-PROJECTION-01** When the projection generator runs in write mode, the system shall derive every AGENTS-compatible skill projection from the canonical skill frontmatter.

**SPEC-PROJECTION-02** When the projection generator writes a projection, the system shall include a generated marker, canonical name and description, an actionable canonical workflow handoff, and canonical verification instructions.

**SPEC-PROJECTION-03** When the projection generator runs, the system shall discover every immediate canonical skill directory and shall not rely on a second hard-coded skill inventory.

**SPEC-PROJECTION-04** When the projection generator runs with `--check`, the system shall fail if any required projection is missing, differs from deterministic output, or remains after its canonical generated skill is removed.

**SPEC-PROJECTION-05** When canonical skill frontmatter is malformed, duplicated, unterminated, or omits its name or description, the system shall fail before writing any projection.

**SPEC-PROJECTION-06** When an existing projection target is a symlink, a regular file without the generated marker, or resolves through a parent outside the requested repository root, the system shall refuse to overwrite it.

**SPEC-PROJECTION-07** When a projection changes, the system shall write it through an atomic same-directory replacement and shall reject a target that changed after validation.

**SPEC-PROJECTION-08** When a canonical skill is removed, the system shall remove an obsolete projection only when its complete content equals deterministic generated output and shall retain marker-bearing or unrelated authored skill files.

**SPEC-PROJECTION-09** When repository skill lint runs, the system shall check every committed SPEC governance projection for canonical drift.

**SPEC-PROJECTION-10** When the projection generator runs in write mode, the system shall reject explicit root selection and shall require the current repository root and exact administrative directory to be registered as a linked Git worktree.

**SPEC-PROJECTION-11** When repository skill validation runs, the system shall strictly parse the SPEC governance plugin manifest and shall require its canonical identity, metadata, author, and exact `./skills/` export before accepting projections.

**SPEC-PROJECTION-12** When repository skill validation runs, the system shall strictly parse each canonical `agents/openai.yaml` interface and shall require a display name, short description, and default prompt that delegates through that exact canonical skill name.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
