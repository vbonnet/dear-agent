# Deepsec Configuration Specification

<!-- Last audited at: 2026-08-29 -->

## EARS Requirements

**DEEPSEC-01** When the repository security scanner loads its project configuration, the system shall select only declared source roots and matcher settings.

**DEEPSEC-02** If scanner configuration is invalid, the system shall fail before reporting a successful security scan.

**DEEPSEC-03** When Deepsec writes scanner-owned project metadata, technology cache, file records, run records, reports, or documented findings exports, the Git boundary shall ignore those generated paths while keeping `deepsec.config.ts`, `INFO.md`, `SETUP.md`, and optional `config.json` trackable.

## BDD Traceability

- Feature: `agm/test/bdd/features/cross_language_implementation_guardrails.feature`
- Package test: `agm/test/bdd/steps/deepsec_output_contract_test.go`
