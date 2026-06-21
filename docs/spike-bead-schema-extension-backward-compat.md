# Spike: Bead Schema Extension — Backward Compatibility

**Status:** Investigation complete
**Date:** 2026-06-20
**Bead:** ce-k578
**Type:** Spike (timeboxed; output feeds W0 of the implementation bead)
**Source:** Dispatch advisory 2026-06-21

---

## Context

A proposal wants to add `strategic_theme` and `strategic_weight` to beads so the
mesh can rank and group work by strategic intent. Before writing W0 requirements
we need to know whether `bd` (Homebrew beads, **v1.0.5**) can carry these fields
without a migration of the 900+ existing beads, and which consumers break. This
spike investigates only; it implements nothing.

**Headline finding:** `bd` already ships a first-class **`metadata`** facility (a
free-form JSON object per bead). It needs *no* schema change, *no* migration, and
it is already queryable. The two fields should land as metadata keys, not as new
columns or a new enum type.

---

## 1. Does `bd` support optional fields cleanly? Migration path for 900+ beads?

**Yes — natively, via `metadata`.** Verified against a throwaway db:

```
bd update <id> --set-metadata strategic_theme=3 --set-metadata strategic_weight=8
bd show <id> --json   →   "metadata": { "strategic_theme": 3, "strategic_weight": 8 }
```

Observations from the round-trip:

- Numeric values are stored **typed** (came back as JSON integers, not strings).
- `metadata` is **omitted entirely** on beads that never set it — it is not
  emitted as `null` or `{}`. So the 900+ existing beads need **zero migration**:
  they simply have no `metadata` key, which deserializes to a nil/empty map.
- Set via `--set-metadata key=value` (repeatable) or `--metadata '<json>'` /
  `--metadata @file.json` on both `create` and `update`.

**Migration path = no migration.** Treat both fields as optional with sensible
defaults applied at read time by consumers (see §4). Backfill, if ever wanted, is
a non-blocking `bd update --set-metadata` sweep, not a schema operation. No `bd`
version bump is required — v1.0.5 already supports everything needed.

| Approach | Existing beads | Migration | Type safety | bd version bump |
|---|---|---|---|---|
| **`metadata` keys (recommended)** | untouched, key absent | none | typed JSON values | no |
| New first-class columns | need backfill / nullable | schema change + backfill 900+ | strong | yes (bd fork) |
| Encode in labels (`theme:growth`) | untouched | none | strings only | no |

---

## 2. Consumer impact — what parses the schema, what breaks?

`bd` is an external binary; dear-agent consumers shell out and parse its output.
**There is no single schema definition in dear-agent** — each consumer selectively
picks fields from `bd … --json`, which is exactly why additive metadata keys are
safe. Two parsing styles exist, both backward-compatible:

- **Lenient JSON** — `engram/hooks-bin/internal/beads/client.go` unmarshals into
  `BeadSummary{id,title,labels}`; Go's `json.Unmarshal` ignores unknown fields, so
  extra `metadata` is silently dropped. **No change** unless it wants to *read* the
  new fields.
- **Text scraping** — that client's `GetBeadTitle` greps `bd show` for `Title:`;
  `agm/internal/a2a/beads/validator.go` only inspects the *description* text.
  Neither looks at metadata. **No change.**

Other named consumers — **orch / meta-o / grooming scripts** (external to this
repo) and **wayfinder** (only `lib/d2-gate-check.sh` touches metadata, reading its
own keys) — are protected by the same rule. They change only to *produce* or
*rank by* the new fields. **`bd` itself** needs nothing: `show`/`list`/`query`
already surface `metadata`.

---

## 3. Theme representation — enum 1-7 vs. tag/string?

**Recommendation: a named string in metadata** (`strategic_theme: "growth"`),
**not** a bare integer enum and **not** a label.

- **Against bare `1-7` integers:** opaque at the call site (`bd show` prints `3` —
  meaningless to a human or agent), and renumbering/reordering themes silently
  corrupts historical beads. Type safety is illusory: metadata is untyped JSON, so
  nothing enforces the `1..7` range anyway — validation must live in the producer
  regardless.
