# Engram Hippocampus Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`engram/hippocampus` consolidates bounded session evidence into durable project
memory. Transcript discovery is a harness adapter concern, while extraction,
security, relevance, contradiction handling, and memory persistence remain
harness- and model-family-neutral.

## EARS Requirements

**EHP-01** When supported harnesses are enumerated, the system shall return Claude Code, Codex CLI, Antigravity, OpenCode, and Pi in canonical order.

**EHP-02** When a canonical harness name or documented alias is selected, the system shall construct the corresponding transcript adapter.

**EHP-03** When an unknown harness is selected, the system shall return an explicit unsupported-harness error.

**EHP-04** When sessions are discovered, the system shall filter by project and lower time bound when those filters are available, honor context cancellation, and return sessions oldest first.

**EHP-05** When a transcript is read, the system shall return only user and assistant text and shall omit tool payloads, progress events, and malformed records.

**EHP-06** When Claude Code sessions are discovered, the adapter shall scan the encoded project directory and skip memory, plan, and subagent directories.

**EHP-07** When Codex sessions are discovered, the adapter shall scan active and archived rollout trees, identify sessions from `session_meta`, and filter by the recorded working directory.

**EHP-08** When Codex transcript text is extracted, the adapter shall accept input-text, output-text, and text message parts from response items.

**EHP-09** When Antigravity sessions are discovered, the adapter shall inspect brain transcript artifacts and use the last-conversation cache for project filtering.

**EHP-10** When Antigravity transcript text is extracted, the adapter shall map `USER_INPUT` to user text and `PLANNER_RESPONSE` to assistant text.

**EHP-11** When OpenCode sessions are discovered, the adapter shall query the configured read-only SQLite store and filter by session directory and update time.

**EHP-12** When OpenCode transcript text is extracted, the adapter shall join ordered message and part records and retain only text parts for user and assistant roles.

**EHP-13** When `OPENCODE_DATA_DIR` is configured, the OpenCode adapter shall use it instead of the default application-data directory.

**EHP-14** When shared project memory exists, the system shall resolve the same hashed `~/.engram/memory/projects` directory for every harness adapter.

**EHP-15** When shared project memory is absent but Claude native memory exists, the system shall use the Claude location only as a compatibility fallback.

**EHP-16** When an LLM side-query callback is configured, the consolidation provider shall remain model-family-neutral and shall not require an Anthropic-specific implementation.

**EHP-17** When no LLM callback is configured or an LLM extraction fails, the system shall preserve pattern-based extraction as the fallback path.

**EHP-18** When LLM transcript input exceeds 20,000 bytes, the system shall truncate it before the side query.

**EHP-19** When LLM output is wrapped in prose or Markdown, the system shall extract and decode the bounded JSON object.

**EHP-20** When memory paths are validated, the system shall reject traversal, path-boundary escapes, and symlink escapes outside the configured memory root.

**EHP-21** When memory filenames mix confusable Latin and Cyrillic characters, the system shall reject the homoglyph risk.

**EHP-22** When memory files are scanned for relevance, the system shall inspect Markdown topic files, exclude `MEMORY.md`, cap the candidate set, and order candidates deterministically.

**EHP-23** When relevant memories are surfaced, the system shall cap selected results and validate each file before returning its content.

**EHP-24** When `MEMORY.md` is parsed and rendered, the system shall preserve preamble, nested sections, headings, and intentional blank lines.

**EHP-25** When autodream runs in dry-run mode, the system shall report a diff without modifying memory files or trigger state.

**EHP-26** When autodream persists memory changes, the system shall write atomically, preserve a backup, enforce topic and line limits, and reset trigger state only after success.

**EHP-27** When consolidation signals are merged, the system shall deduplicate normalized content and apply validated contradiction winners without discarding unrelated memory.

**EHP-28** When session lineage is traversed, the system shall preserve parent relationships and terminate safely on cycles.

**EHP-29** When daily logs and critic decisions are written, the system shall create their directories, append rather than replace prior evidence, and retain structured timestamps.

**EHP-30** When a memory freshness date is unavailable or malformed, the system shall fall back to file modification time and surface a bounded age caveat.

**EHP-31** When Pi sessions are discovered, the adapter shall scan both Pi-native and AGM-private JSONL trees without following symlinks, identify sessions from exact header IDs, filter by absolute cwd and update time, and deduplicate private copies.

**EHP-32** When Pi transcript text is extracted, the adapter shall retain only user and assistant text parts while omitting thinking, tool results, and malformed records.

## BDD Traceability

- Feature: `agm/test/bdd/features/engram_parity.feature`
- Package tests: `engram/hippocampus/*_test.go`
