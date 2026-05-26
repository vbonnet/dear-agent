# ADR-024: Smart Integration Test Selection

**Status**: Accepted
**Date**: 2026-05-26
**Context**: Every PR re-runs the entire `./...` test suite on both Linux
and macOS runners. The Go-tagged integration suites (`//go:build
integration`, plus `e2e` and `contract`) were defined but never wired into
CI, partly because running all of them on every PR would compound the
existing wall-clock cost. We need a way to run them where they pay off —
on PRs that actually touch the code they cover — without giving up the
"green CI means green code" property.

---

## Context

`go test -race -count=1 ./...` runs unconditionally on every PR via
`.github/workflows/ci.yml`. That covers ordinary unit tests, but the
`integration`-tagged tests under `agm/test/integration/`,
`agm/test/regression/`, `internal/sandbox/`, `engram/internal/agent`, and
`engram/internal/guidance` are skipped because the tag isn't passed. The
result is a two-class system: a quick test suite that ships on every PR,
and an integration tier that mostly runs in private worktrees.

The naive fix — pass `-tags=integration` in CI — is bad. Tmux-driven and
multi-process tests are slow, sometimes flaky on the macOS runner (see
[memory note: dear-agent CI flakes](../../README.md)), and produce noise
on PRs that don't touch the code path. We want fine-grained selection.

GitHub Actions' `paths:` filter is the simplest fan-out, and we already use
it for `agm-e2e-install.yml`. It's a coarse instrument: a change under
`agm/` triggers everything under `agm/`, even tests of cousin packages
that don't import the changed file. Worse, it misses real impact when a
shared helper under `pkg/` or `internal/` is modified and an integration
test in `agm/` actually depends on it.

Go's tooling already knows the dependency graph; we just have to query
it.

---

## Decision

Introduce a small Go command, `cmd/test-affected`, that computes the set
of *test-bearing packages* whose transitive dependencies changed between a
base ref and HEAD, and run only those under `-tags=integration` in CI.

### Algorithm

1. `git diff --name-only $BASE_REF...HEAD` produces the list of changed
   repo-relative paths.
2. A small **force-full** allowlist (`go.mod`, `go.sum`, `go.work*`,
   `Makefile`, `.github/workflows/`, the selector's own source)
   short-circuits to "run every test-bearing package." The rule: if a
   change can plausibly affect any test we can't see in the graph, we
   run everything. Keep this list short and obvious.
3. For each remaining path, walk up directories until we hit a known
   package directory (per `go list`'s `Dir` field), record the package
   as **changed**.
4. `go list -deps -test -json -tags=<tags> ./...` produces the full
   package graph including test-only edges. For every test-bearing
   package (`TestGoFiles` or `XTestGoFiles` non-empty) in the main
   module, check whether any element of its `Deps`/`TestImports`/
   `XTestImports` intersects the **changed** set. If so, the package is
   affected.
5. Emit affected packages one per line on stdout. Empty output is a
   **valid pass** — there genuinely are no integration tests to run.

### Wiring

- `cmd/test-affected --run` is the single entry point: with `--run` it
  execs `go test`, without it just prints the package list. No shell
  wrapper sits between the Go tool and the caller — keeping the
  orchestration in Go means it survives `bash -e`, doesn't trip the
  20-line bash policy, and is unit-testable.
- `make test-affected` shells out to `go run ./cmd/test-affected
  --base=origin/main --tags=integration --run`. `make
  test-affected-print` omits `--run` to show what *would* run.
- A new `integration-tests` job in `ci.yml` runs only on `pull_request`
  events. It checks out with `fetch-depth: 0`, fetches the base ref by
  name, installs tmux, and invokes the Go tool directly. The unit-test
  matrix is untouched.
- The selector defaults to falling back to "run everything" if it
  errors. Smart selection is *additive* — losing it should never block
  CI, only un-narrow it.

### Build tags

Integration / e2e / contract tests live behind `//go:build` tags. The
selector takes `--tags=` and forwards it both to `go list` (so it can see
the test-bearing packages at all) and to the final `go test` invocation.
The first CI job opts in to `integration` only; `e2e` and `contract` can
follow once we have appetite for the wall-clock cost.

---

## Consequences

**Positive**

- PRs that touch unrelated subsystems no longer spend wall-clock time on
  tmux/sandbox/parity tests they don't influence. The savings scale with
  how independent the touched code is from the integration suite.
- Integration tests stop being a graveyard tier: we get the value of
  running them on the PRs that actually exercise the dependency edges,
  without paying the full cost on every PR.
- The selector is a Go program, not a YAML pattern soup. It has unit
  tests for every decision branch (`cmd/test-affected/main_test.go`) and
  fails closed: a selector bug falls back to running everything.

**Negative**

- Adds a new piece of infrastructure that PR authors have to understand
  when an integration suite *doesn't* run. The
  `test-affected-print`/`--dry-run` paths exist to make "why was this
  skipped?" debuggable, but it is a step beyond "CI just runs
  everything." Anyone confused by a skip can run `make
  test-affected-print` locally to see the live decision.
- Force-full paths are conservative by design but coarse. A
  Makefile-only change re-runs every integration package; a workflow-file
  comment change does too. Acceptable cost: those changes are infrequent
  and the alternative is silently mis-running tests.
- `go list -deps -test -json ./...` is not free — it walks the entire
  graph. Empirically a few seconds in this repo; small compared to the
  test runs it scopes. Cache the binary in `bin/` if it becomes hot.

**Trust boundary**

The selector is *not* trusted to gate code that hasn't been exercised on
some other path. It runs in parallel with the existing full unit-test
matrix, which still runs `./...` without tags. We keep the property
"green CI ⇒ unit tests passed on Linux and macOS"; we add the property
"green CI ⇒ integration tests passed on the packages whose deps
changed." We do not yet claim "green CI ⇒ all integration tests pass" —
that lives in a release workflow.

---

## Alternatives considered

- **Pure path-based filtering** (extend the `agm-e2e-install.yml`
  pattern): simple, no Go code. Rejected because it misses
  cross-directory dependency edges (a change to `internal/foo`
  legitimately impacts `agm/test/integration` when the latter imports
  it) and overshoots within a directory (every change under `agm/`
  triggers every `agm` integration test).
- **`bazel test`-style sandbox**: enormous lift for a non-Bazel repo.
- **Per-PR labels** (`/integration` to opt in): puts the cognitive load
  on the author. We want the default to be "run what the change
  actually affects," not "opt in to safety."
- **Run `-tags=integration` on every PR, no filtering**: rejected on
  wall-clock and flake grounds. If the integration suite ever becomes
  fast and stable enough to run unconditionally, this selector becomes
  redundant — and the rollback is one CI-file edit.

---

## Follow-ups

- Extend the smart selection to `e2e` and `contract` once integration is
  stable. The selector accepts `--tags=integration,e2e,contract`
  already; only the CI wiring is per-tag.
- Consider a nightly cron that runs the full `-tags=integration ./...`
  on `main` so we catch dependency drift that the per-PR selector
  cannot see (e.g., a refactor that legitimately invalidates a test
  package whose `Deps` it didn't pass through).
- Wire `make test-affected` into the pre-push hook (`install-hooks`) so
  the same selection runs locally before a PR opens.
