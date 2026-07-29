# Dear Agent Neutral Marketplace Specification

<!-- Last audited at: 2026-07-21 -->

## Overview

The `.dear-agent` directory owns the harness-neutral marketplace catalog used
by non-Claude harnesses and by parity validation. It is the canonical catalog
for plugins, capabilities, and fallback discovery surfaces.

## Requirements

**DEAR-MARKET-01** When the neutral marketplace is published, the system shall keep the catalog at `.dear-agent/marketplace.json`.

**DEAR-MARKET-02** When the neutral marketplace catalog is parsed, the system shall use schema version `dear-agent.marketplace/v1`.

**DEAR-MARKET-03** When plugins are declared in the neutral marketplace, the system shall include source, version, description, and capability metadata.

**DEAR-MARKET-04** When active non-Claude harnesses consume marketplace data, the system shall declare each harness's supported discovery surface, using native Codex, OpenCode, and Pi skill modes where configured and `agents-md-skill-fallback` only for AGY.

**DEAR-MARKET-05** When Claude Code consumes marketplace data, the system shall declare the native Claude catalog path as `.claude-plugin/marketplace.json`.

**DEAR-MARKET-06** When Pi consumes marketplace data, the system shall declare `pi-cli` in the neutral catalog and combine that catalog with native `.pi/settings.json` skill discovery.

## BDD Traceability

- `agm/test/bdd/features/harness_config_surface_guardrails.feature` enforces that this directory keeps co-located SPEC coverage.
- `agm/test/bdd/features/marketplace_parity.feature` validates neutral catalog content and harness discovery surfaces.
