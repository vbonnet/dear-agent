# Top-Level & Cross-Component Documentation Audit — 2026-05-17

**Scope:** All top-level files (`README.md`, `ARCHITECTURE.md`, `GOAL.md`,
`ROADMAP.md`, `CONTRIBUTING.md`, `AGENTS.why.md`, `CODEOWNERS`, `llms.txt`,
`go.mod`) and all directories **except** `agm/` and `wayfinder/` (covered by
sibling audits): `docs/` (incl. `docs/alignment/`, `docs/adr/`, `docs/adrs/`),
`engram/`, `codegen/`, `tools/`, `internal/`, `pkg/`, `cmd/`, `scripts/`,
`config/`, `configs/`, `tests/`.

**Method:** Read each doc, then verified every concrete claim (paths, commands,
import paths, acronym expansions, directory existence) against the actual tree.

**Canonical vocabulary used as ground truth** (from project `CLAUDE.md` and the
audit brief):

- **DEAR** = **D**efine → **E**xecute → **A**udit → **R**etro (NOT
  "Enforce"/"Resolve"/"Refine")
- **VROOM** = execution framework: **3 supervisors**
  (Meta-Orchestrator / Orchestrator / Overseer) **+ Workers / Auditors / SREs**
- **Wayfinder** = research & planning / SDLC workflow phase
- **AGM** = Agent Gateway Manager — a tool **used by** VROOM, not vice-versa

> This report **only reports** issues. Nothing was fixed.

---

## Executive Summary — Headline Findings

| # | Finding | Severity |
|---|---------|----------|
| H1 | **The repo's namesake acronym DEAR is expanded incorrectly and pervasively.** Nearly every doc that spells out DEAR uses "Define, **Enforce**, Audit, **Resolve** (& Refine)" — contradicting the authoritative `CLAUDE.md` definition "Define → Execute → Audit → Retro". It is wired into code identifiers (`OnEnforce`/`OnResolve` hooks), so this is not a typo cluster — it is a project-wide definition conflict. | **High** |
| H2 | **VROOM is described with a non-canonical role model.** `ARCHITECTURE.md` and all 4 `docs/alignment/*` files describe VROOM as a flat 5-role "Verifier, Requester, Orchestrator, Overseer, Meta-Orchestrator" model. Canonical is 3 supervisors + Workers/Auditors/SREs. The code (`pkg/vroom/`) uses yet a third vocabulary. No single authoritative VROOM role definition exists. | **High** |
| H3 | **Two ADR directories + three colliding ADR numbering namespaces.** `docs/adr/` (only `ADR-001`) vs `docs/adrs/` (`ADR-008`…`022`); plus `engram/cmd/engram/ADR-001…008` is a separate series. `ADR-001` means two different things. Large numbering gaps; `adr_ref: ADR-020` referenced by governance docs does not exist. | **High** |
| H4 | **`README.md` (SQLite) and `ARCHITECTURE.md` (Dolt) disagree on the storage substrate.** Both stores exist in the tree; no doc reconciles which backs what. | **Medium-High** |
| H5 | **`GOAL.md` documents a `dear-agent` CLI and a `research/` tree that do not exist.** Concrete commands (`dear-agent target/status/ask/diary`) and three `research/...` links are dangling; `research/` is explicitly forbidden by `CLAUDE.md`. | **High** |
| H6 | **Pervasive stale predecessor naming.** `github.com/vbonnet/engram/core/...` import paths in ~39 in-scope docs, and "ai-tools" / "AI Tools" in `GOAL.md`, `ADR-001`, and several tool READMEs. | **High** |

---

## 1. Cross-Cutting / Systemic Issues

### S1 — DEAR acronym expanded incorrectly across the repo *(High)*

Canonical (per `CLAUDE.md`, project-authoritative): **Define → Execute → Audit
→ Retro**. The repo overwhelmingly uses **"Define, Enforce, Audit, Resolve (&
Refine)"** instead. Confirmed occurrences:

