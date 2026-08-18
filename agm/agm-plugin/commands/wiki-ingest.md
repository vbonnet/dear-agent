---
model: haiku
effort: low
content-hash: 95503bfebd837ca691a66b057eac9e1b02f65bd38fb45740971efdf173478cd3
description: Audit backlinks and update the engram-kb index after a page is added. Use when the user has already created a wiki page and wants it integrated.
argument-hint: "--page PATH [--kb PATH] [--no-index]"
allowed-tools: Bash(agm wiki ingest *), Read, Write
---

# Ingest a wiki page

1. Require a repo-relative or absolute page path.
2. Run `agm wiki ingest --page <path>`, forwarding `--kb` or `--no-index` only
   when requested. Pass paths as separate argv values; do not construct shell
   syntax from them.
3. Present AGM's backlink suggestions and index/log result.
4. Apply suggested backlinks only after the user authorizes those page edits.
   Read each target first and make the smallest contextual edit.
5. On failure, show stderr and stop; do not recreate ingest behavior manually.
