# Model Pricing Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`internal/pricing` is the shared local rate-card lookup used by AGM cost reports,
quota monitoring, and model-family budget decisions. It intentionally provides
truthful approximate relative cost, not invoice-grade billing, and must make
unknown pricing explicit to avoid silently applying Claude-specific defaults to
other model families.

## EARS Requirements

**PRICING-01** When pricing is looked up by exact alias, the system shall match aliases case-insensitively.

**PRICING-02** When pricing is looked up by a full model name containing a known alias, the system shall return the alias price.

**PRICING-03** When pricing is looked up for an empty or unknown model, the system shall return `UnknownModel`.

**PRICING-04** When cost is estimated, the system shall multiply input and output tokens by their per-million-token rates and return the summed USD estimate.

**PRICING-05** When cost is estimated for an unknown model, the system shall return zero cost while allowing callers to detect the unknown model through `Lookup`.

**PRICING-06** When model-family quota policy evaluates known models, the system shall preserve relative cost ordering so higher-tier models remain more expensive than lower-tier defaults.

**PRICING-07** When GLM, DeepSeek, Nemotron, or Qwen default-model pricing is recorded, the system shall store a primary rate-card source and as-of date with positive input and output rates.

**PRICING-08** When OpenRouter model-family defaults are selected, the system shall use the provider's current canonical model slug rather than an unverified shorthand identifier.

## BDD Traceability

- `agm/test/bdd/features/quota_parity.feature`
- `agm/test/bdd/features/model_family_parity.feature`

## Package Test Traceability

- `internal/pricing/pricing_test.go`