| File:line | Text | Problem |
|-----------|------|---------|
| `llms.txt:13` | "The core philosophy is DEAR: Define, **Enforce**, Audit, **Resolve**." | E≠Execute, R≠Retro. This is the public machine-readable summary. |
| `ROADMAP.md:219` | "DEAR hook surface (`OnDefine`, `OnEnforce`, `OnAudit`, `OnResolve`)." | Wrong hook names; also load-bearing in code. |
| `docs/workflow-engine/BACKLOG.md:104` | "`OnDefine`, `OnEnforce`, `OnAudit`, `OnResolve`" | Same. |
| `docs/design/substrate-diagnostic.md:138-139` | "DEAR governs *how* you maintain the substrate (Define → **Enforce** → Audit → **Resolve**)." | `GOAL.md:74` cites this doc as authoritative for the substrate diagnostic. |
| `docs/adrs/ADR-018-...md:5` | "Extends the DEAR protocol (Define, **Enforce**, Audit, **Resolve & Refine**)" | |
| `docs/adrs/ADR-010-...md:182` | "DEAR hooks (NEW: OnDefine/**Enforce**/Audit/**Resolve**)" | |
| `docs/adrs/ADR-014-...md:35` | "DEAR hooks ... (Define/**Enforce**/Audit/**Resolve**)" | |
| `docs/adrs/ADR-011-...md:9,45` | "Closes the **A** of DEAR (Define, **Enforce**, **Audit**, **Resolve & Refine**)" | The canonical "DEAR audit subsystem" ADR itself uses the wrong expansion. |

**Aggravating facts:**
- `README.md`, `ARCHITECTURE.md`, `GOAL.md` — the three docs a new reader hits
  first — **never define DEAR at all**, even though the repo is *named*
  `dear-agent`. The only top-level expansion (`llms.txt:13`) is the wrong one.
- The "DEAR audit subsystem" authority, `docs/adrs/ADR-011`, is **Status:
  Proposed** (`ADR-011:3,609`), yet `llms.txt:13` presents DEAR as the
  established "core philosophy".

**Recommended fix:** A single owning decision is required (this is not a blind
`sed` — `OnEnforce`/`OnResolve` are Go identifiers in `pkg/workflow/hooks.go`
and ADR-011/018/010/014 are accepted/proposed records). Either (a) ratify
"Define → Execute → Audit → Retro" everywhere and rename the hooks + supersede
the ADRs, or (b) correct `CLAUDE.md`/the audit canonical. Until reconciled,
add a single authoritative "What DEAR means" section to `README.md` and stop
`llms.txt` from asserting a contested expansion.

### S2 — VROOM role model is non-canonical and internally inconsistent *(High)*

Three different VROOM role vocabularies coexist:

1. **Canonical** (brief / `CLAUDE.md`): 3 supervisors
   (Meta-Orchestrator / Orchestrator / Overseer) + Workers / Auditors / SREs.
2. **`ARCHITECTURE.md:146-147`**: "**VROOM Architecture** — Five-role
   supervisory model: **Verifier, Requester**, Orchestrator, Overseer,
   Meta-Orchestrator." — No "Requester" in canonical; executor tier should be
   Workers/Auditors/SREs, not "Verifier/Requester"; the 3 supervisors are not
   distinguished from executors.
3. **`docs/alignment/MISSION.md:8-13`** `role_mapping`: `verifier / requester /
   orchestrator / overseer / meta_orchestrator` — same non-canonical set;
   `VALUES.md §1-2` and `GOALS.md §2` lean heavily on a "**Verifier** role"
   that is not in the canonical model (Auditors perform verification).
4. **Code** (`pkg/vroom/vroom/`): identifiers observed are `worker`,
   `overseer`, `orchestrator`, `verifier`, `meta-orchestrator` — no
   `requester`, no `auditor`, no `sre`. A *fourth* divergent set.

**Recommended fix:** Establish one authoritative VROOM role definition (an ADR
or `docs/alignment/` doc), then reconcile `ARCHITECTURE.md`, the alignment
frontmatter, and the code to it. At minimum, fix `ARCHITECTURE.md:146-147` and
the `role_mapping` blocks to the canonical 3-supervisor + Workers/Auditors/SREs
model.

### S3 — ADR directory & numbering chaos *(High)*

- **Two directories:** `docs/adr/` contains exactly one file
  (`ADR-001-monorepo-consolidation.md`); every other ADR lives in
  `docs/adrs/` (`ADR-008`…`018`, `022`). Nothing in the repo links to
  `docs/adr/ADR-001` — it is stranded.
