# Harness Configuration Directory Parity Specification

<!-- Last audited at: 2026-07-21 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** Repo-local dot-directory configuration surfaces for supported harnesses.

## Overview

Configuration directory parity means every active harness has a repo-local
dot-directory where hooks, settings, skills, or fallback metadata can live.
These directories are the concrete `.claude/`, `.codex/`, `.agents/`,
`.opencode/`, and `.pi/` equivalents of the harness-specific configuration surfaces. Gemini
keeps `.gemini/` as deprecated compatibility.

## EARS Requirements

**CDP-01** When a harness is active, the system shall declare its repo-local configuration directory.

**CDP-02** When a declared active harness directory is missing, the system shall fail parity validation.

**CDP-03** When Claude Code is active, the system shall use `.claude/` as its configuration directory.

**CDP-04** When Codex CLI is active, the system shall use `.codex/` as its configuration directory.

**CDP-05** When AGY is active, the system shall use `.agents/` as its configuration directory.

**CDP-06** When OpenCode is active, the system shall use `.opencode/` as its configuration directory.

**CDP-07** When Gemini CLI compatibility is present, the system shall keep `.gemini/` separate from the active harness parity set.

**CDP-08** When a new active harness is added, the system shall require configuration-directory parity tests before the harness is considered supported.

**CDP-09** When Pi is active, the system shall use `.pi/` as its configuration directory while keeping the mandatory authorization extension in AGM-private storage.

## BDD Traceability

- Feature: `agm/test/bdd/features/config_directory_parity.feature`
