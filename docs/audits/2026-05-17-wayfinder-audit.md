# Wayfinder Documentation Audit — 2026-05-17

**Scope:** `wayfinder/` directory only (docs, READMEs, ARCHITECTURE/SPEC/SKILL,
code comments). Read-only audit — **no fixes applied**, this report is the
deliverable.

**Method:** Full read of the four top-level docs by the lead auditor; three
parallel sub-auditors covered the `cmd/wayfinder-session/` doc cluster, the
`lib/`/`commands/`/`coordinator/`/`hooks/` doc+comment cluster, and the
`wayfinder/review/` sub-plugin doc cluster. Every path/claim was verified
against the working tree.

## Ground-truth facts established

| Fact | Verified value |
|------|----------------|
| Go module path | `github.com/vbonnet/dear-agent` |
| Repo-root dirs | `agm cmd codegen config configs docs engram internal pkg scripts tests tools wayfinder` |
| `core/`, `core/cortex/`, `cortex/`, `main/`, `plugins/` | **do not exist** |
| `engram/core/` | **does not exist** (only `engram/` exists, no `core/` under it) |
| TypeScript under `wayfinder/` | **0 files** outside `wayfinder/review/` (237 `.go`, 52 `.md`, 14 `.sh`, 9 `.py`, **0 `.ts`**) |
| Phase orchestration / context compiler / W0 detector / scope validator | implemented in **Go** (`wayfinder/internal/{phaseisolation,w0}/*.go`), not TypeScript |
| Validation engine | **Go only** (`wayfinder/cmd/wayfinder-session/internal/validator/*.go`) |
| `wayfinder/internal/` packages (actual) | `analytics corpus events phaseisolation project status tracker w0` |
| `wayfinder/cmd/wayfinder-session/internal/` packages (actual) | `archive buildloop config converter git history integration lintcontext migrate migration orchestrator phasegraph resume retrospective review status taskmanager telemetry tracker validator workspace` |
| `docs/wayfinder/` | **does not exist** |
| `docs/adr/` (singular) | only `ADR-001-monorepo-consolidation.md` |
| `docs/adrs/` (plural) | `ADR-008 … ADR-022` (none Wayfinder-specific) |
| Wayfinder ADRs (`ADR-001-phase-consolidation`, `ADR-002-build-loop-tdd-enforcement`, ADR-003/004/005) | **do not exist anywhere** |
| AGM workspace-isolation test | `agm/internal/dolt/workspace_isolation_test.go` (no `main/` prefix) |
| Slash-command files in `wayfinder/commands/` | only `validate-phase.md` (+ its README); no `start/next/close/rewind/verify` command files |

## Executive summary

The single most serious problem — **flagged by the user and confirmed** — is
that `wayfinder/ARCHITECTURE.md` (and `SPEC.md`, `SKILL.md`, `README.md`)
claim Wayfinder's phase orchestration / validation is implemented in
**TypeScript**. It is not. There is **zero TypeScript** anywhere under
`wayfinder/` except the self-contained `wayfinder/review/` sub-plugin (a
separate multi-persona code-review tool). Phase execution, context
compilation, the W0 detector, signal detection, and scope validation are all
**Go** (with shell scripts in `lib/` and Python in `lib/metacontext/`). The
TypeScript narrative is pervasive: architecture diagrams, component
descriptions, dependency lists, API examples, an `@wayfinder/core` npm import,
and even Go package doc-comments that say *"ported from the TypeScript
implementation in cortex/lib/"*.

Five cross-cutting defect classes account for the bulk of findings:

1. **TypeScript-implementation misclaim** (HIGH) — the user's flagged bug,
   spread across all four top-level docs + two Go package comments.
2. **Stale path prefixes from a defunct monorepo layout** — `core/cortex/`,
   `cortex/`, `engram/core/`, `main/agm/`, `plugins/wayfinder/`,
   `engram/plugins/`. The tree was moved to `wayfinder/...` at repo root;
   docs were never updated.
3. **Dangling ADR references** — `docs/wayfinder/ADR-*` doesn't exist, and
   ADR-002 means "9-phase structure" in SPEC but "build-loop-tdd-enforcement"
   in SKILL (self-contradictory numbering).
4. **Phase terminology drift** — V1 codes (W0/D1–D4/S4–S11) vs V2 names
   (CHARTER…RETRO) vs "9 phases" (SPEC/README) vs "12-phase" (code); plus
   artifact-filename and command-syntax (`/wayfinder:x` vs `/wayfinder-x` vs
   `wayfinder-session x`) inconsistency.
5. **`wayfinder/review/` doc rot** — wrong npm scope (`@engram/` vs
   `@wayfinder/`), `.engram/` vs `.wayfinder/` config paths, contradictory
   test counts and model pricing, and unbacked persona rosters.

**Relationship & VROOM-model checks (task items 4 & 5):**

- **No old 5-role VROOM model references** (Verifier / Requester /
  Orchestrator / Overseer / Meta-Orchestrator) appear anywhere under
  `wayfinder/` — **clean on this axis** (the only "orchestrator" usages are
  `sub-agent-orchestrator` and the Go `orchestrator` package, unrelated to
  the VROOM role model).
- **No Wayfinder↔VROOM↔DEAR relationship is documented at all.** "VROOM"
  does not appear anywhere in `wayfinder/`. This is itself a **documentation
  gap**: the project frames Wayfinder (research & planning) and VROOM
  (execution) and DEAR (Define→Execute→Audit→Retro) as one coherent system,
  but the Wayfinder docs never situate Wayfinder within it. No doc
  *mis-states* the relationship (no conflation found), so this is a GAP
  (recommend adding a "Relationship to VROOM / DEAR" section to
  `wayfinder/ARCHITECTURE.md`), not an inaccuracy.
- **AGM** is referenced legitimately (workspace detection / session
  integration); those claims are accurate apart from the stale `main/agm/`
  path prefix (correct: `agm/`).

