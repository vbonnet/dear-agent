# LLM Wiki Source Adapter Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/source/llmwiki` stores sources as markdown files with YAML frontmatter in
a git-backed knowledge directory. It mirrors Obsidian search semantics while
optionally committing successful writes for inspectable knowledge history.

## Requirements

**SOURCE-LLMWIKI-01** When an llm-wiki adapter opens a missing wiki directory, the system shall create the directory.

**SOURCE-LLMWIKI-02** When an llm-wiki adapter opens a path that is not a directory, the system shall reject the path.

**SOURCE-LLMWIKI-03** When the wiki directory is inside a git repository and git is available, the system shall enable auto-commit by default.

**SOURCE-LLMWIKI-04** When a source URI resolves outside the wiki root, the system shall reject the add request.

**SOURCE-LLMWIKI-05** When a source is added, the system shall write markdown with YAML frontmatter and source content.

**SOURCE-LLMWIKI-06** When auto-commit is enabled, the system shall stage and commit the written source file.

**SOURCE-LLMWIKI-07** When git commit fails after the file is written, the system shall log the error and keep the add operation successful.

**SOURCE-LLMWIKI-08** When scanning the wiki, the system shall skip `.git` and hidden directories.

**SOURCE-LLMWIKI-09** When filtering sources, the system shall apply cue, work-item, and time-window predicates before returning matches.

**SOURCE-LLMWIKI-10** When a query matches multiple fields, the system shall score title hits higher than snippet hits and snippet hits higher than content hits.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
