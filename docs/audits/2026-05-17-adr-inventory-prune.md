# ADR Inventory & Prune Audit

**Date:** 2026-05-17
**Scope:** Every ADR file in the dear-agent monorepo (~92 individual ADR files
across 13 directories, plus 6 aggregate `ADR.md` files flagged for later).
**Test applied (Matt Pocock / `grill-with-docs`):** keep an ADR only if **all
three** are true — (1) hard to reverse, (2) surprising without context, (3) the
result of a real trade-off. Failing any one ⇒ `DELETE` (trivial/obsolete/
superseded), `MERGE` (redundant with a named ADR), or `CONSOLIDATE→CONTEXT.md`
(it only documents vocabulary/architecture). Passing all three but written
poorly ⇒ `KEEP-rewrite`.

**Provenance note.** The top-level set (`docs/adr/`) was audited
directly. Clusters B–E were audited by parallel delegated reviewers reading
each file in full. Their verdicts are **decision-grade recommendations to be
confirmed by the subsystem owner during the follow-up PR** — not blind-execute
instructions. Disposition counts are approximate (a few "weak" trade-offs are
judgement calls).

---

## Executive summary

| Disposition | ~Count | Meaning |
|---|---|---|
| KEEP (clean) | ~8 | Passes all three; already concise |
| KEEP-rewrite | ~38 | Passes all three; bloated/inaccurate/stale-refs — trim, don't delete |
| CONSOLIDATE→CONTEXT.md | ~16 | Vocabulary/convention, not a hard decision |
| MERGE | ~12 | Redundant with a sibling ADR |
| DELETE | ~16 | Bug-fix-as-ADR, obsolete, superseded, or exact duplicate |

**Headline:** the repo has roughly **twice as many ADRs as it should**. The
dominant failure mode is *bug-fix-as-ADR* and *standard-pattern-as-ADR*, plus
heavy LLM padding (fabricated telemetry/ROI tables, code dumps, multi-phase
wishlists) inflating even the good ones 3–5×. The newest short-form ADRs
(`agm/docs/adr/016–019`) are the quality bar.

### Executed in the originating PR (#127) — scoped for reviewability

Mass-editing ~80 nested code-local ADRs in the same PR as the VROOM rewrite
would be an unreviewable, hard-to-revert mega-diff that violates this repo's
own surgical-commit rule (`GOAL.md`, `AGENTS.md`). So this PR executes only the
**in-theme, unambiguous, low-risk** subset:

- **DELETE** `docs/adr/ADR-008-HTTP-Retry-Consolidation.md` — Status `Draft`,
  never accepted, references the dead `ai-tools` repo and non-existent paths;
  routine library swap (fails all three). No inbound refs.
- **DELETE** `engram/internal/telemetry/enrichment/ADR-001-circuit-breaker-custom-implementation.md`
  — confirmed **byte-identical duplicate** of
  `internal/telemetry/enrichment/ADR-001-...`; zero inbound refs.
- **DEAR terminology reconciliation** — disambiguation banners added to
  `docs/adr/ADR-010`, `ADR-011`, `ADR-018` (they propagated the workflow-engine
  *code* DEAR — Define/Enforce/Audit/Resolve — as if it were the canonical
  *process* DEAR — Define/Execute/Audit/Retro). No code renamed; collision
  registered in `CONTEXT.md`.
- The VROOM ADR set (`agm/docs/adr/ADR-020…025`) was already superseded to
  redirect stubs in this PR.
- **CONSOLIDATED the two top-level ADR directories**: `docs/adrs/` (plural) was
  merged into `docs/adr/` (singular — the conventional name, also used by
  `agm/docs/adr/`). ADR numbers were left unchanged (gaps are fine; renumbering
  would break inbound refs and ADR identity). All inbound references (Go doc
  comments, ROADMAP, CONTEXT.md, etc.) were repointed. Nested per-package dirs
  (`pkg/engram/docs/adrs/`, `pkg/progress/docs/adrs/`, …) are a separate
  concern and were intentionally left alone.

Everything else is **deferred to the follow-up surgical PRs** below.

---

## Recommended follow-up PRs (grab-and-go)

Each is independently reviewable by someone with that subsystem's context.

