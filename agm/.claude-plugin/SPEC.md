# AGM Marketplace Configuration Specification

<!-- Last audited at: 2026-07-10 -->

## EARS Requirements

**DECL-AGM-MARKET-01** When Claude-compatible marketplace discovery loads AGM, the system shall expose the versioned marketplace and command integrity metadata.

**DECL-AGM-MARKET-02** If marketplace metadata or command hashes are invalid, the system shall reject the plugin registration.

## BDD Traceability

- Feature: `agm/test/bdd/features/declarative_runtime_guardrails.feature`