> Line numbers from the three cluster sub-audits are reported as-observed;
> the lead auditor independently verified every HIGH finding in the
> top-level docs and the ground-truth facts table.

---

## Findings — top-level docs

### wayfinder/ARCHITECTURE.md

- [HIGH] wayfinder/ARCHITECTURE.md:5-6 — "three layers: session management (Go CLI), phase execution (**TypeScript**), and validation (Go + **TS**)" is false; phase execution is Go (`wayfinder/internal/phaseisolation/*.go`), validation is Go only. — Rewrite as Go (+ shell/Python); delete all TypeScript references.
- [HIGH] wayfinder/ARCHITECTURE.md:27 — Diagram box "Phase Orchestrator (TypeScript)" is false. — Relabel "(Go — `internal/phaseisolation`)".
- [HIGH] wayfinder/ARCHITECTURE.md:26-33 — Diagram box "Validation Engine (Go + TypeScript)" is false; validation is Go only. — Relabel "(Go — `cmd/wayfinder-session/internal/validator`)".
- [HIGH] wayfinder/ARCHITECTURE.md:166-167 — Design decision "Dual-language: Go for CLI/validation …, TypeScript for phase orchestration" is false. — Replace with the real language model (Go core; shell gate-checks; Python metacontext; TS only in `review/`).
- [MEDIUM] wayfinder/ARCHITECTURE.md:65,75,79,83,87,93,97 — "Key Packages" lists `internal/buildloop`, `internal/config`, `internal/git`, `internal/history`, `internal/lintcontext`, `internal/archive`, `internal/migrate`; these live under `wayfinder/cmd/wayfinder-session/internal/`, not `wayfinder/internal/`. — Correct each package path prefix.
- [MEDIUM] wayfinder/ARCHITECTURE.md:47-113 — "Key Packages" omits every package that actually lives at `wayfinder/internal/` (`analytics corpus events phaseisolation project status tracker w0`). — Add the real `wayfinder/internal/*` packages.
- [MEDIUM] wayfinder/ARCHITECTURE.md:117 — `core/cortex/config/phase-dependencies.yaml` does not exist (no `core/cortex/`). — Locate the real phase-dependency config or remove the reference.
- [MEDIUM] wayfinder/ARCHITECTURE.md:39-44 — Artifact filenames `W0-charter.md`, `D1-problem.md`, `D2-research.md` differ from `SPEC.md:92-97` (`W0-project-charter.md`, `D1-problem-validation.md`, `D2-existing-solutions.md`); code (`internal/resume/detector.go:43`) recognizes only `W0-charter.md`/`W0.md`. — Pick one artifact-naming scheme and align ARCHITECTURE, SPEC, and the resume detector.
- [MEDIUM] wayfinder/ARCHITECTURE.md:130-157 — Data-flow uses V1 codes (D1/D2/D3) while `README.md` uses V2 names (CHARTER…RETRO); cross-doc terminology drift. — Standardize on V2 names (keep V1 only in the explicit migration mapping).
- [LOW] wayfinder/ARCHITECTURE.md:161-164 — "No SQLite or hidden state" sits in tension with `cmd/wayfinder-session/migrations/*.sql` (a MySQL `wayfinder_phases→waypoints` schema). — Add a sentence clarifying the SQL schema is an optional/separate backend, not the default filesystem store.

### wayfinder/README.md

- [HIGH] wayfinder/README.md:82 — Dependency "Node.js >= 18.0.0 (TypeScript phase orchestrator)" — no TS phase orchestrator exists; Node is only needed for the `review/` sub-plugin. — Remove or scope strictly to `wayfinder/review/`.
- [HIGH] wayfinder/README.md:26-31 — Commands use colon form (`/wayfinder:start|:next|:close|:run-all-phases|:rewind|:verify`); `SKILL.md:146-151` uses hyphen form (`/wayfinder-start|-next-phase|-run-all-phases|-stop|-rewind`); `:close` vs `-stop`, `:next` vs `-next-phase`, `:verify` absent from SKILL. No backing command file exists in `wayfinder/commands/` for any of them (only `validate-phase`). — Reconcile command syntax across README/SKILL/ARCHITECTURE and confirm the commands actually ship.
- [MEDIUM] wayfinder/README.md:84-85 — Lists only `cobra`/`yaml` as deps but the doc's own framing implies a Node/TS toolchain; under-/mis-states the real dependency set (Go + cobra + yaml + shell + Python). — Align dependency list with reality.

### wayfinder/SPEC.md

