# dear-agent agent instructions

This file is the cross-harness entrypoint for work in this repository. Keep it
as a router: durable policy belongs in the canonical policy files, command
details belong in `--help`, and subsystem facts belong beside their code.

## Canonical policies

Read these before consequential work. They take precedence over examples and
older subsystem documentation:

- [Broken Windows](docs/policies/broken-windows.ai.md): a replacement removes
  the retired path, flags, tests, and documentation in the same change.
- [Harness Hygiene](docs/policies/harness-hygiene.ai.md): keep one owner per
  rule, turn hard requirements into checks, and continuously earn complexity.
- [SPEC authoring](docs/spec-authoring.md): write observable contracts once in
  a harness- and implementation-neutral owner; use applicability for real
  member variation.
- [Anti-stall](docs/policies/anti-stall.ai.md): continue through known work and
  stop only at explicit safety, authority, failure, or completion boundaries.
- [DEAR retrospectives](docs/policies/dear-retro.ai.md): systemic defects get a
  short prevention-focused retro in the configured research repository.
- [Definition of Done](docs/policies/definition-of-done.ai.md): done means
  merged, deployed where applicable, and verified in the real system.
- [Wayfinder canonical workflow](docs/policies/wayfinder-v2-canonical.ai.md):
  the nine-phase workflow is the only active Wayfinder model.
- [Autonomous merge](docs/policies/autonomous-merge.ai.md): routine changes may
  merge after all gates pass; security, product-behavior, and money changes
  require a human merge.

Repository policy never overrides system, user, or orchestrator instructions.
When two repository documents disagree, stop relying on the example, verify the
current code, and repair or quarantine the stale living document in scope.

## Start a task

1. Read [`.dear-agent.yml`](.dear-agent.yml) for artifact routing and declared
   acceptance criteria. `agm acceptance show` renders the latter.
2. Track the work in the canonical Beads store. Always use the explicit form:

   ```bash
   bd --db ~/beads/context-engine/.beads --dolt-auto-commit on <subcommand>
   ```

3. Treat `~/src/dear-agent` as a read-only source checkout. Create a branch in
   `~/worktrees/dear-agent/` and make all edits there.
4. Use the canonical Wayfinder workflow for consequential work. Keep its run
   artifacts in a worktree created from the research repository identified by
   [`.dear-agent.yml`](.dear-agent.yml), never in this repository.
5. Keep one concern per scoped plan and PR. Record unrelated defects in Beads
   instead of expanding the current diff.

## Core Engineering Principles

- Correctness, privacy, and security outrank speed or cost.
- Go is the default for code we own. Rust or TypeScript require an explicit
  ecosystem justification; do not add Python.
- Agent-authored code inverts the human-ease tax: Python, prose docs, and
  simple choices were concessions to human convenience. When agents write
  and maintain the code, typed languages, living docs, and complex policy
  languages (e.g. Rego/OPA) become *more* viable, not less. Their complexity
  must remain continuously earned under the Harness Hygiene review lens;
  deterministic enforcement and a deterministic Definition of Done remain
  delivery gates (see
  [`harness-hygiene.ai.md`](docs/policies/harness-hygiene.ai.md) and
  [`definition-of-done.ai.md`](docs/policies/definition-of-done.ai.md)).
  Without that lens and both gates, the tax is just paid by whoever debugs it
  later.
- Prefer deep modules and typed commands over long prompts, shell pipelines,
  or duplicated flag catalogs.
- Use deterministic, positive enforcement: state what was attempted, the safe
  path, and why it exists.
- A permission or access denial gets no retry or workaround. Defer or escalate
  it and continue only with independent safe work.
- After the same approach fails twice, switch approaches or report the block.
- Follow the [anti-stall contract](docs/policies/anti-stall.ai.md): continue through
  known work, accept empty results, and stop only at its explicit boundaries.
- Supervisor and user redirects are commands. Acknowledge promptly, preserve
  work in a commit, and comply.

## Repository map

| Surface | Current source of truth |
|---|---|
| AGM CLI and command schema | `agm/cmd/agm/` |
| AGM shared operations | `agm/internal/ops/` |
| Harness contract and active set | `agm/internal/agent/interface.go`, `agm/internal/agent/harnesses.go` |
| Engram memory implementation | `engram/` |
| Wayfinder workflow and validator | `wayfinder/` |
| VROOM typed control commands | `cmd/vroom-*`, with role prompts in `agm/supervisors/` |
| Shared packages | `pkg/` |
| Repository-only implementation | `internal/` |
| Living architecture and decisions | nearest `ARCHITECTURE.md` and ADR directory |

Do not restate an interface, harness list, or CLI flag inventory here. Inspect
the linked source or run the owning command with `--help`.

## Build and verification

Use the narrowest relevant gate during development and the repository gates
before publication:

```bash
make preflight          # module download, vet, build, lint
make test-affected      # tests selected from the diff
make lint-specs         # strict EARS validation
make preflight-full     # full local publication gate
```

Targeted Go tests remain valid for iteration. Tests that create Engram sessions
must use `ENGRAM_TEST_MODE=1` and `ENGRAM_TEST_WORKSPACE=test`. CI runs the
single root module with `GOWORK=off`.

Do not copy help output into living prose. For current command forms use, for
example, `agm --help`, `agm session --help`, `agm acceptance show`,
`wayfinder session --help`, and the safe-wrapper `--help` output.

## Guarded delivery

- Commit each logical unit. Do not claim uncommitted work as progress.
- Publish branches with `safe-push`.
- Rebase stale branches with `safe-rebase`; do not resolve semantic conflicts
  automatically.
- Create or close PRs with `safe-pr` and an active Wayfinder project. If no
  approved project exists, escalate with `agm escalate`; there is no bypass
  mode.
- Read PR state and checks with the read-only GitHub commands. Resolve review
  threads with `resolve-review-threads` after addressing the underlying issue.
- Merge eligible routine PRs with `safe-merge`. Create the human-required
  categories named by the autonomous-merge policy with `safe-pr create --draft`;
  do not mark them ready or arm auto-merge. A human owns that transition and
  the merge.
- Keep the Bead `in_progress` while its PR is open. Close it only after the
  merged commit is deployed where applicable and verified against the real
  surface.

Never bypass a safety wrapper by reconstructing its raw command chain. If a
wrapper is missing a required capability, track and repair the wrapper.

## Living Documentation Policy

This repository contains current, normative documentation only:

- Keep `AGENTS.md`, `ARCHITECTURE.md`, ADRs, specifications, and API docs close
  to the code they govern.
- Route plans, research, audits, retrospectives, roadmaps, and Wayfinder run
  state to an `engram-research` worktree according to [`.dear-agent.yml`](.dear-agent.yml).
- A verification date is a claim that every owned code path, command, link, and
  volatile fact was checked. Never stamp a partial review.
- A wrong living-document claim is a defect: correct it, capture the systemic
  cause in a DEAR retro, and add a mechanical prevention when possible.
- Prefer stable invariants and verification commands over latency, coverage,
  test-count, or cost claims without dated reproducible evidence.

Harness-specific root files are import shims. Shared policy changes belong here
or in the seven canonical policy files, not in a harness shim.
