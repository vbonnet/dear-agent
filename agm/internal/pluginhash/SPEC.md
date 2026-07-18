# AGM Plugin Hash Specification

<!-- Last audited at: 2026-07-17 -->

## Requirements

**PLUGINHASH-01** When a plugin Markdown body is hashed, the system shall compute lowercase SHA-256 over the content after the closing frontmatter delimiter with trailing newlines removed.

**PLUGINHASH-02** When a plugin Markdown hash is stamped, the system shall replace only an existing `content-hash` field and shall produce the same bytes on repeated runs.

**PLUGINHASH-03** If plugin Markdown lacks frontmatter delimiters or a content-hash field, the system shall return an explicit error without modifying the source.

## Test Traceability

- Unit package: `agm/internal/pluginhash`
- BDD feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
