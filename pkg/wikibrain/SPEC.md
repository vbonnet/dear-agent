# Wikibrain Knowledge Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/wikibrain` scans, lints, indexes, and suggests backlinks for the
`engram-kb` markdown knowledge base. It keeps the knowledge graph deterministic
and local by relying on markdown structure, wiki links, relative links, and
date metadata rather than remote services.

## Requirements

**WIKIBRAIN-01** When the knowledge base is scanned, the system shall skip repository, tool, template, test, and private directories that are not part of public knowledge analysis.

**WIKIBRAIN-02** When a markdown page has an H1 heading, the system shall use that heading as the page title.

**WIKIBRAIN-03** When a markdown page has no H1 heading, the system shall derive the title from the file name.

**WIKIBRAIN-04** When wiki links contain display aliases, the system shall strip the alias before storing the target.

**WIKIBRAIN-05** When markdown links target HTTP or HTTPS URLs, the system shall exclude them from internal graph checks.

**WIKIBRAIN-06** When `Last updated:` metadata uses a supported date format, the system shall parse it for staleness checks.

**WIKIBRAIN-07** When linting detects a missing internal link target, the system shall emit an error severity broken-link issue.

**WIKIBRAIN-08** When linting detects a page with no inbound links, the system shall emit a warning severity orphan-page issue.

**WIKIBRAIN-09** When linting detects a page older than the stale threshold, the system shall emit a warning severity stale-page issue.

**WIKIBRAIN-10** When linting detects a page without last-updated metadata, the system shall emit an info severity missing-metadata issue.

**WIKIBRAIN-11** When linting detects fewer than two outbound internal links, the system shall emit an info severity coverage-gap issue.

**WIKIBRAIN-12** When backlink auditing finds existing pages that mention a new page and do not already link to it, the system shall return backlink suggestions.

**WIKIBRAIN-13** When generating the page index, the system shall group pages by top-level directory and sort pages by relative path.

## BDD Traceability

- `agm/test/bdd/features/source_knowledge_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
