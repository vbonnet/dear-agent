# ADR-027: BDD Enforcement Policy — go test over Bazel

**Status:** Accepted
**Date:** 2026-06-12
**Deciders:** vbonnet (owner), AGM Foundation Engineering
**Context:** Resolves the policy ambiguity left open by cleanup PR
`cleanup/bdd-dead-scenarios`, which deleted 21 of 24 aspirational feature
files and removed the `@implemented` tag gate that had silently suppressed
them. That cleanup closed the immediate dead-spec limbo; this ADR records the
enforcement model going forward.

Closes: ce-1ak (BDD enforcement policy decision)

---

## Problem

BDD feature files in `agm/test/bdd/features/` are only useful if they run. The
previous regime gated scenarios behind `@implemented`; 21 of 24 feature files
were silently skipped, accumulating aspirational specs that diverged from the
real code. The cleanup PR removed the tag gate, but the structural question
remained: **how do we prevent the next batch of unimplemented specs from
accumulating unnoticed?**

Two options were considered: (A) keep the current go-test approach with an
explicit convention, or (B) adopt Bazel so that feature files with no step
implementations are caught at build-analysis time.

---

## Options

### A — go test + convention (chosen)

`TestFeatures` (in `main_test.go`) discovers every `.feature` file under
`features/` at runtime and runs it. A scenario whose step definitions are not
registered fails with `undefined` rather than being silently skipped. No new
tooling, no build-system change.

**Enforcement mechanism:** the comment in `main_test.go` is the policy contract:

```go
// TestFeatures runs every Gherkin scenario under features/. There is no tag
// filter on purpose: any feature file that exists in this directory MUST run.
// A scenario whose steps are not implemented fails as "undefined" rather than
// being silently skipped, so dead/aspirational specs cannot accumulate. If you
// add a feature file, add its step definitions in the same change.
```

Adding a feature file without step definitions fails CI (the `go test`
step in `ci.yml` runs the full suite including BDD). Adding step definitions
without a feature file produces a compiler warning (unused exported function)
that golangci-lint catches.

**Pros:** zero new tooling; uniform with `make preflight` / `go test ./...`;
no adoption cost; CI already enforces it.

**Cons:** structural enforcement is implicit in test-runner discovery, not
explicit at build/analysis time. A feature file placed outside `features/` (a
wrong directory) would be missed silently — convention guards against this.

### B — Bazel

Model each `.feature` file and its step dependencies as explicit Bazel targets.
A feature with no implemented steps fails at analysis time; orphaned step files
are visible as unused targets.

**Pros:** structural, hermetic, fails earlier than runtime.

**Cons:** introduces Bazel into a `go test`-based monorepo. Conflicts with
`make preflight` uniformity. Large adoption cost for a medium-sized BDD suite.
The structural guarantee it adds over Option A is narrow: the remaining failure
mode (wrong directory) can be caught by a simpler `spec_invariants_test.go`
check instead.

---

## Decision

**Option A (go test + convention)**, with one addition: a `spec_invariants`
test that validates the structural constraint Bazel would have enforced at
analysis time — specifically, that every `.feature` file is discoverable by
`TestFeatures`.

The invariant is: all `.feature` files live under `agm/test/bdd/features/`,
which is already the path `TestFeatures` passes to godog. If a future
contributor puts a feature file in the wrong directory it would be silently
skipped; `spec_invariants_test.go` exists precisely to catch this.

See `agm/test/bdd/spec_invariants_test.go` for the current invariant suite.

---

## Consequences

### Immediate

- No new tooling introduced.
- `TestFeatures` comment is the single authoritative policy statement.
- CI (`go test ./agm/test/bdd/...`) catches undefined steps automatically.

### Follow-up work (none required to close ce-1ak)

- If the feature set grows beyond ~10 files and the "wrong directory" risk
  feels real, add a `filepath.WalkDir` check to `spec_invariants_test.go`
  that fails if any `.feature` file exists under `agm/test/bdd/` but outside
  `agm/test/bdd/features/`.
- Bazel remains an option if the repo adopts it for other reasons (e.g. a
  multi-language build need). The BDD enforcement benefit would then come for
  free with no additional per-file boilerplate.

---

## References

- `agm/test/bdd/main_test.go` — TestFeatures (the enforcer)
- `agm/test/bdd/spec_invariants_test.go` — structural invariants
- Cleanup PR `cleanup/bdd-dead-scenarios` — removed @implemented tag gate
- Bead ce-1ak — this decision was recorded here
