# AGENTS.why.md — Decision Log

Co-located rationale for `AGENTS.md` and other root-level config in this repo.
Append new decisions at the bottom.

## Why this file?

The shared AGENTS.md philosophy establishes that config files have co-located
`.why.md` decision logs. This file explains dear-agent-specific choices so
future agents (and future-me) understand the reasoning, not just the rules.

## Why three tiers for output routing?

The `.dear-agent.yml` config and the CLAUDE.md "Output Routing" section
together implement two of the three tiers used elsewhere in the project:

| Tier              | Mechanism                          | Role                                   |
|-------------------|------------------------------------|----------------------------------------|
| **Instruction**   | `.claude/CLAUDE.md` rule           | Tells agents what the rule is + why    |
| **Configuration** | `.dear-agent.yml`                  | Authoritative answer to "where does X go?" |
| **Enforcement**   | (not implemented; see below)       | Blocks the action at runtime           |

The two-tier (instruction + config) implementation was a deliberate stopping
point. Research artifacts leaked into the canonical code repo twice in the
predecessor (ai-tools); the first time the only signal was a CLAUDE.md
sentence buried in a long file, which agents clearly weren't reading. Adding
a *deterministic config lookup* gives an agent a single short file to read
and the answer it needs, with no judgment required. That handles the failure
mode actually observed.

A third "enforcement" tier (a pretool hook that blocks `Write` / `Edit` to
forbidden paths) was considered and deferred. Reasons:
- The forbidden-path globs (`research/*.md`, `research/*.txt`) are
  forward-looking — dear-agent does not currently have a `research/` tree —
  so a hook would mostly be checking a directory that doesn't exist.
- The existing pretool hooks (pretool-bash-blocker, pretool-npm-safety) are
  Go binaries with their own test suites. Adding a fourth hook is a
  meaningful surface area increase and not justified until the two-tier
  approach has been observed to fail.

If a leak occurs despite the config, escalate to the enforcement tier:
add an `agm/cmd/agm-hooks/pretool-output-routing` hook that reads
`.dear-agent.yml` and rejects writes to forbidden paths.

## Why dear-agent is "code", not "research"

`dear-agent` ships agent infrastructure (AGM, Engram, Wayfinder, codegen).
Research artifacts — analysis docs, transcripts, literature reviews,
findings — belong in the dedicated corpus repo, not interleaved with code.

The corpus repo is `engram-research`. Routing analysis docs and transcripts
there keeps:
- dear-agent history focused on code changes (clean blame, faster `git log`).
- Research artifacts colocated with the rest of the corpus where engram's
  ingestion / indexing tools can find them.

---

## Design Decisions Log

| Date | Decision | Context |
|------|----------|---------|
| 2026-05-02 | Created `.dear-agent.yml` + CLAUDE.md "Output Routing" section | Second incident of research artifacts committed to the canonical code repo (ai-tools, predecessor) instead of engram-research; added deterministic config lookup so agents don't have to infer routing |
| 2026-05-02 | Deferred enforcement-tier hook | Two-tier approach addresses the observed failure (agents not reading CLAUDE.md rules); a hook adds maintenance cost and is forward-looking only since dear-agent has no `research/` tree yet. Revisit if a leak occurs. |
| 2026-05-26 | Extended the `.why.md` pattern to be the **rip-out-tax tracker** for caps (ADR-024 / DEASR) | Meta-retro over recent retros surfaced that throttles, denylists, retry ceilings, and "skip in CI" toggles ship as "temporary" without removal triggers. The `.why.md` pattern already lives next to the code it explains; reusing it for caps avoids a parallel ledger that would drift. See § Why the `.why.md` pattern is also the rip-out-tax tracker below. |

## Why the `.why.md` pattern is also the rip-out-tax tracker

[ADR-024](docs/adr/ADR-024-deasr-push-bike-philosophy.md) renames the process
retrospective loop from DEAR to **DEASR** (adds a Scale-test phase) under the
**push-bike, not training wheels** design constraint. One of its forcing
functions — D5: the rip-out tax — reuses this file's pattern instead of
inventing a new ledger.

**The extended rule.** Any code that scores `caps` on any axis of a DEASR
Scale-test (throttle, cap, denylist, retry ceiling, hard-coded ID, feature
flag set to a non-default value, "skip in CI" toggle, etc.) MUST ship with a
co-located `.why.md` file. The file states:

- **Type:** cap (rip-out tax)
- **Date:** YYYY-MM-DD
- **Why this is here:** the load it relieves, the regression it prevents
- **What it costs to remove:** files / packages / call sites touched;
  migration shape
- **Removal trigger:** the observable signal that says "now safe to remove"
- **Related retro:** link to the DEASR retro that introduced it

**Why this pattern, not a parallel ledger.** Three reasons:

1. **Locality.** The `.why.md` lives next to the code it explains. A
   reviewer looking at the cap sees the rationale without leaving the
   directory. A central ledger needs cross-referencing that decays.
2. **Drift resistance.** When the code moves, the `.why.md` moves with it
   (mv-the-pair is a single mental operation). A central ledger drifts every
   time the cap is refactored.
3. **Continuity.** The pattern already exists in this repo (this file,
   `wayfinder/review/AGENTS.why.md`, the engram MCP `.why.md` filtering).
   Reusing it costs nothing; inventing a new ledger costs cognitive load on
   every author and reviewer.

**What does *not* change.** This is purely additive — the existing decision-log
use of `.why.md` (this file, output-routing rationale, etc.) is untouched. A
`.why.md` may now also document a cap; the original "decisions deserve
co-located rationale" purpose is preserved.

**Cross-references.**

- [ADR-024 § D5: Rip-out tax](docs/adr/ADR-024-deasr-push-bike-philosophy.md)
  — the binding decision.
- [/CONTEXT.md § DEASR](CONTEXT.md#deasr--diagnose--evaluate--scale-test--act--review)
  — the loop this rule is part of.
- [docs/retros/_TEMPLATE.md](docs/retros/_TEMPLATE.md) — the retro template
  that produces the Scale-test scores driving when a `.why.md` is required.
