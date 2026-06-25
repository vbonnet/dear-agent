# dear-deploy

<!-- Last audited at: 2026-06-19 -->

Deploys dear-agent's **host artifacts** — launchd plists and Claude Code hooks —
from their source of truth in this repo to their installed location on the
machine, and audits them for drift. `dear-deploy status` is the manifest-driven
successor to [`drift-check`](../drift-check/README.md) (`make drift-check` now
runs it); dear-deploy also *makes a drifted artifact current* via `sync`.

The deployable set lives in [`deploy/manifest.yaml`](../../deploy/manifest.yaml).

## Artifact kinds

Each manifest entry is one of two `kind`s:

- **file** (default) — launchd plists and compiled hooks. The deployed copy is
  compared and (re)written by **byte content** (SHA-256). These are what `sync`
  and `install` deploy through the atomic sequence below.
- **binary** — a Go program (e.g. `mergeloop`, `vroom-dispatch`). Its source of
  truth is the repo's current commit, not a file in the tree, so `status`
  compares the **`vcs.revision` embedded at build time** (what `go version -m`
  prints) against the repo HEAD: a binary built before a fix landed reports
  `STALE`, an absent one reports `NOT DEPLOYED`. Binaries are **status-only** —
  `sync`/`install` never copy them; they are rebuilt out of band by their
  `remediation` command (e.g. `make install-mergeloop`).

## Why

"Merged to main" is not "deployed on the host." When a redeploy step is skipped,
the fix lives in git but the machine stays broken — silently. drift-check catches
that gap; dear-deploy closes it through a single sanctioned, atomic command so
the redeploy is never a hand-typed `cp` that can half-write a file or be
forgotten.

## Atomic deploy (AGENTS.md principle 9)

Every write goes through a deterministic three-step sequence:

1. **stage** — render the source (token substitution) and write it to a temp
   file in the *same directory* as the target, so the final step is a
   same-filesystem rename. The mode (e.g. `0755` for a hook) is set here.
2. **verify** — read the staged file back and confirm its SHA-256 equals the
   bytes we intended to write. A short write or a mangling filesystem is caught
   *before* the file goes live.
3. **activate** — `rename()` the verified staged file over the target. Rename is
   atomic: a reader (launchd, Claude Code) sees either the old file or the new
   one, never a truncated mix.

On any failure before activate the staged file is removed and the
previously-installed artifact is left untouched. There is **no force/bypass
flag** (ADR-031).

## Subcommands

| Command                        | Effect                                              |
| ------------------------------ | --------------------------------------------------- |
| `dear-deploy list`             | list every deployable artifact (source → deployed)  |
| `dear-deploy status [name...]` | show deployed state vs the manifest (exit 2 = drift) |
| `dear-deploy sync [name...]`   | deploy artifacts that have drifted (idempotent)     |
| `dear-deploy install [name...]`| (re)install artifacts even if unchanged             |

With no names, status/sync/install operate on the whole manifest.

Common flags: `--manifest FILE`, `--repo-root DIR`, `--home DIR`, `--json`,
`--dry-run` (sync/install).

Exit codes: `0` ok/clean · `2` (status) drift or a required artifact missing ·
`1` error.

## Examples

```sh
# What's deployable, and where does it go?
dear-deploy list

# Is the host in sync with main? (gate-friendly: exit 2 on drift)
dear-deploy status

# Build the write-guard hooks, then atomically deploy everything.
make dear-deploy-sync

# Preview a deploy without writing.
dear-deploy sync --dry-run

# Force a single artifact back to its source-of-truth state.
dear-deploy install com.dear-agent.mergeloop

# Audit just the Go daemons: is the deployed binary built from current source?
dear-deploy status mergeloop vroom-dispatch
```

## Relationship to `make drift-check` and `agm admin verify-deployment`

- `make drift-check` → `make deploy-status` → `dear-deploy status`: the
  manifest-driven drift audit. The legacy hash-only detector remains as
  `make drift-check-legacy` (`cmd/drift-check`) for the agm-hook targets not yet
  migrated into the manifest.
- `agm admin verify-deployment` checks the **running** `agm` process against
  `origin/main` ancestry. `dear-deploy status` checks **installed files on
  disk** (plists, hooks, daemon binaries) against the repo. The Overseer runs
  both every tick (overseer.md Steps 13 and 13.5).
