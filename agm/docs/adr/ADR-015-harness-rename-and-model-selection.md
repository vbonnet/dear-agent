# ADR-015: Separate harness and model selection

Status: Accepted (verified 2026-07-17)

## Context

The word `agent` was used for both a CLI runtime and the actor running inside
it. Model aliases are also not interchangeable across harness families.

## Decision

User-facing session creation names the runtime with `--harness` and selects its
model separately with `--model`. The harness registry is finite; the model
registry validates aliases per harness and maps only declared cross-harness
aliases. Legacy terminology may be read for compatibility but new output and
configuration use `harness`.

## Alternatives

Inferring a harness from model names is ambiguous. A single global model list
advertises combinations that cannot launch. Keeping `--agent` perpetuates a
domain collision.

## Consequences

Callers must supply two concepts when overriding defaults. Registry and CLI
validation tests verify supported combinations.
