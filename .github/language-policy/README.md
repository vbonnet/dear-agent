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
| `status`   | `active` (a considered exemption), `grandfathered` (unmigrated backlog), `revoked`, or `expired`. |
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

## The rule: short, or tested

A shell script passes if **either** holds:

1. it is at or under 20 countable lines (non-blank, non-comment); or
2. a bats test under `tests/bats/` exercises it.

Only a script that is **both long and untested** needs a waiver.

This is deliberate. A raw line count measures a proxy; untested complexity is
the actual risk. Crediting a test makes the way out productive: cover the
script, and the waiver becomes unnecessary and can be deleted. The
`shell-matrix` workflow already runs these tests across interpreters, so this
reuses the existing harness rather than inventing a second notion of "tested".

## The ratchet

`baseline.json` declares a ceiling on how many waivers a rule may carry.
`verify-store` fails if the store exceeds it.

**The ceiling may only ever be lowered.** A change that raises it is the change
to question: it means a script was exempted instead of shortened or tested.
When waivers are removed, lower `max_waivers` to match, so the backlog cannot
silently refill. A test asserts the ceiling equals the actual count, so slack
cannot accumulate.

The nightly sweep also reports the calibration ratio and warns while waivers
outnumber passing scripts. It warns rather than fails: the ratchet is the
enforcing half, and a permanently red nightly is a muted one.

The same sweep warns, without failing solely for that reason, when an active
time-bounded waiver is within 30 UTC calendar days of its sunset. Warnings are
ordered by the soonest sunset and name the removal, test, shortening, or
explicit owner-approved renewal paths. Once the sunset date arrives, the
existing fail-closed behavior takes over: the waiver is inactive and the full
sweep reports the resulting policy violation if the script is still long and
untested.

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

Prefer not to. Adding a test or shortening the script clears the entry
permanently; a waiver only defers it.

A waiver must carry a real `reason` and, wherever possible, a `sunset` date. An
open-ended waiver with a generic reason is how this store reached 110 entries
against 22 compliant scripts: 103 of them were granted in the single commit
that introduced the rule, all with the same text and no expiry.

Run the full sweep for the current waiver and passing-script census:

```bash
policy_paths="$(mktemp)"
git ls-files -z '*.sh' > "$policy_paths"
go run ./tools/language-policy sweep --files-from "$policy_paths" -repo .
rm -f "$policy_paths"
```

The durable backlog ceiling lives in `baseline.json`; the burn-down is tracked
by ce-kx3fm. Do not copy a point-in-time census into living guidance.
