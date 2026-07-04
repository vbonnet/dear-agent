# AGM Session Manager Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/manager` defines the newer session-management abstractions for
backend-neutral lifecycle, messaging, state observation, attachment, and
backend registration. The package is the contract layer for moving AGM beyond
direct tmux calls while preserving harness-neutral semantics.

## EARS Requirements

**MGR-01** When a backend implements session lifecycle operations, the system shall expose create, terminate, list, get, and rename through `SessionManager`.

**MGR-02** When a backend implements message delivery, the system shall expose send, recent-output read, and interrupt through `MessageBroker` with caller-supplied context.

**MGR-03** When a backend observes state, the system shall report state, confidence, evidence, delivery readiness, and backend health through `StateReader`.

**MGR-04** When a backend factory is registered, the system shall reject nil factories and duplicate backend names.

**MGR-05** When a backend is requested with an empty name, the system shall resolve the default backend name before looking it up.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
