# Wayfinder Harness Parity Specification

<!-- Last audited at: 2026-07-17 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Wayfinder CLI, SKILL/plugin, MCP, and status discovery across active AGM harnesses.

## Overview

Wayfinder parity means every active AGM harness can discover and operate the
same workflow. Claude Code auto-discovers the plugin's root skill. Codex CLI,
AGY, and OpenCode use the same skill content when available and the
`wayfinder session` CLI as the neutral execution surface. MCP tools expose
session status without requiring the caller to be Claude Code.

## EARS Requirements

**WFP-01** When Wayfinder is installed, the system shall publish a top-level `wayfinder/SKILL.md` artifact.

**WFP-02** When Wayfinder is installed, the system shall publish a Claude plugin manifest for native Claude Code consumption.

**WFP-03** When Wayfinder is installed, the system shall publish the `wayfinder session` CLI as the harness-neutral workflow entrypoint.

**WFP-04** When Wayfinder sessions are queried through MCP, the system shall expose list and detail tools.

**WFP-05** When an active harness lacks native Wayfinder plugin support, the system shall declare a CLI/SKILL fallback surface.

**WFP-06** When an active harness is present in the agent registry, the system shall declare a Wayfinder discovery surface for that harness.

**WFP-07** When Wayfinder phase guidance is used, the system shall keep phase engram resolution harness-neutral.

**WFP-08** When a new active harness is added, the system shall require Wayfinder parity tests before the harness is considered supported.

## BDD Traceability

- Feature: `agm/test/bdd/features/wayfinder_parity.feature`
