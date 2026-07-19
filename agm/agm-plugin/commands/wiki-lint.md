---
model: haiku
effort: low
content-hash: 7785d651322981e1f2238990e52ce5fba34ced7045cdc79630096529bfb3e4ee
description: Lint an engram-kb wiki for broken links, orphan pages, stale pages, and metadata gaps. Use for a read-only wiki health report.
argument-hint: "[--kb PATH] [--json]"
allowed-tools: Bash(agm wiki lint *)
---

# Lint the wiki

1. Run `agm wiki lint --no-append`, forwarding `--kb` or `--json` only when
   requested. Pass a custom path as one argv value. The read-only workflow must
   never append to `log.md`.
2. Show the complete error count, then summarize the highest-priority broken
   links, orphan pages, stale pages, and metadata gaps without changing files.
3. On non-zero exit, distinguish a completed lint with findings from a command
   or configuration failure using AGM's stderr and exit status.