- **Colliding namespaces:** `engram/cmd/engram/` has its **own** `ADR-001`…
  `ADR-008` series (`ADR-001` = "Cobra CLI Framework"). So `ADR-001` and
  `ADR-008` each name two unrelated decisions; `engram/ecphory/ADR.md` adds a
  third inline `ADR-002…021` series.
- **Gaps & dangling refs:** `docs/adrs/` jumps `001 → 008`, skips `019/020/021`
  and `023+`. All four `docs/alignment/*` files declare `adr_ref: ADR-020`,
  which **does not exist** anywhere as a top-level ADR (only an unrelated
  `engram/ecphory/ADR.md:666` "ADR-020").
- `README.md:172` ("docs/ # ADRs and design documents") and
  `ARCHITECTURE.md:226-228` do not acknowledge the split.

**Recommended fix:** Consolidate to one directory (recommend `docs/adrs/`),
move/renumber `ADR-001`, namespace or relocate the engram ADR series, fill or
explicitly tombstone the gaps, and repoint `adr_ref: ADR-020` to a real ADR.

### S4 — Storage substrate: README says SQLite, ARCHITECTURE says Dolt *(Medium-High)*

| Doc | Claim |
|-----|-------|
| `README.md:75` | "Every run is persisted to `~/.agm/loops.db` (WAL-mode **SQLite**)" |
| `README.md:148` | Storage layer = "loops.db · runs.db · Manifests · Sandbox" |
| `ROADMAP.md:70` | "Three packages, one **SQLite** database" |
| `ARCHITECTURE.md:52-55` | Storage layer = "**Dolt DB** · Manifests · Message Queue · Sandbox" |
| `ARCHITECTURE.md:76-78,107,216` | "**Dolt** Storage", "Dolt record", "`internal/dolt/`" |

Both exist in-tree (`agm/internal/dolt/`, `pkg/workspace/dolt/`, and SQLite
`loops.db` via `agm/internal/ops/loop.go`). No doc states *which store backs
what* (loops=SQLite? sessions=Dolt? workflow engine=SQLite per ROADMAP?). The
two primary architecture docs flatly contradict each other.

**Recommended fix:** Add one "Storage" section reconciling the layers
explicitly and make `README.md`/`ARCHITECTURE.md` agree.

### S5 — Stale predecessor naming (module path + "ai-tools") *(High)*

The real module is `github.com/vbonnet/dear-agent` (`go.mod:1`). Documented
predecessor path `github.com/vbonnet/engram/core/...` appears in **~39
in-scope docs** (highest counts: `pkg/telemetry/SPEC.md` ×8,
`engram/retrieval/SPEC.md` ×7, `pkg/telemetry/ARCHITECTURE.md` ×7,
`pkg/llm/SPEC.md` ×6, `pkg/config-loader/README.md` ×4). Verified-wrong copy
targets: `engram/hippocampus/README.md:73,325`, `pkg/context/README.md:4,65,75`,
`pkg/monitoring/README.md:8,238`, `internal/telemetry/agent/README.md` (×2).

"ai-tools" / "AI Tools" predecessor name still in user-facing docs:
`GOAL.md:15,18,21-22,97,99`; `docs/adr/ADR-001-...md:9,19`;
`tools/benchmark-query/README.md:3`; `tools/devlog/README.md:46-48`;
`engram/hooks-bin/README.md:164`; `tools/schema-registry/QUERY-IMPLEMENTATION.md:282`.

**Recommended fix:** Repo-wide replace of the module path (verifying each
against actual `package`/import), and replace user-facing "ai-tools"/"AI
Tools" with "dear-agent".

---

## 2. Top-Level File Findings

