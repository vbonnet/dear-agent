# A2A Persona Loader Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`agm/internal/a2a/personas` loads `.ai.md` persona files with YAML frontmatter
so A2A and Engram coordination can discover stable and experimental review,
worker, researcher, and orchestrator personas.

## EARS Requirements

**A2A-PER-01** When loading a persona file, the system shall require YAML frontmatter followed by markdown content.

**A2A-PER-02** When optional persona fields are omitted, the system shall default version to `1.0.0`, tier to `tier2`, maturity to `stable`, and list fields to empty lists.

**A2A-PER-03** When persona YAML is invalid, the system shall return a contextual parse error naming the source file.

**A2A-PER-04** When a persona is loaded, the system shall retain markdown content and source path separately from serialized metadata.

**A2A-PER-05** When listing personas, the system shall recursively scan `.ai.md` files, skip invalid files with warnings, and return loadable personas.

**A2A-PER-06** When stable or experimental personas are requested, the system shall filter loaded personas by maturity.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_parity.feature`
- Package tests: `agm/internal/a2a/personas/persona_test.go`
