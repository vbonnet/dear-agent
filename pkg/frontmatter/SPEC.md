# Markdown Frontmatter Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `pkg/frontmatter`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**FRONTMATTER-01** When markdown begins with a valid frontmatter delimiter block, the system shall exclude that block from parsed heading sections.

**FRONTMATTER-02** When frontmatter delimiters are absent or malformed, the system shall parse headings as ordinary markdown without panicking.

**FRONTMATTER-03** When headings contain inline formatting, links, or code, the system shall extract their visible text recursively.

**FRONTMATTER-04** When heading hierarchy is parsed, the system shall preserve heading level, source position, and reconstructed raw form.

**FRONTMATTER-05** When nearby headings are compared, the system shall calculate Unicode-aware edit distance for similarity decisions.

**FRONTMATTER-06** When line endings use supported Unix or Windows forms, the system shall detect frontmatter boundaries consistently.

**FRONTMATTER-07** While markdown parsing is called from any supported harness and model family, the system shall produce the same metadata and heading representation.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/frontmatter`
