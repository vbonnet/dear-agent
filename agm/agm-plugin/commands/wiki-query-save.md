---
model: sonnet
effort: medium
content-hash: 0ebc904b9d800b1aeb1dae152bfc573c72d59549a96e0a276c03edfa1abb7696
description: Synthesize an answer from engram-kb and optionally save it as a wiki page. Use when the user wants a source-grounded wiki answer or to persist that answer.
argument-hint: "<question> [--save] [--category research|decisions]"
allowed-tools: Bash(agm wiki query-save *), Read, Glob, Grep, Write(/private/tmp/agm-wiki-*)
---

# Query and optionally save the wiki

1. Search the configured knowledge base with Glob and Grep, read the relevant
   pages, and answer with wikilink citations. Clearly label any statement that
   is not supported by the wiki.
2. Save only when the user requested or confirms persistence.
3. Write the exact question and synthesized answer to unique, private temporary
   files under `/private/tmp/agm-wiki-*`. Never interpolate either value into
   shell syntax.
4. Run `agm wiki query-save --query-file <question-file> --answer-file <answer-file> --category <category>`.
   Add an explicit `--output` only when the user selected a path. Pass every
   path and category as a separate argv value.
5. Report the saved page and backlink/index result. On failure, show stderr and
   stop; do not write into the knowledge base manually.
