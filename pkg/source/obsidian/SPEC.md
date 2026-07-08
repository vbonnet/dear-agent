# Obsidian Source Adapter Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source/obsidian` stores sources as markdown files with YAML frontmatter
inside an Obsidian vault. It keeps the source adapter contract available to
plain-file Obsidian workflows without requiring an Obsidian daemon or plugin.

## Requirements

**SOURCE-OBSIDIAN-01** When an Obsidian adapter opens a missing vault directory, the system shall create the directory.

**SOURCE-OBSIDIAN-02** When an Obsidian adapter opens a path that is not a directory, the system shall reject the path.

**SOURCE-OBSIDIAN-03** When a source URI resolves outside the vault root, the system shall reject the add request.

**SOURCE-OBSIDIAN-04** When a source is added, the system shall write markdown with YAML frontmatter and source content.

**SOURCE-OBSIDIAN-05** When a source is added through an `obsidian://` URI without an extension, the system shall write a `.md` file.

**SOURCE-OBSIDIAN-06** When a source is added with a non-Obsidian URI, the system shall derive a safe slugged markdown path.

**SOURCE-OBSIDIAN-07** When scanning the vault, the system shall skip hidden directories and `.obsidian` configuration.

**SOURCE-OBSIDIAN-08** When scanning markdown without frontmatter, the system shall derive URI and title from the relative path.

**SOURCE-OBSIDIAN-09** When filtering sources, the system shall apply cue, work-item, and time-window predicates before returning matches.

**SOURCE-OBSIDIAN-10** When a query matches multiple fields, the system shall score title hits higher than snippet hits and snippet hits higher than content hits.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