### `README.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 36 | Low | "Go 1.25+" but `go.mod:3` declares `go 1.26.3`. (Same understatement in `CONTRIBUTING.md:9`, `llms.txt:104`.) | State "Go 1.26.3+". |
| 26-31 | Medium | Components table lists only AGM/Engram/Wayfinder. Omits **VROOM** and **DEAR** — the two concepts the project is named for and that `CLAUDE.md` mandates — and `codegen/` (listed as a dir later but never as a product). | Add VROOM/DEAR and clarify codegen's status. |
| 148 | Medium-High | Storage layer "loops.db · runs.db" contradicts `ARCHITECTURE.md:52`. See S4. | Reconcile. |
| 160-173 | Low | Directory Structure omits `config/`, `configs/`, `tests/`; "docs/ # ADRs and design documents" hides the `docs/adr` vs `docs/adrs` split (S3). | Add missing dirs; note ADR location. |
| 169 vs `ARCHITECTURE.md:217` | Medium | README: workflow engine = `pkg/workflow/`. ARCHITECTURE: "Add workflow definition in `internal/workflow/`". Both `pkg/workflow` and `agm/internal/workflow` exist; relationship undocumented. | Document which is the engine vs. definitions. |
| whole file | Medium | DEAR never defined (S1). | Add a DEAR definition section. |

Verified accurate: install paths (`go.mod:1` matches), all 5 `tools/*` dirs in
the Tools table exist, "9-phase" Wayfinder claim matches `wayfinder/README.md:3`.

### `ARCHITECTURE.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 5 | Medium | "organized around **four products**" — `README.md:26-31` and `CONTRIBUTING.md:114-125` list **three** (AGM/Engram/Wayfinder). The 4th is never named. | State the four explicitly or correct to three. |
| 52-55, 76-78, 107, 216 | Medium-High | "Dolt DB"/"Dolt Storage" contradicts README/ROADMAP SQLite (S4). | Reconcile. |
| 71, 86, 101, 112, 137-150, 158, 213-218 | Medium | Paths written as `internal/ops/`, `internal/agent/`, `internal/backend/`, `internal/session/`, `internal/sandbox/`, `internal/dolt/`, `internal/workflow/` etc. — these live under **`agm/internal/`** (verified `agm/internal/dolt`, `agm/internal/workflow`). `README.md:154` correctly says `agm/internal/ops/`, directly contradicting this file. | Prefix `agm/` (or note the package root). |
| 146-147 | High | VROOM "Five-role supervisory model: Verifier, Requester, Orchestrator, Overseer, Meta-Orchestrator" — non-canonical (S2). | Rewrite to 3 supervisors + Workers/Auditors/SREs. |
| 216 | Low | "`internal/dolt/`" → actual path `agm/internal/dolt/`. | Correct path. |
| whole file | Medium | DEAR never mentioned despite repo name (S1). | Add. |

### `GOAL.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 15, 18, 21-22, 97, 99 | High (S5) | "AI Tools" predecessor name used as the product name. | Replace with "dear-agent". |
| 73 | High | Link `research/SUBSTRATE-HYPOTHESIS-FOR-AGENT-INFRASTRUCTURE.md` — **no `research/` dir exists** and `CLAUDE.md` *forbids* one. Dangling + points at a forbidden path. | Remove/relocate to `engram-research` reference. |
| 116, 118 | High | Links `research/cmd/deep-research/youtube/` and `research/cmd/deep-research/cmd/youtube.go` — dangling (no `research/`). | Remove or repoint. |
| 129-178 | High | "dear-agent CLI UX Design" / "Core Commands" documents `dear-agent target/new/status/ask/diary/stop/resume` as concrete commands. **No `dear-agent` binary exists** (`cmd/` has `dear-agent-api/-mcp/-search/-signals`; the CLI is `agm`). Presented as spec with no "aspirational/not-implemented" marker. | Mark clearly as future/aspirational or remove. |
| 174 | Medium | Config file given as `.dear-agent.yaml`; actual file is **`.dear-agent.yml`** (confirmed; `CLAUDE.md`/`ADR-011` use `.yml`). | Fix extension. |
| 199-201 | Medium | "Single Orchestration Layer ... 3 layers collapse to 1" conflicts with the VROOM 3-supervisor model asserted elsewhere; no status marker. | Reconcile / mark as a proposal. |
| whole file | Medium | Grab-bag of shipped reality, vision, and superseded ideas with no per-section status; reads as spec. | Add status markers (Shipped / Planned / Idea). |