- [HIGH] wayfinder/SPEC.md:58 — "session management (Go), phase execution (**TypeScript**), and validation (Go + **TypeScript**)" — false. — Rewrite as Go.
- [HIGH] wayfinder/SPEC.md:79-86 — Diagram "Phase Orchestrator (lib/*.ts)" — `wayfinder/lib/` contains `.sh`/`.py`/`.go`, **no `.ts`**. — Relabel to the Go package.
- [HIGH] wayfinder/SPEC.md:112-119 — "Component 2: Phase Orchestrator (TypeScript)" — false (Go: `internal/phaseisolation`). — Correct language + path.
- [HIGH] wayfinder/SPEC.md:121-128 — "Component 3: Validation Engine (Go + TypeScript)" — Go only. — Remove TypeScript.
- [HIGH] wayfinder/SPEC.md:130-137 — "Component 4: Signal Detector (TypeScript)" — false. — Correct to Go.
- [HIGH] wayfinder/SPEC.md:139-146 — "Component 5: W0 Detector (TypeScript)" — false; implemented in `wayfinder/internal/w0/*.go`. — Correct to Go.
- [HIGH] wayfinder/SPEC.md:329-340 — `import { PhaseOrchestrator } from '@wayfinder/core';` — no such npm package; no TS. — Remove the TS API block or replace with the real Go API.
- [HIGH] wayfinder/SPEC.md:345 — `import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/validator"` — wrong module; actual is `github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/validator`. — Fix the import path.
- [HIGH] wayfinder/SPEC.md:357-368 — `SignalDetector` TypeScript API example — no such code. — Remove or replace with the Go equivalent.
- [HIGH] wayfinder/SPEC.md:226 — "Test Coverage: ≥80% for **TypeScript orchestration**, ≥70% for Go validation" — references nonexistent TS. — Restate as Go coverage targets.
- [MEDIUM] wayfinder/SPEC.md:281-285 — TypeScript/Node.js external libs (`unified`, `remark-parse`, `fast-levenshtein`, `vitest`) cited for scope validation, which is Go (`internal/phaseisolation/scopevalidator.go`). — Remove; these belong only to `review/` if anywhere.
- [MEDIUM] wayfinder/SPEC.md:293-296 — Internal deps `engram/core/pkg/{progress,eventbus,ecphory}` — `engram/core/` does not exist. — Locate real packages or remove.
- [MEDIUM] wayfinder/SPEC.md:187,192,197,202,414 — `See ADR-001/002/003/004` and `docs/adr/` for rationale — none of these Wayfinder ADRs exist; `docs/adr/` has only `ADR-001-monorepo-consolidation.md`. ADR-002 = "9-Phase Structure" here contradicts `SKILL.md` (ADR-002 = build-loop-tdd-enforcement). — Create the ADRs or remove the references; fix the ADR-002 collision.
- [MEDIUM] wayfinder/SPEC.md:289 — `golang.org/x/crypto/sha256` is not a real import path (SHA-256 is stdlib `crypto/sha256`; not present in `internal/validator/hash.go`). — Correct to the actual import.
- [MEDIUM] wayfinder/SPEC.md:4 vs SKILL.md:14 — SPEC `Version: 0.1.0` vs SKILL frontmatter `version: 2.0.0`; both describe a "V2 schema". — Reconcile a single version.
- [LOW] wayfinder/SPEC.md:234-239 — "Code Review Integration: No GitHub/GitLab PR integration" while `wayfinder/review/` ships exactly that in-tree. — Add a cross-reference clarifying `review/` is the (separate) PR-review surface.

### wayfinder/SKILL.md

- [HIGH] wayfinder/SKILL.md:217 — "TypeScript: `core/cortex/lib/{phase-definitions,context-compiler}.ts`" — neither the path nor the language is real; logic is Go (`internal/phaseisolation/contextcompiler.go`, verified to implement summarization/`estimateTokens`). — Replace with the Go package path.
- [MEDIUM] wayfinder/SKILL.md:110,214 — `core/cortex/config/phase-dependencies.yaml` — path does not exist. — Correct or remove.
- [MEDIUM] wayfinder/SKILL.md:215 — `docs/wayfinder/ADR-001-phase-consolidation.md`, `ADR-002-build-loop-tdd-enforcement.md` — `docs/wayfinder/` and these ADRs do not exist; ADR-002 label contradicts `SPEC.md`. — Create/relocate ADRs or remove; resolve numbering.
- [MEDIUM] wayfinder/SKILL.md:118 — "See ADR-005 (revised 2026-03-24)" — ADR-005 does not exist. — Remove or create.
- [MEDIUM] wayfinder/SKILL.md:216 — Go packages path `core/cortex/cmd/wayfinder-session/internal/{phasegraph,lintcontext,telemetry}/` — stale prefix; correct is `wayfinder/cmd/wayfinder-session/internal/...`. — Fix prefix.
- [MEDIUM] wayfinder/SKILL.md:122-138 — V1→V2 mapping table (`S6→DESIGN`, `S7→PLAN`) contradicts `internal/converter/converter.go` (reported `S6→PhaseV2Plan`, `S7→PhaseV2Setup`, `W0→PhaseV2Charter`). — Reconcile the table with `converter.go`.
- [LOW] wayfinder/SKILL.md:207-208 — "Related Beads: oss-b74m.4, oss-b74m.5" — unverifiable bead IDs (canonical tracker is the `ce-6as` epic). — Verify or remove.

---

## Findings — `cmd/wayfinder-session/` doc cluster

### wayfinder/cmd/wayfinder-session/ARCHITECTURE.md

- [HIGH] wayfinder/cmd/wayfinder-session/ARCHITECTURE.md:24,28,179-205,408-425 — Phases described in V1 codes (W0/D1–D4/S6–S8/S11) while the V2 code (`internal/status/types_v2.go`) and sibling `SPEC.md` use V2 names; internal inconsistency. — Rewrite to V2 names (CHARTER…RETRO).
- [HIGH] wayfinder/cmd/wayfinder-session/ARCHITECTURE.md:518-522 — Validator signatures wrong: doc shows a `forceOverride bool` param that `internal/validator/doc_quality_gate.go` (`validateDocQuality(phaseName, projectDir string)` @50, `validateD3Documents(projectDir string)` @69) does not have. — Update signatures to match code.
- [HIGH] wayfinder/cmd/wayfinder-session/ARCHITECTURE.md:101-144,332-337 — Package tree describes only ~5 packages and claims "5 core packages"; the directory has ~22. — Regenerate the tree from `internal/` and fix the count.
- [MEDIUM] wayfinder/cmd/wayfinder-session/ARCHITECTURE.md:213-217,571-575 — CLI examples use nonexistent subcommands (`add-task`, `complete-task`, `roadmap`, `task-status`); real surface is `task add|list|show|update|delete`. — Replace with real syntax.
- [MEDIUM] wayfinder/cmd/wayfinder-session/ARCHITECTURE.md:551,555 — `wayfinder session complete-phase` (space) vs real binary `wayfinder-session` (hyphen). — Fix.
- [LOW] wayfinder/cmd/wayfinder-session/ARCHITECTURE.md:102 — "SPEC.md (348 lines)"; actual is 364. — Drop hardcoded line count.
- [LOW] wayfinder/cmd/wayfinder-session/ARCHITECTURE.md:146,602-604 — Hardcoded test/coverage snapshots ("193 total tests", "49 tests, 81.2%"). — Mark as point-in-time or remove.

### wayfinder/cmd/wayfinder-session/README-STORAGE-CONFIG.md