- **Against labels (`theme:growth`):** labels are first-class and filterable, but
  they inherit to children (`--no-inherit-labels` exists but the default leaks a
  parent's theme onto subtasks), and they share a flat namespace with operational
  labels. Reserve labels for cross-cutting flags, not a single-valued dimension.
- **For named strings in metadata:** human-readable, self-documenting, queryable
  (§ below), and extensible without renumbering. Keep a **canonical enum of valid
  names in code** (the producer validates against it) — that gives the type-safety
  benefit without baking ordinals into the data.

**Filter ergonomics (verified):** `bd query` accepts **dotted metadata paths**
even though they are undocumented in the `Supported fields` list:

```
bd query "metadata.strategic_theme=growth"   → matches
bd query "metadata.strategic_theme=99"       → "No issues found"   (real filtering, not a no-op)
```

Caveat: `bd query`/`bd list --sort` only sort by the documented fields (priority,
created, …). **Sorting/ranking by `strategic_weight` must be done client-side**
after fetching `--json` — bd will filter on metadata but not order by it.

---

## 4. How does `strategic_weight` interact with `priority` and WSJF?

- **WSJF / cost-of-delay does not exist in this codebase.** A repo-wide grep for
  `wsjf|WSJF|cost_of_delay|CostOfDelay` returns nothing. The only ranking field
  `bd` actually has is **`priority` (P0–P4)**. WSJF is conceptual (from the
  advisory), so there is no field to conflict with today.
- **`priority` stays authoritative for scheduling.** It is the field `bd ready`,
  `bd list --sort priority`, and the mesh's dispatch logic already key on.
  `strategic_weight` is **advisory**, a *tie-breaker within a priority band* and a
  grouping signal for reporting — it must **not** override P-level dispatch.
- **Conflict rule (proposed for W0):** sort by `priority` first, then by
  `strategic_weight` descending within equal priority. A high weight never
  promotes a P3 above a P1. If a future WSJF score is introduced, define it as a
  *derived* value (e.g. `weight / estimate`) computed client-side, not a third
  competing stored field.

---

## 5. W0 requirements — what the implementation bead must deliver

1. **No schema migration, no bd version bump.** Fields are `metadata` keys;
   existing 900+ beads remain valid untouched. State this explicitly so a
   reviewer does not expect a migration script.
2. **Canonical theme enum in code** — the validated set of theme names + a
   producer-side validator that rejects unknown themes and out-of-band weights.
3. **Read-side defaults** — consumers treat absent `strategic_theme`/
   `strategic_weight` as "unset" / weight `0`; never assume the key exists.
4. **Client-side ranking helper** — a shared function that sorts a `--json` bead
   list by `(priority, -strategic_weight)`, since `bd --sort` cannot.
5. **Consumer updates are opt-in** — only producers and rank-by-weight readers
   change; lenient/text-scraping consumers are explicitly out of scope.
6. **Tests:** metadata round-trip, absent-field default, query-by-theme,
   client-side weight ordering, and "high weight does not jump priority band."
7. **Docs:** the canonical theme list and the priority-vs-weight conflict rule.

---

## Risks

- **Untyped metadata** — `bd` stores `strategic_weight="lots"` happily. *Mitigation:*
  validate in the producer; it is the only enforcement point.
- **No native sort by weight** — easy to assume `bd list` orders by it. *Mitigation:*
  W0 ships the client-side ranking helper and documents the limitation.
- **Theme drift** — ad-hoc theme strings without a canonical list. *Mitigation:*
  enum-in-code + producer validation (W0 item 2).
- **bd upgrade** — metadata query/format behavior verified on v1.0.5; re-verify on
  upgrade.

## Recommendation

Adopt **named-string `strategic_theme` + numeric `strategic_weight` as `bd`
metadata keys**, validated in producers against a canonical enum, with `priority`
remaining authoritative and weight acting as an in-band tie-breaker. This needs no
migration, no bd version bump, and no changes to existing lenient consumers —
making it the lowest-risk path to W0.
