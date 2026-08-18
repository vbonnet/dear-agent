# Visible Markdown Semantics Specification

<!-- Last audited at: 2026-08-01 -->

**Version:** 1.0
**Status:** Enforced
**Scope:** Provider-neutral classification of normative Markdown source prose.

## Purpose

`internal/markdownvisible` defines which Markdown source bytes policy tools may
treat as visible prose. The contract is independent of any agent harness and
uses whole-document CommonMark structure so copied examples and hidden markup
cannot become normative requirements or traceability by accident.

## EARS Requirements

**MDVIS-01** When a caller classifies a Markdown document, the system shall parse the complete document with the repository CommonMark parser before deciding which source ranges are visible.

**MDVIS-02** When CommonMark classifies source as an indented code block, fenced code block, or raw HTML block, the system shall exclude that source from visible prose.

**MDVIS-03** When an inline HTML comment appears beside ordinary Markdown prose, the system shall blank the comment range while preserving the surrounding visible source text.

**MDVIS-04** When comment delimiters appear inside an inline code span, the system shall preserve the code span and following ordinary prose according to CommonMark precedence.

**MDVIS-05** When the CommonMark parser classifies a fenced-code block inside document containers, the system shall exclude the complete physical opening, content, and closing lines while requiring the closing delimiter to use the same marker, at least the opening marker length, and only a parser-admissible container prefix and trailing whitespace.

**MDVIS-06** When the system returns classified source lines, the system shall preserve source line order and line count, blank excluded bytes rather than shifting visible bytes, and mark lines containing only excluded material as not visible.

## BDD Traceability

- Feature: `agm/test/bdd/features/markdown_visibility.feature`
