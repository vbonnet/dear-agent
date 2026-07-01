---
schema: ai.md/v1
type: guide
status: active
created: 2026-07-01
tokens: 220
title: Wayfinder V2 Is Canonical
description: Wayfinder V2 (9 phases) is the only SDLC model. V1 (13 phases) is dead — do not use or extend it.
tags: [policy, wayfinder, sdlc]
---

# Wayfinder V2 Is Canonical

Wayfinder V2 consolidated the SDLC to **9 phases**: CHARTER, PROBLEM, RESEARCH,
DESIGN, SPEC, PLAN, SETUP, BUILD (TDD loop), RETRO. V1's 13-phase model is retired.

## NEVER

- Use or extend the Wayfinder V1 (13-phase) path.
- Add new phase logic, validators, or defaults to V1.
- Leave code defaulting to V1 while calling V2 canonical.

## ALWAYS

- Treat V2's 9 phases as the only model; drive PRs through V2 + `safe-pr`.
- When you touch Wayfinder phase logic, make V2 the default and delete the V1
  path completely (see broken-windows).
- Keep the SPEC phase's deterministic EARS gate — it is a feature, not a stub.

## REMINDER

Wayfinder gates PRs. V1/V2 ambiguity means the gate behaves unpredictably. There
is one model — V2 — and the transition is not "in progress," it is done.
