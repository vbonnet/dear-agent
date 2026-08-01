# SPEC governance projection generator specification

## EARS Requirements

**SPEC-PROJECTION-01** When the projection generator runs in write mode, the system shall derive every AGENTS-compatible skill projection from the canonical skill frontmatter.

**SPEC-PROJECTION-02** When the projection generator writes a projection, the system shall include a generated marker, canonical name and description, an actionable canonical workflow handoff, an authenticated canonical-resource base, and canonical verification instructions.

**SPEC-PROJECTION-03** When the projection generator runs, the system shall require the immediate canonical skill directories to equal the single fixed `audit-specs` and `write-spec` skill set before planning any projection.

**SPEC-PROJECTION-04** When the projection generator runs with `--check`, the system shall fail if any required projection is missing, differs from deterministic output, or remains after its canonical generated skill is removed; for a stale or obsolete existing entry, the failure shall require human inspection of the path and bytes before explicit removal.

**SPEC-PROJECTION-05** When canonical skill frontmatter is malformed, duplicated, unterminated, or omits its name or description, the system shall fail before writing any projection.

**SPEC-PROJECTION-06** When an existing projection target is a symlink, non-regular, hard-linked, not mode `0644`, larger than 256 KiB, differs from complete deterministic generated output, or resolves through a parent outside the requested repository root, the system shall refuse to overwrite it; the public generated marker alone shall grant no replacement authority.

**SPEC-PROJECTION-07** When a projection target is missing in write mode, the system shall create the final path through descriptor-confined exclusive creation, write and sync through that bound descriptor, and verify that the final path is the same single-link mode-`0644` file with exact deterministic bytes. The system shall never replace an entry that appears before creation and shall never automatically remove a partially written or changed entry after creation; because complete-content publication is not atomic, failures shall retain the entry for human inspection and explicit removal.

**SPEC-PROJECTION-08** When a canonical skill is removed, the system shall identify an obsolete projection only when its complete content equals deterministic generated output, shall retain marker-bearing or unrelated authored skill files, and shall never delete the obsolete entry automatically; the failure shall require human inspection of the path and bytes before explicit removal.

**SPEC-PROJECTION-09** When repository SPEC governance validation runs, the system shall check every committed skill and EARS projection for canonical drift.

**SPEC-PROJECTION-10** When the projection generator runs in write mode, the system shall reject explicit root selection and shall require the current repository root, `.git` pointer, and administrative-directory backpointer to identify the same registered linked Git worktree.

**SPEC-PROJECTION-11** When repository skill validation runs, the system shall strictly parse the native Claude marketplace and isolated plugin manifest, shall require the SPEC governance adapter to use the `spec-governance` plugin root, and shall allow only the fixed `audit-specs` and `write-spec` exports with no plugin-level agents, hooks, MCP servers, or language servers before accepting projections.

**SPEC-PROJECTION-12** When repository skill validation runs, the system shall strictly parse each canonical `agents/openai.yaml` interface and shall require a display name, short description, and default prompt that delegates through that exact canonical skill name.

**SPEC-PROJECTION-13** When the projection generator inventories the canonical EARS package, the system shall require its complete Go source and test file set to match the fixed projection inventory before planning any output.

**SPEC-PROJECTION-14** When the projection generator writes the root EARS package, the system shall derive every generated Go source and test file from the single authored `spec-governance/earslint` core and shall prepend an unambiguous generated-file marker.

**SPEC-PROJECTION-15** When the root EARS projection is missing, stale, non-regular, symlinked, hard-linked, unexpectedly permissioned, or larger than 256 KiB, the system shall fail closed under the same check and no-clobber mutation rules as skill projections.

**SPEC-PROJECTION-16** When the projection generator writes the root EARS adapter specification, the system shall name the canonical grammar owner, label the adapter specification non-normative for grammar behavior, state that it owns no portable behavior, and limit its local contract to the deterministic projection seam.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
