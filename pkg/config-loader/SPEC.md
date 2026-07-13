# Configuration Loader Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/config-loader`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**CONFIG-LOADER-01** When a current-user tilde path is supplied, the system shall expand it to the user home directory and shall leave other path forms unchanged.

**CONFIG-LOADER-02** When a relative path is resolved, the system shall join it to the supplied base directory or current working directory.

**CONFIG-LOADER-03** When a file is searched across candidate directories, the system shall return the first existing path in declared order.

**CONFIG-LOADER-04** When YAML configuration is loaded, the system shall preserve read, path-expansion, and unmarshal errors with path context.

**CONFIG-LOADER-05** When an optional configuration file is absent, the system shall return caller defaults and shall still reject an existing malformed file.

**CONFIG-LOADER-06** When markdown frontmatter is parsed strictly, the system shall require delimiters and valid YAML while preserving the trimmed body.

**CONFIG-LOADER-07** When personas are loaded or listed, the system shall apply documented field defaults, recursive selection, and malformed-file handling.

**CONFIG-LOADER-08** When a persona is explicitly validated, the system shall require documented fields, lowercase kebab-case identity, semantic version shape, and supported severity values.

**CONFIG-LOADER-09** While configuration loading is called from any supported harness and model family, the system shall preserve identical path, YAML, and persona semantics.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/config-loader`
