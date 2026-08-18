# ADR-031: Consumer-owned harness capabilities

Status: Accepted (2026-07-27)

## Context

AGM's harness adapters accumulated one package-level `Agent` interface covering
metadata, create, resume, terminate, readiness, message delivery, history,
import, export, and generic commands. No production consumer required that
whole surface. The facade made unrelated behaviors appear substitutable,
required runtime assertions for capabilities that were actually mandatory, and
coexisted with two adapter registries.

## Decision

`agent.Harness` is the heterogeneous discovery and conformance boundary. It
contains only canonical name, version, and descriptive capabilities.

Adapter constructors return concrete adapter types. A consumer that needs
behavior defines the smallest interface at the point that owns the operation.
For example, pure API delivery requires only context-aware readiness and
context-aware message delivery through `ops.APISessionDeliveryAdapter`.
Harness-specific lifecycle primitives remain available on their concrete
adapters while cross-surface transactions remain in `internal/ops`.

The finite constructor catalog in `factory.go` is the only heterogeneous
adapter catalog. AGM does not maintain a second mutable runtime registry.

## Alternatives

Keeping the universal facade preserves a superficially uniform API but encodes
substitutability AGM does not use. Splitting every adapter method into
package-owned one-method interfaces merely relocates the same abstraction and
forces consumers to depend on contracts they do not own. Making all consumers
depend on concrete types prevents useful, focused test doubles.

## Consequences

Discovery and workflow selection can compare harness metadata without acquiring
lifecycle behavior. Required operational capabilities fail at compile time
rather than after a runtime type assertion. Harness-specific callers can use
their concrete adapter directly, and shared operations can define narrow test
seams.

Adding a new cross-harness operation now requires an explicit consumer-owned
capability contract. This creates more small interfaces, but each one documents
an actual operation instead of predicting a universal harness abstraction.
