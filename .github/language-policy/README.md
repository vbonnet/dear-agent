# Language policy waiver store

`exceptions.jsonl` is the waiver store for the `bash-20-line-limit` rule
enforced by [`language-policy.yml`](../workflows/language-policy.yml).

## Format

One JSON object per line, sorted by `(rule, path)`:

```json
{"rule":"bash-20-line-limit","path":"scripts/example.sh","status":"active","reason":"why this cannot be shortened","approver":"vbonnet","sunset":"2026-12-31","added":"2026-08-19"}
```

| Field      | Meaning                                                                 |
| ---------- | ----------------------------------------------------------------------- |
| `rule`     | Rule the waiver applies to.                                              |
| `path`     | Repo-relative path, no leading `./`.                                     |
| `status`   | `active`, `grandfathered`, `revoked`, or `expired`.                      |
| `reason`   | Why the script cannot meet the rule. Not "pre-existing".                 |
| `approver` | Who granted it.                                                          |
| `sunset`   | `YYYY-MM-DD` when the waiver lapses, or `null` for open-ended.            |
| `added`    | Date granted.                                                            |

## Why line-oriented text, not a database

This store was a committed SQLite binary until 2026-08-19. Text is not a
stylistic preference here; it buys three things a binary store cannot:

- **Per-waiver attribution.** `git blame exceptions.jsonl` names the commit,
  author, and date for each individual waiver. A binary blob blames as one
  opaque file, so "who waived this and why" was unanswerable.
- **Reviewable diffs.** Granting a waiver is a policy decision. In a binary
  store it showed up as `Bin 36864 -> 36864 bytes`, which no reviewer can
  check.
- **No client required.** Reading the policy needed `sqlite3` installed, in CI
  and on every developer machine.

See the retrospective in the research repository:
`retrospectives/2026-08-19-language-policy-binary-store-and-inverted-exceptions.md`.

## Adding a waiver

Append a line, then normalise ordering and formatting:

```bash
go run ./tools/language-policy format -repo .
go run ./tools/language-policy verify-store -repo .
```

`verify-store` fails if the store is unsorted, has duplicates, contains NUL
bytes, or if a binary store (`*.db`, `*.sqlite`, `*.sqlite3`, `*.db3`) has been
added to this directory. The same checks run as ordinary Go tests
(`tools/language-policy/store_test.go`), so `make preflight` catches them
locally before CI does.

A waiver should carry a real `reason` and, wherever possible, a `sunset` date.
An open-ended waiver with a generic reason is how this store grew to 110
entries against 22 compliant scripts.
