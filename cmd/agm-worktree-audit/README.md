# agm-worktree-audit

Read-only audit that scans every git repository under a root directory (default
`~/src`) and reports worktrees and branches that look reclaimable. It never
removes a worktree or deletes a branch. Its job is to produce a clear report
for deciding what to clean up. For linked worktrees beneath its configured
`--worktrees-dir`, the dry-run-default `agm worktree sweep` can reclaim only
worktrees positively classified as clean and merged after fail-closed
active-session checks. A successful execute-mode removal also force-deletes the
selected worktree's local branch so squash-merged branches can be reclaimed;
it never deletes remote branches. Findings outside that configured base remain
report-only and require a separately reviewed, repository-scoped cleanup path.

It is the periodic audit half of Beads `ce-ank`.

## Categories

| Kind                 | Meaning                                                          |
|----------------------|------------------------------------------------------------------|
| `abandoned-worktree` | worktree whose HEAD has no commit within `--worktree-days` (7)   |
| `worktree-no-remote` | worktree branch with no matching `origin/<branch>` (local-only)  |
| `merged-not-deleted` | local branch already merged into the base ref, still present     |
| `stale-unmerged`     | unmerged local branch untouched for ≥ `--branch-days` (14)       |

For each finding the report shows: repo, branch, last commit date, age,
ahead/behind the base ref (`+ahead/-behind`), and whether it is merged.

The base ref is resolved per repo as `origin/HEAD` → `origin/main` →
`origin/master` → `main` → `master`. Repos where none resolve are flagged at
the bottom of the report; their merge/ahead-behind data is unavailable.

> **Squash merges:** a squash-merged branch reports `merged-not-deleted=false`
> with `ahead>0`, because its commits differ from the base even though its
> content is already there. This is deliberate — it avoids false positives.
> Such branches typically surface under `stale-unmerged` once they age out.

## Usage

```sh
go run ./cmd/agm-worktree-audit                 # text report over ~/src
go run ./cmd/agm-worktree-audit --json          # machine-readable
go run ./cmd/agm-worktree-audit --root ~/work   # different root
go run ./cmd/agm-worktree-audit --worktree-days 14 --branch-days 30
```

Exit codes: `0` success, `1` runtime error, `2` usage error.

## Design

`audit.go` holds the pure categorization core (`Categorize`) — no I/O, fully
unit-tested in `audit_test.go`. `collect.go` runs the git commands that turn a
repo into the structs `Categorize` consumes. `main.go` is the CLI and the text
/ JSON renderers. The split keeps the decision logic testable without a git
fixture.
