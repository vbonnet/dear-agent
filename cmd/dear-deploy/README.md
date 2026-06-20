# dear-deploy

<!-- Last audited at: 2026-06-19 -->

Deploys dear-agent's **host artifacts** — launchd plists and Claude Code hooks —
from their source of truth in this repo to their installed location on the
machine. It is the write-side counterpart to
[`drift-check`](../drift-check/README.md): drift-check *detects* that a deployed
artifact is stale; dear-deploy *makes it current*.

The deployable set lives in [`deploy/manifest.yaml`](../../deploy/manifest.yaml).

## Why

"Merged to main" is not "deployed on the host." When a redeploy step is skipped,
the fix lives in git but the machine stays broken — silently. drift-check catches
that gap; dear-deploy closes it through a single sanctioned, atomic command so
the redeploy is never a hand-typed `cp` that can half-write a file or be
forgotten.

## Atomic deploy (CLAUDE.md principle 9)

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
```
