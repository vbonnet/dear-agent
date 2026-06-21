# Design: Bead Schema Strategic Fields — Implementation

**Status:** Design spike (implementation-ready)
**Date:** 2026-06-20
**Bead:** ce-ncib
**Type:** Spike — documents *how* to implement; ships no code
**Builds on:** [`spike-bead-schema-extension-backward-compat.md`](./spike-bead-schema-extension-backward-compat.md) (ce-k578)
**Source:** Dispatch strategic priority integration spike advisory 2026-06-21

---

## Goal

Add two **optional** metadata fields to beads so the mesh can rank and group work
by strategic intent:

- **`strategic_theme`** — enum **1–7** mapping to the strategic priority stack:
  `1=OTel`, `2=Brakes/circuit-breaker`, `3=Memory-migration+grooming`,
  `4=Retro+quality`, `5=Spec-driven`, `6=OAuth`, `7=24/7-continuity`.
  Unset (null) for unaligned beads.
- **`strategic_weight`** — int **1–3** (`1=weak`, `2=moderate`, `3=core deliverable`).
  Null default.

No behavior change. Existing beads default null. Used only as a WSJF / RR-OE
**tie-breaker within a priority band**, never as a primary scheduling key.

---

## TL;DR — the implementation shape

The prior backward-compat spike (ce-k578) established the decisive constraint:
**`bd` is an external Homebrew binary (v1.0.5); dear-agent does not own its schema.**
dear-agent consumers shell out to `bd … --json` and pick fields. Therefore:

1. The two fields land as keys on `bd`'s **existing `metadata` JSON column** —
   **no `ALTER TABLE`, no migration of the 900+ existing beads, no `bd` fork.**
2. The only code that changes lives in **dear-agent**: a small enum + null-safe
   reader struct in the `beads` client package, a producer-side validator, and a
   thin `Client` helper that shells out to `bd … --set-metadata`.
3. `bd show` / `bd list --json` already surface `metadata`, so **display works with
   zero code**.