- [HIGH] README-STORAGE-CONFIG.md:200 — Import `github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/config`; correct is `github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/config`. — Fix.
- [MEDIUM] README-STORAGE-CONFIG.md:256,381 — `cd core/cortex/cmd/wayfinder-session`; correct is `wayfinder/cmd/wayfinder-session`. — Fix both.
- [MEDIUM] README-STORAGE-CONFIG.md:385-387 — Dangling links `CONFIG-SCHEMA.md`, `SYMLINK-BOOTSTRAP.md`, `swarm/workspace-aware-tools/README.md` — none exist. — Remove or repoint.
- [LOW] README-STORAGE-CONFIG.md:351-354 — `cc register`/`cc query` examples are aspirational ("integration pending"). — Mark explicitly as not-yet-implemented.

### wayfinder/cmd/wayfinder-session/CORPUS-CALLOSUM-INTEGRATION.md

- [MEDIUM] CORPUS-CALLOSUM-INTEGRATION.md:260 — Dangling `main/corpus-callosum/README.md` (no `main/`, no `corpus-callosum`). — Remove or mark external.
- [MEDIUM] CORPUS-CALLOSUM-INTEGRATION.md:261 — Dangling `TASK-4.2-ANALYSIS-REPORT.md`. — Remove or restore.
- [LOW] CORPUS-CALLOSUM-INTEGRATION.md:198,206 — `./schema/scripts/register-schema.sh`; scripts are at `scripts/register-schema.sh` (sibling of `schema/`), contradicting this file's own "Files Created" list. — Use `./scripts/...` consistently.
- [LOW] CORPUS-CALLOSUM-INTEGRATION.md:1-9,66-114 — "✅ COMPLETE" with a fabricated all-pass transcript for a `cc` tool not present in the repo. — Reframe as "schema+scripts delivered; CC integration untested".

### wayfinder/cmd/wayfinder-session/TASK_2.3_SUMMARY.md

- [HIGH] TASK_2.3_SUMMARY.md:11,38,63,91,358,380 — Stale root `cortex/cmd/wayfinder-session/...`; correct is `wayfinder/cmd/wayfinder-session/`. — Replace `cortex/`→`wayfinder/`.
- [HIGH] TASK_2.3_SUMMARY.md:253-263,271-280 — Corrupted placeholder: "the git history" used as the `migrate-all` workspace-root argument (real arg is a path, e.g. `~/src/ws/oss/wf`). — Replace the bad find/replace artifact.
- [LOW] TASK_2.3_SUMMARY.md:445 — Unverifiable bead ID `oss-s2f3`. — Normalize or drop.

### wayfinder/cmd/wayfinder-session/MIGRATION_GUIDE.md

- [HIGH] MIGRATION_GUIDE.md:9,16,23,149,164-173,187-219,262-313,361 — Pervasive corrupted "the git history" placeholder used as the workspace-root arg and in `ls`/`cat`/`cd`/`chmod`/`find`/`cp` examples; every example is non-runnable. — Global replace with a real workspace-root path.
- [MEDIUM] MIGRATION_GUIDE.md:380 — Dangling link `docs/tasks/2.3-batch-migration.md`. — Remove or repoint to `TASK_2.3_SUMMARY.md`.
- [MEDIUM] MIGRATION_GUIDE.md:63,96,106,317,324-327 — Phase-mapping prose/table contradicts `internal/converter/converter.go` (claims `W0→D1`/`W0→W0`; converter maps `W0→CHARTER`). — Align with converter.
- [LOW] MIGRATION_GUIDE.md:93-103 — Mapping table contradicts the prose directly below it. — Make table+prose+converter agree.

### wayfinder/cmd/wayfinder-session/SPEC.md

- [HIGH] cmd/wayfinder-session/SPEC.md:64-138 — V2 schema YAML doesn't match `internal/status/types_v2.go` `StatusV2` struct: `current_phase`→`current_waypoint`, `phases`→`waypoint_history`, `quality`→`quality_metrics`; extra `tests:`/`lifecycle_state:` not in struct; example uses V1 `name: "W0"`. — Regenerate from struct tags.
- [MEDIUM] cmd/wayfinder-session/SPEC.md:202-234,310-324 — Mixes V1 codes (D4/S6/S8) with the V2 table above. — Use V2 names consistently.
- [MEDIUM] cmd/wayfinder-session/SPEC.md:294-300 — "Phase Mapping" (`S6→S7`, `W0→W0`) contradicts `converter.go` (`S6→Plan`, `S7→Setup`, `W0→Charter`). — Correct mapping.
- [LOW] cmd/wayfinder-session/SPEC.md:5 — "tight integration with the Engram ecosystem" vs project `dear-agent` (naming drift). — Optional alignment.

### wayfinder/cmd/wayfinder-session/migrations/README.md

- [MEDIUM] migrations/README.md:243-244 — Dangling `ROLLBACK-PLAN.md in swarm directory` and root `TROUBLESHOOTING.md` — neither exists. — Remove/repoint.
- [LOW] migrations/README.md:1-3,234-237 — Frames a MySQL schema as the live storage model though default storage is filesystem (`README-STORAGE-CONFIG.md`). — Clarify it's a separate/optional backend. (Migration scripts `001..004`+`ROLLBACK_004.sql` all exist; phase→waypoint terminology matches code.)

### wayfinder/cmd/wayfinder-session/schema/README.md

