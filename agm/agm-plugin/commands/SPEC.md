# AGM Plugin Command Specification

<!-- Last audited at: 2026-07-17 -->

## Overview

`agm/agm-plugin/commands` contains the installed Claude command extension for
AGM. Cobra owns executable command facts; command Markdown owns only activation,
branching, safety, and completion guidance.

## Requirements

**APC-01** When a plugin command declares allowed tools, the system shall use governed space-separated tool-spec syntax and include every tool required by that command.

**APC-02** When list, search, or status command Markdown is generated, the system shall derive its active command path, supported flags, and argument shape from the unique Cobra tree and shall include the `agm` executable in every invocation.

**APC-03** When installed command Markdown changes, the system shall fail tests if the inventory omits a file or a documented `agm` path or flag does not match its declared Cobra contract.

**APC-04** When a command accepts user-controlled messages, questions, or answers, the system shall pass that content through a file input rather than shell interpolation.

**APC-05** When the AGM exit command runs, the system shall delegate inspection and archival to typed AGM commands and shall treat work as complete only when it is merged, deployed when applicable, and verified.

**APC-06** When plugin content hashes are generated or verified, the system shall use the portable Go implementation and shall hash the normalized Markdown body deterministically.

**APC-07** When a plugin command names active harnesses, the system shall include Claude Code, Codex CLI, AGY, OpenCode, and Pi and shall identify Gemini as deprecated compatibility only.

**APC-08** When a plugin command creates private temporary files for untrusted text, the system shall remove every owned temporary file after the typed AGM command returns, before reporting either success or failure.

**APC-09** When a generated command declares a fallback, the system shall preserve that fallback for unavailable-extension or missing-credential failures while stopping on unrelated non-zero exits.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/agm-plugin/commands/*_test.go`
