# ADR-032: Supervisor escalation chain

Status: Accepted (2026-06-18; verified 2026-07-17)

## Context

Workers need a durable way to ask for decisions they cannot make. Direct human
prompts do not work in every harness, and free-form supervisor messages lose
classification, ancestry, and outcome evidence.

## Decision

`pkg/vroom/escalation` owns the harness-neutral escalation state machine,
policy classification, storage, and audit log. `agm escalate` adapts it to AGM
session ancestry and messaging.

An escalation walks from worker to spawning supervisor and then through the
VROOM supervisor chain. Policy marks product, pricing, security, destructive,
legal, spend, people, and external-communication decisions as
must-reach-human; non-human nodes may recommend but cannot answer them. Clearly
routine permission to continue the assigned task may auto-resolve.

## Alternatives

Harness-specific question tools can deadlock or exclude other harnesses. Sending
plain text to a fixed supervisor loses ancestry and high-stakes enforcement.
Routing every question to a human creates avoidable latency.

## Consequences

The system stores more operational state and relies on correct parent-session
links. Escalation policy, quorum/confer behavior, AGM commands, and supervisor
drain paths are tested in their owning packages.
