# ADR-017: File-Based Pending Messages for Hook Delivery

**Status:** Accepted
**Date:** 2026-03-24
**Context:** Inter-agent message delivery via PreToolUse hooks

## Problem

AGM's existing message delivery relies on the coordination daemon polling the SQLite queue every 30 seconds, then using tmux send-keys to inject messages. This has two drawbacks:

1. **Latency**: Up to 30 seconds between send and delivery
2. **State dependency**: Messages are only delivered when the recipient is in READY state, which requires Astrocyte state detection to be accurate

Claude Code's PreToolUse hook system provides a natural message checkpoint -- every tool call triggers all hooks. If pending messages were available on the filesystem, a hook could inject them with zero latency.

## Decision

Introduce a file-based pending message system alongside the existing SQLite queue:

1. **Write side** (`internal/messages/pending.go`): `WritePendingFile()` writes `.msg` files to `~/.agm/pending/{sessionName}/` with timestamp-based filenames for chronological ordering
2. **Read side** (`pretool-message-check` hook in engram): Reads `.msg` files on every tool call, injects content into agent context via stderr, then removes the files
3. **Coexistence**: File-based delivery supplements (not replaces) the daemon + queue path

### Key design choices:

1. **Filesystem over IPC**: Any process can write a `.msg` file without importing AGM libraries
2. **Timestamp filenames**: `{unix-nanos}-{messageID-prefix}.msg` ensures chronological ordering without an index file
3. **Atomic cleanup**: Hook removes files after delivery; if the hook crashes mid-delivery, files persist for the next tool call
4. **Best-effort**: Errors in the hook never block tool execution (fail-open)

## Alternatives Considered

1. **Unix domain socket**: Rejected -- requires a persistent listener process and doesn't survive agent restarts
2. **Named pipe (FIFO)**: Rejected -- blocking semantics complicate the hook's fail-open requirement
3. **SQLite shared database**: Rejected -- file locking issues when multiple hooks read concurrently
4. **Extend daemon polling to 1s**: Rejected -- increases CPU usage for all sessions, not just those with pending messages

## Consequences

- Messages are delivered on the next tool call (typically < 1 second latency)
- Works only for harnesses with hook support (Claude Code today; other harnesses need adapters)
- No guaranteed exactly-once delivery -- if the hook reads but crashes before cleanup, the message may be re-delivered
- Directory `~/.agm/pending/` must be writable by both AGM and the hook process (same user, no issue in practice)
