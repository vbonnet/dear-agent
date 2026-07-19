# AGM Plugin Hash Command Specification

<!-- Last audited at: 2026-07-17 -->

## Requirements

**PLUGINHASH-CMD-01** When the plugin hash command runs in update mode, the system shall recursively stamp every governed command and skill Markdown target and shall report each result. Only `SPEC.md` is explicitly exempt from hashing.

**PLUGINHASH-CMD-02** When the plugin hash command runs in check mode, the system shall leave files unchanged and shall exit non-zero if any declared content hash is stale.

**PLUGINHASH-CMD-03** If command Markdown cannot be read, lacks a content hash, or cannot be hashed, the system shall stop with the failing path and error visible.

**PLUGINHASH-CMD-04** When repository preflight or CI runs, the system shall verify the command files, registered Claude skill files, and portable AGM skill file without modifying them.

## Test Traceability

- Unit package: `agm/cmd/plugin-hash`
- BDD feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
