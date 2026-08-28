# VROOM Escalation Requirements Specification (EARS)

<!-- Last audited at: 2026-08-28 -->

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

**VROOM-ESC-11** When mutable escalation state is stored or returned in memory, the system shall copy all nested mutable state, including chain data, confer membership, and ballots, so caller aliases cannot mutate stored state.

**VROOM-ESC-12** When mutable escalation state is stored on disk, the system shall write through a synchronized temporary file and publish the complete record so Store readers observe a complete old or new value; on platforms without guaranteed atomic rename, reads shall participate in the cross-process store lock.

**VROOM-ESC-13** When an escalation identifier is empty, contains a path component, or is not a portable lowercase ASCII record name, the store shall reject Create, Update, and Get before accessing a record path.

**VROOM-ESC-14** When a deterministic adjudicator receives an empty or bare non-answer, the system shall classify the answer as incorrect without consulting a model.

**VROOM-ESC-15** When a deterministic adjudicator receives a substantive answer without a model layer, the system shall decline to invent a semantic verdict.

**VROOM-ESC-16** When a model adjudicator is configured for a supported model family, the system shall preserve that family in audit attribution and shall use the injected model layer.

**VROOM-ESC-17** When a model adjudicator fails, the system shall degrade to the deterministic result without inventing an outcome.

**VROOM-ESC-18** When the legacy Claude adjudicator constructors are used, the system shall preserve Claude audit attribution and Anthropic environment-based configuration.

**VROOM-ESC-19** When outcome backfill encounters an already adjudicated event, the system shall preserve the existing outcome.

**VROOM-ESC-20** When escalation analysis groups events, the system shall support misalignment, frequent-question, and cross-agent repetition reports from the append-only log.

**VROOM-ESC-21** When Create receives an identifier that already exists, the store shall return `ErrAlreadyExists` without replacing the existing record.

**VROOM-ESC-22** When concurrent transitions target the same file-backed escalation, the store shall evaluate each mutation against the latest committed record in one serial order, including across independent Store instances in separate processes.

**VROOM-ESC-23** When a mutation callback returns an error or changes the escalation identifier, the store shall retain the prior committed record and return an error without invoking the callback again.

**VROOM-ESC-24** When concurrent vote attempts use the same confer member, the engine shall accept exactly one ballot and shall reject the other attempt as a duplicate without emitting its vote or terminal effects.

**VROOM-ESC-25** When concurrent distinct accepted votes reach quorum, the engine shall retain every accepted ballot, commit exactly one resulting phase transition, and permit only the caller that committed that transition to invoke its terminal effects.

**VROOM-ESC-26** When concurrent answers or forwards compete with another transition, the engine shall validate each against the latest committed record, shall require a non-empty current holder for every non-human answer and forward, and a stale or post-terminal loser shall change no state and invoke no transition effect.

**VROOM-ESC-27** While a file-backed transition waits for its cross-process lock, the store shall stop waiting when its context is canceled or its bounded default wait expires.

**VROOM-ESC-28** When an Engine transition is accepted, the system shall commit its state before invoking the associated session delivery, human dispatch, or audit event under a fresh bounded post-commit context; an effect failure shall not misreport the committed state as an uncommitted mutation.

## Test Traceability

- Package tests: `pkg/vroom/escalation/*_test.go`
- BDD: `agm/test/bdd/features/vroom_runtime_guardrails.feature`
