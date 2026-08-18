# Code Generation Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/codegen`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**CODEGEN-01** When operation declarations are converted to intermediate representations, the system shall resolve each declared request type and preserve the shared surface metadata.

**CODEGEN-02** When request fields are inspected, the system shall omit unexported or unmanaged fields and shall reject malformed managed-field tags.

**CODEGEN-03** When CLI output is generated, the system shall derive commands, flags, validation, and operation invocation from the shared operation definition.

**CODEGEN-04** When MCP output is generated, the system shall derive tool schema and dispatch from the same operation definition used by the CLI surface.

**CODEGEN-05** When skill output is generated, the system shall derive invocation guidance from the shared operation definition, prepend the configured CLI binary to its command path, and preserve operation semantics.

**CODEGEN-06** When parity artifacts are generated, the system shall report operation exposure consistently across declared surfaces.

**CODEGEN-07** When generated output is written, the system shall format deterministic Go source and shall return generation errors with output context.

**CODEGEN-08** While generation is initiated by any supported harness and model family, the system shall produce the same intermediate representation and surface contracts.

**CODEGEN-09** When generated skill permissions are rendered, the system shall use the governed space-separated command pattern rather than retired colon syntax.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/codegen`
