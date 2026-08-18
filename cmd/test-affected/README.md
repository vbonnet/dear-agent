# test-affected

Print the Go test packages whose dependencies changed between a base ref
and HEAD. Designed to drive smart integration-test runs in CI: pipe the
output into `go test`, and you only run what the PR actually touches.

## Quick start

```
# From the repo root:
make test-affected        # run affected integration tests
make test-affected-print  # show what *would* run (no test execution)

# Direct invocation (build first so --run preserves exit code 124):
go build -o /tmp/test-affected ./cmd/test-affected
/tmp/test-affected --base=origin/main --tags=integration         # print
/tmp/test-affected --base=origin/main --tags=integration --run   # exec `go test`
```

## How it works

1. `git diff --name-only $BASE...HEAD` produces the changed file list.
2. A short **force-full** allowlist (`go.mod`, `go.sum`, `Makefile`,
   `.github/workflows/`, the selector itself) short-circuits to "run
   every test-bearing package."
3. Each remaining changed file is mapped to the containing Go package by
   walking up directories.
4. `go list -deps -test -json -tags=<tags> ./...` gives the dependency
   graph including test-only edges.
5. Every test-bearing package whose `Deps` (or `TestImports` /
   `XTestImports`) intersect the changed set is emitted on stdout.

Empty output is a valid pass — there genuinely are no integration tests
to run.

Falls back to running everything if `go list` errors. Smart selection is
**additive**; losing it should never block CI, only un-narrow it.

## Flags

| Flag        | Default        | Meaning                                                          |
|-------------|----------------|------------------------------------------------------------------|
| `--base`    | `origin/main`  | Git ref to diff against                                          |
| `--head`    | `HEAD`         | Git ref to diff towards                                          |
| `--tags`    | (empty)        | Comma-separated build tags forwarded to `go list`                |
| `--root`    | repo root      | Override repo root (default: `git rev-parse --show-toplevel`)    |
| `--all`     | `false`        | Emit *every* affected package, not just test-bearing ones        |
| `--run`     | `false`        | Exec `go test -race -count=1 -timeout=20m` on the selection instead of printing |
| `--verbose` | `false`        | Log per-package decisions to stderr                              |

`--run` gives each package the same 20-minute deadline as required CI and
local preflight, while bounding package discovery at 20 minutes and the
aggregate `go test` command at 55 minutes. The affected-integration workflow
backstops the complete job at 100 minutes.

## Trust boundary

The selector is **not** trusted to gate code that hasn't been exercised
on some other path. CI keeps running `go test ./...` (no tags) on every
PR in parallel with this. We add the property "green CI ⇒ integration
tests passed on the packages whose deps changed." We do not yet claim
"green CI ⇒ all integration tests pass" — that lives in a release
workflow.

See [ADR-028](../../docs/adr/ADR-028-smart-integration-test-selection.md)
for the full rationale.
