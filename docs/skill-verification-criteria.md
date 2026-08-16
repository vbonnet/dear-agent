<!-- Last audited at: 2026-08-11 -->

# Per-Skill Verification Criteria

A living convention for declaring, auditing, and enforcing "done" evidence at
the skill level — integrating with the **Audit** step of DEAR (Define →
Execute → **Audit** → Retro).

---

## Motivation

DEAR's Audit step asks: *"Verify the runnable exit conditions actually hold."*
A global definition-of-done can describe broad quality bars (tests pass, lint
clean), but it cannot answer *what specific evidence must exist* after a given
skill runs. A skill that writes a design doc has different exit conditions than
one that deploys a service.

Per-skill verification criteria fix that gap: each skill declares its own
falsifiable checklist. When the Secondary executes DEAR's Audit step, it
checks the skill's criteria list as a pass/fail gate — no judgment
required. (Per `CONTEXT.md`, verification is the Secondary's responsibility;
"Auditor" names the separate standing role that mines logs and retros.)

---

## Convention

Every skill **should** declare verification criteria. Two equivalent forms are
accepted; prefer the one that fits the skill's file layout:

### Form 1 — YAML frontmatter field (preferred for flat `.md` commands)

Add a `verification_criteria:` list to the YAML frontmatter block:

```yaml
---
name: my-skill
description: ...
model: sonnet
effort: low
verification_criteria:
  - "Output file exists at <declared path>"
  - "`make preflight` exits 0"
  - "No placeholder markers remain in output (`grep -c '{{' file` == 0)"
---
```

This form is machine-readable and can be extracted programmatically by audit
tooling or `cmd/bead-close-guard`.

### Form 2 — Markdown section (preferred for `SKILL.md` files with prose)

Add a `## Verification Criteria` section, typically at the end of the file
before any appendices:

```markdown
## Verification Criteria

The Secondary checks the following in DEAR's Audit step after this skill runs:

- [ ] Condition one (artifact: file exists at path X)
- [ ] Condition two (exit code: `cmd` exits 0)
- [ ] Condition three (observable: coverage ≥ 80%)
```

Both forms are equivalent. A skill may include both — the frontmatter is
machine-readable; the markdown section is human-readable during code review.
When both are present they must agree.

---

## Schema — what makes a valid criterion

Each item is a **concrete, falsifiable statement** about world state after the
skill completes. Three classes:

| Class | What it asserts | Example |
|-------|----------------|---------|
| **Artifact** | A file or directory exists (or doesn't). | `docs/retro.md exists and is non-empty` |
| **Exit code** | A command exits with a specific code. | `` `make preflight` exits 0 `` |
| **Observable** | A measurable property holds. | `coverage ≥ 80%`, `grep finds /PASS/ in output` |

**Prohibited patterns:**

- Vague: *"output looks correct"*, *"is complete"* — untestable, defeats the Audit.
- Implementation-coupled: *"function X is called with arg Y"* — fragile, tests
  shape not behavior.
- Temporal: *"done by Thursday"* — belongs in the bead, not the skill criteria.

---

## Integration with Process DEAR

```
Process DEAR phase   ←→  Skill contract
─────────────────────────────────────────────────────────
Define              ←   `description:` + `verification_criteria:` read upfront;
                        the criteria are the skill's exit conditions
Execute             ←   skill body runs (the normal workflow steps)
Audit               ←   each criterion item checked: pass → continue; fail → block Done
Retro               ←   unmet criteria become findings → fed back to Wayfinder/roadmap
```

If a skill has no criteria declared, the Secondary falls back to the global DoD.
Declaring criteria is an opt-in upgrade: add them incrementally as skills mature.
A skill with criteria is **strictly more auditable** than one without.

---

## Worked example — `/wayfinder` skill

The Wayfinder planning phase produces a structured session artifact. Its
criteria in frontmatter form:

```yaml
verification_criteria:
  - "WAYFINDER-STATUS.md exists in the designated wayfinder session directory"
  - "status in WAYFINDER-STATUS.md is a canonical value such as 'in-progress' or 'completed'"
  - "At least one canonical phase artifact (for example, CHARTER-charter.md) exists in the session dir"
  - "No '{{TODO}}' or '[[fill me in]]' placeholders remain in any phase artifact"
  - "Session directory is non-empty (at least one .md file present)"
```

Each item is checkable with a shell one-liner: `test -f`, `grep -q`, `ls | wc -l`.
The Secondary can run them in sequence with no judgment required.

---

## Precedence and override

Criteria declared in a skill file are **skill-scoped** — they apply whenever
that skill is invoked. They do not replace the global DoD; they extend it. An
invocation must satisfy *both* the global DoD and its skill's criteria to be
considered done.

If a criterion cannot be checked in a given invocation context (e.g. a CI
environment lacks a running service), it may be marked **deferred** with a
note. Deferred criteria are tracked as findings in the Retro step, not silently
skipped.

---

## Linting and enforcement

Skills that declare `verification_criteria:` in frontmatter can be statically
checked by `pkg/skilllint` (see `docs/skill-tiers.md` for the linting
framework). Enforcement tiers today:

| State | Behavior |
|-------|----------|
| Criteria absent | Accepted; the Markdown verification section remains required for portable skills |
| Criteria present, not a nonempty string list | Hard lint failure |
| Criteria present, structurally valid | No finding; reviewers assess whether each statement is falsifiable |

Future: `cmd/bead-close-guard` may gate bead closure on each declared
criterion having been checked in the Audit step. This doc will be updated when
that gate lands.

---

## See also

- `docs/skill-tiers.md` — `model:` / `effort:` frontmatter requirements (CI-enforced today)
- `docs/skill-placement.md` — which repo owns a skill, and how it reaches a session
- `pkg/skilllint/` — skill frontmatter linter
- `CONTEXT.md §DEAR` — canonical Process DEAR definition
- `docs/adr/ADR-035-dear-terminology-disambiguation.md` — Process DEAR vs workflow lifecycle hooks
- `.claude/skills/example-with-criteria.md` — worked example skill file