| PR | Scope | Headline actions |
|---|---|---|
| **FU-1** | `agm/docs/adr/ADR-001…019` | DELETE 005,010,012; MERGE 002+011→001 (adapter cluster); CONSOLIDATE 008,014; KEEP-rewrite the rest; refresh stale README index |
| **FU-2** | agm code-local (`cmd/agm`, `workspace`, `gemini`, `evaluation`, `tmux`, `uuid`, `agm-mcp-server`) | DELETE cmd/agm/005, workspace/003, gemini/002, eval/004, uuid/001; MERGE cmd/agm 006↔007; CONSOLIDATE the standard-pattern set |
| **FU-3** | engram (`cmd/engram/001…008`, `internal`, `ecphory`, `retrieval`, `health`) | DELETE cmd/engram/001, internal/002; MERGE the 3 "*-custom-implementation" ADRs into one; CONSOLIDATE conventions; refresh stale `ADR-INDEX.md` |
| **FU-4** | `pkg/*`, `internal/sandbox`, `tools/*`, `wayfinder` | DELETE progress/002, dod/003, dod/005; MERGE progress 001↔003; CONSOLIDATE the YAML/serialization rationale repeated 4× |
| **FU-5** | top-level heavy rewrites | 009/010 dedup the substrate prose; MERGE 016→015; retitle ADR-011 ("Scheduled Repository Audit Subsystem"); strip LLM padding from 25 KB ADRs; fix `Proposed`-forever status hygiene |
| **FU-6** | aggregate `ADR.md` files (un-audited surface) | Audit `engram/ecphory/ADR.md`, `engram/ecphory/ranking/ADR.md`, `tools/devlog/ADR.md`, `wayfinder/review/ADR.md`, `pkg/monitoring/ADR.md`, `agm/internal/dolt/adr/README.md` — these pack many ADRs per file and were missed by the `ADR-*` filename scan |

---

## Full audit tables

### Top-level — `docs/adr/` (audited directly)

| Path | Decision | Hard? | Surpr? | Tradeoff? | Disposition | Reason |
|---|---|---|---|---|---|---|
| docs/adr/ADR-001-monorepo-consolidation | One monorepo, one go.mod | Y | Y | Y | **KEEP** | Concise, passes all three |
| docs/adr/ADR-002-vroom-execution-architecture | 3-supervisor mesh above AGM | Y | Y | Y | **KEEP** | New canonical (this PR) |
| docs/adr/ADR-008-HTTP-Retry-Consolidation | go-retryablehttp swap | N | N | N | **DELETE ✓PR** | Draft, dead-repo refs, lib swap |
| docs/adr/ADR-009-work-item-substrate | WorkItem ≠ Session | Y | Y | Y | KEEP-rewrite | Defer to ADR-010; stop restating diagnostic prose |
| docs/adr/ADR-010-workflow-engine-architecture | pkg/workflow → SQLite substrate | Y | Y | Y | **KEEP** (DEAR banner ✓PR) | Real; retitle/trim in FU-5 |
| docs/adr/ADR-011-dear-audit-subsystem | Scheduled repo audit on shared DB | Y | Y | Y | KEEP-rewrite (DEAR banner ✓PR) | Retitle to drop "DEAR" ambiguity (FU-5) |
| docs/adr/ADR-012-provider-transport-layer | Resolver + role router over Provider | ~ | Y | Y | KEEP-rewrite | Drop rotting line-number refs |
| docs/adr/ADR-013-tailscale-api | tsnet-bound API, tailnet = auth | Y | Y | Y | **KEEP** | Strong; concise |
| docs/adr/ADR-014-plugin-system | Compiled-in trust only, no .so/WASM | Y | Y | Y | **KEEP** | Strong surprising trade-off |
| docs/adr/ADR-015-signal-aggregator | 2nd SQLite DB; pkg/aggregator | ~ | ~ | Y | KEEP-rewrite | Compress to the naming-collision + 2nd-DB call |
| docs/adr/ADR-016-recommendation-mcp-server | Read-only MCP over signals.db | N | N | ~ | **MERGE→ADR-015** | Mostly schema spec; fold as a section |
| docs/adr/ADR-017-gateway-platform-adapters | In-process bus + Adapter iface | ~ | ~ | Y | KEEP-rewrite | Trim design-doc prose to the binding choices |
| docs/adr/ADR-018-graceful-exit-default | On-by-default; opt-out needs `why:` | Y | Y | Y | **KEEP** (DEAR banner ✓PR) | Accepted; sound |
| docs/adr/ADR-022-backlog-suggestion-system | Task-driven pickup ranking | Y | ~ | Y | KEEP-rewrite | VROOM refs already fixed (this PR) |

### Cluster B — `agm/docs/adr/ADR-001…019`