- [HIGH] schema/README.md:155-156 — `https://github.com/vbonnet/engram` + `core/docs/wayfinder` — wrong repo (`dear-agent`) and nonexistent path. — Correct repo URL/path.
- [MEDIUM] schema/README.md:206 — Dangling `../../../../ai-tools/main/corpus-callosum/README.md`. — Remove/correct.
- [MEDIUM] schema/README.md:207 — Dangling `[Wayfinder Documentation](../../../docs/wayfinder/)` (`docs/wayfinder/` doesn't exist). — Repoint to `wayfinder/SPEC.md`.
- [MEDIUM] schema/README.md:208 — Dangling `swarm/projects/modular-architecture-system/` link. — Remove.

### wayfinder/cmd/wayfinder-session/internal/workspace/{README,INDEX,DELIVERABLE_SUMMARY,VALIDATION_REPORT,TEST_EXECUTION_GUIDE}.md

- [HIGH] internal/workspace/INDEX.md:8-9,25,46,52-68 — File index lists nonexistent `run_tests.sh` and omits the real `workspace_detection_test.go`; "11 files, ~3,300 lines" total is wrong. — Regenerate index from the real directory.
- [HIGH] internal/workspace/DELIVERABLE_SUMMARY.md:93-107,237-245,294-310 — Documents `run_tests.sh` (200 lines, "Executable: Yes") + a fabricated passing transcript; the script does not exist. — Remove the claims; reconcile file list.
- [HIGH] internal/workspace/VALIDATION_REPORT.md:30-33,396-453,463-466 — Reports `run_tests.sh` as delivered with a fabricated "85.2% coverage" execution transcript; script absent. — Remove/replace with real numbers.
- [MEDIUM] internal/workspace/README.md:163-166,303 — `main/agm/internal/dolt/workspace_isolation_test.go` stale; real path `agm/internal/dolt/workspace_isolation_test.go`. — Fix.
- [MEDIUM] internal/workspace/README.md:304 — `cortex/cmd/wayfinder-session/internal/status/` stale → `wayfinder/cmd/...`. — Fix.
- [MEDIUM] internal/workspace/README.md:20,33,42-43 — Instructs `./run_tests.sh` which does not exist (referenced across 3 files). — Restore script or use direct `go test`.
- [MEDIUM] internal/workspace/INDEX.md:3,151-152 — Stale self-location `cortex/...` and stale `main/agm/...` + `cortex/.../status/`. — Fix prefixes.
- [MEDIUM] internal/workspace/DELIVERABLE_SUMMARY.md:14,221,242,336 — Stale `cortex/...` and `main/agm/...` paths. — Fix.
- [MEDIUM] internal/workspace/VALIDATION_REPORT.md:271,398 — Stale `main/agm/...` and `cd cortex/...`. — Fix.
- [MEDIUM] internal/workspace/TEST_EXECUTION_GUIDE.md:16,144,153,178 — Stale `cortex/...` and `core/cortex/...` (CI snippet + pre-commit hook). — Fix to `wayfinder/...`.
- [LOW] internal/workspace/README.md:13-21 — Example value "OSS workspace: the git history" is the same corrupted placeholder. — Replace with a concrete path.

---

## Findings — `lib/`, `commands/`, `coordinator/`, `hooks/` cluster

### wayfinder/commands/validate-phase.md

- [MEDIUM] wayfinder/commands/validate-phase.md:714 — Dangling `cortex/docs/adr/wayfinder-workflow.md`. — Remove or point to `wayfinder/SPEC.md`/`docs/adrs/`.
- [MEDIUM] wayfinder/commands/validate-phase.md:713 — Dangling `WAYFINDER-VALIDATE-PHASE-SPEC.md` (does not exist). — Remove/replace.
- [MEDIUM] wayfinder/commands/validate-phase.md:13,169,360,448,472,567 — Command-syntax drift (`/wayfinder:validate-phase`, `/engram:wayfinder-validate-phase`, `/wayfinder:next-phase`). — Standardize on the `wayfinder-<x>` / `wayfinder-session <x>` forms.
- [MEDIUM] wayfinder/commands/validate-phase.md:21,24 — References `/wayfinder-next-phase`, `/wayfinder-start` with no backing command file in the repo. — Verify the commands exist or correct names.
- [LOW] wayfinder/commands/validate-phase.md:731 — "See Task 1.4 in ROADMAP.md" — no such task. — Remove/update.
- [LOW] wayfinder/commands/validate-phase.md:381-408,552 — S8 labeled "Implementation" here vs "Execute Build" in the README vs `BUILD` in code; lists S9/S10 as distinct gating phases outside the canonical 9-phase set. — Reconcile with `phaseisolation/types.go`.

### wayfinder/commands/validate-phase-README.md

- [HIGH] wayfinder/commands/validate-phase-README.md:76,183,341 — Claims a `--skip-tests` flag exists; `validate-phase.md:174,496` states it was REMOVED and tests cannot be skipped. — Remove all `--skip-tests` mentions.
- [MEDIUM] wayfinder/commands/validate-phase-README.md:382 — Stale `plugins/wayfinder/commands/validate-phase.md` (no `plugins/` tree) → `wayfinder/commands/validate-phase.md`. — Fix.
- [MEDIUM] wayfinder/commands/validate-phase-README.md:381 — Dangling `WAYFINDER-VALIDATE-PHASE-SPEC.md`. — Remove/fix.
- [MEDIUM] wayfinder/commands/validate-phase-README.md:1,11,200,375,377 — Mixed/nonexistent command syntax (`/wayfinder:validate-phase`, `/wayfinder-next-phase`, `/wayfinder-start`, `/wayfinder-stop`, `/engram:bow`). — Standardize + verify existence.
- [LOW] wayfinder/commands/validate-phase-README.md:110,135 — S8 "Execute Build" vs "Implementation" elsewhere. — Reconcile display names.

### wayfinder/lib/tests/README-WAYPOINT-SUMMARIZATION-TESTS.md

- [HIGH] wayfinder/lib/tests/README-WAYPOINT-SUMMARIZATION-TESTS.md:10,156,181-183 — Claims the live impl is "Rule-Based Extraction (TypeScript)" at `engram/plugins/wayfinder/lib/context-compiler.ts` with `context-compiler.test.ts`; actual impl is Go (`wayfinder/internal/phaseisolation/contextcompiler.go`, verified). — Rewrite to describe the Go implementation/paths.
- [MEDIUM] wayfinder/lib/tests/README-WAYPOINT-SUMMARIZATION-TESTS.md:101,107-119 — Stale run instructions `cd engram/plugins/wayfinder` + `npm test -- context-compiler.test.ts`. — Replace with the Go test invocation.
- [LOW] wayfinder/lib/tests/README-WAYPOINT-SUMMARIZATION-TESTS.md:175 — Dangling `./pre-alpha-bonus/PA-034/`. — Remove/relocate.
- [LOW] wayfinder/lib/tests/README-WAYPOINT-SUMMARIZATION-TESTS.md:68,88 — Uses S9 outside the canonical phase set (phase-model drift). — Note/reconcile.

### wayfinder/lib/*.sh and wayfinder/lib/* scripts

- [MEDIUM] wayfinder/lib/d1-gate-check.sh:8 — Hard-codes `W0-charter.md`; canonical artifact name in `SPEC.md:92` (and used by `stop-wayfinder-guard`, `s9-test-count-verification.sh:13`) is `W0-project-charter.md`; this gate will silently skip the canonically-named charter. — Reconcile on one name (or glob `W0-*.md`).
- [LOW] wayfinder/lib/test-s8-gate-check.sh:2, wayfinder/lib/test-s9-test-count-verification.sh:2 — Header comment references `gate-3-violation-fix.md`, which does not exist. — Remove/update the reference.
- CLEAN: `wayfinder-decompose`, `wayfinder-spawn-sub`, `wayfinder-monitor-subs`, `generate-s11-classification-assessment`, and the `d1/d2/s8/s9-gate-check.sh`, `s8-mid-phase-checkpoint.sh`, `s9-test-count-verification.sh` header/usage comments accurately describe behavior. (Note: `wayfinder-monitor-subs` `PHASE_ORDER` spans S4–S10, broader than SPEC's 9-phase set — a SPEC-vs-code phase-model drift, not a script defect.)

### wayfinder/lib/metacontext/*.py

- [MEDIUM] wayfinder/lib/metacontext/__init__.py:14 — Docstring example imports `from plugins.wayfinder.lib.metacontext import ...`; no `plugins/wayfinder/` tree exists; real module is `wayfinder/lib/metacontext`. — Correct the import example.
- CLEAN: module docstrings in `analyzer.py`, `company_override.py`, `conversation_scanner.py`, `dependency_scanner.py`, `file_scanner.py`, `prioritizer.py`, `signal_fusion.py`, `strategy_selector.py` accurately summarize each module.

### wayfinder/internal/w0/*.go and wayfinder/internal/phaseisolation/*.go (package/doc comments)

- [HIGH] wayfinder/internal/w0/detector.go:1-2 — Package doc: *"ported from the TypeScript implementation in cortex/lib/w0-*.ts"* — `cortex/lib/w0-*.ts` does not exist; W0 is implemented in Go here. The dead path misleads anyone seeking the "source". — Drop the path (a historical "originally prototyped in TypeScript" note without a dead path is acceptable).
- [HIGH] wayfinder/internal/phaseisolation/types.go:1-3 — Package doc: *"ported from the TypeScript implementation in cortex/lib/"* — dead path; impl is Go in this package. — Remove the dangling `cortex/lib/` reference.
- [MEDIUM] wayfinder/internal/phaseisolation/types.go:2 — Package doc says "12-phase workflow" while `wayfinder/SPEC.md`/`README.md` say "9 phases" — code-vs-doc phase-count drift (code's 12 V1 IDs map to 9 V2 names). — Reconcile the SPEC/README "9-phase" wording with the code's 12-ID model (explain the mapping).
- [LOW] wayfinder/internal/phaseisolation/definitions.go:19 — Doc comment "maps **TypeScript** phase IDs to Go phase names" risks implying live TS. — Reword to "legacy V1 phase IDs".
- CLEAN: `wayfinder/hooks/cmd/stop-wayfinder-guard/main.go`, `wayfinder/coordinator/coordinator.go`, `wayfinder/coordinator/monitor.go` — package/doc comments accurately match the Go implementation; no stale paths, no language misclaims (independently verified).

---

## Findings — `wayfinder/review/` sub-plugin

> `wayfinder/review/` is a self-contained TypeScript multi-persona
> code-review tool. TypeScript usage here is **correct** and is not flagged.
> No doc in `review/` claims to be the Wayfinder phase orchestrator, claims
> the parent Wayfinder is TypeScript, mis-attributes VROOM/DEAR, or uses the
> deprecated 5-role VROOM terms — those audit axes are **clean** here. The
> issues are stale paths/links, code-vs-doc contradictions, and version drift.

### Cross-cutting (review/)

- [HIGH] Wrong npm scope `@engram/multi-persona-review` vs actual `@wayfinder/multi-persona-review` (`package.json`) — DEPLOYMENT.md:56,60,91,137,200,281,412; DOCUMENTATION.md:668,673,322,379; AGENTS.ai.md:322,334,379,834 (mixed with correct `@wayfinder/` at 478,544). — Normalize to `@wayfinder/`.
- [MEDIUM] `.engram/config.yml` / `.engram/personas` vs source `.wayfinder/` (`src/config-loader.ts:132`, `src/cli.ts:595`) — README.md:315,486-487; DOCUMENTATION.md:90,162,177-179,1737; DEPLOYMENT.md:346,785; docs/ADVANCED-USAGE.md:132; docs/GETTING-STARTED.md:172; docs/TROUBLESHOOTING.md:553,647,650; AGENTS.ai.md:12,94,159,1012-1013. — Standardize on `.wayfinder/`.
- [MEDIUM] Test-count chaos: 300 (README/CONTRIBUTING/DIAGRAMS) vs 279 (CHANGELOG:25) vs 267 (CACHE_ALERT_IMPLEMENTATION.md:275) vs 104 (DOCUMENTATION.md:48, AGENTS.ai.md:54); actual = 22 `*.test.ts` files, none sourced. — Replace hard-coded counts with the real number or remove.
- [HIGH] Claude 4.x wrongly attributed to the Anthropic-direct provider; `src/anthropic-client.ts:43-61` prices only Claude 3.5/3-opus IDs (4.x is VertexAI-only) — README.md:67; SPEC.md:42; docs/DIAGRAMS.md:368-370. — Scope 4.x to the VertexAI-Claude provider.

### review/README.md

- [HIGH] wayfinder/review/README.md:636-639 — Anthropic Haiku priced "$0.25/$1.25"; code (`src/anthropic-client.ts:48-51`) is `$0.80/$4.00`; contradicts ARCHITECTURE.md:234-237. — Correct to code values.
- [MEDIUM] wayfinder/review/README.md:48,1057 — "Production Readiness 8.5/10" — unsourced metric. — Remove or cite.
- [LOW] wayfinder/review/README.md:3-6,296,1051-1052 — Badges/clone URL `github.com/wayfinder/multi-persona-review` vs DOCUMENTATION.md:67 `github.com/vbonnet/engram`. — Pick one canonical repo URL.
- [LOW] wayfinder/review/README.md:1034 — Links `[LICENSE](LICENSE)`; no `LICENSE` in `wayfinder/review/`. — Add file or remove link.

### review/ARCHITECTURE.md

- [HIGH] wayfinder/review/ARCHITECTURE.md:273 — Opus priced `claude-opus-4-6@20260205 — $5/$25`; SPEC.md:283 `$5/$25` but names it "Opus 4.5"; CACHE_METRICS_IMPLEMENTATION.md:228-232 `$15/$75`. Opus pricing & model id internally contradictory. — Reconcile repo-wide.
- [MEDIUM] wayfinder/review/ARCHITECTURE.md:300 — Claims `createVertexAIClaudeReviewer` returns `PersonaReviewer`; code/ADR use `ReviewerFunction`. — Use `ReviewerFunction`.
- [LOW] wayfinder/review/ARCHITECTURE.md:312-316 — Example uses `GoogleAuth({ keyFilename: config.credentialsPath })` but the documented config has no `credentialsPath`. — Align example with config interface.

### review/SPEC.md

- [HIGH] wayfinder/review/SPEC.md:42 — "Anthropic Claude (Direct API): … claude-sonnet-4.5, claude-haiku-4.5, claude-opus-4.5" — direct client supports only Claude 3.5/3-opus. — Scope 4.x to VertexAI.
- [MEDIUM] wayfinder/review/SPEC.md:91,234,242-244 — Persona/default-config block doesn't match `src/config-loader.ts:13-21` `DEFAULT_CONFIG`. — Cite the actual `DEFAULT_CONFIG` paths.
- [MEDIUM] wayfinder/review/SPEC.md:283 — "Claude Opus 4.5 $5/$25" vs ARCHITECTURE.md:273 model id `claude-opus-4-6@20260205`. — Use one model id/version.
- [LOW] wayfinder/review/SPEC.md:54-58,332-333 — Lists AWS/Datadog/Webhook cost sinks as available; `src/cost-sink.ts:388-403` throws "not implemented". — Mark as not-yet-implemented.

### review/ADR.md

- [MEDIUM] wayfinder/review/ADR.md:71-105 — Root ADR-003 = "Abstracted LLM Client" but README.md:62 / ARCHITECTURE.md:1021 use "ADR-003" to mean `docs/adr/003-adversarial-deliberation.md` — two clashing ADR-003 series. — Disambiguate or renumber.
- [LOW] wayfinder/review/ADR.md:386-389 — Claims Zod/JSON-schema validation; code uses hand-written validators (no Zod dependency). — Drop or qualify.

### review/DEPLOYMENT.md

- [MEDIUM] wayfinder/review/DEPLOYMENT.md:97-101,144-150,206-210 — CLI examples use a nonexistent `review` subcommand and wrong flags (`--persona`/`--output-format`/`--file-scan-mode`/`--cost-tracking`); real CLI uses `--personas`/`--format`/`--scan`/`--cost-sink`. — Rewrite to the real CLI grammar.
- [MEDIUM] wayfinder/review/DEPLOYMENT.md:346,785 — `.engram/personas/ci-reviewer.yaml` → `.wayfinder/personas`. — Fix.

### review/DOCUMENTATION.md

- [HIGH] wayfinder/review/DOCUMENTATION.md:1093-1100 — Cache-optimized persona roster (`tech-lead, security-engineer, qa-engineer, product-manager, devops-engineer`) unbacked; only `contrarian.ai.md`/`lead-reviewer.ai.md` ship in `personas/`. — Reconcile the roster to shipped files.
- [MEDIUM] wayfinder/review/DOCUMENTATION.md:4,2100 — "Version 0.1.0-alpha" vs `package.json`/README/CHANGELOG `0.1.0`. — Use `0.1.0`.
- [MEDIUM] wayfinder/review/DOCUMENTATION.md:884,879-886 — Built-in persona names contradict README.md:683-692. — Standardize names.
- [LOW] wayfinder/review/DOCUMENTATION.md:2057 — "Auto-fix planned for Session 12" — stale milestone (ADR-012 says "future enhancement", no Session 12). — Drop the specificity.

### review/docs/API.md

- [HIGH] wayfinder/review/docs/API.md:309-334 — `createCostSink(type, config)` documented two-arg; code (`src/cost-sink.ts:375`) is single-arg `createCostSink(config)`; also wrong in DOCUMENTATION.md:829, ADVANCED-USAGE.md:528. — Document the single-arg signature.
- [MEDIUM] wayfinder/review/docs/API.md:118-135 — `Finding` interface (`category`, `message`, `persona`) contradicts SPEC.md:115-136 / `src/types.ts` (`categories?: string[]`, `description`, `personas: string[]`, `id`, `title`). — Align with the real type.
- [MEDIUM] wayfinder/review/docs/API.md:141-149 — `ReviewResult` shape contradicts SPEC.md:139-151. — Use the SPEC/types shape.
- [LOW] wayfinder/review/docs/API.md:544 — Unverifiable TypeDoc site link `wayfinder.github.io/multi-persona-review`. — Confirm or remove.

### review/ — other docs

- [MEDIUM] wayfinder/review/CONTRIBUTING.md:7 — Dangling `[Code of Conduct](CODE_OF_CONDUCT.md)` (file absent). — Add file or remove link.
- [MEDIUM] wayfinder/review/CHANGELOG.md:15,25 — Stale: "[0.1.0] 2025-11-25, 279 tests"; post-0.1.0 features (cache alerts 2026-02-23, deliberation/ADR-003 2026-03-26) not recorded under [Unreleased]. — Add an [Unreleased] section; reconcile test count.
- [MEDIUM] wayfinder/review/AGENTS.ai.md:54,275-283 — "104 passing tests" and "96.5% cache savings" contradict other docs / `docs/persona-cache-performance-analysis.md` (86.1%). — Align figures.
- [MEDIUM] wayfinder/review/AGENTS.ai.md:597-599 — `--scan` default documented `full`; DOCUMENTATION.md:591/spec say `changed`. — Verify against `src/cli.ts` and use one value.
- [MEDIUM] wayfinder/review/BACKFILL-COMPLETION.md:15,55,142,501-518 — Asserts "all cross-references validated / 10/10 / 104 tests passing" while package scope, dir, test count, and many cross-refs are stale (per findings above). — Mark as a historical artifact with a staleness note, or remove from the doc set.
- [MEDIUM] wayfinder/review/AUTO_TTL_IMPLEMENTATION.md:13,103-106,158-166 — `selectCacheTTL` return contract (`'5min'|'1h'`) contradicts README.md:764-766 and ARCHITECTURE.md:482-489 (`'ephemeral'|'none'`); doc itself admits 1h is non-functional. — Make the three docs agree on the real return type / fixed 5-min TTL.
- [HIGH] wayfinder/review/CACHE_METRICS_IMPLEMENTATION.md:228-232 — Opus base price `$15/$75` contradicts ARCHITECTURE.md:273 / SPEC.md:283 (`$5/$25`). — Reconcile Opus pricing.
- [MEDIUM] wayfinder/review/docs/ADVANCED-USAGE.md:266-285 — Haiku "$0.25/$1.25" contradicts code `$0.80/$4.00`; Gemini Flash price differs from README.md:578. — Align to code; pick one Gemini price.
- [MEDIUM] wayfinder/review/docs/GETTING-STARTED.md:172,395 — `init` "Creates `.engram/config.yml`" (real: `.wayfinder/config.yml`); broken relative link `../docs/persona-optimization-guide.md` (sibling, drop the `../`). — Fix path + link.
- [MEDIUM] wayfinder/review/docs/DIAGRAMS.md:202,386 — "300 tests" unverified, conflicts with other docs. — Drop the absolute number.
- [LOW] wayfinder/review/docs/TROUBLESHOOTING.md:572-584 — Error-code family `API_3xxx` contradicts SPEC.md:295/ADR-011 ranges (`ANTHROPIC_5xxx`/`VERTEXAI_6xxx`; 3xxx = FILE_SCANNER). — Align with ADR-011.
- [LOW] wayfinder/review/docs/TROUBLESHOOTING.md:133,636 — "ANTHROPIC_API_KEY should be exactly 108 characters" — fragile/unsupported. — Remove the exact-length claim.
- [LOW] wayfinder/review/AGENTS.why.md:53,59 — Dangling "Related Patterns" refs (`github-pr-multi-persona-review.ai.md`, `github-connector/AGENTS.ai.md`, `plugins/multi-persona-review/AGENTS.ai.md`) — none exist in-tree. — Repoint or mark external.
- [LOW] wayfinder/review/{docs/persona-cache-performance-analysis.md:46-52, docs/persona-optimization-guide.md, docs/TASK-4.3-SUMMARY.md:38-44} — Persona rosters cite names not in `personas/`. — Mark illustrative or reconcile.
- CLEAN: `wayfinder/review/SECURITY.md`, `wayfinder/review/docs/cache-metrics-dashboard.md`, `wayfinder/review/personas/contrarian.ai.md`, `wayfinder/review/personas/lead-reviewer.ai.md`.

---

## Severity tally

| Severity | Count (approx.) | Theme |
|----------|-----------------|-------|
| HIGH     | ~30 | TypeScript misclaim (user's flagged bug), wrong import/module path, fabricated test transcripts, npm-scope/persona-roster/pricing contradictions, validator-signature mismatch |
| MEDIUM   | ~55 | Stale `core/cortex/`·`cortex/`·`main/agm/`·`plugins/` path prefixes, dangling ADR/spec links, V1↔V2 + command-syntax + config-path drift |
| LOW      | ~20 | Stale line/test counts, unverifiable IDs, cosmetic naming |

## Recommended remediation order

1. **Fix the user-flagged TypeScript misclaim** across `wayfinder/ARCHITECTURE.md`, `SPEC.md`, `SKILL.md`, `README.md`, and the two Go package doc-comments (`internal/w0/detector.go`, `internal/phaseisolation/types.go`). This is the highest-impact correctness fix.
2. Repo-wide find/replace of the dead path prefixes (`core/cortex/`, `cortex/`, `engram/core/`, `main/agm/`→`agm/`, `plugins/wayfinder/`, `engram/plugins/`) and the corrupted "the git history" placeholder.
3. Resolve the ADR situation: either create `docs/wayfinder/ADR-00x-*.md` or rewrite the references; fix the ADR-002 numbering collision.
4. Settle phase terminology (V1 vs V2, "9-phase" vs "12-phase", artifact filenames, command syntax) in one place and propagate.
5. Add a "Relationship to VROOM / DEAR" section to `wayfinder/ARCHITECTURE.md` to close the documentation gap (Wayfinder = research & planning; VROOM = execution; DEAR = Define→Execute→Audit→Retro).
6. Reconcile `wayfinder/review/` npm scope, config paths, model pricing, test counts, and persona roster against the source.

*Audit produced by parallel read-only sub-audits on 2026-05-17. No source files were modified.*