The first-class-column / `ALTER TABLE` path is documented in
[§7 Rejected alternatives](#7-rejected-alternatives) and is **not** the W0 plan.

> **Verified ground truth (this repo, 2026-06-20):**
> - `bd` Dolt `issues` table already has `metadata json NULL DEFAULT (json_object())`.
> - `bd update <id> --set-metadata strategic_theme=1 --set-metadata strategic_weight=3`
>   round-trips as typed JSON integers under `"metadata"` in `bd show --json`.
> - `metadata` is **omitted** on beads that never set it → old beads need no backfill.
> - `bd list --metadata-field strategic_theme=1` and `bd query "metadata.strategic_theme=1"`
>   filter natively; **sorting by a metadata value is not supported** (client-side only).

---

## 1. Exact Go type changes

All changes are additive and live in
`engram/hooks-bin/internal/beads/` (the only dear-agent package that models a
bead). Today it defines exactly:

```go
// types.go (current)
type BeadSummary struct {
    ID     string   `json:"id"`
    Title  string   `json:"title"`
    Labels []string `json:"labels"`
}
```

### 1a. Canonical theme enum (new file `strategic.go`)

The enum is the single source of truth for valid themes. It exists **in code**,
not in the database — `bd` metadata is untyped JSON, so validation must live in the
producer regardless (ce-k578 §3, §5).

```go
package beads

// StrategicTheme is the strategic-priority-stack ordinal carried in bead
// metadata under the key "strategic_theme". The zero value means "unset" and is
// what an absent metadata key deserializes to — callers must treat 0 as null.
type StrategicTheme int

const (
    ThemeUnset          StrategicTheme = 0 // no strategic alignment recorded
    ThemeOTel           StrategicTheme = 1
    ThemeBrakes         StrategicTheme = 2 // circuit-breaker
    ThemeMemoryGrooming StrategicTheme = 3 // memory-migration + grooming
    ThemeRetroQuality   StrategicTheme = 4
    ThemeSpecDriven     StrategicTheme = 5
    ThemeOAuth          StrategicTheme = 6
    ThemeContinuity     StrategicTheme = 7 // 24/7 continuity
)

// themeNames is the human-readable label for each ordinal, used by display and
// by the producer-side error message. Index 0 is the unset sentinel.
var themeNames = [...]string{
    ThemeUnset:          "unset",
    ThemeOTel:           "otel",
    ThemeBrakes:         "brakes",
    ThemeMemoryGrooming: "memory-grooming",
    ThemeRetroQuality:   "retro-quality",
    ThemeSpecDriven:     "spec-driven",
    ThemeOAuth:          "oauth",
    ThemeContinuity:     "continuity",
}

// Valid reports whether t is a recognized theme ordinal (1..7). ThemeUnset is
// NOT valid for a producer write — "unset" means "do not set the key at all".
func (t StrategicTheme) Valid() bool { return t >= ThemeOTel && t <= ThemeContinuity }

func (t StrategicTheme) String() string {
    if int(t) < 0 || int(t) >= len(themeNames) {
        return "invalid"
    }
    return themeNames[t]
}

// StrategicWeight is the in-band tie-breaker carried under "strategic_weight".
// Zero means unset; valid producer values are 1..3.
type StrategicWeight int

const (
    WeightUnset    StrategicWeight = 0
    WeightWeak     StrategicWeight = 1
    WeightModerate StrategicWeight = 2
    WeightCore     StrategicWeight = 3
)

func (w StrategicWeight) Valid() bool { return w >= WeightWeak && w <= WeightCore }
```

### 1b. Null-safe carrier on the parsed bead (extend `types.go`)

`bd --json` returns `metadata` as a free-form object. Model it as a typed,
**pointer-optional** projection so "absent" is distinguishable from "zero" without
threading a raw map through every consumer:

```go
// Strategic holds the optional strategic fields read from a bead's metadata.
// Both pointers are nil when the corresponding metadata key is absent, which is
// the case for every bead created before this feature and every unaligned bead.
type Strategic struct {
    Theme  *StrategicTheme  `json:"strategic_theme,omitempty"`
    Weight *StrategicWeight `json:"strategic_weight,omitempty"`
}

// BeadSummary gains an additive, optional metadata projection. Go's
// json.Unmarshal leaves Metadata nil when bd omits the "metadata" object, so
// existing lenient consumers that ignore this field are unaffected.
type BeadSummary struct {
    ID       string   `json:"id"`
    Title    string   `json:"title"`
    Labels   []string `json:"labels"`
    Metadata Metadata `json:"metadata,omitempty"`
}

// Metadata is the raw bd metadata object. Use Strategic() for typed access.
type Metadata map[string]any

// Strategic extracts the typed strategic projection, tolerating bd's JSON-number
// representation (float64 after a generic unmarshal). Missing/garbage keys yield
// nil pointers rather than errors — reads must never break on legacy data.
func (m Metadata) Strategic() Strategic {
    var s Strategic
    if t, ok := asInt(m["strategic_theme"]); ok {
        v := StrategicTheme(t)
        s.Theme = &v
    }
    if w, ok := asInt(m["strategic_weight"]); ok {
        v := StrategicWeight(w)
        s.Weight = &v
    }
    return s
}

// asInt coerces bd's JSON numbers (float64) or ints to int; returns ok=false for
// absent or non-numeric values (e.g. a hand-edited strategic_weight="lots").
func asInt(v any) (int, bool) {
    switch n := v.(type) {
    case float64:
        return int(n), true
    case int:
        return n, true
    case json.Number:
        if i, err := n.Int64(); err == nil {
            return int(i), true
        }
    }
    return 0, false
}
```

This is the entire type surface. No other dear-agent struct models a bead.

---

## 2. Storage & migration

### Recommended: no migration

The fields are keys on `bd`'s pre-existing `metadata json` column
(`metadata json YES (json_object())` in the live Dolt schema). Concretely:

- **Existing 900+ beads:** untouched. They have no `strategic_theme` /
  `strategic_weight` key; `Metadata.Strategic()` returns nil pointers.
- **Migration script:** none. Backfill, if ever desired, is a non-blocking
  `bd update --set-metadata` sweep — an ordinary data write, not a DDL operation.
- **`bd` version:** no bump. v1.0.5 already stores, displays, and filters metadata.

State this explicitly in any implementation PR so a reviewer does not expect a
migration file.

### The `ALTER TABLE` that we deliberately do *not* ship

For completeness — and because the bead brief asked for it — the first-class-column
form would be:

```sql
-- REJECTED for W0 (see §7). Would have to be applied to bd's embedded Dolt
-- `issues` table, which dear-agent does not own and bd's own migrations manage.
ALTER TABLE issues
  ADD COLUMN strategic_theme  INT NULL,
  ADD COLUMN strategic_weight INT NULL;
```

Why this is not the plan: dear-agent cannot ship a migration against a database
whose schema is owned and migrated by an external Homebrew binary. Such columns
would be invisible to `bd`'s ORM, unwritable through `bd` commands, and at risk of
being dropped or colliding on the next `bd` upgrade. See §7.

---

## 3. `bd` CLI surface

**No new `bd` flags are needed** — `bd` v1.0.5 already exposes everything via the
generic metadata facility. dear-agent wraps the relevant invocations.

### Write (producer)

```bash
# On create or update — repeatable --set-metadata, typed integer values:
bd create "..." --set-metadata strategic_theme=1 --set-metadata strategic_weight=3
bd update <id>  --set-metadata strategic_theme=1 --set-metadata strategic_weight=3
bd update <id>  --unset-metadata strategic_theme   # clear (mark unaligned)
```

### Read / display

```bash
bd show <id>            # human view already prints the metadata object
bd show <id> --json     # → "metadata": { "strategic_theme": 1, "strategic_weight": 3 }
bd list --json          # metadata object included per bead (omitted when unset)
```

### Filter (post-W0; native, no code)

```bash
bd list  --metadata-field strategic_theme=1      # exact-match filter
bd list  --has-metadata-key strategic_theme      # any aligned bead
bd query "metadata.strategic_theme=1"            # dotted path; real filtering
```

> **Limitation (verified):** `bd` filters on metadata but **cannot sort/rank by a
> metadata value**. Ordering by `strategic_weight` is a **client-side** operation
> after fetching `--json`. This is why ranking integration (§5) is a Go helper, not
> a `bd --sort` flag.

### dear-agent `Client` additions (`beads/client.go`)

Mirror the existing thin-wrapper style (each method shells out to `bd`):

```go
// SetStrategic writes validated strategic fields onto an existing bead. It is the
// single producer-side enforcement point — bd will happily store garbage, so all
// range checking happens here (§4).
func (c *Client) SetStrategic(beadID string, theme StrategicTheme, weight StrategicWeight) error {
    if !c.IsAvailable() {
        return fmt.Errorf("bd CLI not available")
    }
    if !theme.Valid() {
        return fmt.Errorf("strategic_theme %d out of range (valid: 1-7)", theme)
    }
    if !weight.Valid() {
        return fmt.Errorf("strategic_weight %d out of range (valid: 1-3)", weight)
    }
    cmd := exec.CommandContext(context.Background(), c.bdPath, "update", beadID,
        "--set-metadata", fmt.Sprintf("strategic_theme=%d", int(theme)),
        "--set-metadata", fmt.Sprintf("strategic_weight=%d", int(weight)),
    ) //nolint:gosec // bdPath from exec.LookPath, args controlled
    if _, err := cmd.Output(); err != nil {
        return fmt.Errorf("bd update --set-metadata failed: %w", err)
    }
    return nil
}

// ReadStrategic fetches a bead and returns its typed strategic projection.
// Absent keys yield nil pointers — never an error — so it is safe on legacy beads.
func (c *Client) ReadStrategic(beadID string) (Strategic, error) {
    if !c.IsAvailable() {
        return Strategic{}, fmt.Errorf("bd CLI not available")
    }
    cmd := exec.CommandContext(context.Background(), c.bdPath, "show", beadID, "--json") //nolint:gosec
    out, err := cmd.Output()
    if err != nil {
        return Strategic{}, fmt.Errorf("bd show --json failed: %w", err)
    }
    // bd show --json returns an array of one object.
    var beads []BeadSummary
    if err := json.Unmarshal(out, &beads); err != nil {
        return Strategic{}, fmt.Errorf("JSON parse failed: %w", err)
    }
    if len(beads) == 0 {
        return Strategic{}, nil
    }
    return beads[0].Metadata.Strategic(), nil
}
```

---

## 4. Null-safe reads & validation

### Null-safe reads

- **The metadata key is absent on every legacy and unaligned bead.** `bd` omits
  `metadata` entirely when nothing set it — it is not `null` or `{}`. After
  `json.Unmarshal`, `BeadSummary.Metadata` is `nil`; `nil.Strategic()` returns
  zero pointers safely (Go method on nil map is fine for reads).
- **Consumers must treat nil as "unset"**, not as theme 0 / weight 0 with meaning.
  The pointer-optional `Strategic` makes the distinction unavoidable at the call
  site:

```go
s := bead.Metadata.Strategic()
if s.Weight != nil {
    // participate in weight tie-break
} else {
    // legacy / unaligned: default weight, no tie-break influence
}
```

- **Lenient consumers stay untouched.** `client.go`'s `GetBeadByUUID`,
  `ListBeadsBySession`, etc. unmarshal into `BeadSummary` and ignore `Metadata`;
  Go drops unknown JSON fields, so adding the field changes nothing for them. The
  text-scraping paths (`GetBeadTitle`, `a2a/beads/validator.go`) never look at
  metadata.

### Validation

`bd` metadata is **untyped JSON** — it will store `strategic_weight="lots"`
without complaint. The **producer is the only enforcement point**:

- `StrategicTheme.Valid()` → `1..7`; `StrategicWeight.Valid()` → `1..3`.
- `Client.SetStrategic` rejects out-of-range values with a clear error
  (`strategic_theme 9 out of range (valid: 1-7)`) **before** shelling out.
- On read, `Metadata.Strategic()` is **defensive, not strict**: a hand-edited
  out-of-range or non-numeric value yields a nil pointer (treated as unset) rather
  than a hard failure, so one bad bead cannot crash a ranking pass. A separate
  `bd`-lint pass (post-W0) can surface beads whose stored value fails `Valid()`.

---

## 5. Ranking integration (post-W0, design only)

`pkg/backlog/rank.go` blends `priority`, unblocking `leverage`, and `effort` via
`RankWeights`. `strategic_weight` slots in as an **in-band tie-breaker** that
**never crosses a priority band** (ce-k578 §4):

- Primary order stays `priority` (what `bd ready` / dispatch already key on).
- Within equal blended score, break ties by `strategic_weight` **descending**, then
  the existing phase/ID tiebreak. A `weight=3` P3 never jumps a `weight=1` P1.
- `strategic_theme` is a **grouping/reporting** dimension, not a rank input.

This is intentionally **out of W0 scope** and listed here so the data model lands
in a shape the ranker can later consume without rework (typed `Strategic`, nil =
no tie-break influence).

---

## 6. W0 scope — minimum viable

Deliver the smallest slice that makes the fields real and safe, with display
working end-to-end:

| In W0 | Out of W0 (later beads) |
|---|---|
| `strategic.go`: theme/weight enums + `Valid()` | `bd list --metadata-field` filtering wired into any dear-agent flow |
| `types.go`: `Strategic`, `Metadata`, `Metadata.Strategic()` null-safe reader | `pkg/backlog/rank.go` weight tie-break (§5) |
| `Client.SetStrategic` (validated producer) + `Client.ReadStrategic` | Backfill sweep of existing beads |
| Confirm `bd show` / `bd show --json` display metadata (native — no code) | `bd`-lint pass for out-of-range stored values |
| Unit tests (§ below) | Theme-based reporting / dashboards |

**Explicitly NOT in W0:** any `ALTER TABLE` / migration, any `bd` version bump,
any change to lenient or text-scraping consumers, any sort/filter-by-strategic
behavior in the mesh.

### W0 tests

1. **Round-trip:** `SetStrategic` then `ReadStrategic` returns the same theme+weight.
2. **Absent-field default:** a bead with no metadata → `Strategic{}` (nil pointers).
3. **Validation:** `SetStrategic` rejects theme `0` and `8`, weight `0` and `4`.
4. **Defensive read:** `strategic_weight="lots"` and `strategic_theme=99` →
   nil/unset, no error.
5. **Lenient compat:** unmarshalling a metadata-bearing bead into the old
   `BeadSummary` consumers still yields correct `ID`/`Title`/`Labels`.

---

## 7. Rejected alternatives

### A. First-class columns + `ALTER TABLE` (rejected)

```sql
ALTER TABLE issues
  ADD COLUMN strategic_theme INT NULL,
  ADD COLUMN strategic_weight INT NULL;
```

Rejected because dear-agent **does not own** the `bd` Dolt schema — `bd` is an
external Homebrew binary that manages its own migrations. New columns would be
unwritable through `bd` commands, invisible to its JSON output, and liable to be
dropped or to collide on the next `bd` upgrade. It would also force a backfill of
900+ existing beads and effectively a `bd` fork. The `metadata` column gives the
same data with none of this cost.

### B. Encode theme in labels (`theme:otel`) (rejected)

Labels are first-class and filterable, but they **inherit to children** by default
(a parent's theme leaks onto subtasks) and share a flat namespace with operational
labels. Reserve labels for cross-cutting flags, not a single-valued dimension.

### C. Named-string theme vs. integer enum (noted)

ce-k578 §3 argued for a **named string** (`strategic_theme="otel"`) over a bare
integer, since `bd show` printing `3` is opaque and renumbering corrupts history.
This design follows the bead brief's explicit `1–7` enum for the **stored** value
(stable ordinal, compact tie-break key) but **keeps the canonical name↔ordinal map
in code** (`themeNames`, §1a) so display and logs are human-readable and the enum
is the validation source of truth. If reviewers prefer the stored value to be the
name, only the `--set-metadata` value and `asInt`/`String` plumbing change — the
rest of the design is identical.

---

## Risks

- **Untyped metadata** — `bd` stores out-of-range/garbage values happily.
  *Mitigation:* validate in the producer (`SetStrategic`); read defensively.
- **No native sort by weight** — easy to assume `bd list` orders by it.
  *Mitigation:* ranking is a client-side Go helper (§5); documented limitation.
- **Theme renumber drift** — reordering the 1–7 stack silently reinterprets stored
  ordinals. *Mitigation:* treat the enum as append-only; never reuse an ordinal.
- **`bd` upgrade** — metadata format/query verified on v1.0.5; re-verify on upgrade.

---

## Recommendation

Implement `strategic_theme` (1–7) and `strategic_weight` (1–3) as **keys on `bd`'s
existing `metadata` JSON column**, with a canonical enum + producer-side validation
in dear-agent's `beads` package and a null-safe typed reader. This needs **no
migration, no `bd` version bump, and no changes to existing consumers** — the
lowest-risk path to W0. `priority` stays authoritative for scheduling;
`strategic_weight` is an advisory in-band tie-breaker layered in post-W0.