### `llms.txt`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 13 | High | "DEAR: Define, **Enforce**, Audit, **Resolve**" — wrong expansion in the public machine-readable summary (S1). | Correct or remove the expansion. |
| 94 | Medium | Lists `pkg/benchmark` only; `pkg/benchmark` **and** `pkg/benchmarks` both exist and are distinct (statistical A/B harness vs. SWE-bench suite runner) — undisambiguated (see P-1). | Disambiguate or list both. |
| 104 | Low | "Requires Go 1.25+" vs `go.mod:3` `go 1.26.3`. | "Go 1.26.3+". |
| whole file | Low-Medium | "Products" lists AGM/Engram/Wayfinder; **VROOM absent** though it is core per `CLAUDE.md`/`MISSION.md`. | Add VROOM. |

### `CONTRIBUTING.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 9 | Low | "Go 1.25 or later" vs `go.mod:3` `go 1.26.3`. | "Go 1.26.3+". |
| 68 | Low-Medium | Section header "## Pre-commit Hooks" but body (70-94) describes a **pre-push** hook exclusively. | Rename header to "Pre-push Hook". |
| 84 | Medium | Manual install `cp scripts/hooks/pre-push .git/hooks/pre-push` — **`scripts/hooks/` does not exist** and there is no `scripts/...pre-push` file. Actual installers: `scripts/install-git-hooks.sh`, `scripts/git-hooks/` (cf. session chezmoi note `scripts/git-hooks/pre-commit`). The documented manual path is broken. | Correct to the real installer/path. |

Verified accurate: `make install-hooks`, `make act-validate`, `make act-lint`,
`make act-test` all exist as real `Makefile` targets (`Makefile:16,19,23,28,50`).

### `ROADMAP.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 219, 605-609 | High (S1) | `OnDefine/OnEnforce/OnAudit/OnResolve`; `[resolved]` state + "on_failure resolvers" — embeds the non-canonical DEAR. | Reconcile per S1. |
| 70 | Medium-High (S4) | "one SQLite database" reinforces the README/ARCHITECTURE storage conflict. | Reconcile. |
| 208, 244, 458, 640-649 | Low | `dear-agent workflow lint`, `dear-agent search`, `dear-agent roles list`, `dear-agent workflow dev` — unified CLI that doesn't exist (binaries are separate: `cmd/workflow-*`, `cmd/dear-agent-search`). Acceptable as a *roadmap* (future) but should say so. | Note these as target UX, not current. |
| 49-53, 682-684 | Low/None | `git -C ~/src/engram-research show ...` external references — documented external source-of-truth, not dangling. | No action (note only). |

Verified accurate: `docs/adrs/ADR-009/010/011` links resolve;
`docs/workflow-engine/BACKLOG.md` exists; `docs/workflow-engine.md` exists.

### `CODEOWNERS`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| `/research/  @vbonnet` | Medium | Assigns ownership to a **nonexistent and forbidden** `/research/` path (`CLAUDE.md` forbids a `research/` tree). | Remove the `/research/` rule. |
| whole file | Low | Omits `cmd/`, `codegen/`, `config/`, `configs/`, `internal/`, `docs/`, `tests/` (default `* @vbonnet` covers them, so cosmetic). | Optionally add for clarity. |

### `AGENTS.why.md`

Verified accurate. Internally consistent; correctly states "dear-agent does
not currently have a `research/` tree" (`AGENTS.why.md:35`) — which makes the
`GOAL.md` and `CODEOWNERS` `research/` references provably wrong. No issues.

---

## 3. `docs/` Findings

### `docs/alignment/{MISSION,VISION,VALUES,GOALS}.md`

| File:loc | Severity | Issue | Fix |
|----------|----------|-------|-----|
| All four, frontmatter `adr_ref: ADR-020` | High | **ADR-020 does not exist** (S3). Four governance docs cite a missing authority. | Repoint to a real ADR. |
| `MISSION.md:7` `scope: ai-tools` | Medium (S5) | Stale predecessor name. | `scope: dear-agent`. |
| `MISSION.md:8-13` `role_mapping`; `VALUES.md §1-2`; `GOALS.md §2`; `VISION.md` (Requester-implied) | High (S2) | Non-canonical VROOM roles (`verifier/requester/...`); "Verifier role" load-bearing but not canonical. | Reconcile to canonical model. |

Otherwise the lexicographic-hierarchy content is coherent and internally
consistent (VALUES↔GOALS↔VISION agree with each other).

