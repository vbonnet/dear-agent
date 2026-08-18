# AGM Send Delivery Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/send` provides injectable message delivery orchestration for AGM
send operations. It preserves delivery order, records per-recipient outcomes,
and avoids unsafe parallel tmux writes by making multi-recipient delivery
sequential.

## EARS Requirements

**SEND-01** When no delivery jobs are supplied, the system shall return no delivery results and shall not invoke the delivery function.

**SEND-02** When no delivery function is supplied, the system shall use the configured default delivery function.

**SEND-03** When multiple delivery jobs are supplied, the system shall deliver them sequentially in input order.

**SEND-04** When context cancellation is observed before a job is delivered, the system shall record a cancelled result for that job and continue recording outcomes for remaining jobs; when delivery begins, the system shall pass the same caller context into the delivery function so cancellation can interrupt command-scoped I/O within that job.

**SEND-05** When a delivery function returns an error, the system shall record a failed result with the original recipient, message ID, duration, and error.

## BDD Traceability

- Feature: `agm/test/bdd/features/harness_parity.feature`
