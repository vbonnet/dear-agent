# Spike: Adversarial Debate as a Workflow Primitive

**Status:** Draft (spike — investigation only, no implementation)
**Date:** 2026-06-21
**Bead:** ce-onen
**Type:** spike / investigation
**Companions:** [[ce-ynyb]] spike-pattern adoption · multi-lens retro (`agm/internal/ops/retro_analyze.go`) · trust-inversion / verifier-families (`pkg/audit/verifier.go`, ROADMAP §6.5)

---

## Question to resolve

Is **structured adversarial debate** — a Pro agent and a Con agent arguing a
position, settled by a Judge — worth building as a reusable VROOM workflow
primitive? If so, what is the minimal agent API contract, judge schema, and
harness, and where does it beat the multi-lens review we already run?

This spike produces enough signal to write W0 requirements for an
implementation bead (or to decide *not* to build it). It stops before code.

---

## TL;DR (the signal)

- **Build it, but scoped.** Debate is worth a primitive for a **narrow class
  of decisions**: genuine two-sided *judgment calls* where the failure mode is
  premature convergence, not missing coverage.
- **It is not a replacement for multi-lens review.** Multi-lens maximizes
  *coverage* (N blind angles, no interaction). Debate maximizes *resolution
  quality* on a single contested axis (two angles that must engage each
  other's strongest form). They compose: lenses *find* the dispute, debate
  *settles* it.
- **Minimal harness is small** — it is the multi-lens fan-out plus (a) a shared
  context object both sides see, (b) one rebuttal round, and (c) a judge that
  must *synthesize*, not just pick. Reuses the existing structured-output and
  verifier-family seams.
- **Strongest use cases:** architecture decisions (GOOD) and conflicting worker
  outputs (GOOD). **Weakest:** code-review merge verdicts (POOR — adversarial
  *verification* already covers this better).

---

## 1. Agent API contract

A debate is one function call over a single contested proposition. Three agent
roles, two of them symmetric (Pro/Con), one judge. The Pro and Con agents never
see the judge; the judge sees both transcripts plus the original input.

### 1.1 Debate input schema

```jsonc
{
  "position":  "string",   // the proposition under debate, stated so it CAN be
                           // argued both ways, e.g. "This feature should ship
                           // as a separate microservice rather than in the
                           // monolith." NOT a question — a claim.
  "context":   "string",   // shared briefing both sides receive verbatim:
                           // relevant code excerpts, constraints, prior
                           // decisions (ADRs), the bead text. This is the
                           // single most important field — debate quality is
                           // bounded by context quality.
  "domain":    "architecture | prioritization | conflict | review | other",
  "rounds":    1,          // rebuttal rounds after opening (default 1; see §5)
  "stakes":    "low | medium | high"  // drives model tier + round count
}
```

**Design note.** `position` must be a *falsifiable claim*, not a question.
"Monolith vs microservice?" gives both agents the same job; "Ship as a
microservice" gives Pro and Con disjoint jobs. The harness should reject a
`position` that parses as a question and ask the caller to restate it.

### 1.2 Pro-agent prompt template

```
You are the PRO advocate in a structured debate. Your job is to make the
STRONGEST honest case FOR this position. You are an advocate, not a neutral
analyst — surface every real argument in favor, but never fabricate evidence
or misrepresent the context. A judge will score you on logical soundness and
evidence quality, and will penalize overreach.

POSITION (argue FOR): {{position}}
DOMAIN: {{domain}}
SHARED CONTEXT:
{{context}}

{{#if opponent_opening}}
The CON advocate argued:
{{opponent_opening}}

Now REBUT. Concede any point that is genuinely correct (concessions raise your
credibility with the judge), and attack the points that are weak or unsupported.
{{/if}}

Output:
- claim: one-sentence restatement of your strongest case
- arguments: list of {point, evidence, strength: strong|moderate|weak}
- concessions: points from the opponent you accept as valid (empty on opening)
- key_risk_if_wrong: the single best argument AGAINST your own side (steelman)
```

The `key_risk_if_wrong` field is deliberate: forcing each advocate to name the
best counter-argument prevents one-sided collapse and gives the judge material
for synthesis.

### 1.3 Con-agent prompt template

Identical structure with the stance inverted:

```
You are the CON advocate ... make the STRONGEST honest case AGAINST this
position ...
POSITION (argue AGAINST): {{position}}
...
```

Symmetry matters: Pro and Con must be the *same model at the same tier* with
mirror-image prompts, so the judge is scoring arguments, not a tier asymmetry.

### 1.4 Judge schema (output)

```jsonc
{
  "winner":     "pro | con | draw",
  "confidence": 0.0,        // 0–1; the harness gates on this (see §2.4)
  "reasoning":  "string",   // why the winner won, citing specific arguments
  "scores": {               // per-side rubric totals (see §2.1)
    "pro": { "logic": 0, "evidence": 0, "novelty": 0, "relevance": 0 },
    "con": { "logic": 0, "evidence": 0, "novelty": 0, "relevance": 0 }
  },
  "synthesis":  "string",   // the SYNTHETIC position (see §2.2) — REQUIRED,
                           // even when winner is decisive
  "residual_uncertainty": "string"  // what debate did NOT resolve; feeds the
                                     // caller's decision to escalate or accept
}
```

`synthesis` is non-optional on purpose. A judge that only picks a winner has
thrown away the loser's concessions — the whole value of debate over a coin
flip is that the *output is better than either input*.

---

## 2. Judge scoring schema

### 2.1 Score rubric

Each side is scored 0–5 on four axes; the judge totals them but is told the
total is advisory, not mechanical (a single decisive logical flaw can sink a
high-scoring side):

| Axis | Question | Weight |
|------|----------|--------|
| **Logical soundness** | Do the arguments actually follow? Any fallacy, non-sequitur, or unsupported leap? | highest |
| **Evidence quality** | Are claims grounded in the supplied context (code, ADRs, constraints) vs. asserted from priors? | high |
| **Novelty** | Did this side surface a consideration the other missed entirely? | medium |
| **Relevance** | Do the arguments bear on *this* decision, or argue a generic principle? | medium |

Logic and evidence dominate; novelty and relevance break ties. The judge is
explicitly instructed to **penalize confident overreach** — an advocate who
overstates loses evidence points — which keeps the advocacy framing from
degrading into bluffing.

### 2.2 Synthesis rule

The synthesis is mechanical in *shape*, judgment in *content*:

> **Synthetic position = winner's surviving arguments + every concession the
> loser extracted + the loser's strongest argument that the winner did not
> rebut.**

Concretely the judge is instructed:

1. Start from the winning side's claim.
2. Subtract any argument the loser successfully refuted (don't carry dead
   points forward).
3. Add each point the *winner conceded* — these are now constraints on the
   winning position, not refutations of it.
4. Add the loser's single best *un-rebutted* argument as a documented risk or
   guardrail.
5. State the result as an actionable position, plus `residual_uncertainty` for
   what neither side resolved.

This is why debate ≠ vote. The output is a *third* position: "Ship as a
microservice (Pro wins on blast-radius isolation), **but** behind the monolith's
existing auth gateway (Con's conceded point), and revisit if request volume
stays under X (Con's un-rebutted scaling argument)."

### 2.3 Why a judge and not a vote

Pro/Con produce advocacy, which is intentionally biased. Averaging two biased
outputs gives mush. A judge is a *third role* with a different objective
function (find truth, not win), and it is the only role that sees both
transcripts. This mirrors the existing **verifier-family** seam in
`pkg/audit/verifier.go`: the judge is a different "family" from the advocates,
so its verdict carries cross-family trust rather than same-family agreement.

### 2.4 Confidence gating

`confidence` is the integration point with the rest of the mesh. The caller
gates on it exactly like the existing **confidence-gated spike output** pattern
([[ce-90si]]):

- `confidence ≥ 0.75` → accept synthesis, proceed.
- `0.4 ≤ confidence < 0.75` → accept synthesis but flag `residual_uncertainty`
  for human/supervisor review.
- `confidence < 0.4` or `winner == draw` → debate did not resolve it; escalate
  (more context, higher tier, or human decision). A draw is a *signal*, not a
  failure — it means the decision is genuinely balanced and shouldn't be made
  by an LLM alone.

---

## 3. Use case evaluation

Scored GOOD / MARGINAL / POOR on: *is there a genuine two-sided judgment call,
and does adversarial engagement beat parallel coverage?*

### 3.1 Architecture decisions — **GOOD**

*"Monolith vs microservice for this feature," "sync vs event-driven," "extend
`bd` schema vs new sidecar table."*

- Genuinely two-sided, high-stakes, hard to reverse. Exactly the failure mode
  debate targets: an agent reasoning solo anchors on its first instinct and
  rationalizes. Forcing a steelmanned opposition surfaces the trade-off the
  solo path buries.
- The synthesis output maps cleanly onto an ADR: winner = decision, concessions
  = constraints, residual = "revisit when." See the project's own
  monolith-vs-sidecar deliberations.
- **Caveat:** quality is bounded by `context`. Garbage context → confident
  garbage debate. Pair with a context-gathering lens first.

### 3.2 Bead prioritization disputes (P0 vs P1) — **MARGINAL**

*"Is this a P0 or a P1?"*

- Two-sided, yes, but the dispute is usually about *facts not yet established*
  (blast radius, user impact, dependency count), not about *reasoning over
  known facts*. Debate is strong at the latter, weak at the former — two agents
  can eloquently argue past each other when the real gap is missing data.
- Works **only** when priority criteria are explicit and supplied as context
  (a rubric). Then debate is a sharp tie-breaker. Without a rubric it produces
  confident noise.
- Cheaper alternative for most cases: a single classifier against an explicit
  rubric. Reserve debate for the genuinely contested 10%.

### 3.3 Conflicting worker outputs (A says X, B says Y) — **GOOD**

*Worker A's PR claims approach X; worker B's claims Y; they're incompatible.*

- This is debate's *native* shape — the two positions already exist, fully
  formed, with their own evidence. You skip the advocacy-generation step: seed
  Pro with A's output and Con with B's, run one rebuttal round, judge.
- Synthesis is exactly what you want: not "A wins, discard B," but "A's
  structure with B's error-handling, because B conceded the structure point and
  A never rebutted B's edge-case." Directly actionable as a merge.
- Strongest fit because the inputs are real artifacts, not LLM-generated
  stances — lowest risk of fabricated argument. This is likely the **first**
  use case to implement.

### 3.4 Code-review merge verdicts (safe to merge?) — **POOR**

*"Is this diff safe enough to merge?"*

- This looks like a yes/no judgment but it is really a *coverage* problem: the
  risk is an un-checked failure mode (a missed injection path, an unhandled
  error), not an under-argued one. Debate's two-channel structure is *worse*
  than multi-lens here — you want N independent reviewers each hunting a
  different bug class, not two reviewers arguing about one.
- The project **already has the right primitive**: adversarial *verification*
  via verifier families and the trust-inversion contract (`pkg/audit`,
  ROADMAP §6.5). That's cross-family review of an artifact, which is what
  "safe to merge" needs.
- Debate could play a *narrow* role: only when two reviewers already disagree
  on a specific finding's severity (blocker vs nit) — i.e. it degenerates to
  the §3.3 conflict case. Not a primary use case.

| Use case | Verdict | Why |
|----------|---------|-----|
| Architecture decisions | **GOOD** | Two-sided judgment, high stakes, synthesis → ADR |
| Bead prioritization | **MARGINAL** | Often a data gap, not a reasoning gap; needs explicit rubric |
| Conflicting worker outputs | **GOOD** | Native shape; real artifacts as inputs; synthesis → merge |
| Code-review merge verdict | **POOR** | Coverage problem; adversarial *verification* already fits better |

---

## 4. Comparison to the existing multi-lens review pattern

We already run **multi-lens** analysis (`agm/internal/ops/retro_analyze.go`:
root-cause / recurrence / remediation / … lenses, each producing structured
insights independently and in parallel). What does debate add?

| | **Multi-lens review** | **Adversarial debate** |
|---|---|---|
| Topology | N agents, **no interaction**, blind to each other | 2 advocates that **engage**, + 1 judge |
| Optimizes for | **Coverage** — surface everything | **Resolution** — settle one contested axis |
| Failure mode it fixes | Tunnel vision / missed angle | Premature convergence / unchallenged anchor |
| Inputs | One artifact, many questions | One proposition, two stances |
| Output | Union of findings (more is better) | One synthesized position (better, not more) |
| Cost | N × single-pass | 2×(1+rounds) + judge ≈ 4–6× single-pass |
| Cross-talk | None (a feature — keeps lenses independent) | Required (the point) |

**What debate adds beyond parallel lenses:** *engagement*. Multi-lens agents
never see each other's output, so a weak argument is never challenged — it just
sits in the union. Debate forces each side to confront the other's *strongest*
form (the rebuttal round) and forces the judge to weigh them against each
other. You get a *decision*, not a pile of considerations.

**What multi-lens does better:** breadth and independence. For "find all the
problems," interaction is a *bug* — you want N uncorrelated searches, because
two agents who talk will converge and miss the same things together. Debate's
two-channel structure would *reduce* coverage.

**Decision rule — when to use which:**

- Use **multi-lens** when the goal is to *not miss anything* (review, audit,
  retro, find-all-bugs). Coverage problem → independent lenses.
- Use **debate** when the goal is to *settle a contested call* and you already
  know the axis of disagreement (architecture choice, conflicting outputs).
  Resolution problem → engaged advocates + judge.
- **Compose them:** run multi-lens first to *discover* disputes, then spin up a
  debate per dispute the lenses surfaced. Lenses find; debate settles. This
  composition is the strongest argument for building debate as a *primitive*
  rather than a one-off — it slots in as the second stage of an existing
  fan-out.

---

## 5. Implementation path (minimal harness)

Debate is **multi-lens fan-out + three deltas**. The project already has the
fan-out machinery (parallel agent invocation), structured-output validation,
and confidence gating. The new pieces are small.

### 5.1 What already exists (reuse, don't rebuild)

- **Parallel agent invocation** — the multi-lens runner shape.
- **Structured output / schema validation** — the judge and advocate outputs
  are just schemas ([[ce-t8kn]] output-schema work).
- **Confidence gating** — §2.4 reuses [[ce-90si]] verbatim.
- **Verifier-family / cross-family trust** (`pkg/audit/verifier.go`) — the
  judge is a different family from the advocates; the trust-inversion contract
  already models "verified by a different family."

### 5.2 The three deltas

1. **Shared context object** both advocates receive verbatim (multi-lens gives
   each lens the artifact but no shared *briefing*; debate needs both sides on
   identical footing).
2. **One rebuttal round** — a second advocate call that receives the opponent's
   opening. This is the only stateful step. Default `rounds: 1`; high-stakes
   `rounds: 2`. More than 2 shows sharply diminishing returns and risks the
   advocates converging into agreement (which defeats the point).
3. **Synthesizing judge** — a judge prompt that must emit `synthesis` per §2.2,
   not just `winner`.

### 5.3 Minimal Go/workflow sketch

```go
type DebateInput struct {
    Position string `json:"position"`
    Context  string `json:"context"`
    Domain   string `json:"domain"`
    Rounds   int    `json:"rounds"` // default 1
    Stakes   string `json:"stakes"`
}

type Advocacy struct {
    Claim       string     `json:"claim"`
    Arguments   []Argument `json:"arguments"`
    Concessions []string   `json:"concessions"`
    KeyRisk     string     `json:"key_risk_if_wrong"`
}

type Verdict struct {
    Winner               string             `json:"winner"`
    Confidence           float64            `json:"confidence"`
    Reasoning            string             `json:"reasoning"`
    Scores               map[string]Rubric  `json:"scores"`
    Synthesis            string             `json:"synthesis"`
    ResidualUncertainty  string             `json:"residual_uncertainty"`
}

// RunDebate: opening (parallel) → rebuttal (parallel) → judge (single).
func RunDebate(ctx context.Context, in DebateInput) (Verdict, error) {
    // 1. Opening statements — Pro and Con in parallel, mirror prompts, same tier.
    pro, con := parallel(
        advocate(ctx, RolePro, in, nil),
        advocate(ctx, RoleCon, in, nil),
    )
    // 2. Rebuttal rounds — each side sees the other's prior statement.
    for r := 0; r < max(in.Rounds, 1); r++ {
        pro, con = parallel(
            advocate(ctx, RolePro, in, con),
            advocate(ctx, RoleCon, in, pro),
        )
    }
    // 3. Judge — different family, sees both transcripts + original input.
    return judge(ctx, in, pro, con) // validates against Verdict schema
}
```

~3 calls deep, fully reuses the parallel runner and the structured-output
validator. No new infrastructure — it is a composition of existing primitives
with a synthesis-shaped judge prompt.

### 5.4 Recommended first slice (tracer bullet)

Ship the **conflicting-worker-outputs** case (§3.3) first:

- Inputs are *real artifacts* (two worker outputs), so no fabricated-argument
  risk and no advocacy-generation step — seed Pro/Con directly.
- One rebuttal round, single judge, synthesis → merge recommendation.
- Validates the judge schema and synthesis rule against ground truth (you can
  check whether the synthesized merge actually compiles/passes).
- If synthesis quality holds here, generalize to architecture decisions (§3.1).

### 5.5 Open questions for W0 (the implementation bead)

1. **Advocate model tier** — same tier both sides (fairness) but which? Sonnet
   likely sufficient for advocacy; the *judge* may warrant a higher tier since
   it does the synthesis. Ties into `docs/design-vroom-model-tiering.md`.
2. **Position auto-extraction** — can a pre-step turn "these two PRs conflict"
   into a falsifiable `position` automatically, or must the caller supply it?
3. **Draw handling policy** — escalate to human vs. multi-lens fallback vs.
   re-run with more context. §2.4 proposes escalate; needs a decision.
4. **Judge self-consistency** — does running the judge twice agree? If not,
   the confidence number is unreliable and we may need a 3-judge panel for
   high-stakes calls (cost trade-off).

---

## Recommendation

**Build it, scoped to the two GOOD use cases, starting with the
conflicting-worker tracer bullet.** It is a cheap composition over existing
primitives (parallel runner + structured output + confidence gate + verifier
families), it fills a real gap multi-lens cannot (resolution vs coverage), and
it composes with multi-lens as a two-stage discover→settle pipeline. Do **not**
position it as a code-review or prioritization tool — those are better served
by the adversarial *verification* and rubric-classifier primitives we already
have. File the implementation bead with the §5.5 open questions as its W0
uncertainties.
