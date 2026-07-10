# VROOM Escalation Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Escalation classification, routing, persistence, adjudication, and analysis.

## EARS Requirements

**VROOM-ESC-01** When an escalation engine is created without a required graph, messenger, human dispatch, store, or logger dependency, the system shall reject the configuration.

**VROOM-ESC-02** When an escalation request lacks an origin session, question, valid kind, or valid mode, the system shall reject the request before routing it.

**VROOM-ESC-03** When the classifier auto-resolves an escalation, the system shall persist the answer and shall not route the escalation to a supervisor.

**VROOM-ESC-04** When an escalation has a valid parent, the system shall route it to that parent and record the hop in its chain.

**VROOM-ESC-05** When an escalation reaches the end of its chain, a cycle, or its hop limit, the system shall dispatch it to the human or the configured VROOM entry.

**VROOM-ESC-06** When a must-reach-human escalation is answered by a non-human session, the system shall reject the answer.

**VROOM-ESC-07** When an escalation is answered, the system shall persist the terminal answer and shall notify the origin session.

**VROOM-ESC-08** When a blocking caller waits past its deadline, the system shall return `ErrAwaitTimeout` without resolving the escalation.

**VROOM-ESC-09** When an approval policy category has no name, no patterns, or an invalid regular expression, the system shall reject the policy.

**VROOM-ESC-10** When text matches a human-required approval category, the system shall prevent automatic approval and shall return the first matching category.

**VROOM-ESC-11** When mutable escalation state is stored in memory, the system shall copy chain data on writes and reads so callers cannot mutate stored state by alias.

**VROOM-ESC-12** When mutable escalation state is stored on disk, the system shall write through a temporary file and atomically rename the complete record.

**VROOM-ESC-13** When an escalation identifier contains a path component, the file store shall reject the write.

**VROOM-ESC-14** When a deterministic adjudicator receives an empty or bare non-answer, the system shall classify the answer as incorrect without consulting a model.

**VROOM-ESC-15** When a deterministic adjudicator receives a substantive answer without a model layer, the system shall decline to invent a semantic verdict.

**VROOM-ESC-16** When a model adjudicator is configured for a supported model family, the system shall preserve that family in audit attribution and shall use the injected model layer.

**VROOM-ESC-17** When a model adjudicator fails, the system shall degrade to the deterministic result without inventing an outcome.

**VROOM-ESC-18** When the legacy Claude adjudicator constructors are used, the system shall preserve Claude audit attribution and Anthropic environment-based configuration.

**VROOM-ESC-19** When outcome backfill encounters an already adjudicated event, the system shall preserve the existing outcome.

**VROOM-ESC-20** When escalation analysis groups events, the system shall support misalignment, frequent-question, and cross-agent repetition reports from the append-only log.

## Test Traceability

- Package tests: `pkg/vroom/escalation/*_test.go`
- BDD: `agm/test/bdd/features/vroom_runtime_guardrails.feature`
