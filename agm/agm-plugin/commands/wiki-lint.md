---
model: haiku
effort: low
content-hash: c43476e295f44c9b5b08c4d3dd9a54866cb2393792003706cc9b29007fe92d78
description: Lint an engram-kb wiki for broken links, orphan pages, stale pages, and metadata gaps. Use for a read-only wiki health report.
argument-hint: "[--kb PATH] [--json]"
allowed-tools: Bash(agm wiki lint *)
---

# Lint the wiki

1. Run `agm wiki lint`, forwarding `--kb`, `--json`, or `--no-append` only when
   requested. Pass a custom path as one argv value.
2. Show the complete error count, then summarize the highest-priority broken
   links, orphan pages, stale pages, and metadata gaps without changing files.
3. On non-zero exit, distinguish a completed lint with findings from a command
   or configuration failure using AGM's stderr and exit status.
