# Dead Link Checker Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Repository-contained Markdown link validation.

## EARS Requirements

**DEAD-LINKS-01** When Markdown files are discovered, the system shall walk repository content while excluding configured generated or dependency directories.

**DEAD-LINKS-02** When a link is external or contains only an anchor, the system shall skip filesystem validation for that link.

**DEAD-LINKS-03** When a local link contains an anchor, the system shall validate the target path without the anchor fragment.

**DEAD-LINKS-04** When a root-relative link is encountered, the system shall resolve it from the repository root.

**DEAD-LINKS-05** When a local link target does not exist, the system shall report the source file and broken target.

**DEAD-LINKS-06** When Markdown contains long lines, the system shall scan links without truncating the line.

## Test Traceability

- Package tests: `tools/dead-links/main_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