DELETE: 005 (manifest versioning — dead, ADR-012 removed YAML), 010
(orchestrator-resume — still Proposed, stale orchestrator concept), 012 (Dolt
migration test infra — spent one-time tactic).
MERGE→ADR-001: 002 (command-translation), 011 (gemini adapter) — same
adapter decision three times.
CONSOLIDATE→CONTEXT.md: 008 (status aggregation — trivial), 014 (slog —
industry default/changelog).
KEEP (clean): 016, 017, 018, 019.
KEEP-rewrite: 001 (absorb 002+011), 003, 004, 006, 007, 009, 013, 015 — strip
code dumps / "Validation"/"Monitoring"/"Lessons" sections.
*Note:* 001↔004 and 006↔007 are split/contradicting decisions to reconcile.

### Cluster C — agm code-local (cmd/agm, workspace, gemini, evaluation, tmux, uuid, agm-mcp-server)

DELETE: cmd/agm/005 (unified-init — self-described bug fix), workspace/003
(interactive-selection — trivial), gemini/002 (use official SDK — obvious),
evaluation/004 (pluggable-alerter — textbook 1-method iface), uuid/001
(TrimSpace bug fix).
MERGE: cmd/agm 006↔007 (same-day test-isolation, contradict on `--allow-test-name`).
CONSOLIDATE→CONTEXT.md: cmd/agm/003 (DI pattern), workspace/002 (atomic
rename), gemini/003 (full-history — V1 limitation), evaluation/002
(interface-for-vendor-independence).
KEEP-rewrite: cmd/agm 001, 002, 004; workspace/001; agm-mcp-server/001;
gemini/001; evaluation 001, 003; tmux 0001, 0002 (the tmux pair preserves
genuinely surprising tmux 3.4 gotchas — high value).

### Cluster D — engram (cmd/engram 001–008, internal, ecphory, retrieval, health)

DELETE: cmd/engram/001 (cobra — default), internal/ADR-002 (lipgloss — lib swap).
MERGE: cmd/engram/003→002; cli/ADR-003→ engram security ADR-006; the 3
"*-custom-implementation" ADRs (cli validation, ecphory rate-limit, telemetry
circuit-breaker) → one "prefer custom over third-party for narrow infra" ADR.
CONSOLIDATE→CONTEXT.md: cmd/engram/002 (error struct), health/001, health/002
(auto-fix tactics).
KEEP (clean): cmd/engram/006 (security validation), retrieval/001, retrieval/003.
KEEP-rewrite: cmd/engram 004, 005, 007, 008; health/003; retrieval/002.
*Note:* `ADR-INDEX.md` is stale — covers only the 2024 cmd/engram set, misses
~11 newer ADRs.

### Cluster E — pkg/*, internal/sandbox, internal/telemetry, tools/*, wayfinder

DELETE: `engram/internal/telemetry/enrichment/ADR-001` (exact dup — **✓PR**);
progress/002 (TTY detection — trivial); dod-enforcer/003 (sequential — YAGNI);
dod-enforcer/005 (hardcoded timeouts — tunable constant).
MERGE: progress 001↔003 (one progress-design note).
CONSOLIDATE→CONTEXT.md: sandbox/002 (platform detection), pkg/engram/001
(YAML frontmatter), pkg/llm/002 (per-tool config), dod-enforcer/001 (YAML),
spec-review/002 (CLI-wrapper) — the YAML/serialization rationale recurs 4×.
KEEP-rewrite: sandbox 001, 003; pkg/engram 002, 003; pkg/llm/001;
dod-enforcer 002, 004; spec-review 001, 003; wayfinder gate-9 (keep the
zero-tolerance / no-override sub-decisions, demote the rest).

---

## Cross-cutting patterns (apply during every follow-up)

1. **Bug-fix-as-ADR** is the #1 deletion reason — files whose own text says
   "purely a bug fix" / "none identified" for alternatives. Move to commit
   history, delete the ADR.
2. **Standard-pattern-as-ADR** (DI, atomic-rename, interface-for-DIP, YAML
   choice, cobra) → CONTEXT.md conventions, not ADRs.
3. **LLM padding** (fabricated telemetry/ROI tables, code listings, "V2 Future
   Enhancements", "Lessons Learned") inflates good ADRs 3–5×. Every KEEP-rewrite
   = cut to Context → Decision → Alternatives → Consequences.
4. **Unmanaged supersession**: ADR-009 chains (007/008→009 in agm; 001→004,
   006→007). Add explicit `Superseded-by:` headers so a reader knows which is
   authoritative.
5. **Stale indexes**: `agm/docs/adr/README.md` and `engram/.../ADR-INDEX.md`
   both stop years/versions short of reality.
6. **Volatile anchors**: ADRs citing `file.go:84-89` or `~/src/engram-research/*`
   paths will rot — replace with stable symbol/ADR references (and note
   `.dear-agent.yml` forbids a `research/` tree in this repo).
