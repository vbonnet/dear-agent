# Dead Link Checker Requirements Specification (EARS)

<!-- Last audited at: 2026-07-18 -->

**Version**: 2.0
**Status**: Active
**Scope**: Repository-contained Markdown link validation.

## EARS Requirements

**DEAD-LINKS-01** When Markdown files are discovered, the system shall analyze every tracked Markdown file including files under hidden directories.

**DEAD-LINKS-02** When Markdown contains fenced or indented code, the system shall not interpret link-like code text as a reference.

**DEAD-LINKS-03** When Markdown contains inline, reference, or image links, the system shall resolve every local destination from the source file or repository root.

**DEAD-LINKS-04** When a local Markdown destination includes a fragment, the system shall require that fragment to identify a heading or explicit anchor in the target document.

**DEAD-LINKS-05** When a pure fragment destination is encountered, the system shall validate it against the source document.

**DEAD-LINKS-06** When a destination uses a URI scheme, the system shall exclude it from deterministic local validation.

**DEAD-LINKS-07** When a local link target or anchor does not exist, the system shall report its source file, line, and target.

**DEAD-LINKS-08** When a baseline is supplied, the system shall suppress current findings whose normalized source and target pair is recorded in that baseline.

**DEAD-LINKS-09** When a current finding is absent from the supplied baseline, the system shall report new debt and exit with status 1.

**DEAD-LINKS-10** When a baseline entry has no current finding, the system shall report the stale exception and exit with status 1.

**DEAD-LINKS-11** When a baseline revision is supplied, the system shall reject entries added to the current baseline after that revision.

**DEAD-LINKS-12** When discovery, document reading, baseline parsing, or revision lookup fails, the system shall exit with operational-error status 2.

**DEAD-LINKS-13** When multiple links target one Markdown document, the system shall reuse that document's parsed anchor inventory.

**DEAD-LINKS-14** When a heading contains inline Markdown, the system shall derive its anchor from rendered heading text rather than raw markup.

**DEAD-LINKS-15** When generated heading anchors collide across base names or suffixes, the system shall reserve every emitted identifier and choose the next unused suffix.

**DEAD-LINKS-16** When no root is supplied, the command shall resolve Git's top-level directory from the current working directory before discovering tracked Markdown.

## Test Traceability

- Package tests: `tools/dead-links/main_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
