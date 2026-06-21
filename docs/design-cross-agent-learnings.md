# Design: Migrating Cross-Agent Learnings out of Dispatch Memory

**Status:** Spike / investigation (ce-228u). No implementation.
**Problem:** ~75 hard-won engineering policies live only in one agent's local
memory store (`~/.claude/projects/-Users-vbonnet-src-dear-agent/memory/*.md`,
indexed by `MEMORY.md`). That store is per-machine, per-agent ("Dispatch"-only),
30-day-GC'd, and invisible to every other agent, to CI, and to humans. A policy
that exists only there is re-learned the hard way by the next agent.

This doc inventories those policies, evaluates durable destinations already
present in dear-agent, and recommends a minimal-risk migration.

## 1. Inventory

The memory files fall into four impact classes:

| Class | Count (approx) | Examples |
|-------|------|----------|
| **Security / guardrails** | ~18 | `~/src` read-only (deny-net is porous), no direct push to protected `main`, no `--force`/`--no-verify`, override-guard `--reason` (ADR-031 bypass removal), git identity = GitHub noreply, PII in conversation-log repos, OAuth-mesh 3-store fragmentation |
| **DoD / process compliance** | ~15 | leave bead open until PR *merged*, `required_conversation_resolution` ⇒ resolve Gemini threads via GraphQL, `safe-pr`/`safe-merge` are the only sanctioned PR/merge paths, run `make preflight` before push, retros must ship the patch in the same PR |
| **Architectural invariants** | ~20 | Go is the default (no Python), bash 20-line limit ⇒ promote to a Go tool, one root Go module, 3-supervisor VROOM model with `CONTEXT.md`/ADR-002 as SoT, OTLP over gRPC not HTTP, don't re-implement the existing tracer / worktree-reaper / sweep backfill, beads is the single source of truth |
| **Environment / toil facts** | ~22 | `gtimeout` not `timeout`, always `GIT_TERMINAL_PROMPT=0 gtimeout 30` on push (keychain hangs), chezmoi-deploy is the only sanctioned apply, known CI flakes ⇒ re-run don't investigate, prefer CLI flags over env-var prefixes |

The first three classes are *durable policy*; the fourth is mostly *operational
trivia* that ages fast and is lower value to migrate.

## 2. Destination options

Every candidate destination **already exists** in this repo:

- **`.claude/CLAUDE.md`** — scoped instruction file, loaded into every agent
  session here. Already holds nine MANDATORY Core Engineering Principles. Best
  for *normative, always-relevant* rules. Cost: context budget; only ~10–15
  items can live here before it bloats.
- **CI gates / hooks** — `.github/workflows/` (e.g. `language-policy.yml`
  bash-20-line, `routing-enforcement.yml`, `bypassed-merge-audit.yml`,
  `branch-protection-audit.yml`) and `.claude/hooks/` (`pretool-bypass-guard`,
  `pretool-bead-close-guard`). The only **deterministic, non-bypassable** layer.
  Best for invariants that can be mechanically checked. Cost: must be codable as
  a check; false positives block work.
- **ADRs** (`docs/adr/`, ADR-001..032) — durable *decision + rationale*. Best
  for architectural choices that need the "why" preserved (VROOM model, OTLP
  transport, bypass removal already lives in ADR-031). Cost: prose, not enforced.
- **`.ai.md` engram files** — repo-native guidance format with a real
  validator (`pkg/validator`: checks frontmatter `type`, embedded context,
  examples, vague verbs). Best for *task/reference guidance* that benefits from
  structured, lint-checked authoring. Cost: format discipline; validator is the
  gate, not CI-blocking by default.
- **Skills / `AGENTS.md`** — procedural runbooks (`docs/skill-tiers.md` already
  governs them). Best for *how-to* sequences an agent executes (the safe-pr /
  resolve-threads / merge dance). Cost: only invoked when an agent reaches for them.
- **Bead labels / metadata** — good for *tracking* a migration or flagging a
  recurring trap, not for the policy text itself.

## 3. Recommendation matrix

| Policy type | Primary destination | Why |
|-------------|--------------------|-----|
| Security guardrail (mechanically checkable) | **CI gate / hook** + ADR for rationale | Determinism beats prose; agents route around un-enforced rules |
| Security guardrail (judgment-based) | **CLAUDE.md** (terse) + ADR | Always in context; ADR carries the why |
| DoD / process compliance | **CI gate or hook** where possible, else **skill** | DoD must fail closed; multi-step flows become runbooks |
| Architectural invariant | **ADR** (decision) + **CLAUDE.md** one-liner | ADR preserves rationale; CLAUDE.md keeps it top-of-mind |
| Domain/task guidance | **`.ai.md` engram** | Native validated format, structured for agents |
| Environment / toil fact | **stays in memory** (low value) or a single `docs/ENVIRONMENT.md` | Ages fast; not worth a durable channel each |

Rule of thumb: **prefer the most deterministic destination a policy can
tolerate** — hook/CI > CLAUDE.md > ADR/`.ai.md` > memory. Enforcement that
teaches (CLAUDE.md principle 2) plus a gate that can't be bypassed is the
strongest combination; several policies want *both* a CI gate and an ADR/CLAUDE
line so the "why" survives the "no".

## 4. Migration path (minimal-risk first step)

This is a spike, so the goal is the smallest reversible move that proves the pipeline:

1. **Triage, don't bulk-migrate.** Tag each of the ~75 memory entries with one
   of the four classes above (a one-pass labeling, output to a checklist bead
   under ce-228u). Most "environment/toil" entries are dropped immediately.
2. **First slice: the security + DoD policies that already have an ADR or gate.**
   Many are *already half-migrated* (ADR-031 bypass removal, language-policy
   bash limit, routing-enforcement). The first PR just adds the missing
   CLAUDE.md cross-references and back-links memory ⇒ canonical doc, so the two
   stop drifting. Zero new enforcement, fully reversible.
3. **Second slice: promote 3–5 un-enforced invariants to ADRs** (e.g. "don't
   re-implement the existing tracer/reaper", OTLP-over-gRPC), one ADR per PR.
4. **Only then** propose new CI gates/hooks for the checkable guardrails — each
   as its own scoped plan (per CLAUDE.md principle 1), because a new failing
   gate is the highest-risk, least-reversible change.

Net: start by *linking* memory to destinations that already exist, migrate
prose (ADRs) next, and defer new enforcement until the policy is proven stable.
