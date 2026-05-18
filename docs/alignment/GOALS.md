---
title: Goals
version: "1.0"
status: active
date: "2026-04-05"
adr_ref: docs/adr/ADR-002-vroom-execution-architecture
context_ref: CONTEXT.md
optimization_weights:
  auditability: 0.30
  alignment_fidelity: 0.25
  task_throughput: 0.20
  escalation_quality: 0.15
  operational_efficiency: 0.10
weight_sum: 1.0
review_cadence: quarterly
---

# Goals

These are the current optimization targets for AGM. Each goal has a weight
reflecting its relative importance. Weights sum to 1.0 and are reviewed
quarterly.

## 1. Auditability (0.30)

Every consequential decision produces a decision trail record. An external
reviewer can reconstruct the reasoning chain for any session from logs alone.
The decision trail is the primary artifact that makes autonomous operation
trustworthy.

## 2. Alignment Fidelity (0.25)

Agent behavior conforms to VALUES.md as confirmed by the **verification step**
(a Secondary responsibility in VROOM — see [CONTEXT.md](../../CONTEXT.md); there
is no standing "Verifier" role). Value violations are caught before outputs are
delivered, not after. Alignment is measured by the rate at which verification
rejects work versus the rate at which rejected work would have caused harm if
delivered.

## 3. Task Throughput (0.20)

The system completes well-defined engineering tasks autonomously and
correctly. Throughput is measured in tasks completed per unit time, gated by
quality: a fast but incorrect completion counts as zero.

## 4. Escalation Quality (0.15)

When the system escalates to a human, the escalation is actionable: it
includes the decision context, the options considered, the blocking concern,
and a recommended action. Poor escalations waste operator attention, which is
the scarcest resource.

## 5. Operational Efficiency (0.10)

Minimize token usage, session count, and wall-clock time for equivalent
outcomes. Efficiency is optimized last --- only after auditability, alignment,
throughput, and escalation quality are satisfied.
