# Wayfinder architecture

<!-- Last audited at: 2026-07-17 -->

Wayfinder is a filesystem-backed Go CLI. Its narrow public interface is
`wayfinder session`; its durable state is `WAYFINDER-STATUS.md`.

## Data flow

```text
operator or agent
  -> wayfinder session command
  -> canonical status parser
  -> transition validator
  -> atomic status/history update
  -> optional tracker and event publication
```

The command layer is in `cmd/wayfinder-session/commands`. The status package
owns schema 2.0 parsing and the nine named phases. The validator package owns
deterministic phase gates. Tracker, history, review, lint-context, and
telemetry packages are adapters around that core.

## State boundary

`WAYFINDER-STATUS.md` contains YAML frontmatter with:

- `schema_version: "2.0"`;
- project type, risk, lifecycle, and current phase;
- phase history and outcomes;
- optional roadmap tasks and quality evidence.

Parsing is fail-closed. Missing or unsupported schema versions do not fall back
to another model. Commands write through the canonical status serializer;
callers must not mutate the file directly.

## Validation boundary

Starting a phase requires the nearest preceding non-skipped phase to be
complete. Completing a phase requires an in-progress phase, a meaningful
artifact, and all phase-specific gates. BUILD additionally requires real
implementation evidence and successful applicable checks.

The stop hook reads the same canonical state. Invalid active state blocks
stopping; absence of a session remains a no-op.

## Extension rules

- Add a command under the existing Cobra session tree; do not create a second
  state machine.
- Add observable behavior to a co-located strict-EARS `SPEC.md` and tests.
- Keep provider-specific review adapters behind deterministic validators.
- Preserve the named phase vocabulary and schema 2.0 compatibility boundary.
- Put migration history, audits, and design exploration in Engram Research,
  outside active runtime instructions.

## Verification

```sh
go test ./wayfinder/...
go test ./agm/test/bdd/steps
go test ./agm/test/bdd -run TestFeatures
```
