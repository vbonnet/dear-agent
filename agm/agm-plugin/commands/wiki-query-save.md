---
model: sonnet
effort: medium
content-hash: 0701307a13b8f3810098ac58583446f14b3184980a38b70b4ad378d156203407
description: Synthesize an answer from engram-kb and optionally save it as a wiki page. Use when the user wants a source-grounded wiki answer or to persist that answer.
argument-hint: "<question> [--save] [--category research|decisions]"
allowed-tools: Bash(agm wiki query-save *), Bash(rm -f -- /tmp/agm-wiki-*), Read, Glob, Grep, Write(/tmp/agm-wiki-*)
---

# Query and optionally save the wiki

1. Search the configured knowledge base with Glob and Grep, read the relevant
   pages, and answer with wikilink citations. Clearly label any statement that
   is not supported by the wiki.
2. Save only when the user requested or confirms persistence.
3. Write the exact question and synthesized answer to unique, private temporary
   files under `/tmp/agm-wiki-*`. Never interpolate either value into
   shell syntax.
4. Run `agm wiki query-save --query-file <question-file> --answer-file <answer-file> --category <category>`.
   Add an explicit `--output` only when the user selected a path. Pass every
   path and category as a separate argv value.
5. Always run `rm -f -- <question-file> <answer-file>` immediately after AGM
   exits, before reporting success or failure. Neither temporary file may
   survive either path.
6. Report the saved page and backlink/index result. On failure, show stderr and
   stop; do not write into the knowledge base manually.