### `docs/adr/ADR-001-monorepo-consolidation.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 19 | Medium | "Consolidate into a single monorepo (`ai-tools`)" — repo is `dear-agent`; ADR never superseded/reconciled after the rename. | Add a superseding note or update. |
| 21 | Low-Medium | "`agm/` — AGM (renamed from agm)" — circular/nonsensical. | Correct the provenance. |
| location | High (S3) | Sole file in `docs/adr/` (singular); collides with `engram/cmd/engram/ADR-001` (different decision). | Relocate/renumber. |

### `docs/adrs/ADR-011-dear-audit-subsystem.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 3, 609 | Medium | **Status: Proposed** (not Accepted), yet `llms.txt:13` presents DEAR as the established "core philosophy". | Either accept it or stop other docs asserting it as settled. |
| 9, 45 | High (S1) | "DEAR (Define, Enforce, Audit, Resolve & Refine)" — wrong expansion in the doc that is *about* DEAR's Audit phase. | Reconcile per S1. |

### `docs/design/substrate-diagnostic.md`

| Line | Severity | Issue | Fix |
|------|----------|-------|-----|
| 138-139 | High (S1) | "DEAR governs ... (Define → Enforce → Audit → Resolve)" — `GOAL.md:74` cites this as authoritative. | Reconcile per S1. |

Other `docs/` files (`CI_GATES*`, `DEPLOYMENT`, `E2E_WORKFLOWS`, `RECOVERY`,
`SCALING`, `USER_GUIDE`, `RBAC_PROFILES`, `MIGRATION_GUIDE`, `PERFORMANCE`,
`platform-support`, `branch-protection`, `codegraph`, `deepsec`,
`features/term-denylist`, `retros/*`, `benchmarks/README`) were not exhaustively
line-audited; spot checks found no additional contradictions beyond the
systemic S1/S5 issues where DEAR/module-path strings appear.

---

## 4. Sub-Directory Findings (in-scope dirs only)

Delegated deep-audit of `engram/ codegen/ tools/ internal/ pkg/ cmd/ scripts/
config/ configs/ tests/`. Key items (cross-cutting S1/S5 already folded above):

