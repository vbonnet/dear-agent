# AGM Plugin Hash Command Specification

<!-- Last audited at: 2026-07-17 -->

## Requirements

**PLUGINHASH-CMD-01** When the plugin hash command runs in update mode, the system shall stamp every inventoried command Markdown file and shall report each result. Only `SPEC.md` is explicitly exempt from hashing.

**PLUGINHASH-CMD-02** When the plugin hash command runs in check mode, the system shall leave files unchanged and shall exit non-zero if any declared content hash is stale.

**PLUGINHASH-CMD-03** If command Markdown cannot be read, lacks a content hash, or cannot be hashed, the system shall stop with the failing path and error visible.

## Test Traceability

- Unit package: `agm/cmd/plugin-hash`
- BDD feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
