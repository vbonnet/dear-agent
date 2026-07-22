# Wayfinder validator architecture

<!-- Last audited at: 2026-07-17 -->

The validator is the deterministic transition boundary for canonical Wayfinder
status. It does not execute a simulated workflow or manufacture successful
review, test, deployment, or monitoring outcomes.

## Start gates

`CanStartPhase` checks:

1. the named phase exists;
2. it is not already active or complete;
3. the nearest preceding non-skipped phase is complete;
4. DESIGN has bounded, substantive RESEARCH evidence.

## Completion gates

`CanCompletePhase` checks, in order:

1. the phase exists and is in progress;
2. a `<PHASE>-*.md` deliverable exists and is substantive;
3. methodology freshness, pending questions, and phase boundaries;
4. git claims, compilation, child projects, and phase-specific gates;
5. document quality and code-deliverable verification where applicable.

SPEC uses a deterministic strict-EARS parser. PLAN uses the architecture review
adapter when configured. Document review removes one canonical leading YAML
frontmatter block before checking the Markdown body; a malformed leading block
fails closed. BUILD rejects placeholder-only evidence and runs bounded
build/test checks for discovered code.

## Trust boundaries

- Files are size-bounded before parsing.
- Paths are contained inside the project directory.
- External commands use fixed argument construction and timeouts.
- Provider-backed reviews supplement deterministic checks; they do not replace
  phase ordering or artifact validation.
- Missing or malformed active status is an error, not implicit success.

## Extension rule

Add a gate only for an observable phase invariant. Put its behavior in
`SPEC-solution-requirements.md`, implement it as a focused function, and cover both accept and
reject paths. Do not add prose-only thresholds or a second state machine.

## Verification

```sh
go test ./wayfinder/cmd/wayfinder-session/internal/validator
go test ./agm/test/bdd -run TestFeatures
```