| ID | File | Severity | Issue | Fix |
|----|------|----------|-------|-----|
| P-1 | `pkg/benchmark/doc.go` vs `pkg/benchmarks/doc.go` | Medium | Two near-identically-named packages, genuinely distinct (statistical A/B harness vs. SWE-Bench/Vibe suite runner), **no doc disambiguates them**. Navigation hazard. | Add a cross-ref line in each `doc.go`. |
| P-2 | `config/` vs `configs/` | Medium | Both top-level: `config/` = `roles.yaml` (role→model registry); `configs/` = `configs/workflows/*.yaml` (workflow templates). Each internally well-documented but **nothing explains why both exist** — a real "which do I edit?" trap. | Add a pointer note in each, or consolidate. |
| E-1 | `engram/mcp/README.md` (whole file) | High | Documents the MCP server as **Python-only** (`pip install`, `python engram_mcp_server.py`, tree rooted at `mcp-server/`). Reality: `engram/mcp/` is a **dual Python + TypeScript** impl (`package.json`, `tsconfig.json`, `src/index.ts`, `bin: engram-mcp-server → dist/index.js`); dir is `engram/mcp/` not `mcp-server/`. (This is also the answer to the brief's TypeScript question: **TypeScript exists** — `engram/mcp/src/`, plus `wayfinder/review/src/` and `.deepsec/` out of scope.) | Rewrite README around the TS server; note Python coexists or state canonical. |
| T-1 | `tools/schema-registry/README.md` + `Makefile` + `API.md` + `QUERY-IMPLEMENTATION.md` | High | Documents a "Corpus Callosum" `cc`/`cc-mcp-server` CLI. Reality: **no `cmd/`, no `package main`** — only `internal/` libs. Install steps, every `cc <cmd>` example, the `Makefile` targets, and `../../swarm/...` spec links are all dead. | Rewrite as a library, or restore the entrypoints. |
| T-2 | `tools/devlog/README.md:40,46-48` | Medium | `go install github.com/vbonnet/dear-agent/devlog@latest` (no such module path — should be `.../tools/devlog`); `cd ai-tools/devlog-cli` does not exist (tool is `tools/devlog/`). | Correct both paths. |
| T-3 | `tools/devlog/docs/README-patterns.md:31` | Medium | `[SPEC.md](SPEC.md)` dangling — sibling is `SPEC-patterns.md`. | Repoint link. |
| E-2 | `engram/hooks-bin/README.md:378,353` | Medium | Dangling `[SPEC.md](cmd/sessionstart-guardian/SPEC.md)`; no `sessionstart-guardian` under `engram/hooks-bin/cmd/`. | Remove/correct. |
| E-3 | `engram/hippocampus/README.md:29,42` | Low | Embeds the **ecphory** C4 diagram under a "Hippocampus Component Diagram" caption — mislabeled. | Generate a real hippocampus diagram or relabel. |
| E-4 | `engram/README.md:6-15` | Low | Component table omits the existing, documented `retrieval/` directory. | Add a `retrieval/` row. |
| P-3 | `pkg/workspace/dolt/README.md:429` | Medium | Dangling `[MIGRATION-GUIDE.md](./MIGRATION-GUIDE.md)`; likely meant `pkg/workspace/MIGRATION.md` one level up. | Repoint or add file. |
| X-1 | `tools/spec-review/ARCHITECTURE.md`, `DEPENDENCY-*.md` | Low | Links into `../../swarm/...` (predecessor workspace layout) — outside the repo. | Drop/repoint. |
| X-2 | `scripts/README-verify-cli-docs.md` | Low | Install snippet uses a legacy `main/scripts/...` worktree-prefix assumption. | Cosmetic; align to current layout. |

**Verified clean (in scope):** `codegen/doc.go`; `cmd/` (all 21 binaries
referenced by `configs/workflows/README.md` exist; no doc files); `tests/`
(no docs); `scripts/verify-cli-docs.sh` ↔ its README options match;
`pkg/cliframe`, `pkg/output-formatter`, `pkg/validator`, `pkg/validation/scope`,
`internal/sandbox`, `internal/ci`, `configs/workflows/README.md` — accurate
apart from S1/S5 string occurrences. **No DEAR/VROOM mis-expansion was found
inside these sub-dir docs** — the DEAR/VROOM problems are concentrated in the
top-level + `docs/` files (S1, S2), which is this audit's core scope.

---

## 5. Priority Summary

**High (fix first):**
S1 (DEAR expansion, repo-wide), S2 (VROOM model), S3 (ADR dirs/numbering),
S5 (stale module path / "ai-tools"), H5/`GOAL.md` (nonexistent `dear-agent`
CLI + forbidden `research/` links), `ARCHITECTURE.md:146-147` (VROOM),
alignment `adr_ref: ADR-020`, `engram/mcp/README.md` (E-1),
`tools/schema-registry` (T-1).

**Medium:**
S4 (SQLite vs Dolt), `ARCHITECTURE.md:5` "four products" & path prefixes,
`GOAL.md` `.yaml`/status markers, `CONTRIBUTING.md:84` broken hook path,
`CODEOWNERS` `/research/`, `ADR-001` staleness, `ADR-011` Proposed-vs-asserted,
P-1 (benchmark/benchmarks), P-2 (config/configs), T-2/T-3/E-2/P-3.

**Low:**
Go-version understatement (README/CONTRIBUTING/llms.txt), README/llms.txt
completeness (DEAR/VROOM/dirs absent), E-3, E-4, X-1, X-2, CONTRIBUTING header
label, CODEOWNERS completeness.

**Overall picture:** Repo-level docs do *not* accurately describe the repo on
its two namesake concepts. DEAR (the "D" and "R" in `dear-agent`) is expanded
the wrong way nearly everywhere it appears and never defined where a reader
would look; VROOM has four competing role taxonomies; the AGM/Engram/Wayfinder
relationships are stated consistently (AGM is the tool, Wayfinder is the
9-phase SDLC workflow, Engram is memory) but **VROOM and DEAR are essentially
undocumented at the top level** despite being mandated by `CLAUDE.md`. Add
TypeScript honesty (`engram/mcp/` is TS+Python), reconcile the storage
substrate, and consolidate ADRs.

*End of report. No files other than this report were modified.*
