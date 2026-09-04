# Marketplace Harness Parity Specification

<!-- Last audited at: 2026-09-04 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** SKILL and plugin marketplace discovery across active AGM harnesses.

## Overview

Marketplace parity means every active harness has a declared way to discover
the same AGM, Wayfinder, research-pipeline, SPEC-governance, and YouTube
command/SKILL bundles.
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

**MKT-11** When the SPEC-governance package is declared, the neutral catalog and Claude marketplace shall each contain exactly one `spec-governance` entry with version `0.1.0`, literal source `./spec-governance`, repository `https://github.com/vbonnet/dear-agent`, license `Apache-2.0`, and dear-agent authorship.

**MKT-12** When the SPEC-governance Claude marketplace entry is validated, the system shall require `strict: true` and shall reject component definitions or other behavior-bearing supplemental fields.

**MKT-13** When the SPEC-governance Claude plugin manifest is validated, the system shall require matching authority metadata, shall allow no fields beyond that descriptive metadata and skill exports, and shall export exactly `audit-specs` and `write-spec` from their canonical directories.

**MKT-14** When a provider-default command, agent, hook, MCP server, LSP server, executable, setting, output style, theme, monitor, workflow, or additional skill surface appears in the SPEC-governance plugin source, the system shall reject the package.

**MKT-15** When catalog or manifest JSON is decoded, the system shall reject duplicate object fields recursively, trailing values, and case aliases of known authority fields while retaining compatible neutral supplemental metadata that does not alias an authority field.

**MKT-16** When marketplace validation reads a catalog, manifest, SPEC-governance source root, skill tree, skill entrypoint, or exported reference, the system shall use bounded descriptor-anchored no-follow and nonblocking inspection, require stable regular-file identity with one hard link, and reject escapes, symlinks, nonregular objects, replacement races, and bounds violations.

**MKT-17** When the host cannot provide descriptor-anchored no-follow filesystem inspection, the system shall fail SPEC-governance source validation closed.

**MKT-18** When canonical skill content is validated, the system shall apply the existing skill-lint rules to the bytes read through the anchored descriptor rather than rereading a mutable path.

**MKT-19** When SPEC-governance marketplace source validation succeeds, the system shall report only catalog, manifest, inventory, and source-schema evidence and shall not claim marketplace registration, installation, enabled state, discovery, invocation, or runtime loading.

## BDD Traceability

- Feature: `agm/test/bdd/features/marketplace_parity.feature`
