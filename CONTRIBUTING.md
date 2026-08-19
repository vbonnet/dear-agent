# Contributing to dear-agent

Thank you for your interest in contributing!

## Development Setup

### Prerequisites

- Go 1.25 or later
- tmux (for AGM integration tests)
- Git
- [act](https://github.com/nektos/act) (optional, for local CI)

### Clone and Build

```bash
git clone https://github.com/vbonnet/dear-agent.git
cd dear-agent
```

### Build Products

```bash
# AGM
go build -o bin/agm ./agm/cmd/agm

# Engram
go build -o bin/engram ./engram/cmd/engram

# Wayfinder
go build ./wayfinder/...

# Tools
go build -o bin/benchmark-query ./tools/benchmark-query
```

### Running Tests

```bash
# Run all tests
GOWORK=off go test ./...

# Run tests for a specific product
go test ./agm/...
go test ./engram/...
go test ./wayfinder/...

# Run tests for a shared package
go test ./pkg/costtrack/...
```

### Using the Root Makefile

```bash
# Run full local CI validation (lint + tests) via act
make act-validate

# Run lint only
make act-lint

# Run tests only
make act-test
```

## Pre-push Hook

Pushing to the default branch runs `make preflight` (lint + build + vet) and
aborts on failure, so a broken push fails fast before the GitHub round-trip.

On the maintainer host this is wired up globally via `core.hooksPath`
(`~/.config/git/hooks`, chezmoi-managed): the global `pre-push` hook runs
`make preflight` for any repo that defines a `preflight` target — no per-repo
install needed. (There used to be a `make install-preflight-hook` target; it was
removed in bead ce-hft2 because a repo-local `.git/hooks/pre-push` is silently
bypassed when `core.hooksPath` is set, making it a redundant no-op.)

On a host **without** a global `core.hooksPath`, run `make preflight` manually
before pushing, or drop a one-line `exec make preflight` pre-push hook into
`.git/hooks/`.

### What `make preflight` checks

| Check | Purpose |
|-------|---------|
| `go build ./...` | Catches compile errors |
| `go vet ./...` | Catches common Go mistakes |
| `golangci-lint run ./...` | Enforces code style and quality |

## Making Changes

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Make your changes
4. Run tests to ensure nothing is broken
5. Commit with a clear message (use [Conventional Commits](https://www.conventionalcommits.org/))
6. Push and open a Pull Request

## Code Style

- Run `gofmt` on all Go code
- Use `golangci-lint` for linting
- Follow standard Go project layout conventions
- Add package doc comments to new packages (see `pkg/*/doc.go`)

## Repository Layout

| Directory | What belongs here |
|-----------|-------------------|
| `agm/` | AGM session management product |
| `engram/` | Engram memory system product |
| `wayfinder/` | Wayfinder SDLC workflow product |
| `tools/` | Standalone CLI utilities |
| `cmd/` | Additional CLI entry points |
| `codegen/` | Code generation framework |
| `pkg/` | Shared packages (importable by external projects) |
| `internal/` | Private packages (not importable externally) |
| `scripts/` | Build, CI, and utility scripts |
| `docs/` | Cross-cutting documentation |

## Pull Request Process

1. Ensure all tests pass
2. Update documentation if needed
3. Describe what your PR does and why
4. Link any related issues

### Small, stacked PRs

Agents should prefer small, targeted pull requests that are easy to review and
test independently. When a change is large, split it into a GitHub stacked PR
series: land mechanical refactors, renames, generated updates, or pure test
scaffolding first, then put the risky behavior change in a focused follow-up PR.

#### Size budget

Aim for **at most 400 changed lines and at most 15 changed files** in one PR.

That is a *target to design toward*, not a limit to creep up to. It is derived
from what this repository already does: across the 200 most recent merges to
`main`, the median PR was 238 changed lines, and 59% already met both numbers.
It is a description of a normal change here, not an aspiration.

The CI thresholds are **ceilings, not targets**. `.github/workflows/pr-size-scope.yml`
comments once a PR crosses 1,000 changed lines, 50 changed files, or 4 top-level
areas. A PR at 999 lines is not "within budget" — it is four times over budget
and one line under the alarm. Do not treat the alarm as the goal.

Two reasons the budget is about review quality, not tidiness:

- **Human review stops happening.** Past a few hundred lines a reviewer skims
  rather than reads, and approval starts meaning "nothing obviously alarming"
  instead of "I checked this."
- **Agent review degrades too.** A large diff spends the reviewer's attention on
  bulk rather than on the few lines that carry the risk, so specific defects get
  missed in exactly the PRs where a miss is most expensive.

When a change genuinely cannot fit — a mechanical rename across many call sites,
a generated-file refresh, a vendored update — say so explicitly in the PR
description, and separate the mechanical part from anything hand-written so a
reviewer can skip the bulk and concentrate on the rest.

##### How to split, in order

1. **Mechanical first.** Renames, moves, generated output, formatting. These
   should be reviewable by confirming nothing changed but names and locations.
2. **Enabling refactor next.** New seams, extracted interfaces, signature
   changes — still no behavior change.
3. **Behavior last.** The actual new logic, built on names and seams that are
   already on `main`.

Land each step before opening the next, so every PR is based on `main` and gets
the full review protocol (see the caveat below). If you have already written the
whole thing in one branch, `git reset --soft <base>` and re-commit it in that
order rather than opening one PR that mixes all three.

Each PR in the stack must stand on its own: it should have a clear purpose,
pass the relevant tests, and be independently understandable from its diff and
description. Do not bundle unrelated concerns into one monster PR just because
they were discovered in the same session.

**Caveat — the five-dimension review protocol (see [REVIEW.md](REVIEW.md))
only triggers for PRs based on `main`.** A PR whose base is another open PR's
branch does not get that review at all.

Targeting `main` is necessary but **not** sufficient. In the current keyless
configuration `.github/workflows/review.yml` skips the gate when its plan
reports `review_relevant=false` and publishes a neutral result that says so
explicitly — so an ordinary stack member with no changed SPEC and no REVIEW.md
§3 escalation trigger gets a green-looking neutral check, not a review. Read
the check's text rather than its colour before concluding a PR was reviewed.

Two ways to stay honest about this:
- Open each stack member only after its predecessor has landed on `main`, so
  every PR in the sequence is itself based on `main`, or
- If you do stack branch-on-branch, **restack** each descendant onto `main`
  once its predecessor lands, then retarget its base. A `safe-merge` landing
  is a squash (`internal/safegit/merge.go` passes `--squash`), so the
  predecessor's new `main` commit is not an ancestor of a descendant created
  from the pre-merge branch — retargeting alone leaves the descendant's diff
  carrying its predecessor's changes again.

  Use `--onto`, not a plain rebase. A plain `git rebase main` still selects
  every one of the predecessor's commits for replay, because none of them is
  an ancestor of `main` by SHA after the squash — so a multi-commit
  predecessor conflicts against its own already-landed changes. Restack with
  the old parent tip as the cut point:

  ```sh
  git rebase --onto main <old-parent-tip> <descendant-branch>
  ```

  **This second option is not currently supported end to end — prefer the
  first.** Two gaps, both real:

  - `safe-rebase` has no `--onto` mode. It runs a plain `git rebase <base>`
    (`internal/safegit/rebase.go`, `attemptRebase`), so a squash-merged parent
    is exactly the case it handles badly. Restacking by hand therefore skips
    its latest-`origin/main` fetch, protected-branch check, automatic conflict
    abort, and audit trail.
  - A restack rewrites the branch, so landing it needs a force-push — and
    `safe-push` never force-pushes, by design. There is no sanctioned command
    that completes this workflow.

  Tracked as `ce-x2ekc`. Until it lands, stack only when you are prepared to
  land each predecessor first, or get a maintainer to complete the restack.
  Either way, wait for that PR's own exact-head review before merging, and
  don't rely on `safe-merge`'s fallback "validate all reported checks" path as
  a substitute for it.

**Bead tracking:** `safe-pr create` defaults every PR in a session to the
same first bead and stamps `Closes <bead>` — unless the PR body you pass
already mentions that bead ID anywhere, in which case `safe-pr` skips the
auto-stamp (see `internal/safepr.StampedArgs`/`referencesBead`). In a stack,
the default behavior closes the bead the moment the *first* PR merges, while
later stack members are still open — `agm pr scan-orphaned` will then flag
them as tracking a closed bead. Give each PR in a stack its own bead, or, to
share one bead across the stack without early-closing it, reference the bead
yourself in each non-final PR's body using non-closing language (e.g. "Part
of `<bead>`", not "Closes `<bead>`") so `safe-pr`'s auto-stamp is suppressed;
only the stack's final PR should carry an actual `Closes <bead>`.

## License

By contributing, you agree that your contributions will be licensed under the
Apache License 2.0.
