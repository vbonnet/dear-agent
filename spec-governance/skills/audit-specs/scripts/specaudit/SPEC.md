# SPEC Audit Inventory and Report Specification

## EARS Requirements

**SPECAUDIT-01** When inventory receives an explicit repository identity and pinned Git revision, the system shall read only tracked `SPEC.md` and BDD feature objects from that revision and shall emit deterministically ordered evidence seeds.

**SPECAUDIT-02** When inventory parses a specification, the system shall skip fenced examples, count only identified canonical EARS requirements, restrict BDD references to the traceability section, and record anonymous, nonconforming, missing, or nonreciprocal evidence as diagnostics.

**SPECAUDIT-03** When report validation runs, the system shall recompute the supplied inventory from Git objects at its pinned revision before accepting semantic findings or a zero-finding result.

**SPECAUDIT-04** When report validation checks a finding, the system shall require the current-owner set to equal the exact pinned requirement-evidence paths, require authenticated evidence for an existing proposed owner, and reject incomplete ownership rationale, ranks, applicability, or unsafe positive verdicts.

**SPECAUDIT-05** When report validation checks BDD impact with one or more selected features, the system shall require every selected feature to exist at the pinned revision and reciprocally reference at least one current owner in both directions, and shall require every current owner to be reciprocally represented by at least one selected feature; when no feature is selected, the system shall require a non-positive finding with BDD consequence `none`.

**SPECAUDIT-06** When rendering an authenticated audit artifact, the system shall emit self-contained offline HTML that escapes evidence and retains every decision field, owner topology, applicability row, BDD path, exclusion, methodology fact, and limitation; each applicability-evidence record shall visibly retain its path, line, requirement ID, and escaped excerpt.

**SPECAUDIT-07** When inventory or rendering completes, the system shall emit artifact bytes only to standard output and shall not create, replace, or delete a caller-selected filesystem path.

**SPECAUDIT-08** When an audit names a comparison revision, the system shall label it as comparison-only unless a separate inventory authenticates that revision.

**SPECAUDIT-09** When a supported harness invokes the collector from an unrelated working directory, the system shall execute the `specaudit` package and EARS library inside the authenticated skill distribution and shall produce the same command behavior without consulting a working-directory-relative executable path.

**SPECAUDIT-10** When the collector invokes Git for pinned evidence, the system shall strip inherited Git repository and configuration overrides, disable replacement objects and prompts, and terminate the subprocess if its combined output exceeds the configured byte ceiling.

**SPECAUDIT-11** When an installed audit launcher executes the collector, the system shall ignore inherited Go workspace, flag, environment-file, and toolchain settings and shall retain the authenticated distribution's command behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
- Feature: `agm/test/bdd/features/spec_governance_tooling.feature`
