# DEAR Retro: `agm migrate v2-to-v3` Has Almost No Persistent Delta Left

**Date:** 2026-06-03
**Bead:** ce-6as.71 (P0) — "AGM migration tooling (agm migrate v2-to-v3, dry-run)"
**Author:** scoped worker (agm-migrate-v2v3)

## Define

ce-6as.71 asked for the missing Go cobra `agm migrate` command family,
specifically `agm migrate v2-to-v3` with a dry-run mode. The bead had been
**blocked** since 2026-01-16 on a stated prerequisite (ce-6as.103): "the v3
manifest schema is undefined — cannot migrate to an undefined target without
inventing a product decision."

The scoped task was to implement the command.

## Execute (what was actually built)

- A pure, tested converter `manifest.MigrateV2ToV3(*Manifest) (*ManifestV3,
  []FieldChange)` plus `ManifestV3.Downgrade()` (the missing converters — the
  repo already had `ManifestV1`, `Manifest` (v2), and `ManifestV3` structs and
  validators, but nothing that converted between v2 and v3).
- `agm migrate v2-to-v3 [--dry-run]` in `cmd/agm/migrate.go`: loads all
  sessions from the canonical store, computes the upgrade, validates against the
  v3 schema, and backfills the v3-required `harness` field for legacy sessions.

## Audit (the finding that reshaped the work)

Investigating the current architecture showed the original blocker has **not
been cleared — it has relocated**, and the headline migration's persistent
delta is now nearly empty:

1. **`ManifestV3` exists but is orphaned.** `internal/manifest/v3.go` defines a
   concrete v3 schema (v2 + `harness`, `model`, `harness_history`). It is not
   wired into the Dolt store; nothing reads or writes it.

2. **The YAML manifest backend was removed in Phase 6.** `manifest.Write/Read/
   List` are now deprecated no-op stubs (`ErrYAMLBackendRemoved`). The v2 YAML
   `manifest.yaml` files that a `schema_version: 2.0 -> 3.0` migration was meant
   to rewrite are no longer the source of truth.

3. **`schema_version` is not persisted.** Dolt's `agm_sessions` table has no
   `schema_version` column; the adapter hardcodes `m.SchemaVersion = "2.0"` on
   read (`internal/dolt/sessions.go:623`, `frecency.go:207`). There is no
   per-session field to bump to "3.0".

4. **`harness` is already backfilled at the storage layer.** Both `CreateSession`
   and `UpdateSession` default an empty harness to `"claude-code"`
   (`sessions.go:71`, `:208`), so in practice no stored session has an empty
   harness for the migration to fix.

5. **`harness_history` (the one genuinely-new v3 field) has no backing column.**
   It lives only on the `ManifestV3` struct.

Net: in the current Dolt-based architecture, a v2→v3 *data* migration has no
meaningful persistent delta. Schema evolution is owned by the **Dolt numbered
migration system** (`internal/dolt/migrations.go`, currently at v016, auto-
applied on adapter init), which superseded the manifest `schema_version`
concept entirely.

The command was therefore built to be **honest about this**: it validates and
reports v3 readiness, backfills `harness` only where genuinely empty, and its
plan output explicitly states that `schema_version` and `harness_history` are
not written per session.

## The recurring lesson

The bead was filed under the old YAML-manifest mental model. Two large
architecture moves since then — Phase 6 YAML removal and the Dolt numbered-
migration system — quietly drained the task of its data-migration content
without the bead ever being re-scoped. A P0 sat blocked for ~5 months against a
target that the architecture had already obsoleted.

When a long-lived bead names a concrete artifact (`schema_version 2.0 -> 3.0`),
re-verify that artifact still exists before implementing against it.

## Follow-ups (filed separately — NOT done here, would be scope creep)

- **Decide whether v3 is wanted at all**, and if so, wire `ManifestV3` into the
  store: a Dolt migration (017) adding `harness_history` persistence + adapter
  read/write + emitting `harness_history` entries on harness switch. This is a
  product/schema decision (the same one ce-6as.103 names), not migration
  tooling. Tracked as a new follow-up bead linked to ce-6as.71.
- If legacy on-disk `~/.claude/sessions/*/manifest.yaml` files in the wild still
  matter post–Phase 6, a *yaml-to-dolt* reconciliation tool is a separate,
  concrete piece of work — distinct from the manifest `v2-to-v3` schema axis.
