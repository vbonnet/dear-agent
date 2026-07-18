# AGM Plugin Hash Command Specification

<!-- Last audited at: 2026-07-17 -->

## Requirements

**PLUGINHASH-CMD-01** When the plugin hash command runs in update mode, the system shall stamp every inventoried Markdown file that declares a content hash and shall report each result.

**PLUGINHASH-CMD-02** When the plugin hash command runs in check mode, the system shall leave files unchanged and shall exit non-zero if any declared content hash is stale.

**PLUGINHASH-CMD-03** If plugin Markdown cannot be read or hashed, the system shall stop with the failing path and error visible.

## Test Traceability

- Unit package: `agm/cmd/plugin-hash`
