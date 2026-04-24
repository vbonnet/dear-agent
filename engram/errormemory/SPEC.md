# SPEC: core/errormemory

## Purpose

Package `errormemory` is the shared library underlying the session error-memory system.
It provides the data model, persistent store, backfill logic, and maintenance utilities
used by both the hook binaries and the CLI tool.

## File Paths

- Source: `errormemory/`
  - `types.go` — types and constants
  - `store.go` — JSONL persistence and upsert
  - `backfill.go` — log-file backfill parser
  - `maintenance.go` — pruning, ranking, stats, and summary formatting
- Default database: `~/.agm/error-memory.jsonl`
- Lock file: `~/.agm/error-memory.jsonl.lock`

## Data Model

### ErrorRecord (persisted per JSONL line)

| Field            | Type       | Description                                    |
|------------------|------------|------------------------------------------------|
| `id`             | string     | SHA-256(pattern + "|" + error_category)[:16]  |
| `pattern`        | string     | Pattern name (e.g., bash-blocker rule name)    |
| `error_category` | string     | Source category; see source constants below    |
| `command_sample` | string     | Last offending command (best-effort)           |
| `remediation`    | string     | Human-readable fix suggestion                  |
| `count`          | int        | Cumulative occurrences                         |
| `first_seen`     | time.Time  | Timestamp of first occurrence                  |
| `last_seen`      | time.Time  | Timestamp of most recent occurrence            |
| `ttl_expiry`     | time.Time  | last_seen + 30 days; record pruned after this  |
| `session_ids`    | []string   | Up to last 5 session IDs that hit this pattern |
| `source`         | string     | Originating subsystem                          |

### Source Constants

- `bash-blocker` — pretool-bash-blocker hook
- `astrocyte` — astrocyte subsystem
- `permission-prompt` — permission prompt events

### ErrorSummary / ErrorSummaryEntry (session-injection payload)

`ErrorSummary` wraps a list of `ErrorSummaryEntry` items with a generation timestamp
and approximate token count. Entries carry: pattern, remediation, count, last_seen (string).

### Limits

| Constant           | Value          |
|--------------------|----------------|
| DefaultTTL         | 30 days        |
| DefaultDBPath      | ~/.agm/error-memory.jsonl |
| MaxRecords         | 5,000          |
| MaxSummaryTokens   | 500            |
| MaxSummaryEntries  | 10             |

## API Contract

### Store

```
NewStore(path string) *Store
(*Store).Load() ([]ErrorRecord, error)        // returns [] if file absent
(*Store).Save(records []ErrorRecord) error    // atomic: temp file + rename
(*Store).Upsert(rec ErrorRecord) (ErrorRecord, error)
```

**Upsert semantics:** deduplicates on SHA-256 ID (pattern + error_category).
On match: increments count, updates last_seen/ttl_expiry, merges command_sample
and remediation (non-empty wins), keeps last 5 session IDs.
On new: sets count=1 if zero, TTLExpiry = last_seen + 30d.

### Maintenance

```
PruneExpired(records []ErrorRecord) []ErrorRecord
TopN(records []ErrorRecord, n int) []ErrorRecord    // scored: count * 1/(days+1)
Stats(records []ErrorRecord) DBStats
FormatSummary(records []ErrorRecord, maxEntries int) (string, int)
```

`FormatSummary` produces lines of the form:
`  - Do NOT use PATTERN -- REMEDIATION (Nx, last Xd ago)`
Token estimate: ceil(words / 0.75).

### Backfill

```
BackfillFromLog(logPath string) (*BackfillResult, error)
```

Parses `pretool-bash-blocker.log`. Regex for DENIED lines:
`^\[YYYY-MM-DD HH:MM:SS\] DENIED:\s+PATTERN\s+-\s+REMEDIATION$`
Also extracts `session_id` from embedded JSON in "Raw input" lines.

## Performance Guarantees

- **Load:** sequential buffered scan; O(n) lines; skips malformed lines silently.
- **Save:** writes to a temp file in the same directory then `os.Rename` (atomic on POSIX).
- **Locking:** `O_CREATE|O_EXCL` file lock; up to 3 retries × 100 ms = 300 ms max wait.
- **Backfill scanner buffer:** 256 KB per line to handle long log lines.
- **Ranking:** in-memory sort; not persisted; O(n log n).

## Failure Modes (Fail-Open Design)

- Missing DB file: `Load` returns empty slice, no error.
- Malformed JSONL lines: silently skipped; remaining lines still parsed.
- Lock acquisition failure (after 3 attempts): `Upsert` returns error; callers log
  and continue — they always exit 0.
- Save failure: temp file cleaned up; original DB untouched (atomic swap never happens).
- Home directory lookup failure in `expandPath`: raw path used as-is.
