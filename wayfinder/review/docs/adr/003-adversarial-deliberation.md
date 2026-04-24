# ADR-003: Adversarial Deliberation for Multi-Persona Review

**Status:** Accepted
**Date:** 2026-03-26
**Authors:** Engram Contributors

## Context

The multi-persona review system runs each persona (security engineer, performance
expert, etc.) independently and then aggregates their findings. While this produces
comprehensive coverage, it has a blind spot: contradictions between personas go
undetected, trade-offs are never surfaced, and low-confidence findings survive
unchallenged.

For example, a security persona may recommend adding input validation layers that
a performance persona would flag as unnecessary overhead. Without deliberation,
both findings appear side-by-side with no indication they represent a genuine
tension the developer must resolve.

Prior art in structured debate (e.g., adversarial collaboration in academia,
red-team/blue-team exercises) shows that explicitly challenging positions
surfaces hidden assumptions and improves decision quality.

## Decision

We introduce an opt-in **adversarial deliberation** phase that runs after
independent persona reviews complete. The architecture follows a lead-reviewer-
mediated model:

1. **Brief compilation** -- Findings from all personas are grouped and a
   `DeliberationBrief` is assembled with the code context.

2. **Tension identification** -- A lead-reviewer agent scans the brief for
   contradictions, trade-offs, and under-supported claims, producing a list of
   `Tension` objects.

3. **Challenge rounds** -- For each unresolved tension, the lead reviewer
   formulates `Challenge` prompts directed at specific personas. Persona agents
   respond with an updated `stance` (support / oppose / revise / withdraw) and
   `confidence` level.

4. **Convergence check** -- After each round the engine checks three stop
   conditions: timeout, token budget, and convergence threshold (ratio of
   resolved tensions). Deliberation stops when any condition is met or
   `maxRounds` is reached.

5. **Memo synthesis** -- The lead reviewer produces a `DeliberationMemo` with
   a GO / NO-GO / CONDITIONAL decision, final findings, recommendations, and
   full round history.

The lead reviewer uses Opus for synthesis quality, while persona agents use Sonnet
for cost-effectiveness. Deliberation is opt-in via the `ReviewOptions.deliberation`
configuration flag with constraint defaults (3 rounds, 50K token budget, 2-minute
timeout, 0.8 convergence threshold).

## Alternatives Considered

### Peer-to-Peer Debate

**Architecture**: Personas directly challenge each other without an orchestrator.

**Pros**:
- Rich direct debate between domain experts
- No orchestrator overhead

**Cons**:
- N² communication explosion (8 personas = 64 possible conversation pairs)
- Context isolation broken (shared conversation history contaminates independent thinking)
- Prompt caching ineffective (shared context invalidates persona-specific caching)
- No synthesis agent to produce final memo

**Cost**: 4-5x more expensive than lead-reviewer approach (192 API calls vs. ~50)

**Rejected**: Complexity and cost outweigh benefits. Context isolation is critical for unbiased persona analysis.

### Round-Robin Facilitation

**Architecture**: Personas take turns facilitating discussion on their expertise area.

**Pros**:
- Distributed orchestration (no single point of failure)
- Persona ownership of deliberation topics

**Cons**:
- Coordination complexity (who facilitates when?)
- Inconsistent synthesis (each facilitator has different perspective)
- Still requires N² communication (facilitator broadcasts to all)

**Rejected**: Coordination overhead doesn't justify the benefit of distributed facilitation.

### Asynchronous Message-Passing

**Architecture**: Personas post findings to a shared queue, read and react asynchronously.

**Pros**:
- Fully parallel (no waiting for sequential rounds)
- Horizontally scalable

**Cons**:
- No real-time debate (async responses lose conversational flow)
- Convergence detection unclear (when to stop?)
- Still requires synthesis agent for final memo

**Rejected**: Deliberation benefits from real-time back-and-forth. Async is better suited for long-running reviews, not the 2-3 minute deliberation target.

## Consequences

### Positive

- **Contradiction detection** -- Tensions between personas are explicitly
  surfaced with structured positions and resolution records.
- **Higher-confidence output** -- Low-confidence or unsupported findings are
  challenged and either strengthened or withdrawn.
- **Opt-in, zero impact on existing users** -- Deliberation is disabled by
  default (`enabled: false`). Existing review workflows are unchanged.
- **Testable in isolation** -- `Challengeable` and `LeadReviewer` interfaces
  enable full unit testing with mocks.

### Negative / Trade-offs

- **Additional cost** -- Typical deliberation adds ~$0.07 per review session
  (2-3 rounds with cached prefixes). Users should budget for this.
- **Added latency** -- 2-3 challenge rounds add 5-15 seconds depending on
  model and tension count.
- **Complexity** -- Nine new interfaces and a deliberation engine increase the
  codebase surface area. This is mitigated by clean interface boundaries and
  comprehensive test coverage (58 unit tests).

### Risks

- **Infinite loops** -- Mitigated by hard `maxRounds`, `timeoutMs`, and
  `maxDeliberationTokens` limits with conservative defaults.
- **Lead reviewer bias** -- The lead reviewer is a single agent that could
  introduce systematic bias. Future work could rotate lead reviewer personas
  or use ensemble voting.
