# Marketplace Harness Parity Specification

<!-- Last audited at: 2026-07-21 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** SKILL and plugin marketplace discovery across active AGM harnesses.

## Overview

Marketplace parity means every active harness has a declared way to discover
the same AGM, Wayfinder, research-pipeline, and YouTube command/SKILL bundles.
Claude Code uses its native `.claude-plugin/marketplace.json` format. Codex
CLI, AGY, OpenCode, and Pi use the harness-neutral `.dear-agent/marketplace.json`
catalog and AGENTS.md/SKILL fallback instructions until those harnesses
provide a native marketplace format.

## EARS Requirements

**MKT-01** When AGM publishes a marketplace catalog, the system shall include a harness-neutral `.dear-agent/marketplace.json` catalog.

**MKT-02** When AGM publishes Claude Code plugins, the system shall keep the root `.claude-plugin/marketplace.json` plugin entries aligned with the harness-neutral catalog.

**MKT-03** When a plugin appears in the harness-neutral catalog, the system shall require its source directory to exist.

**MKT-04** When a plugin declares a command capability, the system shall require the plugin source to contain a commands directory.

**MKT-05** When a plugin declares a skills capability, the system shall require the plugin source to contain a SKILL.md file or skills directory.

**MKT-06** When an active harness is present in the agent registry, the system shall declare a marketplace discovery surface for that harness.

**MKT-07** When a harness lacks a native marketplace format, the system shall declare an explicit fallback mode rather than reusing Claude-only marketplace assumptions.

**MKT-08** When Claude Code consumes the catalog, the system shall use `native-claude-plugin-marketplace` mode.

**MKT-09** When Codex CLI, AGY, OpenCode, or Pi consume the catalog, the system shall use an AGENTS.md/SKILL fallback mode.

**MKT-10** When a new active harness is added, the system shall require marketplace parity tests before the harness is considered supported.

## BDD Traceability

- Feature: `agm/test/bdd/features/marketplace_parity.feature`
