# BDD / SPEC Verification Suite for AGM

This directory makes AGM's SPECs executable. Gherkin feature files encode
SPEC invariants as behaviour, and `godog` (Cucumber for Go) runs them against
the real `internal/ops` and `internal/contracts` packages — no mock adapters,
no API keys.

## The one rule: every feature file runs

`TestFeatures` (in `main_test.go`) executes **every** `.feature` file under
`features/` with **no tag filter**. A scenario whose steps are not implemented
fails as `undefined` instead of being silently skipped. This is deliberate:

> If a BDD test exists, it MUST run.

There is no `@implemented` gate and no "backlog" tag. If you add a feature
file, add its step definitions in the same change, or the suite goes red.
Dead/aspirational specs cannot accumulate here again.

## What's covered

Three feature files, all driven by real implementation code:

| Feature                    | Drives                                              |
|----------------------------|----------------------------------------------------|
| `trust_protocol.feature`   | `ops.TrustRecord` / `TrustScore` / `TrustHistory`  |
| `scan_loop.feature`        | `ops.DefaultCrossCheckConfig`, SLO contract values |
| `stall_detection.feature`  | stall-type invariants, SLO contract thresholds     |

Alongside the godog runner, `spec_invariants_test.go` holds:

- `TestSPECInvariants_*` — Go assertions of SPEC invariants against the
  implementation.
- `TestContractDrift` — runs the contract-drift checker against the live SPEC
  files.

## Directory structure

```
test/bdd/
├── features/                 # Gherkin feature files (all of them run)
│   ├── trust_protocol.feature
│   ├── scan_loop.feature
│   └── stall_detection.feature
├── steps/                    # godog step definitions (one file per feature)
│   ├── trust_protocol_steps.go
│   ├── scan_loop_steps.go
│   └── stall_detection_steps.go
├── main_test.go              # godog runner — registers the step groups
├── spec_invariants_test.go   # SPEC invariant + contract-drift Go tests
└── README.md                 # this file
```

## Running

```bash
cd agm

make test-bdd          # godog feature tests (TestFeatures)
make verify-contracts  # contract drift only (TestContractDrift)

# everything in this package (what CI runs via `go test -race ./...`):
go test ./test/bdd/...
```

## Adding a scenario

1. Add the `Scenario` to an existing feature file (or create a new
   `features/<name>.feature`).
2. Implement every step in a `steps/<name>_steps.go` file. Each step group
   registers its own per-scenario `ctx.Before` for state — there is no shared
   test environment to wire.
3. If you created a new feature file, register its step group in
   `InitializeScenario` (`main_test.go`).
4. Run `go test ./test/bdd/...`. An unimplemented step shows up as `undefined`
   and fails the build — that is the point.

## CI

The root `ci.yml` "Build & Test" job runs `go test -race -count=1 ./...`,
which executes this package (godog features, invariant tests, contract drift)
on every PR. There is no separate opt-in target to forget to call.
