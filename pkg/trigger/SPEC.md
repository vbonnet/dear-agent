# Trigger Runtime Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**TRIGGER-01** When trigger specifications are registered, the system shall index them by event type and preserve source path metadata.

**TRIGGER-02** When an event is matched, the system shall require every configured predicate and return matches in priority order.

**TRIGGER-03** When no event or predicate matches, the system shall return an empty match set.

**TRIGGER-04** When trigger scope is resolved, the system shall prefer Wayfinder identity, then git identity, then directory fallback.

**TRIGGER-05** When global, project, or session scope is evaluated, the system shall compare the corresponding normalized identifiers.

**TRIGGER-06** When cooldown is active, the system shall suppress repeated injection until the configured duration expires.

**TRIGGER-07** When trigger state is persisted, the system shall create its parent directory and restore recorded injection times.

**TRIGGER-08** When a matched Engram is injected, the system shall create the target directory and write the selected content deterministically.

**TRIGGER-09** While triggers run under any supported harness and model family, the system shall preserve identical matching, scope, cooldown, and injection behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/agent_utility_parity.feature`

## Test Traceability

- Unit package: `pkg/trigger`
